"""Headless Textual Pilot tests and live bff-dev verification for the Notes TUI (#34)."""

from __future__ import annotations

import asyncio
import os
import signal
import socket
import subprocess
import tempfile
import threading
import time
import unittest
import urllib.error
import urllib.request
from pathlib import Path

import notes_tui_app
from google.protobuf import timestamp_pb2
from template.notes.v1 import notes_connect, notes_pb2
from textual.widgets import Button, Input, Static


def _required_env(name: str) -> str:
    value = os.environ.get(name, "")
    if not value:
        raise RuntimeError(f"{name} is not set; run this test through bazel test")
    return value


def _free_port() -> int:
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as sock:
        sock.bind(("127.0.0.1", 0))
        return int(sock.getsockname()[1])


def _wait_ready(url: str, proc: subprocess.Popen[str], timeout_s: float = 15.0) -> None:
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
    deadline = time.monotonic() + timeout_s
    while time.monotonic() < deadline:
        if proc.poll() is not None:
            stdout, stderr = proc.communicate(timeout=2)
            raise RuntimeError(f"bff-dev exited early ({proc.returncode}):\n{stdout}\n{stderr}")
        try:
            with opener.open(url, timeout=0.5):
                return
        except urllib.error.HTTPError:
            return
        except OSError:
            time.sleep(0.05)
    raise TimeoutError(f"bff-dev did not become ready at {url}")


class _BffDevInstance:
    def __init__(self, bff_bin: str, data_dir: Path, port: int) -> None:
        self._bff_bin = bff_bin
        self._data_dir = data_dir
        self.port = port
        self.endpoint = f"http://127.0.0.1:{port}"
        self._proc: subprocess.Popen[str] | None = None

    def start(self, *, ready_timeout_s: float = 15.0) -> None:
        argv = [
            self._bff_bin,
            f"--addr=127.0.0.1:{self.port}",
            "--allowed-origin=http://127.0.0.1:4173",
            "--store=local",
            f"--data-dir={self._data_dir}",
        ]
        proc = subprocess.Popen(
            argv,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
        )
        self._proc = proc
        try:
            _wait_ready(f"{self.endpoint}/", proc, timeout_s=ready_timeout_s)
        except BaseException:
            self.stop()
            raise

    def stop(self) -> None:
        proc = self._proc
        if proc is None:
            return
        try:
            if proc.poll() is None:
                try:
                    proc.send_signal(signal.SIGTERM)
                except ProcessLookupError:
                    pass
                try:
                    proc.wait(timeout=5.0)
                except subprocess.TimeoutExpired:
                    try:
                        proc.kill()
                    except ProcessLookupError:
                        pass
                    proc.wait(timeout=5.0)
            else:
                proc.wait(timeout=5.0)
        finally:
            if proc.stdout is not None:
                proc.stdout.close()
            if proc.stderr is not None:
                proc.stderr.close()
            self._proc = None


class _FakeNotesReader:
    """Realistic deterministic fake implementing the generated NoteServiceClient.get_note contract."""

    def __init__(self) -> None:
        self.calls: list[tuple[str, float | None]] = []
        self.responses: dict[str, notes_pb2.Note | notes_connect.ConnectError] = {}
        self.started_event: threading.Event | None = None
        self.release_event: threading.Event | None = None

    def get_note(
        self,
        request: notes_pb2.GetNoteRequest,
        *,
        timeout_seconds: float | None = None,
    ) -> notes_pb2.GetNoteResponse:
        self.calls.append((request.name, timeout_seconds))
        if self.started_event is not None:
            self.started_event.set()
        if self.release_event is not None:
            if not self.release_event.wait(timeout=5.0):
                raise notes_connect.ConnectError("deadline_exceeded", "fake gate timed out")
        outcome = self.responses.get(request.name)
        if outcome is None:
            raise notes_connect.ConnectError("not_found", f"note {request.name!r} was not found")
        if isinstance(outcome, notes_connect.ConnectError):
            raise outcome
        return notes_pb2.GetNoteResponse(note=outcome)


def _widget_text(app: notes_tui_app.NotesTuiApp, selector: str) -> str:
    return str(app.query_one(selector, Static).render())


