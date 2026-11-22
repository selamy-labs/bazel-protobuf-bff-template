#!/usr/bin/env python3
"""Verify immutable handoff transport and terminal actual-container runtime & verifier evidence."""

from __future__ import annotations

import argparse
import hashlib
import importlib.util
import json
import re
import sys
from pathlib import Path
from typing import Any

HEX64_RE = re.compile(r"^[0-9a-f]{64}$")
ALLOWED_BLOCKING_RESULTS = frozenset({"PASSED", "SKIPPED_NOT_APPLICABLE"})


def _load_verify_handoff(script_path: Path):
    spec = importlib.util.spec_from_file_location("verify_handoff", script_path)
    if spec is None or spec.loader is None:
        raise ValueError(f"unable to load verify_handoff from {script_path}")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def _sha256_regular_file(path: Path) -> str:
    if path.is_symlink() or not path.is_file():
        raise ValueError(f"expected regular non-symlink file: {path.name}")
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        while chunk := handle.read(65536):
            digest.update(chunk)
    return digest.hexdigest()


def extract_transport(archive: Path, expected_sha256: str, dest: Path, verify_handoff_script: Path) -> None:
    """Verify exact SHA-256 of the downloaded transport tarball and safely extract regular entries."""
    if not HEX64_RE.fullmatch(expected_sha256):
        raise ValueError("invalid expected handoff archive sha256")
    actual_sha256 = _sha256_regular_file(archive)
    if actual_sha256 != expected_sha256:
        raise ValueError(f"handoff transport archive sha256 mismatch ({actual_sha256} != {expected_sha256})")
    vh = _load_verify_handoff(verify_handoff_script)
    vh.read_tar_files(archive, extract_to=dest)


def _require_json_dict(raw: bytes | None, label: str) -> dict[str, Any]:
    if raw is None:
        raise ValueError(f"missing {label}")
    parsed = json.loads(raw.decode("utf-8"))
    if not isinstance(parsed, dict):
        raise ValueError(f"{label} must be a JSON object")
    return parsed


def _verify_command_result(runtime_files: dict[str, bytes], revision: str) -> None:
    src_rev_raw = runtime_files.get("source-revision.txt")
    if src_rev_raw is None or src_rev_raw.decode("utf-8").strip() != revision:
        raise ValueError("runtime acceptance source-revision.txt missing or mismatched")
    cmd = _require_json_dict(runtime_files.get("command-result.json"), "command-result.json")
    exit_code = cmd.get("exit")
    if isinstance(exit_code, bool) or exit_code != 0:
        raise ValueError("runtime acceptance command-result.json did not exit 0")
    if cmd.get("witness") != "actual-local-container" or cmd.get("source") != revision:
        raise ValueError("runtime acceptance command-result.json witness or source mismatch")


def _verify_image_identity(
    runtime_files: dict[str, bytes],
    handoff: Path,
    tag: str,
    revision: str,
    expected_digest: str,
    layout: Path,
) -> None:
    identity = _require_json_dict(runtime_files.get("image-identity.json"), "image-identity.json")
    manifest_hex = expected_digest.split(":", 1)[1]
    manifest = json.loads((layout / "blobs" / "sha256" / manifest_hex).read_text(encoding="utf-8"))
    expected_config_digest = manifest["config"]["digest"]
    expected_archive_digest = "sha256:" + _sha256_regular_file(handoff / f"bff-image-{tag}.tar")
    checks = (
        ("revision", revision),
        ("manifestDigest", expected_digest),
        ("configDigest", expected_config_digest),
        ("archiveDigest", expected_archive_digest),
    )
    for field, expected in checks:
        if identity.get(field) != expected:
            raise ValueError(f"runtime acceptance {field} mismatch")


def _suite_file(suite: dict[str, Any], inherited_file: str) -> str:
    candidate = suite.get("file")
    return candidate if isinstance(candidate, str) and candidate else inherited_file


def _collect_suite_specs(suite: Any, inherited_file: str) -> list[tuple[str, dict[str, Any]]]:
    if not isinstance(suite, dict):
        raise ValueError("Playwright suite entry must be an object")
    suite_file = _suite_file(suite, inherited_file)
    raw_specs = suite.get("specs") or []
    if not isinstance(raw_specs, list) or not all(isinstance(spec, dict) for spec in raw_specs):
        raise ValueError("Playwright spec entries must be objects")
    collected = [(suite_file, spec) for spec in raw_specs]
    if suite.get("suites"):
        collected.extend(_collect_playwright_specs(suite["suites"], suite_file))
    return collected


def _collect_playwright_specs(suites: Any, inherited_file: str = "") -> list[tuple[str, dict[str, Any]]]:
    if not isinstance(suites, list):
        raise ValueError("Playwright report suites must be a list")
    collected: list[tuple[str, dict[str, Any]]] = []
    for suite in suites:
        collected.extend(_collect_suite_specs(suite, inherited_file))
    return collected


