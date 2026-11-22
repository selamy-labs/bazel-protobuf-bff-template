"""Unit and sandboxed shell execution tests for the BFF release gate and qualified-handoff publisher."""

import copy
import hashlib
import importlib.util
import io
import json
import os
import re
import shutil
import subprocess
import sys
import tarfile
import tempfile
import unittest
from pathlib import Path

RELEASE_DIR = Path(__file__).resolve().parents[1]
REPO_ROOT = RELEASE_DIR.parents[2]
VERIFY_HANDOFF = Path(
    os.environ.get("RELEASE_VERIFY_HANDOFF_SCRIPT") or (REPO_ROOT / "tools" / "release" / "verify_handoff.py")
)

GATE_SPEC = importlib.util.spec_from_file_location("release_gate", RELEASE_DIR / "gate.py")
assert GATE_SPEC is not None and GATE_SPEC.loader is not None
gate = importlib.util.module_from_spec(GATE_SPEC)
GATE_SPEC.loader.exec_module(gate)

VH_SPEC = importlib.util.spec_from_file_location("verify_handoff", VERIFY_HANDOFF)
assert VH_SPEC is not None and VH_SPEC.loader is not None
vh = importlib.util.module_from_spec(VH_SPEC)
VH_SPEC.loader.exec_module(vh)

COMMIT = "a" * 40
TAG = "v0.1.0-rc1"
PINNED = "example-region1-docker.example.invalid/example-bff-000000/tools/publish@sha256:" + ("b" * 64)
HANDOFF_URI = f"gs://example-bff-000000-handoffs/releases/{TAG}/handoff.tar.gz#1700000000000001"


def valid_fixtures():
    branch = {"commit": {"sha": COMMIT}}
    rules = [
        {
            "type": "required_status_checks",
            "parameters": {
                "strict_required_status_checks_policy": True,
                "required_status_checks": [
                    {"context": name, "integration_id": 15368} for name in ("build-test", "e2e", "policy", "gate")
                ],
            },
        },
        {
            "type": "pull_request",
            "parameters": {
                "required_approving_review_count": 1,
                "require_last_push_approval": True,
            },
        },
    ]
    runs = [
        {
            "id": 1001,
            "head_sha": COMMIT,
            "event": "push",
            "head_branch": "main",
            "path": ".github/workflows/ci.yml",
            "status": "completed",
            "conclusion": "success",
        }
    ]
    jobs = [
        {"name": name, "status": "completed", "conclusion": "success"}
        for name in ("build-test", "e2e", "policy", "gate")
    ]
    return branch, rules, runs, jobs


def _write_tar(dest: Path, members: list[tuple[str, bytes]], gz: bool = False) -> None:
    with tarfile.open(dest, "w:gz" if gz else "w:") as tf:
        for name, data in members:
            ti = tarfile.TarInfo(name)
            ti.size = len(data)
            ti.mode = 0o644
            tf.addfile(ti, io.BytesIO(data))


def _write_sums(handoff: Path, tag: str) -> None:
    pub = list(vh.public_asset_names(tag))

    def sha(name: str) -> str:
        return hashlib.sha256((handoff / name).read_bytes()).hexdigest()

    (handoff / "SHA256SUMS").write_text("".join(f"{sha(n)}  {n}\n" for n in pub), encoding="utf-8")
    all_assets = sorted(pub + ["SHA256SUMS", f"image-layout-{tag}.tar.gz"])
    (handoff / "HANDOFF.sha256").write_text("".join(f"{sha(n)}  {n}\n" for n in all_assets), encoding="utf-8")


def _default_playwright_report(ok: bool = True) -> dict:
    status = "passed" if ok else "failed"
    return {
        "stats": {"expected": 1 if ok else 0, "unexpected": 0 if ok else 1, "flaky": 0, "skipped": 0},
        "suites": [
            {
                "title": "runtime.spec.mjs",
                "file": "runtime.spec.mjs",
                "specs": [
                    {
                        "title": "the production UI shares the origin and preserves missing-bearer refusal",
                        "ok": ok,
                        "tests": [{"results": [{"status": status}]}],
                    }
                ],
            }
        ],
    }


def _default_verify_receipt(revision: str) -> dict:
    return {
        "schema_version": 1,
        "stage": "full",
        "requested_stage": "full",
        "revision": revision,
        "worktree_dirty": False,
        "offline": False,
        "result": "PASSED",
        "expected_checks": 2,
        "checks": [
            {"check": "go-test", "result": "PASSED", "blocking": True},
            {"check": "e2e", "result": "PASSED", "blocking": True},
        ],
    }


