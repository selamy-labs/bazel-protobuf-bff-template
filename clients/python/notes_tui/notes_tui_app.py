"""Shared Textual TUI application for keyboard-driven Notes reads over //api:notes_py (#34).

All domain validation, persistence, and subject isolation remain authoritative in the BFF service.
"""

from __future__ import annotations

from typing import Protocol

from template.notes.v1 import notes_connect, notes_pb2
from textual.app import App, ComposeResult
from textual.binding import Binding
from textual.containers import Horizontal, Vertical, VerticalScroll
from textual.widgets import Button, Footer, Header, Input, Static

_ERROR_HINTS: dict[str, str] = {
    "not_found": "Verify the note resource name (notes/<id>) and active subject.",
    "invalid_argument": "Check the note resource name format (notes/<id>) and try again.",
    "unauthenticated": "Provide --subject=<id> on loopback or --bearer_token=<token>.",
    "permission_denied": "Confirm the active identity has permission to read this note.",
    "unavailable": "Verify the BFF endpoint is running and reachable, then press Ctrl+R to retry.",
    "deadline_exceeded": "The RPC timed out; check service responsiveness or raise --timeout_seconds.",
    "failed_precondition": "Check the endpoint URL; HTTP redirects are refused fail-closed.",
    "unimplemented": "Verify the endpoint serves template.notes.v1.NoteService.",
    "internal": "The server or response payload failed unexpectedly; inspect server logs and retry.",
}

_DEFAULT_ERROR_HINT = "Review the RPC error details above, check the endpoint/identity, and retry."


class NotesReader(Protocol):
    """Minimal unary read contract satisfied by generated NoteServiceClient and test fakes."""

    def get_note(
        self,
        request: notes_pb2.GetNoteRequest,
        *,
        timeout_seconds: float | None = None,
    ) -> notes_pb2.GetNoteResponse: ...


def format_connect_error(err: notes_connect.ConnectError) -> str:
    """Format a typed ConnectError into actionable terminal text without hiding server details."""
    lines = [
        f"Code: [{err.code}]",
        f"Message: {err.message}",
    ]
    if err.violations:
        lines.append("Validation violations:")
        for violation in err.violations:
            location = violation.field or "<request>"
            lines.append(f"  - {location} ({violation.rule_id}): {violation.message}")
    hint = _ERROR_HINTS.get(err.code, _DEFAULT_ERROR_HINT)
    lines.append(f"Next step: {hint}")
    return "\n".join(lines)


