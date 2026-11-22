#!/usr/bin/env python3
"""Explicit, fail-closed exact-main CI release gate for BFF template image publication."""

import argparse
import json
import re
import subprocess
import sys

REPOSITORY = "selamy-labs/bazel-protobuf-bff-template"
SHA = re.compile(r"[0-9a-f]{40}\Z")
TAG = re.compile(r"v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?\Z")
DIGEST = re.compile(r"sha256:[0-9a-f]{64}\Z")
PINNED_IMAGE = re.compile(r"[a-z0-9.-]+(?::[0-9]+)?(?:/[a-z0-9._-]+)+@sha256:[0-9a-f]{64}\Z")
PROJECT = re.compile(r"[a-z][a-z0-9-]{4,28}[a-z0-9]\Z")
REGION = re.compile(r"[a-z]+-[a-z]+[0-9]+\Z")
REGISTRY = re.compile(r"[a-z][a-z0-9-]{0,61}[a-z0-9]\Z")
HANDOFF_URI = re.compile(r"gs://[a-z0-9][a-z0-9._-]+/[A-Za-z0-9._/-]+#[1-9][0-9]*\Z")
SHA256_HEX = re.compile(r"[0-9a-f]{64}\Z")
SA_SUFFIX = "".join((".", "iam", ".", "gserviceaccount", ".", "com"))
REQUIRED_JOBS = {"build-test", "e2e", "policy", "gate"}


def _require_strict_rules(rules: list) -> None:
    status_rules = [r.get("parameters", {}) for r in rules if r.get("type") == "required_status_checks"]
    has_strict_checks = any(
        p.get("strict_required_status_checks_policy") is True
        and REQUIRED_JOBS
        <= {c.get("context") for c in p.get("required_status_checks", []) if c.get("integration_id") == 15368}
        for p in status_rules
    )
    if not has_strict_checks:
        raise ValueError("effective rules lack strict GitHub Actions-bound required checks")
    review_rules = [r.get("parameters", {}) for r in rules if r.get("type") == "pull_request"]
    has_approval = any(
        p.get("required_approving_review_count", 0) >= 1 and p.get("require_last_push_approval") is True
        for p in review_rules
    )
    if not has_approval:
        raise ValueError("effective rules lack independent pull request approval")


def _select_main_run(runs: list, sha: str) -> dict:
    candidates = [
        r
        for r in runs
        if r.get("head_sha") == sha
        and r.get("event") == "push"
        and r.get("head_branch") == "main"
        and r.get("path") == ".github/workflows/ci.yml"
    ]
    if not candidates:
        raise ValueError("no exact-main CI run")
    return max(candidates, key=lambda r: r["id"])


def _require_green_jobs(jobs: list) -> None:
    selected = [j for j in jobs if j.get("name") in REQUIRED_JOBS]
    all_present = {j["name"] for j in selected} == REQUIRED_JOBS
    all_green = all(j.get("status") == "completed" and j.get("conclusion") == "success" for j in selected)
    if not all_present or not all_green:
        raise ValueError("required exact-main CI jobs are not all successful")


def check(branch: dict, rules: list, runs: list, jobs: list, sha: str) -> int:
    """Verify exact-main revision, strict ruleset, independent approval, and green CI."""
    if not SHA.fullmatch(sha) or branch.get("commit", {}).get("sha") != sha:
        raise ValueError("requested revision is not current main")
    _require_strict_rules(rules)
    run = _select_main_run(runs, sha)
    if run.get("status") != "completed" or run.get("conclusion") != "success":
        raise ValueError("latest exact-main CI run is not successful")
    _require_green_jobs(jobs)
    return int(run["id"])


def call(*args: str) -> str:
    return subprocess.check_output(args, text=True).strip()


def api(path: str):
    return json.loads(call("gh", "api", f"repos/{REPOSITORY}/{path}"))


def check_connection(data: dict, expected: str) -> None:
    """Confirm Cloud Build connection repository resolves only to this repository."""
    allowed = (f"https://github.com/{REPOSITORY}", f"https://github.com/{REPOSITORY}.git")
    if data.get("name") != expected or data.get("remoteUri") not in allowed:
        raise ValueError("Cloud Build repository does not resolve to BFF template")