class _ObservedNotesTuiApp(notes_tui_app.NotesTuiApp):
    """Test subclass signaling after each RPC completion callback returns on the UI loop."""

    def __init__(
        self,
        client: notes_tui_app.NotesReader,
        *,
        endpoint_label: str,
        initial_name: str = "",
        timeout_seconds: float | None = None,
    ) -> None:
        super().__init__(
            client,
            endpoint_label=endpoint_label,
            initial_name=initial_name,
            timeout_seconds=timeout_seconds,
        )
        self.completed_callbacks: list[tuple[str, int]] = []
        self._callback_done: asyncio.Event | None = None

    def reset_callback_signal(self) -> None:
        self._callback_done = asyncio.Event()

    async def wait_for_callback(self, timeout_s: float = 5.0) -> None:
        if self._callback_done is None:
            raise RuntimeError("reset_callback_signal must be called before wait_for_callback")
        await asyncio.wait_for(self._callback_done.wait(), timeout=timeout_s)

    def _apply_success(self, seq: int, note: notes_pb2.Note) -> None:
        try:
            super()._apply_success(seq, note)
        finally:
            self.completed_callbacks.append(("success", seq))
            if self._callback_done is not None:
                self._callback_done.set()

    def _apply_error(self, seq: int, err: notes_connect.ConnectError) -> None:
        try:
            super()._apply_error(seq, err)
        finally:
            self.completed_callbacks.append(("error", seq))
            if self._callback_done is not None:
                self._callback_done.set()