def build_qualified_handoff(
    handoff: Path,
    tag: str = TAG,
    revision: str = COMMIT,
    runtime_digest_override: str | None = None,
    runtime_revision_override: str | None = None,
    identity_only_runtime: bool = False,
    command_exit: int = 0,
    playwright_ok: bool = True,
    verify_receipt_override: dict | None = None,
) -> str:
    """Create a complete credentialless prepare.sh-format handoff directory and return its OCI digest."""
    handoff.mkdir(parents=True, exist_ok=True)
    layer_buf = io.BytesIO()
    with tarfile.open(fileobj=layer_buf, mode="w:") as tf:
        ti = tarfile.TarInfo("bff")
        payload = b"qualified-bff-binary\n"
        ti.size = len(payload)
        ti.mode = 0o555
        tf.addfile(ti, io.BytesIO(payload))
    layer_bytes = layer_buf.getvalue()
    layer_hex = hashlib.sha256(layer_bytes).hexdigest()
    layer_digest = f"sha256:{layer_hex}"

    cfg = {
        "architecture": "amd64",
        "os": "linux",
        "config": {"User": "65532:65532", "Entrypoint": ["/bff"]},
        "rootfs": {"type": "layers", "diff_ids": [layer_digest]},
    }
    cfg_bytes = json.dumps(cfg, separators=(",", ":"), sort_keys=True).encode("utf-8")
    cfg_hex = hashlib.sha256(cfg_bytes).hexdigest()
    cfg_digest = f"sha256:{cfg_hex}"

    manifest = {
        "schemaVersion": 2,
        "mediaType": "application/vnd.oci.image.manifest.v1+json",
        "config": {
            "mediaType": "application/vnd.oci.image.config.v1+json",
            "digest": cfg_digest,
            "size": len(cfg_bytes),
        },
        "layers": [
            {
                "mediaType": "application/vnd.oci.image.layer.v1.tar",
                "digest": layer_digest,
                "size": len(layer_bytes),
            }
        ],
    }
    man_bytes = json.dumps(manifest, separators=(",", ":"), sort_keys=True).encode("utf-8")
    man_hex = hashlib.sha256(man_bytes).hexdigest()
    man_digest = f"sha256:{man_hex}"

    index_bytes = json.dumps(
        {
            "schemaVersion": 2,
            "manifests": [
                {
                    "mediaType": "application/vnd.oci.image.manifest.v1+json",
                    "digest": man_digest,
                    "size": len(man_bytes),
                }
            ],
        }
    ).encode("utf-8")
    _write_tar(
        handoff / f"image-layout-{tag}.tar.gz",
        [
            ("oci-layout", b'{"imageLayoutVersion":"1.0.0"}\n'),
            ("index.json", index_bytes),
            (f"blobs/sha256/{layer_hex}", layer_bytes),
            (f"blobs/sha256/{cfg_hex}", cfg_bytes),
            (f"blobs/sha256/{man_hex}", man_bytes),
        ],
        gz=True,
    )

    docker_tar = handoff / f"bff-image-{tag}.tar"
    _write_tar(
        docker_tar,
        [
            (f"{cfg_hex}.json", cfg_bytes),
            (f"{layer_hex}.tar", layer_bytes),
            (
                "manifest.json",
                json.dumps([{"Config": f"{cfg_hex}.json", "Layers": [f"{layer_hex}.tar"]}]).encode("utf-8"),
            ),
        ],
    )
    archive_digest = "sha256:" + hashlib.sha256(docker_tar.read_bytes()).hexdigest()

    identity_json = json.dumps(
        {
            "revision": runtime_revision_override or revision,
            "manifestDigest": runtime_digest_override or man_digest,
            "configDigest": cfg_digest,
            "archiveDigest": archive_digest,
            "diffIDs": [layer_digest],
        }
    ).encode("utf-8")
    runtime_members: list[tuple[str, bytes]] = [("image-identity.json", identity_json)]
    if not identity_only_runtime:
        cmd_result = json.dumps(
            {
                "exit": command_exit,
                "witness": "actual-local-container",
                "source": runtime_revision_override or revision,
            }
        ).encode("utf-8")
        report_json = json.dumps(_default_playwright_report(ok=playwright_ok)).encode("utf-8")
        runtime_members.extend(
            [
                ("source-revision.txt", f"{runtime_revision_override or revision}\n".encode()),
                ("command-result.json", cmd_result),
                ("report.json", report_json),
            ]
        )
    _write_tar(handoff / f"runtime-acceptance-{tag}.tar.gz", runtime_members, gz=True)

    receipt = _default_verify_receipt(revision) if verify_receipt_override is None else verify_receipt_override
    (handoff / "LICENSE").write_text("license\n", encoding="utf-8")
    (handoff / "go-x509roots-BSD-3-Clause.txt").write_text("notice\n", encoding="utf-8")
    (handoff / f"provenance-{tag}.md").write_text("provenance\n", encoding="utf-8")
    (handoff / f"verify-receipt-{tag}.json").write_text(json.dumps(receipt) + "\n", encoding="utf-8")
    (handoff / f"source-revision-{tag}.txt").write_text(f"{revision}\n", encoding="utf-8")
    (handoff / f"image-digest-{tag}.txt").write_text(f"{man_digest}\n", encoding="utf-8")
    _write_sums(handoff, tag)
    return man_digest


