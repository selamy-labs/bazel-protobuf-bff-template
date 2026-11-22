#!/usr/bin/env python3
"""Execute only an exact, native-approved Cloud Build saved-plan BFF deployment."""

import base64
import hashlib
import importlib.util
import json
import os
import re
import subprocess
import tempfile
import zipfile
from pathlib import Path, PurePosixPath

PINNED_TOFU_VERSION = "1.13.2"


def require(condition: bool, message: str) -> None:
    if not condition:
        raise ValueError(message)


def output(*args: str) -> str:
    return subprocess.check_output(args, text=True)


def sha(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def verify_approval(
    build: dict, project: str, build_id: str, trigger_id: str, account: str, approvers: list[str]
) -> None:
    """Confirm Cloud Build native human approval matches trigger, deployer SA, and reviewer list."""
    approval = build.get("approval") or {}
    result = approval.get("result") or {}
    require(build.get("projectId") == project and build.get("id") == build_id, "build identity mismatch")
    require(build.get("buildTriggerId") == trigger_id and bool(trigger_id), "unexpected trigger")
    require(build.get("serviceAccount") == account, "unexpected deployer identity")
    require(
        approval.get("state") == "APPROVED" and (approval.get("config") or {}).get("approvalRequired") is True,
        "native approval missing",
    )
    require(
        result.get("decision") == "APPROVED"
        and result.get("approverAccount") in approvers
        and bool(result.get("approvalTime")),
        "approved human evidence missing",
    )


def unpack(archive: Path, target: Path) -> None:
    """Safely extract the reviewed bundle archive without symlinks, traversal, or embedded state."""
    with zipfile.ZipFile(archive) as bundle:
        items = bundle.infolist()
        require(len(items) <= 5000 and sum(x.file_size for x in items) <= 100_000_000, "bundle too large")
        require(len({x.filename for x in items}) == len(items), "duplicate bundle member")
        for item in items:
            path = PurePosixPath(item.filename)
            is_symlink = (item.external_attr >> 16) & 0o170000 == 0o120000
            require(not path.is_absolute() and ".." not in path.parts and not is_symlink, "unsafe bundle member")
            require(".terraform" not in path.parts, "bundle cannot supply provider binaries or backend metadata")
        bundle.extractall(target)


def main() -> None:
    env = os.environ
    project, region, build_id = (env[k] for k in ("BFF_PROJECT", "BFF_REGION", "BUILD_ID"))
    trigger, account = (env[k] for k in ("BFF_TRIGGER_ID", "BFF_DEPLOYER"))
    approvers = json.loads(env["BFF_APPROVERS"])
    require(
        isinstance(approvers, list) and bool(approvers) and all(isinstance(a, str) and "@" in a for a in approvers),
        "missing human approvers",
    )

    build = json.loads(
        output("gcloud", "builds", "describe", build_id, "--project", project, "--region", region, "--format=json")
    )
    verify_approval(build, project, build_id, trigger, account, approvers)

    source = env["BFF_BUNDLE_URI"]
    require(
        re.fullmatch(r"gs://[a-z0-9][a-z0-9._-]+/[A-Za-z0-9._/-]+#[1-9][0-9]*", source) is not None,
        "bundle must name a specific object generation",
    )
    image = env["BFF_IMAGE"]
    digest = output(
        "gcloud", "artifacts", "docker", "images", "describe", image, "--format=value(image_summary.digest)"
    ).strip()
    require("@" in image and digest == image.rsplit("@", 1)[1], "registry digest mismatch")

    for name in ("BFF_BUNDLE_SHA256", "BFF_PLAN_SHA256"):
        require(re.fullmatch(r"[0-9a-f]{64}", env[name]) is not None, "invalid SHA-256")
    require(
        json.loads(output("tofu", "version", "-json"))["terraform_version"] == PINNED_TOFU_VERSION,
        "wrong OpenTofu version",
    )

    config_rel = env.get("BFF_CONFIG_RELPATH", "deploy/gcp/service")
    rel_path = PurePosixPath(config_rel)
    require(not rel_path.is_absolute() and ".." not in rel_path.parts, "unsafe config path")

    with tempfile.TemporaryDirectory(prefix="bff-release-") as directory:
        root = Path(directory)
        bundle = root / "bundle.zip"
        subprocess.run(["gcloud", "storage", "cp", source, str(bundle)], check=True)
        require(sha(bundle) == env["BFF_BUNDLE_SHA256"], "bundle hash mismatch")
        unpack(bundle, root)
        plan = root / "saved.plan"
        config = root / rel_path
        require(plan.is_file() and config.is_dir() and sha(plan) == env["BFF_PLAN_SHA256"], "saved plan hash mismatch")

        guard_file = root / "plan_guard.py"
        guard_file.write_bytes(base64.b64decode(env["BFF_GUARD_B64"], validate=True))
        spec = importlib.util.spec_from_file_location("plan_guard", guard_file)
        assert spec is not None and spec.loader is not None
        guard = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(guard)

        os.environ.update(TF_IN_AUTOMATION="1", TF_INPUT="0", TF_WORKSPACE="default", TF_DATA_DIR=str(root / "tfdata"))
        subprocess.run(["tofu", "-chdir=" + str(config), "init", "-input=false", "-lockfile=readonly"], check=True)
        rendered = json.loads(output("tofu", "-chdir=" + str(config), "show", "-json", str(plan)))
        require(sha(plan) == env["BFF_PLAN_SHA256"], "saved plan changed during inspection")
        guard.check(
            rendered,
            image,
            project,
            region,
            env["BFF_SERVICE"],
            expected_changes=json.loads(env["BFF_SCOPE"]),
            expected_runtime_sa=env.get("BFF_RUNTIME_SA"),
            expected_notes_bucket=env.get("BFF_NOTES_BUCKET"),
        )
        require(sha(plan) == env["BFF_PLAN_SHA256"], "saved plan changed before apply")
        subprocess.run(["tofu", "-chdir=" + str(config), "apply", "-input=false", str(plan)], check=True)


if __name__ == "__main__":
    main()