def _verify_playwright_test(test: Any) -> None:
    results = test.get("results") if isinstance(test, dict) else None
    if not isinstance(results, list) or not results:
        raise ValueError("Playwright runtime test missing terminal result")
    if not isinstance(results[-1], dict) or results[-1].get("status") != "passed":
        raise ValueError("Playwright runtime test terminal status is not passed")


def _verify_playwright_spec(spec: dict[str, Any]) -> None:
    if spec.get("ok") is not True:
        raise ValueError(f"Playwright runtime spec failed: {spec.get('title', '<unnamed>')}")
    tests = spec.get("tests")
    if not isinstance(tests, list) or not tests:
        raise ValueError("Playwright runtime spec missing test executions")
    for test in tests:
        _verify_playwright_test(test)


def _verify_playwright_report(runtime_files: dict[str, bytes]) -> None:
    report = _require_json_dict(runtime_files.get("report.json"), "report.json")
    stats = report.get("stats")
    if not isinstance(stats, dict):
        raise ValueError("Playwright report.json missing stats")
    expected = stats.get("expected")
    if isinstance(expected, bool) or not isinstance(expected, int) or expected < 1:
        raise ValueError("Playwright report.json has no passing specs")
    if any(stats.get(k) != 0 for k in ("unexpected", "flaky", "skipped")):
        raise ValueError("Playwright report.json records unexpected, flaky, or skipped specs")
    specs = _collect_playwright_specs(report.get("suites"))
    if len(specs) != expected:
        raise ValueError("Playwright report.json spec count does not match expected stats")
    if not any(file_name.endswith("runtime.spec.mjs") for file_name, _ in specs):
        raise ValueError("Playwright report.json missing runtime.spec.mjs suite")
    for _, spec in specs:
        _verify_playwright_spec(spec)


def _verify_quality_check(entry: Any) -> None:
    if not isinstance(entry, dict):
        raise ValueError("verify receipt check entry must be an object")
    if entry.get("blocking", True) and entry.get("result") not in ALLOWED_BLOCKING_RESULTS:
        raise ValueError(f"verify receipt blocking check did not pass: {entry.get('check', '<unnamed>')}")


def _verify_quality_receipt(handoff: Path, tag: str, revision: str) -> None:
    receipt_path = handoff / f"verify-receipt-{tag}.json"
    receipt = _require_json_dict(receipt_path.read_bytes(), receipt_path.name)
    if receipt.get("shard") not in (None, ""):
        raise ValueError("verify receipt must be an unsharded full run, not a single shard")
    is_clean_pass = (
        receipt.get("revision") == revision
        and receipt.get("stage") == "full"
        and receipt.get("result") == "PASSED"
        and receipt.get("worktree_dirty") is False
        and receipt.get("offline") is False
        and not receipt.get("problems")
    )
    if not is_clean_pass:
        raise ValueError("verify receipt is not a clean full PASSED run for this revision")
    expected_checks = receipt.get("expected_checks")
    checks = receipt.get("checks")
    valid_count = (
        not isinstance(expected_checks, bool)
        and isinstance(expected_checks, int)
        and expected_checks >= 1
        and isinstance(checks, list)
        and len(checks) >= expected_checks
    )
    if not valid_count:
        raise ValueError("verify receipt checks are missing or incomplete")
    for entry in checks:
        _verify_quality_check(entry)


def verify_evidence(
    handoff: Path,
    tag: str,
    revision: str,
    expected_digest: str,
    layout: Path,
    verify_handoff_script: Path,
) -> None:
    """Verify complete terminal actual-container runtime acceptance and full verify receipt."""
    vh = _load_verify_handoff(verify_handoff_script)
    runtime_files = vh.read_tar_files(handoff / f"runtime-acceptance-{tag}.tar.gz")
    _verify_command_result(runtime_files, revision)
    _verify_image_identity(runtime_files, handoff, tag, revision, expected_digest, layout)
    _verify_playwright_report(runtime_files)
    _verify_quality_receipt(handoff, tag, revision)


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest="command", required=True)

    ext = sub.add_parser("extract-transport")
    ext.add_argument("--archive", type=Path, required=True)
    ext.add_argument("--expected-sha256", required=True)
    ext.add_argument("--dest", type=Path, required=True)
    ext.add_argument("--verify-handoff", type=Path, required=True)

    ver = sub.add_parser("verify-evidence")
    ver.add_argument("--handoff", type=Path, required=True)
    ver.add_argument("--tag", required=True)
    ver.add_argument("--revision", required=True)
    ver.add_argument("--expected-digest", required=True)
    ver.add_argument("--layout", type=Path, required=True)
    ver.add_argument("--verify-handoff", type=Path, required=True)

    args = parser.parse_args(argv)
    try:
        if args.command == "extract-transport":
            extract_transport(args.archive, args.expected_sha256, args.dest, args.verify_handoff)
        else:
            verify_evidence(
                args.handoff,
                args.tag,
                args.revision,
                args.expected_digest,
                args.layout,
                args.verify_handoff,
            )
    except Exception as exc:
        print(f"publish-image: {exc}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