class NotesTuiPilotTest(unittest.IsolatedAsyncioTestCase):
    async def test_keyboard_navigation_loading_and_success_against_fake(self) -> None:
        fake = _FakeNotesReader()
        created = timestamp_pb2.Timestamp()
        created.FromJsonString("2026-11-24T09:00:00Z")
        fake.responses["notes/alpha-1"] = notes_pb2.Note(
            name="notes/alpha-1",
            title="Synthetic [alpha] note",
            body="Rendered cleanly with markup disabled.",
            create_time=created,
        )
        fake.started_event = threading.Event()
        fake.release_event = threading.Event()

        app = notes_tui_app.NotesTuiApp(
            fake,
            endpoint_label="http://127.0.0.1:8080 (subject=ana)",
            timeout_seconds=3.0,
        )
        async with app.run_test(size=(100, 30)) as pilot:
            # Initial state: idle with note-name input focused.
            self.assertEqual(app.view_state, "idle")
            self.assertEqual(app.focused, app.query_one("#note-name-input", Input))
            self.assertIn("Ready", _widget_text(app, "#status"))

            # Keyboard entry + Enter transitions immediately to loading state while RPC is in flight.
            await pilot.press(*"notes/alpha-1", "enter")
            self.assertTrue(fake.started_event.wait(timeout=5.0), "fake RPC was not invoked")
            await pilot.pause()

            self.assertEqual(app.view_state, "loading")
            self.assertIn("Loading notes/alpha-1...", _widget_text(app, "#status"))
            self.assertTrue(app.query_one("#fetch-button", Button).disabled)

            # Release fake RPC and verify success state renders note fields literally.
            fake.release_event.set()
            await app.workers.wait_for_complete()
            await pilot.pause()

            self.assertEqual(app.view_state, "success")
            self.assertFalse(app.query_one("#fetch-button", Button).disabled)
            self.assertEqual(_widget_text(app, "#status"), "Loaded notes/alpha-1")
            self.assertEqual(_widget_text(app, "#note-title"), "Title: Synthetic [alpha] note")
            self.assertIn("notes/alpha-1", _widget_text(app, "#note-meta"))
            self.assertIn("2026-11-24T09:00:00Z", _widget_text(app, "#note-meta"))
            self.assertEqual(_widget_text(app, "#note-body"), "Rendered cleanly with markup disabled.")
            self.assertEqual(_widget_text(app, "#error-panel"), "")
            self.assertEqual(fake.calls, [("notes/alpha-1", 3.0)])

            # Tab moves focus to Fetch button; Enter re-fetches; Ctrl+R refreshes even while focused on input;
            # Escape blurs input so single-key `r` refreshes and Ctrl+L focuses the note ID input again.
            fake.started_event = None
            fake.release_event = None
            await pilot.press("tab")
            self.assertEqual(app.focused, app.query_one("#fetch-button", Button))
            await pilot.press("enter")
            await app.workers.wait_for_complete()
            await pilot.pause()
            self.assertEqual(len(fake.calls), 2)

            await pilot.press("ctrl+r")
            await app.workers.wait_for_complete()
            await pilot.pause()
            self.assertEqual(len(fake.calls), 3)

            await pilot.press("escape")
            self.assertIsNone(app.focused)
            await pilot.press("r")
            await app.workers.wait_for_complete()
            await pilot.pause()
            self.assertEqual(len(fake.calls), 4)

            await pilot.press("ctrl+l")
            self.assertEqual(app.focused, app.query_one("#note-name-input", Input))

    async def test_typed_rpc_errors_clear_stale_note_and_display_actionable_hints(self) -> None:
        fake = _FakeNotesReader()
        fake.responses["notes/ok-1"] = notes_pb2.Note(
            name="notes/ok-1",
            title="Initial title",
            body="Initial body that must clear on error.",
        )
        fake.responses["notes/missing-1"] = notes_connect.ConnectError(
            "not_found",
            "note not found",
            http_status=404,
        )
        fake.responses["bad-id"] = notes_connect.ConnectError(
            "invalid_argument",
            "validation error",
            violations=(
                notes_connect.FieldViolation(
                    field="name",
                    rule_id="get_note.name.pattern",
                    message="value must match notes/<id>",
                ),
            ),
            http_status=400,
        )
        fake.responses["notes/unauth-1"] = notes_connect.ConnectError(
            "unauthenticated",
            "missing bearer token",
            http_status=401,
        )
        fake.responses["notes/down-1"] = notes_connect.ConnectError(
            "unavailable",
            "service unreachable: connection refused",
        )

        app = notes_tui_app.NotesTuiApp(
            fake,
            endpoint_label="http://127.0.0.1:8080 (subject=ana)",
            initial_name="notes/ok-1",
        )
        async with app.run_test(size=(100, 30)) as pilot:
            await app.workers.wait_for_complete()
            await pilot.pause()
            self.assertEqual(app.view_state, "success")
            self.assertEqual(_widget_text(app, "#note-title"), "Title: Initial title")

            cases = [
                ("notes/missing-1", "[not_found]", "active subject"),
                ("bad-id", "[invalid_argument]", "name (get_note.name.pattern)"),
                ("notes/unauth-1", "[unauthenticated]", "--bearer_token"),
                ("notes/down-1", "[unavailable]", "Ctrl+R to retry"),
            ]
            name_input = app.query_one("#note-name-input", Input)
            for query, expected_code, expected_detail in cases:
                with self.subTest(query=query):
                    name_input.value = query
                    name_input.focus()
                    await pilot.press("enter")
                    await app.workers.wait_for_complete()
                    await pilot.pause()

                    self.assertEqual(app.view_state, "error")
                    self.assertIn(expected_code, _widget_text(app, "#status"))
                    self.assertIn(expected_code, _widget_text(app, "#error-panel"))
                    self.assertIn(expected_detail, _widget_text(app, "#error-panel"))
                    # Stale note content must never remain visible after an error.
                    self.assertEqual(_widget_text(app, "#note-title"), "")
                    self.assertEqual(_widget_text(app, "#note-meta"), "")
                    self.assertEqual(_widget_text(app, "#note-body"), "")

            # Empty submission produces an actionable prompt without issuing an RPC.
            calls_before = len(fake.calls)
            name_input.value = "   "
            name_input.focus()
            await pilot.press("enter")
            await pilot.pause()
            self.assertEqual(len(fake.calls), calls_before)
            self.assertEqual(app.view_state, "error")
            self.assertIn("No note ID entered", _widget_text(app, "#error-panel"))

    async def test_blank_submission_during_inflight_read_invalidates_pending_result(self) -> None:
        fake = _FakeNotesReader()
        fake.responses["notes/alpha-1"] = notes_pb2.Note(
            name="notes/alpha-1",
            title="Synthetic [alpha] note",
            body="Rendered cleanly with markup disabled.",
        )
        fake.responses["notes/err-1"] = notes_connect.ConnectError(
            "unavailable",
            "stale remote error that must not overwrite blank prompt",
        )

        app = _ObservedNotesTuiApp(
            fake,
            endpoint_label="http://127.0.0.1:8080 (subject=ana)",
            timeout_seconds=3.0,
        )
        async with app.run_test(size=(80, 24)) as pilot:
            name_input = app.query_one("#note-name-input", Input)
            fetch_button = app.query_one("#fetch-button", Button)

            # 1. Hold both a pending success and a pending remote error; blank submission supersedes each,
            #    sends no extra RPC, restores Fetch immediately, and persists after the stale UI callback runs.
            for stale_query, expected_callback in (
                ("notes/alpha-1", "success"),
                ("notes/err-1", "error"),
            ):
                with self.subTest(stale_query=stale_query):
                    fake.started_event = threading.Event()
                    fake.release_event = threading.Event()
                    app.reset_callback_signal()
                    name_input.value = stale_query
                    name_input.focus()
                    await pilot.press("enter")
                    self.assertTrue(fake.started_event.wait(timeout=5.0), "gated RPC was not invoked")
                    await pilot.pause()
                    self.assertEqual(app.view_state, "loading")
                    self.assertTrue(fetch_button.disabled)
                    stale_seq = app._request_seq
                    calls_after_start = len(fake.calls)

                    name_input.value = "   "
                    name_input.focus()
                    await pilot.press("enter")
                    await pilot.pause()
                    self.assertEqual(len(fake.calls), calls_after_start)
                    self.assertEqual(app.view_state, "error")
                    self.assertFalse(fetch_button.disabled)
                    self.assertIn("No note ID entered", _widget_text(app, "#error-panel"))
                    self.assertEqual(_widget_text(app, "#note-title"), "")
                    self.assertEqual(_widget_text(app, "#note-meta"), "")
                    self.assertEqual(_widget_text(app, "#note-body"), "")

                    fake.release_event.set()
                    await app.wait_for_callback(timeout_s=5.0)
                    await app.workers.wait_for_complete()
                    await pilot.pause()
                    self.assertEqual(app.completed_callbacks[-1], (expected_callback, stale_seq))
                    self.assertEqual(len(fake.calls), calls_after_start)
                    self.assertEqual(app.view_state, "error")
                    self.assertFalse(fetch_button.disabled)
                    self.assertIn("No note ID entered", _widget_text(app, "#error-panel"))
                    self.assertNotIn("[unavailable]", _widget_text(app, "#error-panel"))
                    self.assertEqual(_widget_text(app, "#note-title"), "")
                    self.assertEqual(_widget_text(app, "#note-meta"), "")
                    self.assertEqual(_widget_text(app, "#note-body"), "")

            # 2. A subsequent valid read after blank supersession succeeds cleanly.
            fake.started_event = None
            fake.release_event = None
            app.reset_callback_signal()
            name_input.value = "notes/alpha-1"
            name_input.focus()
            await pilot.press("enter")
            await app.wait_for_callback(timeout_s=5.0)
            await app.workers.wait_for_complete()
            await pilot.pause()

            self.assertEqual(app.view_state, "success")
            self.assertFalse(fetch_button.disabled)
            self.assertEqual(_widget_text(app, "#status"), "Loaded notes/alpha-1")
            self.assertEqual(_widget_text(app, "#note-title"), "Title: Synthetic [alpha] note")
            self.assertEqual(_widget_text(app, "#note-body"), "Rendered cleanly with markup disabled.")
            self.assertEqual(_widget_text(app, "#error-panel"), "")

    async def test_live_bff_dev_real_service_read_and_errors(self) -> None:
        bff_bin = str(Path(_required_env("BFF_DEV_BIN")).resolve())
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        server = _BffDevInstance(bff_bin, Path(tmp.name) / "notes-store", _free_port())
        self.addCleanup(server.stop)
        server.start()

        # Seed one synthetic note via the shared generated Connect client as ana.
        ana_client = notes_connect.NoteServiceClient(server.endpoint, test_subject="ana", timeout_seconds=5.0)
        created = ana_client.create_note(
            notes_pb2.CreateNoteRequest(
                note=notes_pb2.Note(
                    title="TUI real-service note",
                    body="Loaded directly from bff-dev over Connect.",
                )
            )
        )
        note_name = created.note.name

        # 1. Success path as ana + terminal SVG capture check.
        ana_app = notes_tui_app.NotesTuiApp(
            ana_client,
            endpoint_label=f"{server.endpoint} (subject=ana)",
            timeout_seconds=5.0,
        )
        async with ana_app.run_test(size=(80, 24)) as pilot:
            await pilot.press(*note_name, "enter")
            await ana_app.workers.wait_for_complete()
            await pilot.pause()

            self.assertEqual(ana_app.view_state, "success")
            self.assertEqual(_widget_text(ana_app, "#status"), f"Loaded {note_name}")
            self.assertEqual(_widget_text(ana_app, "#note-title"), "Title: TUI real-service note")
            self.assertEqual(_widget_text(ana_app, "#note-body"), "Loaded directly from bff-dev over Connect.")

            rendered_lines = [
                "".join(seg.text for seg in strip).rstrip()
                for strip in ana_app.screen._compositor.render_full_update().strips
            ]
            rendered_screen = "\n".join(rendered_lines)
            self.assertIn("TUI real-service note", rendered_screen)
            self.assertIn("Loaded directly from bff-dev over Connect.", rendered_screen)

            svg = ana_app.export_screenshot()
            self.assertIn("<svg", svg)
            self.assertIn("TUI", svg)

            # Malformed ID hits real bff-dev protovalidate -> typed invalid_argument with violations.
            name_input = ana_app.query_one("#note-name-input", Input)
            name_input.value = "bad-id"
            name_input.focus()
            await pilot.press("enter")
            await ana_app.workers.wait_for_complete()
            await pilot.pause()

            self.assertEqual(ana_app.view_state, "error")
            self.assertIn("[invalid_argument]", _widget_text(ana_app, "#status"))
            self.assertIn("Validation violations:", _widget_text(ana_app, "#error-panel"))

        # 2. Cross-subject refusal as bob -> real server not_found.
        bob_client = notes_connect.NoteServiceClient(server.endpoint, test_subject="bob", timeout_seconds=5.0)
        bob_app = notes_tui_app.NotesTuiApp(
            bob_client,
            endpoint_label=f"{server.endpoint} (subject=bob)",
            initial_name=note_name,
            timeout_seconds=5.0,
        )
        async with bob_app.run_test(size=(100, 30)) as pilot:
            await bob_app.workers.wait_for_complete()
            await pilot.pause()
            self.assertEqual(bob_app.view_state, "error")
            self.assertIn("[not_found]", _widget_text(bob_app, "#status"))
            self.assertIn("active subject", _widget_text(bob_app, "#error-panel"))

        # 3. Missing auth -> real server unauthenticated.
        anon_client = notes_connect.NoteServiceClient(server.endpoint, timeout_seconds=5.0)
        anon_app = notes_tui_app.NotesTuiApp(
            anon_client,
            endpoint_label=f"{server.endpoint} (unauthenticated)",
            initial_name=note_name,
            timeout_seconds=5.0,
        )
        async with anon_app.run_test(size=(100, 30)) as pilot:
            await anon_app.workers.wait_for_complete()
            await pilot.pause()
            self.assertEqual(anon_app.view_state, "error")
            self.assertIn("[unauthenticated]", _widget_text(anon_app, "#status"))


