"""End-to-end smoke verification and release-record tests using local HTTP/GCS stubs."""

import importlib.util
import json
import threading
import unittest
import urllib.parse
from contextlib import contextmanager
from dataclasses import dataclass, field
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path

SMOKE_DIR = Path(__file__).resolve().parents[1]
EXPECTED_BUCKET = "example-notes-bucket"
OWNER_SUB = "user-sub-123"
BFF_TOKEN = "synthetic-bff-token"
GCS_TOKEN = "synthetic-gcs-token"
GCS_PREFIX = f"/storage/v1/b/{EXPECTED_BUCKET}/o/"


def load_module(name: str, path: Path):
    spec = importlib.util.spec_from_file_location(name, path)
    assert spec is not None and spec.loader is not None
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


verify_deployed = load_module("verify_deployed", SMOKE_DIR / "verify_deployed.py")
release_record = load_module("release_record", SMOKE_DIR / "release_record.py")


@dataclass
class _StubState:
    allow_anonymous_gcs: bool = False
    corrupt_stored_body: bool = False
    allow_unauth_create: bool = False
    objects: dict = field(default_factory=dict)
    counter: int = 0
    lock: threading.Lock = field(default_factory=threading.Lock)


class _StubServer(ThreadingHTTPServer):
    def __init__(self, state: _StubState):
        self.stub_state = state
        super().__init__(("127.0.0.1", 0), _StubHandler)


class _StubHandler(BaseHTTPRequestHandler):
    server: _StubServer

    def log_message(self, format, *args):
        return

    def _send(self, code: int, content_type: str, body: bytes):
        self.send_response(code)
        self.send_header("Content-Type", content_type)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def _send_json(self, code: int, body: dict):
        self._send(code, "application/json", json.dumps(body).encode())

    def _check_gcs_read_auth(self, auth: str | None) -> bool:
        if not auth:
            if self.server.stub_state.allow_anonymous_gcs:
                return True
            self._send_json(403, {"error": "forbidden"})
            return False
        if auth != f"Bearer {GCS_TOKEN}":
            self._send_json(401, {"error": "unauthorized"})
            return False
        return True

    def _handle_gcs_get(self, path: str):
        if not self._check_gcs_read_auth(self.headers.get("Authorization")):
            return
        key = urllib.parse.unquote(path.removeprefix(GCS_PREFIX))
        with self.server.stub_state.lock:
            obj = self.server.stub_state.objects.get(key)
        if obj is None:
            self._send_json(404, {"error": "not found"})
            return
        self._send_json(200, obj)

    def _handle_create_note(self, auth: str | None, payload: dict):
        state = self.server.stub_state
        if (not auth and not state.allow_unauth_create) or (auth and auth != f"Bearer {BFF_TOKEN}"):
            self._send_json(401, {"code": "unauthenticated"})
            return
        note_in = payload.get("note") or {}
        with state.lock:
            state.counter += 1
            note_id = f"n-{state.counter}"
            stored = {
                "owner": OWNER_SUB,
                "id": note_id,
                "title": note_in["title"],
                "body": "CORRUPTED" if state.corrupt_stored_body else note_in["body"],
                "create_time": "2026-11-21T00:00:00Z",
            }
            state.objects[verify_deployed.note_object_name(OWNER_SUB, note_id)] = stored
        self._send_json(
            200,
            {
                "note": {
                    "name": f"notes/{note_id}",
                    "title": note_in["title"],
                    "body": note_in["body"],
                    "createTime": stored["create_time"],
                }
            },
        )

    def _handle_get_note(self, auth: str | None, payload: dict):
        if auth != f"Bearer {BFF_TOKEN}":
            self._send_json(401, {"code": "unauthenticated"})
            return
        note_id = payload["name"].removeprefix("notes/")
        key = verify_deployed.note_object_name(OWNER_SUB, note_id)
        with self.server.stub_state.lock:
            obj = self.server.stub_state.objects.get(key)
        if obj is None:
            self._send_json(404, {"code": "not_found"})
            return
        default_body = "durable readback smoke payload" if self.server.stub_state.corrupt_stored_body else obj["body"]
        self._send_json(
            200,
            {
                "note": {
                    "name": f"notes/{note_id}",
                    "title": obj["title"],
                    "body": payload.get("echo") or default_body,
                    "createTime": obj["create_time"],
                }
            },
        )

    def do_GET(self):
        path = urllib.parse.urlparse(self.path).path
        if path == "/":
            self._send(200, "text/html; charset=utf-8", b"<!doctype html><html><body>BFF</body></html>")
            return
        if path.startswith(GCS_PREFIX):
            self._handle_gcs_get(path)
            return
        self._send(404, "text/plain", b"not found")

    def do_POST(self):
        raw = self.rfile.read(int(self.headers.get("Content-Length", "0")))
        payload = json.loads(raw.decode()) if raw else {}
        auth = self.headers.get("Authorization")
        if self.path == "/template.notes.v1.NoteService/CreateNote":
            self._handle_create_note(auth, payload)
            return
        if self.path == "/template.notes.v1.NoteService/GetNote":
            self._handle_get_note(auth, payload)
            return
        self._send(404, "text/plain", b"not found")

    def do_DELETE(self):
        if not self.path.startswith(GCS_PREFIX):
            self._send(404, "text/plain", b"not found")
            return
        if self.headers.get("Authorization") != f"Bearer {GCS_TOKEN}":
            self._send_json(401, {"error": "unauthorized"})
            return
        key = urllib.parse.unquote(self.path.removeprefix(GCS_PREFIX))
        with self.server.stub_state.lock:
            existed = self.server.stub_state.objects.pop(key, None)
        self._send(204 if existed is not None else 404, "application/json", b"")


