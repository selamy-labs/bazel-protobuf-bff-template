"""Thin absl-py Notes CLI calling the BFF Notes service via generated Protobuf/Connect bindings (#29).

All domain validation, persistence, and subject isolation remain authoritative in the BFF service.
"""

from __future__ import annotations

import json
import sys
from collections.abc import Sequence

from absl import app, flags
from google.protobuf import json_format
from template.notes.v1 import notes_connect, notes_pb2

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
_TITLE = flags.DEFINE_string(
    "title",
    "",
    "Note title for the `create` command (validated authoritatively by the BFF).",
)
_BODY = flags.DEFINE_string(
    "body",
    "",
    "Note body text for the `create` command.",
)
_NAME = flags.DEFINE_string(
    "name",
    "",
    "Resource name (notes/<id>) for the `get` command.",
)
_OUTPUT = flags.DEFINE_enum(
    "output",
    "text",
    ["text", "json"],
    "Output format for returned notes.",
)


def _usage_error(message: str) -> int:
    print(f"notes-cli: usage error: {message}", file=sys.stderr)
    print("usage: notes_cli <create|get> --endpoint=<url> [--subject=<id>|--bearer_token=<tok>]", file=sys.stderr)
    return 2


def _format_note(note: notes_pb2.Note, output_format: str) -> str:
    if output_format == "json":
        return json_format.MessageToJson(note, preserving_proto_field_name=False)
    create_time = note.create_time.ToJsonString() if note.HasField("create_time") else ""
    return "\n".join(
        [
            f"name: {note.name}",
            f"title: {note.title}",
            f"body: {note.body}",
            f"create_time: {create_time}",
        ]
    )


def _print_connect_error(err: notes_connect.ConnectError, output_format: str) -> None:
    if output_format == "json":
        payload = {
            "error": {
                "code": err.code,
                "message": err.message,
                "violations": [{"field": v.field, "rule_id": v.rule_id, "message": v.message} for v in err.violations],
            }
        }
        print(json.dumps(payload, sort_keys=True), file=sys.stderr)
        return
    print(f"notes-cli: rpc error [{err.code}]: {err.message}", file=sys.stderr)
    for violation in err.violations:
        location = violation.field or "<request>"
        print(f"  violation {location} ({violation.rule_id}): {violation.message}", file=sys.stderr)


def execute(argv: Sequence[str]) -> int:
    if len(argv) < 2:
        return _usage_error("missing command; expected `create` or `get`")
    if len(argv) > 2:
        return _usage_error(f"unexpected positional arguments: {' '.join(argv[2:])}")
    command = argv[1]
    if command not in ("create", "get"):
        return _usage_error(f"unknown command {command!r}; expected `create` or `get`")
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

    try:
        if command == "create":
            request = notes_pb2.CreateNoteRequest(
                note=notes_pb2.Note(title=_TITLE.value, body=_BODY.value),
            )
            response = client.create_note(request)
        else:
            if not _NAME.value.strip():
                return _usage_error("--name is required for `get`")
            request = notes_pb2.GetNoteRequest(name=_NAME.value)
            response = client.get_note(request)
    except notes_connect.ConnectError as err:
        _print_connect_error(err, _OUTPUT.value)
        return 1

    print(_format_note(response.note, _OUTPUT.value))
    return 0


def _main(argv: Sequence[str]) -> None:
    raise SystemExit(execute(argv))


def main() -> None:
    app.run(_main)


if __name__ == "__main__":
    main()
