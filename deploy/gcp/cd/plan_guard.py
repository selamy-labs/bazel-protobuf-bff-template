#!/usr/bin/env python3
"""Verify the digest, Cloud Run identity, runtime SA, and persistence wiring in a saved OpenTofu plan."""

import argparse
import hashlib
import json
import re
import subprocess
from pathlib import Path

IMAGE = re.compile(r"[a-z0-9.-]+(?:/[a-z0-9._-]+)+@sha256:[0-9a-f]{64}\Z")
SHA256 = re.compile(r"[0-9a-f]{64}\Z")


def contains_unknown_image(value) -> bool:
    """Return True if template, containers, or image fields remain unknown in the plan."""
    if isinstance(value, dict):
        return any(
            (key in ("template", "containers", "image", "service_account", "env") and item is True)
            or contains_unknown_image(item)
            for key, item in value.items()
        )
    if isinstance(value, list):
        return any(contains_unknown_image(item) for item in value)
    return False


def _collect_mutations(managed: list[dict]) -> dict[str, list[str]]:
    actual: dict[str, list[str]] = {}
    for resource in managed:
        actions = resource.get("change", {}).get("actions")
        if actions == ["no-op"]:
            continue
        address = resource.get("address")
        valid = (
            isinstance(address, str) and bool(address) and address not in actual and actions in (["create"], ["update"])
        )
        if not valid:
            raise ValueError("plan contains an invalid, duplicate, or unsupported mutation")
        actual[address] = actions
    return actual


def _verify_scope(managed: list[dict], expected_changes: dict[str, list[str]] | None) -> None:
    if expected_changes is not None:
        if not isinstance(expected_changes, dict) or not expected_changes:
            raise ValueError("expected mutation scope must be a nonempty address/action map")
        if _collect_mutations(managed) != expected_changes:
            raise ValueError("plan mutation scope differs from the reviewed address/action map")
    if any("delete" in r.get("change", {}).get("actions", []) for r in managed):
        raise ValueError("plan contains a delete or replacement")


def _verify_service_target(change: dict, project: str, region: str, service: str) -> dict:
    after = change.get("after") or {}
    unknown = change.get("after_unknown") or {}
    if any(unknown.get(k) for k in ("project", "location", "name")) or contains_unknown_image(unknown):
        raise ValueError("runtime identity or image is unknown")
    if (after.get("project"), after.get("location"), after.get("name")) != (project, region, service):
        raise ValueError("runtime identity does not match the approved target")
    return after


def _verify_notes_bucket(container: dict, expected_notes_bucket: str | None) -> None:
    if expected_notes_bucket is None:
        return
    env_map = {e.get("name"): e.get("value") for e in (container.get("env") or []) if isinstance(e, dict)}
    if env_map.get("BFF_STORE") != "object" or env_map.get("BFF_OBJECT_BUCKET") != expected_notes_bucket:
        raise ValueError("runtime persistence environment does not match approved notes bucket")


def _verify_runtime_container(
    after: dict,
    image: str,
    expected_runtime_sa: str | None,
    expected_notes_bucket: str | None,
) -> None:
    templates = after.get("template") or []
    if not isinstance(templates, list) or len(templates) != 1:
        raise ValueError("runtime must have exactly one template")
    tmpl = templates[0]
    if expected_runtime_sa is not None and tmpl.get("service_account") != expected_runtime_sa:
        raise ValueError("runtime service account does not match approved identity")
    containers = tmpl.get("containers") or []
    if not isinstance(containers, list) or len(containers) != 1 or containers[0].get("image") != image:
        raise ValueError("runtime must use exactly the approved image")
    _verify_notes_bucket(containers[0], expected_notes_bucket)


def check(
    plan: dict,
    image: str,
    project: str,
    region: str,
    service: str,
    expected_changes: dict[str, list[str]] | None = None,
    expected_runtime_sa: str | None = None,
    expected_notes_bucket: str | None = None,
) -> None:
    """Verify exact target, approved image digest, runtime SA, and bucket binding."""
    if not IMAGE.fullmatch(image):
        raise ValueError("expected image must be digest-pinned")
    changes = plan.get("resource_changes")
    if not isinstance(changes, list):
        raise ValueError("plan lacks resource changes")
    managed = [r for r in changes if r.get("mode") == "managed"]
    _verify_scope(managed, expected_changes)
    services = [r for r in managed if r.get("type") == "google_cloud_run_v2_service"]
    if len(services) != 1:
        raise ValueError("plan must contain exactly one Cloud Run service")
    after = _verify_service_target(services[0].get("change", {}), project, region, service)
    _verify_runtime_container(after, image, expected_runtime_sa, expected_notes_bucket)


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ("plan", "plan-sha256", "image", "project", "region", "service"):
        parser.add_argument("--" + name, required=True)
    parser.add_argument("--scope", help="JSON file containing the independently reviewed address/action map")
    parser.add_argument("--runtime-sa", help="Expected Cloud Run runtime service account email")
    parser.add_argument("--notes-bucket", help="Expected private GCS notes bucket name")
    args = parser.parse_args()
    if not SHA256.fullmatch(args.plan_sha256):
        raise ValueError("plan SHA-256 must be lowercase hexadecimal")
    path = Path(args.plan)
    digest = hashlib.sha256(path.read_bytes()).hexdigest()
    if digest != args.plan_sha256:
        raise ValueError("saved plan hash does not match the approved hash")
    rendered = subprocess.check_output(["tofu", "show", "-json", str(path)], text=True)
    if hashlib.sha256(path.read_bytes()).hexdigest() != digest:
        raise ValueError("saved plan changed during inspection")
    scope = json.loads(Path(args.scope).read_text(encoding="utf-8")) if args.scope else None
    check(
        json.loads(rendered),
        args.image,
        args.project,
        args.region,
        args.service,
        expected_changes=scope,
        expected_runtime_sa=args.runtime_sa,
        expected_notes_bucket=args.notes_bucket,
    )
    print(
        json.dumps(
            {
                "plan_sha256": digest,
                "image": args.image,
                "project": args.project,
                "region": args.region,
                "service": args.service,
            },
            sort_keys=True,
        )
    )


if __name__ == "__main__":
    main()