def _validate_handoff_inputs(
    commit: str, tag: str, expected_digest: str, handoff_uri: str, handoff_sha256: str, publish_image: str
) -> None:
    valid_uri = bool(HANDOFF_URI.fullmatch(handoff_uri)) and ".." not in handoff_uri.split("/")
    if not valid_uri:
        raise ValueError("handoff URI must be a generation-pinned gs:// object without traversal")
    if not SHA256_HEX.fullmatch(handoff_sha256):
        raise ValueError("handoff archive SHA-256 must be a lowercase 64-hex digest")
    if (
        not SHA.fullmatch(commit)
        or not TAG.fullmatch(tag)
        or not DIGEST.fullmatch(expected_digest)
        or not PINNED_IMAGE.fullmatch(publish_image)
    ):
        raise ValueError("source revision, release tag, expected digest, and publisher image must be immutable")


def build_command(
    connection_repo: str,
    commit: str,
    tag: str,
    expected_digest: str,
    handoff_uri: str,
    handoff_sha256: str,
    project: str,
    region: str,
    repository: str,
    publisher_sa: str,
    publish_image: str,
) -> list[str]:
    """Validate immutable identifiers and return the gcloud builds submit command."""
    component = r"[A-Za-z0-9][A-Za-z0-9._-]*"
    conn_pattern = (
        rf"projects/{re.escape(project)}/locations/{re.escape(region)}"
        rf"/connections/{component}/repositories/{component}"
    )
    sa_pattern = (
        rf"projects/{re.escape(project)}/serviceAccounts/"
        rf"[a-z][a-z0-9-]{{4,28}}[a-z0-9]@{re.escape(project)}{re.escape(SA_SUFFIX)}"
    )
    if (
        not PROJECT.fullmatch(project)
        or not REGION.fullmatch(region)
        or not REGISTRY.fullmatch(repository)
        or not re.fullmatch(conn_pattern, connection_repo)
    ):
        raise ValueError("connection and registry must belong to the reviewed project and region")
    if not re.fullmatch(sa_pattern, publisher_sa):
        raise ValueError("publisher service account must belong to the reviewed project")
    _validate_handoff_inputs(commit, tag, expected_digest, handoff_uri, handoff_sha256, publish_image)
    substitutions = ",".join(
        (
            f"_PUBLISHER_SERVICE_ACCOUNT={publisher_sa}",
            f"_COMMIT={commit}",
            f"_TAG={tag}",
            f"_EXPECTED_DIGEST={expected_digest}",
            f"_HANDOFF_URI={handoff_uri}",
            f"_HANDOFF_SHA256={handoff_sha256}",
            f"_REGION={region}",
            f"_REPOSITORY={repository}",
            f"_PUBLISH_IMAGE={publish_image}",
        )
    )
    return [
        "gcloud",
        "builds",
        "submit",
        connection_repo,
        f"--revision={commit}",
        "--config=deploy/gcp/release/cloudbuild.yaml",
        f"--project={project}",
        f"--region={region}",
        f"--substitutions={substitutions}",
    ]


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    for name in (
        "commit",
        "tag",
        "expected-digest",
        "handoff-uri",
        "handoff-sha256",
        "project",
        "region",
        "repository",
        "connection-repo",
        "publisher-sa",
        "publish-image",
    ):
        parser.add_argument("--" + name, required=True)
    parser.add_argument("--execute", action="store_true", help="submit build after separately authorized review")
    args = parser.parse_args()
    if call("git", "rev-parse", "HEAD") != args.commit or call("git", "status", "--porcelain"):
        raise ValueError("gate must run from a clean checkout of the exact commit")
    branch = api("branches/main")
    rules = api("rules/branches/main?per_page=100")
    runs = api(f"actions/workflows/ci.yml/runs?event=push&branch=main&head_sha={args.commit}&per_page=100")[
        "workflow_runs"
    ]
    run = _select_main_run(runs, args.commit)
    jobs = api(f"actions/runs/{run['id']}/jobs?filter=latest&per_page=100")["jobs"]
    run_id = check(branch, rules, runs, jobs, args.commit)
    command = build_command(
        args.connection_repo,
        args.commit,
        args.tag,
        args.expected_digest,
        args.handoff_uri,
        args.handoff_sha256,
        args.project,
        args.region,
        args.repository,
        args.publisher_sa,
        args.publish_image,
    )
    connection = json.loads(call("gcloud", "builds", "repositories", "describe", args.connection_repo, "--format=json"))
    check_connection(connection, args.connection_repo)
    print(json.dumps({"commit": args.commit, "github_ci_run": run_id, "command": command, "submitted": args.execute}))
    if args.execute:
        subprocess.run(command, check=True)


if __name__ == "__main__":
    try:
        main()
    except (ValueError, KeyError, subprocess.CalledProcessError) as exc:
        print(f"release gate: {exc}", file=sys.stderr)
        sys.exit(1)
