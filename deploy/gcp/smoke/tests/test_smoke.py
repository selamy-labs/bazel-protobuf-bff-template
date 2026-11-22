"""End-to-end smoke verification and release-record tests using local HTTP/GCS stubs."""

import importlib.util
import json
import threading
import unittest
import urllib.parse
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path

SMOKE_DIR = Path(__file__).resolve().parents[1]


def load_module(name: str, path: Path):
    spec = importlib.util.spec_from_file_location(name, path)
    assert spec is not None and spec.loader is not None
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


verify_deployed = load_module("verify_deployed", SMOKE_DIR / "verify_deployed.py")
release_record = load_module("release_record", SMOKE_DIR / "release_record.py")


def start_stub_servers(
    allow_anonymous_gcs: bool = False, corrupt_stored_body: bool = False, allow_unauth_create: bool = False
):
    state = {"objects": {}, "counter": 0}
    lock = threading.Lock()
    expected_bucket = "example-notes-bucket"
    owner_sub = "user-sub-123"
    bff_token = "synthetic-bff-token"
    gcs_token = "synthetic-gcs-token"

    class Handler(BaseHTTPRequestHandler):
        def log_message(self, format, *args):
            return

        def _send(self, code: int, content_type: str, body: bytes):
            self.send_response(code)
            self.send_header("Content-Type", content_type)
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

        def do_GET(self):
            parsed = urllib.parse.urlparse(self.path)
            if parsed.path == "/":
                self._send(200, "text/html; charset=utf-8", b"<!doctype html><html><body>BFF</body></html>")
                return
            prefix = f"/storage/v1/b/{expected_bucket}/o/"
            if parsed.path.startswith(prefix):
                auth = self.headers.get("Authorization")
                if not auth:
                    if allow_anonymous_gcs:
                        pass
                    else:
                        self._send(403, "application/json", b'{"error":"forbidden"}')
                        return
                elif auth != f"Bearer {gcs_token}":
                    self._send(401, "application/json", b'{"error":"unauthorized"}')
                    return
                key = urllib.parse.unquote(parsed.path.removeprefix(prefix))
                with lock:
                    obj = state["objects"].get(key)
                if obj is None:
                    self._send(404, "application/json", b'{"error":"not found"}')
                    return
                self._send(200, "application/json", json.dumps(obj).encode("utf-8"))
                return
            self._send(404, "text/plain", b"not found")

        def do_POST(self):
            length = int(self.headers.get("Content-Length", "0"))
            raw = self.rfile.read(length)
            payload = json.loads(raw.decode("utf-8")) if raw else {}
            auth = self.headers.get("Authorization")
            if self.path == "/template.notes.v1.NoteService/CreateNote":
                if not auth and not allow_unauth_create:
                    self._send(401, "application/json", b'{"code":"unauthenticated"}')
                    return
                if auth and auth != f"Bearer {bff_token}":
                    self._send(401, "application/json", b'{"code":"unauthenticated"}')
                    return
                note_in = payload.get("note") or {}
                with lock:
                    state["counter"] += 1
                    note_id = f"n-{state['counter']}"
                    stored = {
                        "owner": owner_sub,
                        "id": note_id,
                        "title": note_in["title"],
                        "body": "CORRUPTED" if corrupt_stored_body else note_in["body"],
                        "create_time": "2026-11-21T00:00:00Z",
                    }
                    key = verify_deployed.note_object_name(owner_sub, note_id)
                    state["objects"][key] = stored
                resp = {
                    "note": {
                        "name": f"notes/{note_id}",
                        "title": note_in["title"],
                        "body": note_in["body"],
                        "createTime": stored["create_time"],
                    }
                }
                self._send(200, "application/json", json.dumps(resp).encode("utf-8"))
                return
            if self.path == "/template.notes.v1.NoteService/GetNote":
                if auth != f"Bearer {bff_token}":
                    self._send(401, "application/json", b'{"code":"unauthenticated"}')
                    return
                note_id = payload["name"].removeprefix("notes/")
                key = verify_deployed.note_object_name(owner_sub, note_id)
                with lock:
                    obj = state["objects"].get(key)
                if obj is None:
                    self._send(404, "application/json", b'{"code":"not_found"}')
                    return
                resp = {
                    "note": {
                        "name": f"notes/{note_id}",
                        "title": obj["title"],
                        "body": payload.get("echo")
                        or (obj["body"] if not corrupt_stored_body else "durable readback smoke payload"),
                        "createTime": obj["create_time"],
                    }
                }
                self._send(200, "application/json", json.dumps(resp).encode("utf-8"))
                return
            self._send(404, "text/plain", b"not found")

        def do_DELETE(self):
            prefix = f"/storage/v1/b/{expected_bucket}/o/"
            if self.path.startswith(prefix):
                if self.headers.get("Authorization") != f"Bearer {gcs_token}":
                    self._send(401, "application/json", b'{"error":"unauthorized"}')
                    return
                key = urllib.parse.unquote(self.path.removeprefix(prefix))
                with lock:
                    existed = state["objects"].pop(key, None)
                self._send(204 if existed is not None else 404, "application/json", b"")
                return
            self._send(404, "text/plain", b"not found")

    server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    url = f"http://127.0.0.1:{server.server_port}"
    return server, url, owner_sub, expected_bucket, bff_token, gcs_token, state