def pack_handoff_archive(handoff_dir: Path, archive_path: Path) -> str:
    """Pack a handoff directory into a tar.gz transport archive and return its lowercase SHA-256."""
    members = [(entry.name, entry.read_bytes()) for entry in sorted(handoff_dir.iterdir())]
    _write_tar(archive_path, members, gz=True)
    return hashlib.sha256(archive_path.read_bytes()).hexdigest()


def _install_fake_tools(
    bindir: Path,
    root: Path,
    crane_digest: str,
    gcs_uri: str | None,
    gcs_archive: Path | None,
    with_helper: bool = True,
) -> None:
    if with_helper:
        helper = bindir / "docker-credential-gcr"
        helper.write_text("#!/usr/bin/env bash\nexit 0\n", encoding="utf-8")
        helper.chmod(0o755)

    gcloud = bindir / "gcloud"
    gcloud.write_text(
        "#!/usr/bin/env bash\n"
        "set -euo pipefail\n"
        f'printf "%s\\n" "$*" >> "{root}/gcloud.log"\n'
        'if [[ "${1:-}" == "storage" && "${2:-}" == "cp" ]]; then\n'
        "  shift 2\n"
        '  if [[ "${1:-}" == "--" ]]; then shift; fi\n'
        f'  if [[ -n "{gcs_uri or ""}" && "${{1:-}}" == "{gcs_uri or ""}" && -f "{gcs_archive or ""}" ]]; then\n'
        f'    cp -- "{gcs_archive or ""}" "$2"\n'
        "    exit 0\n"
        "  fi\n"
        '  echo "gcloud storage cp: object not found: ${1:-}" >&2\n'
        "  exit 1\n"
        "fi\n"
        'echo "unexpected gcloud call: $*" >&2\n'
        "exit 1\n",
        encoding="utf-8",
    )
    gcloud.chmod(0o755)

    crane = bindir / "crane"
    crane.write_text(
        "#!/usr/bin/env bash\n"
        "set -euo pipefail\n"
        f'printf "%s\\n" "$*" >> "{root}/crane.log"\n'
        'if [[ "$1" == "digest" ]]; then\n'
        f'  printf "%s\\n" "{crane_digest}"\n'
        "fi\n",
        encoding="utf-8",
    )
    crane.chmod(0o755)


