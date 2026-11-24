"""Interactive Textual TUI entrypoint calling the BFF Notes service via //api:notes_py (#34).

All domain validation, persistence, and subject isolation remain authoritative in the BFF service.
"""

from __future__ import annotations

import sys
from collections.abc import Sequence

import notes_tui_app
from absl import app, flags
from template.notes.v1 import notes_connect

_ENDPOINT = flags.DEFINE_string(
    "endpoint",
    "",
    "Base HTTP(S) URL of the Notes BFF service (for example http://127.0.0.1:8080).",
)
_SUBJECT = flags.DEFINE_string(
    "subject",
    "",
    "Synthetic development-profile subject sent as Bearer test-identity.<subject> (loopback only).",
)
_BEARER_TOKEN = flags.DEFINE_string(
    "bearer_token",
    "",
    "Explicit Bearer token for authenticated endpoints (mutually exclusive with --subject).",
)
_TIMEOUT_SECONDS = flags.DEFINE_float(
    "timeout_seconds",
    5.0,
    "RPC deadline in seconds (must be positive).",
)
_NAME = flags.DEFINE_string(
    "name",
    "",
    "Optional initial note resource name (notes/<id>) to load on startup.",
)


def _usage_error(message: str) -> int:
    print(f"notes-tui: usage error: {message}", file=sys.stderr)
    print(
        "usage: notes_tui --endpoint=<url> [--subject=<id>|--bearer_token=<tok>] [--name=notes/<id>]",
        file=sys.stderr,
    )
    return 2


def _endpoint_label(endpoint: str, subject: str, has_bearer: bool) -> str:
    cleaned = endpoint.strip().rstrip("/")
    if subject:
        return f"{cleaned} (subject={subject})"
    if has_bearer:
        return f"{cleaned} (bearer token)"
    return f"{cleaned} (unauthenticated)"


def execute(argv: Sequence[str]) -> int:
    if len(argv) > 1:
        return _usage_error(f"unexpected positional arguments: {' '.join(argv[1:])}")
    if not _ENDPOINT.value.strip():
        return _usage_error("--endpoint is required")

    try:
        client = notes_connect.NoteServiceClient(
            _ENDPOINT.value,
            bearer_token=_BEARER_TOKEN.value or None,
            test_subject=_SUBJECT.value or None,
            timeout_seconds=_TIMEOUT_SECONDS.value,
        )
    except ValueError as err:
        return _usage_error(str(err))

    tui = notes_tui_app.NotesTuiApp(
        client,
        endpoint_label=_endpoint_label(_ENDPOINT.value, _SUBJECT.value, bool(_BEARER_TOKEN.value)),
        initial_name=_NAME.value,
        timeout_seconds=_TIMEOUT_SECONDS.value,
    )
    tui.run()
    return 0


def _main(argv: Sequence[str]) -> None:
    raise SystemExit(execute(argv))


def main() -> None:
    app.run(_main)


if __name__ == "__main__":
    main()