class DeployedSmokeTests(unittest.TestCase):
    def test_full_smoke_and_cleanup_lifecycle(self):
        srv, url, owner, bucket, bff_tok, gcs_tok, state = start_stub_servers()
        try:
            cfg = verify_deployed.SmokeConfig(
                service_url=url,
                bearer_token=bff_tok,
                owner_subject=owner,
                bucket=bucket,
                storage_token=gcs_tok,
                storage_endpoint=url,
            )
            receipt = verify_deployed.run_smoke(cfg)
            self.assertTrue(receipt["passed"])
            self.assertEqual(receipt["anonymous_storage_status"], 403)
            self.assertTrue(receipt["durable_readback_verified"])
            self.assertTrue(receipt["cleanup_verified_404"])
            self.assertEqual(state["objects"], {})
        finally:
            srv.shutdown()
            srv.server_close()

    def test_fails_if_anonymous_gcs_returns_200(self):
        srv, url, owner, bucket, bff_tok, gcs_tok, _ = start_stub_servers(allow_anonymous_gcs=True)
        try:
            cfg = verify_deployed.SmokeConfig(
                service_url=url,
                bearer_token=bff_tok,
                owner_subject=owner,
                bucket=bucket,
                storage_token=gcs_tok,
                storage_endpoint=url,
            )
            with self.assertRaisesRegex(RuntimeError, "anonymous storage read expected 401/403"):
                verify_deployed.run_smoke(cfg)
        finally:
            srv.shutdown()
            srv.server_close()

    def test_fails_if_durable_readback_mismatches(self):
        srv, url, owner, bucket, bff_tok, gcs_tok, _ = start_stub_servers(corrupt_stored_body=True)
        try:
            cfg = verify_deployed.SmokeConfig(
                service_url=url,
                bearer_token=bff_tok,
                owner_subject=owner,
                bucket=bucket,
                storage_token=gcs_tok,
                storage_endpoint=url,
            )
            with self.assertRaisesRegex(RuntimeError, "durable storage record mismatch"):
                verify_deployed.run_smoke(cfg)
        finally:
            srv.shutdown()
            srv.server_close()

    def test_fails_if_unauthenticated_connect_returns_200(self):
        srv, url, owner, bucket, bff_tok, gcs_tok, _ = start_stub_servers(allow_unauth_create=True)
        try:
            cfg = verify_deployed.SmokeConfig(
                service_url=url,
                bearer_token=bff_tok,
                owner_subject=owner,
                bucket=bucket,
                storage_token=gcs_tok,
                storage_endpoint=url,
            )
            with self.assertRaisesRegex(RuntimeError, "unauthenticated CreateNote expected 401"):
                verify_deployed.run_smoke(cfg)
        finally:
            srv.shutdown()
            srv.server_close()

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
