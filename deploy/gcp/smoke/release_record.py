#!/usr/bin/env python3
"""Validate and emit the canonical BFF end-to-end Cloud Run release record."""

import argparse
import json
import re
import sys
import urllib.parse
from pathlib import Path

SHA40 = re.compile(r"[0-9a-f]{40}\Z")
DIGEST = re.compile(r"sha256:[0-9a-f]{64}\Z")

REQUIRED_KEYS = (
    "source_revision",
    "github_ci_run_id",
    "cloud_build_id",
    "image_digest",
    "dev_service_url",
    "dev_revision",
    "smoke_evidence",
    "dev_rollback_evidence",
)


def _normalize_https_url(raw: object, label: str) -> str:
    text = str(raw or "").strip().rstrip("/")
    parsed = urllib.parse.urlparse(text)
    if parsed.scheme != "https" or not parsed.netloc:
        raise ValueError(f"{label} must be an https URL with a host")
    return text


def _validate_core_fields(record: dict) -> str:
    missing = [key for key in REQUIRED_KEYS if key not in record]
    if missing:
        raise ValueError(f"release record missing keys: {', '.join(missing)}")
    if not SHA40.fullmatch(str(record["source_revision"])):
        raise ValueError("source_revision must be a 40-character commit SHA")
    run_id = record["github_ci_run_id"]
    if isinstance(run_id, bool) or not isinstance(run_id, int) or run_id <= 0:
        raise ValueError("github_ci_run_id must be a positive integer")
    if not str(record["cloud_build_id"]).strip():
        raise ValueError("cloud_build_id must be non-empty")
    if not DIGEST.fullmatch(str(record["image_digest"])):
        raise ValueError("image_digest must be sha256:<64 hex>")
    dev_url = _normalize_https_url(record["dev_service_url"], "dev_service_url")
    if not str(record["dev_revision"]).strip():
        raise ValueError("dev_revision must be non-empty")
    return dev_url


def _validate_smoke_evidence(
    smoke: object,
    expected_service_url: str,
    expected_revision: str,
    expected_digest: str,
    label: str = "smoke_evidence",
) -> None:
    if not isinstance(smoke, dict) or smoke.get("passed") is not True:
        raise ValueError(f"{label}.passed must be true")
    smoke_url = _normalize_https_url(smoke.get("service_url"), f"{label}.service_url")
    if smoke_url != expected_service_url:
        raise ValueError(f"{label}.service_url does not match dev_service_url")
    if str(smoke.get("observed_revision", "")).strip() != expected_revision:
        raise ValueError(f"{label}.observed_revision does not match claimed revision")
    if str(smoke.get("observed_image_digest", "")) != expected_digest:
        raise ValueError(f"{label}.observed_image_digest does not match claimed image digest")
    if smoke.get("durable_readback_verified") is not True or smoke.get("cleanup_verified_404") is not True:
        raise ValueError(f"{label} must verify durable readback and scoped cleanup")
    if smoke.get("anonymous_storage_status") not in (401, 403):
        raise ValueError(f"{label} must verify anonymous storage denial (401/403)")


def _validate_rollback_evidence(rb: object, image_digest: str, dev_revision: str, dev_service_url: str) -> None:
    if not isinstance(rb, dict):
        raise ValueError("dev_rollback_evidence must be an object")
    prev_digest = str(rb.get("previous_digest", ""))
    serving_after = str(rb.get("serving_digest_after_rollback", ""))
    rb_rev = str(rb.get("rollback_revision", "")).strip()
    if not DIGEST.fullmatch(prev_digest) or not DIGEST.fullmatch(serving_after):
        raise ValueError("dev_rollback_evidence digests must be valid sha256 digests")
    if prev_digest == image_digest:
        raise ValueError("rollback previous_digest must differ from forward image_digest")
    if serving_after != prev_digest:
        raise ValueError("serving_digest_after_rollback must match previous_digest")
    if not rb_rev or rb_rev == dev_revision:
        raise ValueError("rollback_revision must be non-empty and distinct from dev_revision")
    _validate_smoke_evidence(
        rb.get("rollback_smoke_evidence"),
        dev_service_url,
        rb_rev,
        prev_digest,
        label="dev_rollback_evidence.rollback_smoke_evidence",
    )


def validate_record(record: dict) -> dict:
    """Validate provenance links across CI, Cloud Build, dev rollout, smoke proof, and rollback."""
    dev_url = _validate_core_fields(record)
    _validate_smoke_evidence(
        record["smoke_evidence"], dev_url, str(record["dev_revision"]).strip(), record["image_digest"]
    )
    _validate_rollback_evidence(
        record["dev_rollback_evidence"], record["image_digest"], str(record["dev_revision"]).strip(), dev_url
    )
    return record


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--input", required=True, help="JSON release record candidate")
    parser.add_argument("--output", help="Optional destination path for normalized JSON record")
    args = parser.parse_args()
    data = json.loads(Path(args.input).read_text(encoding="utf-8"))
    validated = validate_record(data)
    rendered = json.dumps(validated, sort_keys=True, indent=2)
    print(rendered)
    if args.output:
        Path(args.output).write_text(rendered + "\n", encoding="utf-8")


if __name__ == "__main__":
    try:
        main()
    except (ValueError, json.JSONDecodeError) as exc:
        print(f"release record invalid: {exc}", file=sys.stderr)
        sys.exit(1)