class NotesTuiBinaryCliFlagsTest(unittest.TestCase):
    def setUp(self) -> None:
        self.tui_bin = str(Path(_required_env("NOTES_TUI_BIN")).resolve())

    def _run_tui(self, *args: str) -> subprocess.CompletedProcess[str]:
        return subprocess.run(
            [self.tui_bin, *args],
            capture_output=True,
            text=True,
            timeout=15,
            check=False,
        )

    def test_help_and_invalid_invocations_fail_fast_without_terminal(self) -> None:
        help_res = self._run_tui("--help")
        self.assertEqual(help_res.returncode, 1, f"stdout={help_res.stdout!r} stderr={help_res.stderr!r}")
        combined = help_res.stdout + help_res.stderr
        for flag in ("--endpoint", "--subject", "--bearer_token", "--timeout_seconds", "--name"):
            self.assertIn(flag, combined)

        cases = [
            ((), "--endpoint is required"),
            (("extra", "--endpoint=http://127.0.0.1:8080"), "unexpected positional"),
            (("--endpoint=https://example.invalid", "--subject=ana"), "loopback"),
            (("--endpoint=http://example.invalid", "--bearer_token=secret-tok"), "https://"),
            (("--endpoint=http://127.0.0.1:8080", "--subject=ana", "--bearer_token=tok"), "at most one"),
            (("--endpoint=http://127.0.0.1:8080", "--subject=ana", "--timeout_seconds=0"), "positive finite"),
        ]
        for args, expected in cases:
            with self.subTest(args=args):
                res = self._run_tui(*args)
                self.assertEqual(res.returncode, 2, f"stdout={res.stdout!r} stderr={res.stderr!r}")
                self.assertIn(expected, res.stderr)


if __name__ == "__main__":
    unittest.main()
