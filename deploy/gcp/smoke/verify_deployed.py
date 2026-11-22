#!/usr/bin/env python3
"""Verify a deployed BFF Cloud Run service end-to-end with durable GCS readback and scoped cleanup."""

import argparse
import json
import sys
import urllib.error
import urllib.parse
import urllib.request
from dataclasses import dataclass


@dataclass(frozen=True)
class SmokeConfig:
    """Parameters describing a deployed BFF smoke verification target."""

    service_url: str
    bearer_token: str
    owner_subject: str
    bucket: str
    storage_token: str
    storage_endpoint: str = "https://storage.googleapis.com"
    prefix: str = "notes"
    title: str = "smoke-e2e-verification"
    body: str = "durable readback smoke payload"


def note_object_name(owner_sub: str, note_id: str, prefix: str = "notes") -> str:
    """Return the exact hex-encoded GCS object key used by bff/adapters/store/object."""
    if not owner_sub or not note_id:
        raise ValueError("owner_sub and note_id are required")
    clean_prefix = prefix.strip("/")
    if not clean_prefix or ".." in clean_prefix:
        raise ValueError("invalid object prefix")
    return f"{clean_prefix}/{owner_sub.encode('utf-8').hex()}/{note_id.encode('utf-8').hex()}.json"


def _http_call(
    url: str,
    method: str = "GET",
    headers: dict[str, str] | None = None,
    body: bytes | None = None,
    timeout: float = 15.0,
) -> tuple[int, dict[str, str], bytes]:
    req = urllib.request.Request(url, data=body, headers=headers or {}, method=method)
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            return resp.status, {k.lower(): v for k, v in resp.headers.items()}, resp.read()
    except urllib.error.HTTPError as err:
        return err.code, {k.lower(): v for k, v in err.headers.items()}, err.read()


def verify_same_origin_page(base_url: str) -> None:
    """Assert GET / serves HTTP 200 HTML for the same-origin frontend."""
    url = base_url.rstrip("/") + "/"
    status, headers, raw = _http_call(url)
    if status != 200:
        raise RuntimeError(f"GET {url} returned status {status}")
    content_type = headers.get("content-type", "")
    text = raw.decode("utf-8", errors="replace").lower()
    if "text/html" not in content_type and "<html" not in text:
        raise RuntimeError(f"GET {url} did not return HTML content")


def verify_unauthenticated_refused(base_url: str, title: str = "smoke-unauth", body: str = "probe") -> None:
    """Assert Connect CreateNote rejects missing bearer authorization with 401."""
    url = base_url.rstrip("/") + "/template.notes.v1.NoteService/CreateNote"
    payload = json.dumps({"note": {"title": title, "body": body}}).encode("utf-8")
    status, _, _ = _http_call(url, method="POST", headers={"Content-Type": "application/json"}, body=payload)
    if status != 401:
        raise RuntimeError(f"unauthenticated CreateNote expected 401, got {status}")


def create_and_get_note(base_url: str, bearer_token: str, title: str, body: str) -> str:
    """Call Connect CreateNote and GetNote as the authenticated caller and return note_id."""
    if not bearer_token:
        raise ValueError("bearer_token is required")
    headers = {
        "Authorization": f"Bearer {bearer_token}",
        "Content-Type": "application/json",
    }
    create_url = base_url.rstrip("/") + "/template.notes.v1.NoteService/CreateNote"
    create_body = json.dumps({"note": {"title": title, "body": body}}).encode("utf-8")
    status, _, raw = _http_call(create_url, method="POST", headers=headers, body=create_body)
    if status != 200:
        raise RuntimeError(f"CreateNote failed with HTTP {status}")
    created = json.loads(raw.decode("utf-8"))
    note = created.get("note") or {}
    resource_name = note.get("name", "")
    if not resource_name.startswith("notes/"):
        raise RuntimeError(f"CreateNote returned invalid resource name: {resource_name!r}")
    note_id = resource_name.removeprefix("notes/")

    get_url = base_url.rstrip("/") + "/template.notes.v1.NoteService/GetNote"
    get_body = json.dumps({"name": resource_name}).encode("utf-8")
    get_status, _, get_raw = _http_call(get_url, method="POST", headers=headers, body=get_body)
    if get_status != 200:
        raise RuntimeError(f"GetNote failed with HTTP {get_status}")
    fetched = (json.loads(get_raw.decode("utf-8"))).get("note") or {}
    if fetched.get("name") != resource_name or fetched.get("title") != title or fetched.get("body") != body:
        raise RuntimeError("GetNote readback mismatch against CreateNote response")
    return note_id


def _gcs_object_url(storage_endpoint: str, bucket: str, object_name: str, media: bool) -> str:
    b_enc = urllib.parse.quote(bucket, safe="")
    o_enc = urllib.parse.quote(object_name, safe="")
    suffix = "?alt=media" if media else ""
    return f"{storage_endpoint.rstrip('/')}/storage/v1/b/{b_enc}/o/{o_enc}{suffix}"


def verify_anonymous_storage_denied(storage_endpoint: str, bucket: str, object_name: str) -> int:
    """Confirm unauthenticated GCS read of the note object fails with 401 or 403."""
    url = _gcs_object_url(storage_endpoint, bucket, object_name, media=True)
    status, _, _ = _http_call(url, method="GET")
    if status not in (401, 403):
        raise RuntimeError(f"anonymous storage read expected 401/403, got {status}")
    return status