class ReleaseGateTests(unittest.TestCase):
    def test_valid_exact_main_passes(self):
        branch, rules, runs, jobs = valid_fixtures()
        self.assertEqual(gate.check(branch, rules, runs, jobs, COMMIT), 1001)

    def test_rejects_non_main_commit(self):
        branch, rules, runs, jobs = valid_fixtures()
        branch["commit"]["sha"] = "f" * 40
        with self.assertRaisesRegex(ValueError, "current main"):
            gate.check(branch, rules, runs, jobs, COMMIT)

    def test_rejects_non_strict_or_unpinned_integration_rules(self):
        branch, rules, runs, jobs = valid_fixtures()
        bad = copy.deepcopy(rules)
        bad[0]["parameters"]["required_status_checks"][0]["integration_id"] = 999
        with self.assertRaisesRegex(ValueError, "GitHub Actions-bound"):
            gate.check(branch, bad, runs, jobs, COMMIT)

    def test_rejects_missing_last_push_approval_rule(self):
        branch, rules, runs, jobs = valid_fixtures()
        bad = copy.deepcopy(rules)
        bad[1]["parameters"]["require_last_push_approval"] = False
        with self.assertRaisesRegex(ValueError, "independent pull request approval"):
            gate.check(branch, bad, runs, jobs, COMMIT)

    def test_rejects_non_push_event_run(self):
        branch, rules, runs, jobs = valid_fixtures()
        bad_runs = copy.deepcopy(runs)
        bad_runs[0]["event"] = "pull_request"
        with self.assertRaisesRegex(ValueError, "no exact-main CI run"):
            gate.check(branch, rules, bad_runs, jobs, COMMIT)

    def test_rejects_failed_required_job(self):
        branch, rules, runs, jobs = valid_fixtures()
        bad_jobs = copy.deepcopy(jobs)
        bad_jobs[1]["conclusion"] = "failure"
        with self.assertRaisesRegex(ValueError, "not all successful"):
            gate.check(branch, rules, runs, bad_jobs, COMMIT)

    def test_connection_must_match_bff_repo(self):
        expected = "projects/example-bff-000000/locations/example-region1/connections/gh/repositories/bff"
        gate.check_connection(
            {"name": expected, "remoteUri": "https://github.com/selamy-labs/bazel-protobuf-bff-template.git"},
            expected,
        )
        with self.assertRaisesRegex(ValueError, "does not resolve"):
            gate.check_connection(
                {"name": expected, "remoteUri": "https://github.com/other/repo.git"},
                expected,
            )

    def test_build_command_validates_project_scoped_publisher_and_immutable_handoff_uri(self):
        proj = "example-bff-000000"
        region = "example-region1"
        digest = "sha256:" + ("d" * 64)
        handoff_sha = "e" * 64
        conn = f"projects/{proj}/locations/{region}/connections/gh/repositories/bff"
        sa = f"projects/{proj}/serviceAccounts/bff-release-publisher@{proj}{gate.SA_SUFFIX}"
        cmd = gate.build_command(
            conn, COMMIT, TAG, digest, HANDOFF_URI, handoff_sha, proj, region, "bff-images", sa, PINNED
        )
        self.assertIn(f"_PUBLISHER_SERVICE_ACCOUNT={sa}", cmd[-1])
        self.assertIn(f"_EXPECTED_DIGEST={digest}", cmd[-1])
        self.assertIn(f"_HANDOFF_URI={HANDOFF_URI}", cmd[-1])
        self.assertIn(f"_HANDOFF_SHA256={handoff_sha}", cmd[-1])
        self.assertNotIn("_HANDOFF_DIR=", cmd[-1])
        self.assertNotIn("_BUILD_IMAGE=", cmd[-1])
        with self.assertRaisesRegex(ValueError, "publisher service account"):
            bad_sa = f"projects/other-proj-000000/serviceAccounts/bff-release-publisher@{proj}{gate.SA_SUFFIX}"
            gate.build_command(
                conn, COMMIT, TAG, digest, HANDOFF_URI, handoff_sha, proj, region, "bff-images", bad_sa, PINNED
            )
        with self.assertRaisesRegex(ValueError, "generation-pinned"):
            gate.build_command(
                conn,
                COMMIT,
                TAG,
                digest,
                "gs://example-bff-000000-handoffs/releases/v0.1.0-rc1/handoff.tar.gz",
                handoff_sha,
                proj,
                region,
                "bff-images",
                sa,
                PINNED,
            )
        with self.assertRaisesRegex(ValueError, "SHA-256"):
            gate.build_command(
                conn, COMMIT, TAG, digest, HANDOFF_URI, "not-a-sha", proj, region, "bff-images", sa, PINNED
            )