class NotesTuiApp(App[None]):
    """Interactive keyboard-driven Notes reader over the generated Connect client."""

    TITLE = "Notes TUI"
    SUB_TITLE = "Typed Connect GetNote reference"

    CSS = """
    Screen {
        layout: vertical;
    }

    #workspace {
        padding: 0 2;
        height: 1fr;
    }

    #endpoint-bar {
        color: $text-muted;
        height: 1;
    }

    #query-bar {
        height: auto;
    }

    #note-name-input {
        width: 1fr;
        margin-right: 1;
    }

    #fetch-button {
        min-width: 12;
    }

    #status {
        padding: 0 1;
        border: round $panel;
    }

    #status.state-idle {
        border: round $primary-muted;
        color: $text-muted;
    }

    #status.state-loading {
        border: round $warning;
        color: $warning;
    }

    #status.state-success {
        border: round $success;
        color: $success;
    }

    #status.state-error {
        border: round $error;
        color: $error;
    }

    #result-panel {
        border: round $panel;
        padding: 0 1;
        height: 1fr;
    }

    #note-title {
        text-style: bold;
    }

    #note-meta {
        color: $text-muted;
        margin-bottom: 1;
    }

    #error-panel {
        color: $error;
    }

    .hidden {
        display: none;
    }
    """

    BINDINGS = [
        Binding("ctrl+q", "quit", "Quit", priority=True),
        Binding("ctrl+l", "focus_input", "Focus ID", priority=True),
        Binding("ctrl+r", "refresh_note", "Fetch", priority=True),
        Binding("escape", "blur_input", "Unfocus", show=False),
        Binding("q", "quit", "Quit", show=False),
        Binding("f", "focus_input", "Focus ID", show=False),
        Binding("r", "refresh_note", "Fetch", show=False),
    ]

    def __init__(
        self,
        client: NotesReader,
        *,
        endpoint_label: str,
        initial_name: str = "",
        timeout_seconds: float | None = None,
    ) -> None:
        super().__init__()
        self._client = client
        self._endpoint_label = endpoint_label
        self._initial_name = initial_name.strip()
        self._timeout_seconds = timeout_seconds
        self._request_seq = 0
        self.view_state = "idle"

    def compose(self) -> ComposeResult:
        yield Header()
        with Vertical(id="workspace"):
            yield Static(f"Endpoint: {self._endpoint_label}", id="endpoint-bar", markup=False)
            with Horizontal(id="query-bar"):
                yield Input(
                    value=self._initial_name,
                    placeholder="notes/<id>",
                    id="note-name-input",
                )
                yield Button("Fetch", id="fetch-button", variant="primary")
            yield Static(
                "Ready — enter a note ID (e.g. notes/<id>) and press Enter.",
                id="status",
                classes="state-idle",
                markup=False,
            )
            with VerticalScroll(id="result-panel"):
                yield Static("", id="note-title", classes="hidden", markup=False)
                yield Static("", id="note-meta", classes="hidden", markup=False)
                yield Static("", id="note-body", classes="hidden", markup=False)
                yield Static("", id="error-panel", classes="hidden", markup=False)
        yield Footer()

    def on_mount(self) -> None:
        name_input = self.query_one("#note-name-input", Input)
        name_input.focus()
        if self._initial_name:
            self._start_fetch(self._initial_name)

    def on_input_submitted(self, event: Input.Submitted) -> None:
        if event.input.id == "note-name-input":
            self._start_fetch(event.value.strip())

    def on_button_pressed(self, event: Button.Pressed) -> None:
        if event.button.id == "fetch-button":
            name = self.query_one("#note-name-input", Input).value.strip()
            self._start_fetch(name)

    def action_focus_input(self) -> None:
        self.query_one("#note-name-input", Input).focus()

    def action_blur_input(self) -> None:
        self.set_focus(None)

    def action_refresh_note(self) -> None:
        name = self.query_one("#note-name-input", Input).value.strip()
        self._start_fetch(name)

    def _set_status(self, state: str, text: str) -> None:
        self.view_state = state
        status = self.query_one("#status", Static)
        status.set_classes(f"state-{state}")
        status.update(text)

    def _clear_note_display(self) -> None:
        for selector in ("#note-title", "#note-meta", "#note-body"):
            widget = self.query_one(selector, Static)
            widget.update("")
            widget.add_class("hidden")

    def _show_error_panel(self, text: str) -> None:
        panel = self.query_one("#error-panel", Static)
        panel.update(text)
        panel.remove_class("hidden")

    def _clear_error_panel(self) -> None:
        panel = self.query_one("#error-panel", Static)
        panel.update("")
        panel.add_class("hidden")

    def _start_fetch(self, name: str) -> None:
        self._request_seq += 1
        seq = self._request_seq
        if not name:
            self.workers.cancel_group(self, "get-note")
            self.query_one("#fetch-button", Button).disabled = False
            self._clear_note_display()
            self._set_status("error", "Enter a note ID (e.g. notes/<id>) before fetching.")
            self._show_error_panel(
                "No note ID entered.\nNext step: Type a resource name such as notes/<id> and press Enter."
            )
            return

        self.query_one("#fetch-button", Button).disabled = True
        self._clear_error_panel()
        self._set_status("loading", f"Loading {name}...")
        self.run_worker(
            lambda: self._fetch_blocking(seq, name),
            name="get-note",
            group="get-note",
            exclusive=True,
            thread=True,
        )

    def _fetch_blocking(self, seq: int, name: str) -> None:
        request = notes_pb2.GetNoteRequest(name=name)
        try:
            response = self._client.get_note(request, timeout_seconds=self._timeout_seconds)
        except notes_connect.ConnectError as err:
            self.call_from_thread(self._apply_error, seq, err)
            return
        except Exception as err:
            fallback = notes_connect.ConnectError("unknown", f"unexpected client error: {err}")
            self.call_from_thread(self._apply_error, seq, fallback)
            return
        self.call_from_thread(self._apply_success, seq, response.note)

    def _apply_success(self, seq: int, note: notes_pb2.Note) -> None:
        if seq != self._request_seq:
            return
        self.query_one("#fetch-button", Button).disabled = False
        create_time = note.create_time.ToJsonString() if note.HasField("create_time") else ""
        self._set_status("success", f"Loaded {note.name}")
        title_widget = self.query_one("#note-title", Static)
        meta_widget = self.query_one("#note-meta", Static)
        body_widget = self.query_one("#note-body", Static)
        title_widget.update(f"Title: {note.title}")
        meta_widget.update(f"Name: {note.name} | Created: {create_time}")
        body_widget.update(note.body)
        for widget in (title_widget, meta_widget, body_widget):
            widget.remove_class("hidden")
        self._clear_error_panel()

    def _apply_error(self, seq: int, err: notes_connect.ConnectError) -> None:
        if seq != self._request_seq:
            return
        self.query_one("#fetch-button", Button).disabled = False
        self._clear_note_display()
        self._set_status("error", f"RPC error [{err.code}]: {err.message}")
        self._show_error_panel(format_connect_error(err))