@contextmanager
def running_stub_server(
    allow_anonymous_gcs: bool = False, corrupt_stored_body: bool = False, allow_unauth_create: bool = False
):
    state = _StubState(allow_anonymous_gcs, corrupt_stored_body, allow_unauth_create)
    server = _StubServer(state)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    url = f"http://127.0.0.1:{server.server_port}"
    cfg = verify_deployed.SmokeConfig(
        service_url=url,
        bearer_token=BFF_TOKEN,
        owner_subject=OWNER_SUB,
        bucket=EXPECTED_BUCKET,
        storage_token=GCS_TOKEN,
        storage_endpoint=url,
    )
    try:
        yield cfg, state
    finally:
        server.shutdown()
        server.server_close()


class DeployedSmokeTests(unittest.TestCase):
    def test_full_smoke_and_cleanup_lifecycle(self):
        with running_stub_server() as (cfg, state):
            receipt = verify_deployed.run_smoke(cfg)
            self.assertTrue(receipt["passed"])
            self.assertEqual(receipt["anonymous_storage_status"], 403)
            self.assertTrue(receipt["durable_readback_verified"])
            self.assertTrue(receipt["cleanup_verified_404"])
            self.assertEqual(state.objects, {})

    def test_fails_if_anonymous_gcs_returns_200(self):
        with running_stub_server(allow_anonymous_gcs=True) as (cfg, _):
            with self.assertRaisesRegex(RuntimeError, "anonymous storage read expected 401/403"):
                verify_deployed.run_smoke(cfg)

    def test_fails_if_durable_readback_mismatches(self):
        with running_stub_server(corrupt_stored_body=True) as (cfg, _):
            with self.assertRaisesRegex(RuntimeError, "durable storage record mismatch"):
                verify_deployed.run_smoke(cfg)

    def test_fails_if_unauthenticated_connect_returns_200(self):
        with running_stub_server(allow_unauth_create=True) as (cfg, _):
            with self.assertRaisesRegex(RuntimeError, "unauthenticated CreateNote expected 401"):
                verify_deployed.run_smoke(cfg)

    def test_cleanup_refuses_non_smoke_prefix(self):
        with self.assertRaisesRegex(ValueError, "must start with 'smoke-'"):
            verify_deployed.cleanup_synthetic_note(
                "https://storage.example.invalid",
                "bucket",
                verify_deployed.note_object_name("owner", "id1"),
                "tok",
                "real-user-note",
                "owner",
                "id1",
            )


class ReleaseRecordTests(unittest.TestCase):
    def _valid_record(self):
        forward = "sha256:" + ("a" * 64)
        previous = "sha256:" + ("b" * 64)
        url = "https://bff-dev.example.invalid"
        return {
            "source_revision": "c" * 40,
            "github_ci_run_id": 44891290772,
            "cloud_build_id": "cb-0001",
            "image_digest": forward,
            "dev_service_url": url,
            "dev_revision": "bff-00002-abc",
            "smoke_evidence": {
                "service_url": url,
                "observed_revision": "bff-00002-abc",
                "observed_image_digest": forward,
                "passed": True,
                "anonymous_storage_status": 403,
                "durable_readback_verified": True,
                "cleanup_verified_404": True,
            },
            "dev_rollback_evidence": {
                "previous_digest": previous,
                "serving_digest_after_rollback": previous,
                "rollback_revision": "bff-00001-xyz",
                "rollback_smoke_evidence": {
                    "service_url": url,
                    "observed_revision": "bff-00001-xyz",
                    "observed_image_digest": previous,
                    "passed": True,
                    "anonymous_storage_status": 403,
                    "durable_readback_verified": True,
                    "cleanup_verified_404": True,
                },
            },
        }

    def test_valid_release_record_passes(self):
        rec = self._valid_record()
        self.assertEqual(release_record.validate_record(rec), rec)

    def test_rejects_missing_fields_or_identical_rollback_digest(self):
        rec = self._valid_record()
        rec["dev_rollback_evidence"]["previous_digest"] = rec["image_digest"]
        rec["dev_rollback_evidence"]["serving_digest_after_rollback"] = rec["image_digest"]
        with self.assertRaisesRegex(ValueError, "must differ"):
            release_record.validate_record(rec)

        incomplete = self._valid_record()
        del incomplete["cloud_build_id"]
        with self.assertRaisesRegex(ValueError, "missing keys"):
            release_record.validate_record(incomplete)

    def test_rejects_mixed_target_smoke_receipt(self):
        rec = self._valid_record()
        rec["smoke_evidence"]["service_url"] = "https://other-service.example.invalid"
        with self.assertRaisesRegex(ValueError, "service_url does not match"):
            release_record.validate_record(rec)

        missing_url = self._valid_record()
        del missing_url["smoke_evidence"]["service_url"]
        with self.assertRaisesRegex(ValueError, "https URL"):
            release_record.validate_record(missing_url)

    def test_rejects_unmatched_observed_revision_digest_or_rollback_smoke(self):
        bad_rev = self._valid_record()
        bad_rev["smoke_evidence"]["observed_revision"] = "bff-99999-zzz"
        with self.assertRaisesRegex(ValueError, "observed_revision"):
            release_record.validate_record(bad_rev)

        bad_dig = self._valid_record()
        bad_dig["smoke_evidence"]["observed_image_digest"] = "sha256:" + ("9" * 64)
        with self.assertRaisesRegex(ValueError, "observed_image_digest"):
            release_record.validate_record(bad_dig)

        bad_rb = self._valid_record()
        bad_rb["dev_rollback_evidence"]["rollback_smoke_evidence"]["service_url"] = "https://wrong.example.invalid"
        with self.assertRaisesRegex(ValueError, "service_url does not match"):
            release_record.validate_record(bad_rb)