class PublishImageScriptTests(unittest.TestCase):
    def _setup_sandbox(
        self,
        root: Path,
        crane_digest: str | None = None,
        with_helper: bool = True,
        with_adc: bool = False,
        stage_gcs: bool = True,
        tamper_inner_file: bool = False,
        runtime_digest_override: str | None = None,
        runtime_revision_override: str | None = None,
        identity_only_runtime: bool = False,
        command_exit: int = 0,
        playwright_ok: bool = True,
        verify_receipt_override: dict | None = None,
    ):
        bindir = root / "bin"
        producer_handoff = root / "producer-only-handoff"
        gcs_store = root / "remote-gcs"
        home = root / "home"
        for d in (bindir, gcs_store, home):
            d.mkdir()
        digest = build_qualified_handoff(
            producer_handoff,
            tag=TAG,
            revision=COMMIT,
            runtime_digest_override=runtime_digest_override,
            runtime_revision_override=runtime_revision_override,
            identity_only_runtime=identity_only_runtime,
            command_exit=command_exit,
            playwright_ok=playwright_ok,
            verify_receipt_override=verify_receipt_override,
        )
        if tamper_inner_file:
            (producer_handoff / f"source-revision-{TAG}.txt").write_text(("f" * 40) + "\n", encoding="utf-8")

        archive_path = gcs_store / "handoff.tar.gz"
        archive_sha = pack_handoff_archive(producer_handoff, archive_path)
        shutil.rmtree(producer_handoff)

        if with_adc:
            adc_dir = home / ".config" / "gcloud"
            adc_dir.mkdir(parents=True)
            (adc_dir / "application_default_credentials.json").write_text("{}", encoding="utf-8")

        _install_fake_tools(
            bindir,
            root,
            crane_digest or digest,
            HANDOFF_URI if stage_gcs else None,
            archive_path if stage_gcs else None,
            with_helper=with_helper,
        )

        env = {
            "PATH": f"{bindir}:/usr/bin:/bin",
            "HOME": str(home),
            "RELEASE_COMMIT": COMMIT,
            "RELEASE_TAG": TAG,
            "RELEASE_EXPECTED_DIGEST": digest,
            "RELEASE_HANDOFF_URI": HANDOFF_URI,
            "RELEASE_HANDOFF_SHA256": archive_sha,
            "RELEASE_VERIFY_HANDOFF_SCRIPT": str(VERIFY_HANDOFF),
            "RELEASE_BUILD_ID": "build-123",
            "RELEASE_PROJECT": "example-bff-000000",
            "RELEASE_REGION": "example-region1",
            "RELEASE_REPOSITORY": "bff-images",
            "RELEASE_RECEIPT_PATH": str(root / "receipt.json"),
        }
        return env, archive_path, digest, archive_sha

    def test_gate_submission_joins_cloudbuild_template_and_publishes_in_clean_git_workspace(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            env, _, digest, archive_sha = self._setup_sandbox(root)
            proj, region, repo = "example-bff-000000", "example-region1", "bff-images"
            conn = f"projects/{proj}/locations/{region}/connections/gh/repositories/bff"
            sa = f"projects/{proj}/serviceAccounts/bff-release-publisher@{proj}{gate.SA_SUFFIX}"
            cmd = gate.build_command(
                conn, COMMIT, TAG, digest, HANDOFF_URI, archive_sha, proj, region, repo, sa, PINNED
            )
            sub_arg = next(arg for arg in cmd if arg.startswith("--substitutions="))
            subs = dict(item.split("=", 1) for item in sub_arg.removeprefix("--substitutions=").split(","))

            cb_text = (RELEASE_DIR / "cloudbuild.yaml").read_text(encoding="utf-8")
            cb_vars = set(re.findall(r"\$\{(_[A-Z0-9_]+)\}", cb_text))
            self.assertEqual(cb_vars, set(subs))

            # Construct a clean Git-only workspace with NO local handoff directory present.
            git_ws = root / "clean-git-workspace"
            (git_ws / "deploy" / "gcp" / "release").mkdir(parents=True)
            (git_ws / "tools" / "release").mkdir(parents=True)
            for name in ("publish-image.sh", "verify_runtime_evidence.py"):
                shutil.copyfile(RELEASE_DIR / name, git_ws / "deploy" / "gcp" / "release" / name)
            shutil.copyfile(VERIFY_HANDOFF, git_ws / "tools" / "release" / "verify_handoff.py")

            step_env = {"PATH": env["PATH"], "HOME": env["HOME"], "RELEASE_RECEIPT_PATH": env["RELEASE_RECEIPT_PATH"]}
            built_in = {"BUILD_ID": "build-789", "PROJECT_ID": proj}
            for match in re.finditer(r"^\s*-\s+([A-Z0-9_]+)=(.+)$", cb_text, re.M):
                key, raw_val = match.group(1), match.group(2).strip()
                val = re.sub(r"\$\{(_[A-Z0-9_]+)\}", lambda m: subs[m.group(1)], raw_val)
                val = re.sub(r"\$([A-Z0-9_]+)", lambda m: built_in[m.group(1)], val)
                step_env[key] = val

            proc = subprocess.run(
                ["bash", "deploy/gcp/release/publish-image.sh"],
                cwd=git_ws,
                env=step_env,
                capture_output=True,
                text=True,
                check=False,
            )
            self.assertEqual(proc.returncode, 0, proc.stderr)
            receipt = json.loads((root / "receipt.json").read_text(encoding="utf-8"))
            self.assertEqual(receipt["digest"], digest)
            self.assertEqual(receipt["handoff_uri"], HANDOFF_URI)
            self.assertEqual(receipt["handoff_sha256"], archive_sha)
            self.assertEqual(receipt["cloud_build_id"], "build-789")
            self.assertTrue(receipt["runtime_witness_verified"])
            self.assertIn(HANDOFF_URI, (root / "gcloud.log").read_text(encoding="utf-8"))
            self.assertTrue((root / "crane.log").exists())

    def test_publish_image_refuses_missing_gcs_handoff_before_crane(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            env, _, _, _ = self._setup_sandbox(root, stage_gcs=False)
            proc = subprocess.run(
                ["bash", str(RELEASE_DIR / "publish-image.sh")], env=env, capture_output=True, text=True, check=False
            )
            self.assertNotEqual(proc.returncode, 0)
            self.assertIn("object not found", proc.stderr)
            self.assertFalse((root / "crane.log").exists())

    def test_publish_image_refuses_unpinned_handoff_uri_before_download_or_crane(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            env, _, _, _ = self._setup_sandbox(root)
            env["RELEASE_HANDOFF_URI"] = "gs://example-bff-000000-handoffs/releases/v0.1.0-rc1/handoff.tar.gz"
            proc = subprocess.run(
                ["bash", str(RELEASE_DIR / "publish-image.sh")], env=env, capture_output=True, text=True, check=False
            )
            self.assertNotEqual(proc.returncode, 0)
            self.assertIn("generation-pinned", proc.stderr)
            self.assertFalse((root / "gcloud.log").exists())
            self.assertFalse((root / "crane.log").exists())

    def test_publish_image_refuses_tampered_transport_archive_before_crane(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            env, archive_path, _, _ = self._setup_sandbox(root)
            archive_path.write_bytes(archive_path.read_bytes() + b"tampered")
            proc = subprocess.run(
                ["bash", str(RELEASE_DIR / "publish-image.sh")], env=env, capture_output=True, text=True, check=False
            )
            self.assertNotEqual(proc.returncode, 0)
            self.assertIn("sha256 mismatch", proc.stderr)
            self.assertFalse((root / "crane.log").exists())

    def test_publish_image_refuses_tampered_inner_handoff_before_crane(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            env, _, _, _ = self._setup_sandbox(root, tamper_inner_file=True)
            proc = subprocess.run(
                ["bash", str(RELEASE_DIR / "publish-image.sh")], env=env, capture_output=True, text=True, check=False
            )
            self.assertNotEqual(proc.returncode, 0)
            self.assertIn("verify-handoff:", proc.stderr)
            self.assertFalse((root / "crane.log").exists())

    def test_publish_image_refuses_identity_only_runtime_archive_with_valid_sums(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            env, _, _, _ = self._setup_sandbox(root, identity_only_runtime=True)
            proc = subprocess.run(
                ["bash", str(RELEASE_DIR / "publish-image.sh")], env=env, capture_output=True, text=True, check=False
            )
            self.assertNotEqual(proc.returncode, 0)
            self.assertIn("source-revision.txt", proc.stderr)
            self.assertFalse((root / "crane.log").exists())

    def test_publish_image_refuses_nonzero_runtime_command_exit_with_valid_sums(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            env, _, _, _ = self._setup_sandbox(root, command_exit=1)
            proc = subprocess.run(
                ["bash", str(RELEASE_DIR / "publish-image.sh")], env=env, capture_output=True, text=True, check=False
            )
            self.assertNotEqual(proc.returncode, 0)
            self.assertIn("did not exit 0", proc.stderr)
            self.assertFalse((root / "crane.log").exists())

    def test_publish_image_refuses_failed_playwright_report_with_valid_sums(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            env, _, _, _ = self._setup_sandbox(root, playwright_ok=False)
            proc = subprocess.run(
                ["bash", str(RELEASE_DIR / "publish-image.sh")], env=env, capture_output=True, text=True, check=False
            )
            self.assertNotEqual(proc.returncode, 0)
            self.assertIn("Playwright report.json", proc.stderr)
            self.assertFalse((root / "crane.log").exists())

    def test_publish_image_refuses_empty_verify_receipt_with_valid_sums(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            env, _, _, _ = self._setup_sandbox(root, verify_receipt_override={})
            proc = subprocess.run(
                ["bash", str(RELEASE_DIR / "publish-image.sh")], env=env, capture_output=True, text=True, check=False
            )
            self.assertNotEqual(proc.returncode, 0)
            self.assertIn("verify receipt", proc.stderr)
            self.assertFalse((root / "crane.log").exists())

    def test_publish_image_refuses_sharded_passed_verify_receipt_with_valid_sums(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            sharded = _default_verify_receipt(COMMIT) | {"shard": "policy"}
            env, _, _, _ = self._setup_sandbox(root, verify_receipt_override=sharded)
            proc = subprocess.run(
                ["bash", str(RELEASE_DIR / "publish-image.sh")], env=env, capture_output=True, text=True, check=False
            )
            self.assertNotEqual(proc.returncode, 0)
            self.assertIn("unsharded full run", proc.stderr)
            self.assertFalse((root / "crane.log").exists())

    def test_publish_image_refuses_runtime_digest_mismatch_even_with_valid_sums(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            env, _, _, _ = self._setup_sandbox(root, runtime_digest_override="sha256:" + ("9" * 64))
            proc = subprocess.run(
                ["bash", str(RELEASE_DIR / "publish-image.sh")], env=env, capture_output=True, text=True, check=False
            )
            self.assertNotEqual(proc.returncode, 0)
            self.assertIn("manifestDigest mismatch", proc.stderr)
            self.assertFalse((root / "crane.log").exists())

    def test_publish_image_refuses_expected_digest_mismatch_before_crane(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            env, _, _, _ = self._setup_sandbox(root)
            env["RELEASE_EXPECTED_DIGEST"] = "sha256:" + ("0" * 64)
            proc = subprocess.run(
                ["bash", str(RELEASE_DIR / "publish-image.sh")], env=env, capture_output=True, text=True, check=False
            )
            self.assertNotEqual(proc.returncode, 0)
            self.assertIn("handoff digest mismatch", proc.stderr)
            self.assertFalse((root / "crane.log").exists())

    def test_publish_image_refuses_well_known_adc_file(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            env, _, _, _ = self._setup_sandbox(root, with_adc=True)
            proc = subprocess.run(
                ["bash", str(RELEASE_DIR / "publish-image.sh")], env=env, capture_output=True, text=True, check=False
            )
            self.assertNotEqual(proc.returncode, 0)
            self.assertIn("well-known ADC file", proc.stderr)
            self.assertFalse((root / "gcloud.log").exists())
            self.assertFalse((root / "crane.log").exists())

    def test_publish_image_refuses_post_push_digest_mismatch(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            env, _, _, _ = self._setup_sandbox(root, crane_digest="sha256:" + ("c" * 64))
            proc = subprocess.run(
                ["bash", str(RELEASE_DIR / "publish-image.sh")], env=env, capture_output=True, text=True, check=False
            )
            self.assertNotEqual(proc.returncode, 0)
            self.assertIn("published digest mismatch", proc.stderr)

    def test_publish_image_refuses_missing_credential_helper(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            env, _, _, _ = self._setup_sandbox(root, with_helper=False)
            isolated_bin = root / "isolated_bin"
            isolated_bin.mkdir()
            (isolated_bin / "crane").symlink_to(root / "bin" / "crane")
            (isolated_bin / "gcloud").symlink_to(root / "bin" / "gcloud")
            (isolated_bin / "python3").symlink_to(Path(sys.executable))
            for tool in ("bash", "cp", "dirname", "env", "mkdir", "mktemp", "rm", "tr"):
                resolved = shutil.which(tool)
                if resolved:
                    os.symlink(resolved, isolated_bin / tool)
            env["PATH"] = str(isolated_bin)
            proc = subprocess.run(
                ["/bin/bash", str(RELEASE_DIR / "publish-image.sh")],
                env=env,
                capture_output=True,
                text=True,
                check=False,
            )
            self.assertNotEqual(proc.returncode, 0)
            self.assertIn("credential helper", proc.stderr)
            self.assertFalse((root / "crane.log").exists())
