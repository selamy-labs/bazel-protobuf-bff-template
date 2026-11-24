"""Integration and negative controls for the Python absl-py Notes CLI (#29)."""

from __future__ import annotations

import dataclasses
import http.server
import json
import os
import signal
import socket
import subprocess
import sys
import tempfile
import threading
import time
import unittest
import urllib.error
import urllib.request
from collections.abc import Callable
from pathlib import Path


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
    def __init__(
        self,
        bff_bin: str,
        data_dir: Path,
        port: int,
        *,
        cmd: list[str] | None = None,
    ) -> None:
        self._bff_bin = bff_bin
        self._data_dir = data_dir
        self._cmd = cmd
        self.port = port
        self.endpoint = f"http://127.0.0.1:{port}"
        self._proc: subprocess.Popen[str] | None = None

    def start(
        self,
        *,
        ready_timeout_s: float = 15.0,
        on_acquire: Callable[[int], None] | None = None,
    ) -> None:
        if self._proc is not None:
            raise RuntimeError("bff-dev instance is already started")
        argv = self._cmd or [
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
        if on_acquire is not None:
            on_acquire(proc.pid)
        try:
            _wait_ready(f"{self.endpoint}/", proc, timeout_s=ready_timeout_s)
        except BaseException:
            self.stop()
            raise

    def stop(self, *, term_timeout_s: float = 5.0, kill_timeout_s: float = 5.0) -> None:
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
                    proc.wait(timeout=term_timeout_s)
                except subprocess.TimeoutExpired:
                    try:
                        proc.kill()
                    except ProcessLookupError:
                        pass
                    proc.wait(timeout=kill_timeout_s)
            else:
                proc.wait(timeout=kill_timeout_s)
        finally:
            if proc.stdout is not None:
                proc.stdout.close()
            if proc.stderr is not None:
                proc.stderr.close()
            self._proc = None


class _SilentHTTPServer(http.server.ThreadingHTTPServer):
    allow_reuse_address = True
    daemon_threads = True


@dataclasses.dataclass
class _PlannedResponse:
    status: int
    content_type: str | None
    body: bytes


class NotesCliTest(unittest.TestCase):
    def setUp(self) -> None:
        self.cli_bin = str(Path(_required_env("NOTES_CLI_BIN")).resolve())
        self.bff_bin = str(Path(_required_env("BFF_DEV_BIN")).resolve())
        self._tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self._tmp.cleanup)
        self.data_dir = Path(self._tmp.name) / "notes-store"
        self.server = self._start_server()

    def _start_server(self) -> _BffDevInstance:
        instance = _BffDevInstance(self.bff_bin, self.data_dir, _free_port())
        self.addCleanup(instance.stop)
        instance.start()
        return instance

    def _start_http_server(
        self,
        handler_cls: type[http.server.BaseHTTPRequestHandler],
    ) -> tuple[_SilentHTTPServer, str]:
        server = _SilentHTTPServer(("127.0.0.1", 0), handler_cls)
        thread = threading.Thread(target=server.serve_forever, kwargs={"poll_interval": 0.05}, daemon=True)

        def _cleanup() -> None:
            server.shutdown()
            server.server_close()
            thread.join(timeout=5)

        self.addCleanup(_cleanup)
        thread.start()
        port = int(server.server_address[1])
        return server, f"http://127.0.0.1:{port}"

    def _run_cli(self, *args: str) -> subprocess.CompletedProcess[str]:
        return subprocess.run(
            [self.cli_bin, *args],
            capture_output=True,
            text=True,
            timeout=15,
            check=False,
        )

    def test_help_lists_commands_and_flags(self) -> None:
        res = self._run_cli("--help")
        # absl.app prints module docstring + flags to stdout and exits 1 on --help.
        self.assertEqual(res.returncode, 1, f"stdout={res.stdout!r} stderr={res.stderr!r}")
        combined = res.stdout + res.stderr
        self.assertIn("--endpoint", combined)
        self.assertIn("--subject", combined)
        self.assertIn("--timeout_seconds", combined)

    def test_invalid_invocations_fail_with_usage_code_2(self) -> None:
        cases = [
            ((), "missing command"),
            (("delete", f"--endpoint={self.server.endpoint}"), "unknown command"),
            (("create",), "--endpoint is required"),
            (("get", f"--endpoint={self.server.endpoint}", "--subject=ana"), "--name is required"),
            (
                ("get", "--endpoint=https://example.invalid", "--subject=ana", "--name=notes/1"),
                "loopback",
            ),
            (
                ("get", "--endpoint=http://example.invalid", "--bearer_token=secret-tok", "--name=notes/1"),
                "https://",
            ),
            (
                (
                    "get",
                    f"--endpoint={self.server.endpoint}",
                    "--subject=ana",
                    "--bearer_token=tok",
                    "--name=notes/1",
                ),
                "at most one",
            ),
            (
                ("get", f"--endpoint={self.server.endpoint}", "--subject=ana", "--timeout_seconds=0", "--name=notes/1"),
                "positive finite",
            ),
        ]
        for args, expected_substring in cases:
            with self.subTest(args=args):
                res = self._run_cli(*args)
                self.assertEqual(res.returncode, 2, f"stdout={res.stdout!r} stderr={res.stderr!r}")
                self.assertIn(expected_substring, res.stderr)

    def test_roundtrip_persistence_across_restart_and_cross_subject_refusal(self) -> None:
        created = self._run_cli(
            "create",
            f"--endpoint={self.server.endpoint}",
            "--subject=ana",
            "--title=CLI note",
            "--body=Persisted via local store.",
            "--output=json",
        )
        self.assertEqual(created.returncode, 0, created.stderr)
        created_doc = json.loads(created.stdout)
        note_name = created_doc["name"]
        self.assertRegex(note_name, r"^notes/[a-z0-9-]+$")
        self.assertEqual(created_doc["title"], "CLI note")
        self.assertEqual(created_doc["body"], "Persisted via local store.")

        fetched = self._run_cli(
            "get",
            f"--endpoint={self.server.endpoint}",
            "--subject=ana",
            f"--name={note_name}",
        )
        self.assertEqual(fetched.returncode, 0, fetched.stderr)
        self.assertIn(f"name: {note_name}", fetched.stdout)
        self.assertIn("title: CLI note", fetched.stdout)
        self.assertIn("body: Persisted via local store.", fetched.stdout)

        # Restart bff-dev against the same local data directory to prove persistence.
        self.server.stop()
        self.server = self._start_server()

        after_restart = self._run_cli(
            "get",
            f"--endpoint={self.server.endpoint}",
            "--subject=ana",
            f"--name={note_name}",
            "--output=json",
        )
        self.assertEqual(after_restart.returncode, 0, after_restart.stderr)
        restarted_doc = json.loads(after_restart.stdout)
        self.assertEqual(restarted_doc["name"], note_name)
        self.assertEqual(restarted_doc["title"], "CLI note")
        self.assertEqual(restarted_doc["body"], "Persisted via local store.")

        # Typed cross-subject refusal: bob cannot read ana's note.
        denied = self._run_cli(
            "get",
            f"--endpoint={self.server.endpoint}",
            "--subject=bob",
            f"--name={note_name}",
            "--output=json",
        )
        self.assertEqual(denied.returncode, 1)
        denied_doc = json.loads(denied.stderr)
        self.assertEqual(denied_doc["error"]["code"], "not_found")

    def test_server_validation_and_unauthenticated_and_unavailable_errors(self) -> None:
        invalid_title = self._run_cli(
            "create",
            f"--endpoint={self.server.endpoint}",
            "--subject=ana",
            "--title=   ",
            "--body=blank title should fail server-side",
            "--output=json",
        )
        self.assertEqual(invalid_title.returncode, 1)
        invalid_doc = json.loads(invalid_title.stderr)
        self.assertEqual(invalid_doc["error"]["code"], "invalid_argument")
        self.assertTrue(invalid_doc["error"]["violations"], f"expected violations in {invalid_doc}")
        self.assertEqual(invalid_doc["error"]["violations"][0]["rule_id"], "note.title.required")

        unauth = self._run_cli(
            "create",
            f"--endpoint={self.server.endpoint}",
            "--title=No auth",
        )
        self.assertEqual(unauth.returncode, 1)
        self.assertIn("[unauthenticated]", unauth.stderr)

        closed_port = _free_port()
        unreachable = self._run_cli(
            "get",
            f"--endpoint=http://127.0.0.1:{closed_port}",
            "--subject=ana",
            "--name=notes/missing",
            "--timeout_seconds=1.0",
        )
        self.assertEqual(unreachable.returncode, 1)
        self.assertIn("[unavailable]", unreachable.stderr)

    def test_redirect_refused_and_credentials_confined(self) -> None:
        target_hits: list[tuple[str, str | None]] = []
        redirect_hits: list[tuple[str, str | None]] = []
        redirect_status = {"code": 302}

        class _TargetHandler(http.server.BaseHTTPRequestHandler):
            def do_POST(self) -> None:
                target_hits.append((self.path, self.headers.get("Authorization")))
                self.send_response(200)
                self.send_header("Content-Type", "application/proto")
                self.end_headers()

            def do_GET(self) -> None:
                target_hits.append((self.path, self.headers.get("Authorization")))
                self.send_response(200)
                self.end_headers()

            def log_message(self, format: str, *args: object) -> None:
                return

        _, target_url = self._start_http_server(_TargetHandler)

        class _RedirectHandler(http.server.BaseHTTPRequestHandler):
            def do_POST(self) -> None:
                length = int(self.headers.get("Content-Length", "0"))
                if length > 0:
                    self.rfile.read(length)
                redirect_hits.append((self.path, self.headers.get("Authorization")))
                self.send_response(redirect_status["code"])
                self.send_header("Location", f"{target_url}{self.path}")
                self.end_headers()

            def log_message(self, format: str, *args: object) -> None:
                return

        _, redirect_url = self._start_http_server(_RedirectHandler)

        cases = [
            (302, "--subject=ana", "Bearer test-identity.ana"),
            (307, "--bearer_token=secret-bearer-token", "Bearer secret-bearer-token"),
        ]
        for code, auth_flag, expected_auth in cases:
            with self.subTest(code=code, auth_flag=auth_flag):
                redirect_status["code"] = code
                redirect_hits.clear()
                target_hits.clear()
                res = self._run_cli(
                    "get",
                    f"--endpoint={redirect_url}",
                    auth_flag,
                    "--name=notes/1",
                    "--output=json",
                )
                self.assertEqual(res.returncode, 1, f"stdout={res.stdout!r} stderr={res.stderr!r}")
                self.assertEqual(res.stdout, "")
                doc = json.loads(res.stderr)
                self.assertEqual(doc["error"]["code"], "failed_precondition")
                self.assertIn(f"HTTP {code}", doc["error"]["message"])
                self.assertEqual(len(redirect_hits), 1)
                self.assertEqual(redirect_hits[0][1], expected_auth)
                self.assertEqual(target_hits, [], "redirect target must never receive requests or credentials")

    def test_response_envelope_and_malformed_protobuf_fail_as_typed_rpc_errors(self) -> None:
        # Valid binary protobuf wire encoding of GetNoteResponse{note: Note{name: "notes/1", title: "Ok", body: "Hi"}}.
        valid_get_note_wire = b"\x0a\x11\x0a\x07notes/1\x12\x02Ok\x1a\x02Hi"
        response_plan = _PlannedResponse(
            status=200,
            content_type="application/proto",
            body=valid_get_note_wire,
        )

        class _EnvelopeHandler(http.server.BaseHTTPRequestHandler):
            def do_POST(self) -> None:
                length = int(self.headers.get("Content-Length", "0"))
                if length > 0:
                    self.rfile.read(length)
                body = response_plan.body
                self.send_response(response_plan.status)
                if response_plan.content_type is not None:
                    self.send_header("Content-Type", response_plan.content_type)
                self.send_header("Content-Length", str(len(body)))
                self.end_headers()
                if body:
                    self.wfile.write(body)

            def log_message(self, format: str, *args: object) -> None:
                return

        _, synth_url = self._start_http_server(_EnvelopeHandler)

        # Positive control: valid HTTP 200 + application/proto (with media-type params) decodes cleanly.
        response_plan.status = 200
        response_plan.content_type = "application/proto; charset=utf-8"
        response_plan.body = valid_get_note_wire
        ok = self._run_cli("get", f"--endpoint={synth_url}", "--subject=ana", "--name=notes/1", "--output=json")
        self.assertEqual(ok.returncode, 0, ok.stderr)
        ok_doc = json.loads(ok.stdout)
        self.assertEqual(ok_doc["name"], "notes/1")
        self.assertEqual(ok_doc["title"], "Ok")
        self.assertEqual(ok_doc["body"], "Hi")

        # Negative controls: non-200 2xx, non-proto media type, missing media type, malformed protobuf bytes.
        bad_cases = [
            ("204_empty", 204, "application/proto", b"", "unknown", "HTTP status 204"),
            ("200_text_html_empty", 200, "text/html", b"", "unknown", "content-type"),
            ("200_missing_content_type", 200, None, b"", "unknown", "content-type"),
            ("200_malformed_wire", 200, "application/proto", b"\x08", "internal", "malformed protobuf"),
        ]
        for label, status, content_type, body, expected_code, expected_msg in bad_cases:
            with self.subTest(case=label):
                response_plan.status = status
                response_plan.content_type = content_type
                response_plan.body = body
                res = self._run_cli(
                    "get",
                    f"--endpoint={synth_url}",
                    "--subject=ana",
                    "--name=notes/1",
                    "--output=json",
                )
                self.assertEqual(res.returncode, 1, f"stdout={res.stdout!r} stderr={res.stderr!r}")
                self.assertEqual(res.stdout, "", "invalid RPC response must never render an empty Note")
                self.assertNotIn("Traceback", res.stderr)
                doc = json.loads(res.stderr)
                self.assertEqual(doc["error"]["code"], expected_code)
                self.assertIn(expected_msg, doc["error"]["message"])

    def test_interrupted_response_read_fails_as_typed_rpc_error(self) -> None:
        listener = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        listener.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        listener.bind(("127.0.0.1", 0))
        listener.listen(4)
        listener.settimeout(5.0)
        port = int(listener.getsockname()[1])

        def _serve_truncated() -> None:
            try:
                conn, _ = listener.accept()
            except OSError:
                return
            with conn:
                conn.settimeout(2.0)
                # Drain the entire HTTP request (headers + body) before closing so the kernel sends a clean
                # FIN after the truncated response instead of RST from unread request bytes.
                buf = b""
                while b"\r\n\r\n" not in buf:
                    chunk = conn.recv(4096)
                    if not chunk:
                        break
                    buf += chunk
                head, _, body_so_far = buf.partition(b"\r\n\r\n")
                content_length = 0
                for raw_line in head.split(b"\r\n")[1:]:
                    name, _, val = raw_line.partition(b":")
                    if name.strip().lower() == b"content-length":
                        content_length = int(val.strip())
                remaining = content_length - len(body_so_far)
                while remaining > 0:
                    chunk = conn.recv(min(4096, remaining))
                    if not chunk:
                        break
                    remaining -= len(chunk)
                # Declare 64 response bytes, write only 4, then half-close cleanly to trigger IncompleteRead.
                header = (
                    b"HTTP/1.1 200 OK\r\n"
                    b"Content-Type: application/proto\r\n"
                    b"Content-Length: 64\r\n"
                    b"Connection: close\r\n"
                    b"\r\n"
                    b"\x0a\x02hi"
                )
                conn.sendall(header)
                try:
                    conn.shutdown(socket.SHUT_WR)
                except OSError:
                    pass

        thread = threading.Thread(target=_serve_truncated, daemon=True)

        def _cleanup() -> None:
            listener.close()
            thread.join(timeout=5)

        self.addCleanup(_cleanup)
        thread.start()

        res = self._run_cli(
            "get",
            f"--endpoint=http://127.0.0.1:{port}",
            "--subject=ana",
            "--name=notes/1",
            "--output=json",
        )
        self.assertEqual(res.returncode, 1, f"stdout={res.stdout!r} stderr={res.stderr!r}")
        self.assertEqual(res.stdout, "", "interrupted response read must never emit stdout")
        self.assertNotIn("Traceback", res.stderr)
        doc = json.loads(res.stderr)
        self.assertEqual(doc["error"]["code"], "unavailable")
        self.assertIn("incomplete response", doc["error"]["message"])

    def test_failed_start_cleans_up_child_and_stop_is_safe_after_exit(self) -> None:
        # 1. Readiness timeout cleans up a live child that never binds its port.
        acquired_pids: list[int] = []
        hanging = _BffDevInstance(
            self.bff_bin,
            self.data_dir,
            _free_port(),
            cmd=[sys.executable, "-c", "import time; time.sleep(60)"],
        )
        self.addCleanup(hanging.stop)
        with self.assertRaises(TimeoutError):
            hanging.start(ready_timeout_s=0.25, on_acquire=acquired_pids.append)
        self.assertEqual(len(acquired_pids), 1)
        self.assertIsNone(hanging._proc)
        with self.assertRaises(ProcessLookupError):
            os.kill(acquired_pids[0], 0)
        # Idempotent repeated stop after cleanup is safe.
        hanging.stop()

        # 2. Early-exit child cleans up and repeated stop after exit does not raise.
        exiting = _BffDevInstance(
            self.bff_bin,
            self.data_dir,
            _free_port(),
            cmd=[sys.executable, "-c", "import sys; sys.exit(1)"],
        )
        self.addCleanup(exiting.stop)
        with self.assertRaises(RuntimeError):
            exiting.start(ready_timeout_s=2.0)
        self.assertIsNone(exiting._proc)
        exiting.stop()

        # 3. Stopping an instance whose child already exited before stop() is safe and bounded.
        already_exited = _BffDevInstance(self.bff_bin, self.data_dir, _free_port())
        proc = subprocess.Popen(
            [sys.executable, "-c", "import sys; sys.exit(0)"],
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
        )
        already_exited._proc = proc
        self.addCleanup(already_exited.stop)
        proc.wait(timeout=5)
        already_exited.stop()
        self.assertIsNone(already_exited._proc)


if __name__ == "__main__":
    unittest.main()