def verify_durable_readback(
    storage_endpoint: str,
    bucket: str,
    object_name: str,
    storage_token: str,
    expected_owner: str,
    expected_id: str,
    expected_title: str,
    expected_body: str,
) -> dict:
    """Read the persisted JSON note directly from GCS using an authorized token."""
    if not storage_token:
        raise ValueError("storage_token is required")
    url = _gcs_object_url(storage_endpoint, bucket, object_name, media=True)
    status, _, raw = _http_call(url, method="GET", headers={"Authorization": f"Bearer {storage_token}"})
    if status != 200:
        raise RuntimeError(f"authorized storage readback returned HTTP {status}")
    record = json.loads(raw.decode("utf-8"))
    if (
        record.get("owner") != expected_owner
        or record.get("id") != expected_id
        or record.get("title") != expected_title
        or record.get("body") != expected_body
    ):
        raise RuntimeError("durable storage record mismatch")
    return record


def cleanup_synthetic_note(
    storage_endpoint: str,
    bucket: str,
    object_name: str,
    storage_token: str,
    title: str,
    expected_owner: str,
    expected_id: str,
    prefix: str = "notes",
) -> None:
    """Delete only a synthetic `smoke-` object whose key matches owner+id and verify 404."""
    if not title.startswith("smoke-"):
        raise ValueError("refusing cleanup: synthetic title must start with 'smoke-'")
    expected_key = note_object_name(expected_owner, expected_id, prefix=prefix)
    if object_name != expected_key:
        raise ValueError("refusing cleanup: object key does not match synthetic owner/id")
    headers = {"Authorization": f"Bearer {storage_token}"}
    del_url = _gcs_object_url(storage_endpoint, bucket, object_name, media=False)
    del_status, _, _ = _http_call(del_url, method="DELETE", headers=headers)
    if del_status not in (200, 204):
        raise RuntimeError(f"synthetic note delete returned HTTP {del_status}")
    check_url = _gcs_object_url(storage_endpoint, bucket, object_name, media=True)
    check_status, _, _ = _http_call(check_url, method="GET", headers=headers)
    if check_status != 404:
        raise RuntimeError(f"deleted note still accessible after cleanup: HTTP {check_status}")


def run_smoke(cfg: SmokeConfig) -> dict:
    """Execute the full live verification, returning a non-secret JSON receipt."""
    if not cfg.title.startswith("smoke-"):
        raise ValueError("smoke title must start with 'smoke-' so cleanup stays strictly scoped")
    verify_same_origin_page(cfg.service_url)
    verify_unauthenticated_refused(cfg.service_url)
    note_id = create_and_get_note(cfg.service_url, cfg.bearer_token, cfg.title, cfg.body)
    object_name = note_object_name(cfg.owner_subject, note_id, prefix=cfg.prefix)
    anon_status = verify_anonymous_storage_denied(cfg.storage_endpoint, cfg.bucket, object_name)
    record = verify_durable_readback(
        cfg.storage_endpoint,
        cfg.bucket,
        object_name,
        cfg.storage_token,
        cfg.owner_subject,
        note_id,
        cfg.title,
        cfg.body,
    )
    cleanup_synthetic_note(
        cfg.storage_endpoint,
        cfg.bucket,
        object_name,
        cfg.storage_token,
        cfg.title,
        cfg.owner_subject,
        note_id,
        prefix=cfg.prefix,
    )
    return {
        "service_url": cfg.service_url,
        "bucket": cfg.bucket,
        "note_id": note_id,
        "object_name": object_name,
        "create_time": record.get("create_time"),
        "anonymous_storage_status": anon_status,
        "durable_readback_verified": True,
        "cleanup_verified_404": True,
        "passed": True,
    }


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--service-url", required=True)
    parser.add_argument("--bearer-token", required=True)
    parser.add_argument("--owner-subject", required=True)
    parser.add_argument("--bucket", required=True)
    parser.add_argument("--storage-token", required=True)
    parser.add_argument("--storage-endpoint", default="https://storage.googleapis.com")
    parser.add_argument("--prefix", default="notes")
    parser.add_argument("--title", default="smoke-e2e-verification")
    parser.add_argument("--body", default="durable readback smoke payload")
    parser.add_argument("--out", help="Optional path to write JSON smoke receipt")
    args = parser.parse_args()
    cfg = SmokeConfig(
        service_url=args.service_url,
        bearer_token=args.bearer_token,
        owner_subject=args.owner_subject,
        bucket=args.bucket,
        storage_token=args.storage_token,
        storage_endpoint=args.storage_endpoint,
        prefix=args.prefix,
        title=args.title,
        body=args.body,
    )
    receipt = run_smoke(cfg)
    payload = json.dumps(receipt, sort_keys=True, indent=2)
    print(payload)
    if args.out:
        with open(args.out, "w", encoding="utf-8") as handle:
            handle.write(payload + "\n")


if __name__ == "__main__":
    try:
        main()
    except (RuntimeError, ValueError, json.JSONDecodeError) as exc:
        print(f"smoke check failed: {exc}", file=sys.stderr)
        sys.exit(1)
