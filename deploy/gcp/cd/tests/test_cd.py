"""Unit tests for BFF saved-plan guard and native Cloud Build approval execution."""

import copy
import importlib.util
import tempfile
import unittest
import zipfile
from pathlib import Path

CD_DIR = Path(__file__).resolve().parents[1]


def load_module(name: str, path: Path):
    spec = importlib.util.spec_from_file_location(name, path)
    assert spec is not None and spec.loader is not None
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


plan_guard = load_module("plan_guard", CD_DIR / "plan_guard.py")
execute = load_module("cd_execute", CD_DIR / "execute.py")

PROJECT = "example-bff-hosted-000000"
REGION = "example-region1"
SERVICE = "bff"
DIGEST = "sha256:" + ("a" * 64)
PREV_DIGEST = "sha256:" + ("b" * 64)
IMAGE = f"{REGION}-docker.example.invalid/{PROJECT}/bff-images/bff@{DIGEST}"
PREV_IMAGE = f"{REGION}-docker.example.invalid/{PROJECT}/bff-images/bff@{PREV_DIGEST}"
RUNTIME_SA = "bff-runtime@example.invalid"
NOTES_BUCKET = "example-bff-hosted-000000-notes"


def valid_plan(image: str = IMAGE, action: str = "update", bucket: str = NOTES_BUCKET, runtime_sa: str = RUNTIME_SA):
    return {
        "resource_changes": [
            {
                "mode": "managed",
                "type": "google_cloud_run_v2_service",
                "address": "google_cloud_run_v2_service.bff",
                "change": {
                    "actions": [action],
                    "after": {
                        "project": PROJECT,
                        "location": REGION,
                        "name": SERVICE,
                        "template": [
                            {
                                "service_account": runtime_sa,
                                "containers": [
                                    {
                                        "image": image,
                                        "env": [
                                            {"name": "BFF_STORE", "value": "object"},
                                            {"name": "BFF_OBJECT_BUCKET", "value": bucket},
                                        ],
                                    }
                                ],
                            }
                        ],
                    },
                    "after_unknown": {},
                },
            }
        ]
    }


class PlanGuardTests(unittest.TestCase):
    def test_valid_forward_and_rollback_plans_pass(self):
        plan_guard.check(
            valid_plan(IMAGE, "create"),
            IMAGE,
            PROJECT,
            REGION,
            SERVICE,
            expected_changes={"google_cloud_run_v2_service.bff": ["create"]},
            expected_runtime_sa=RUNTIME_SA,
            expected_notes_bucket=NOTES_BUCKET,
        )
        plan_guard.check(
            valid_plan(PREV_IMAGE, "update"),
            PREV_IMAGE,
            PROJECT,
            REGION,
            SERVICE,
            expected_changes={"google_cloud_run_v2_service.bff": ["update"]},
            expected_runtime_sa=RUNTIME_SA,
            expected_notes_bucket=NOTES_BUCKET,
        )

    def test_rejects_delete_or_replace(self):
        plan = valid_plan()
        plan["resource_changes"][0]["change"]["actions"] = ["delete", "create"]
        with self.assertRaisesRegex(ValueError, "unsupported mutation|delete"):
            plan_guard.check(plan, IMAGE, PROJECT, REGION, SERVICE)

    def test_rejects_scope_drift(self):
        with self.assertRaisesRegex(ValueError, "mutation scope differs"):
            plan_guard.check(
                valid_plan(),
                IMAGE,
                PROJECT,
                REGION,
                SERVICE,
                expected_changes={"google_cloud_run_v2_service.bff": ["create"]},
            )

    def test_rejects_unknown_image(self):
        plan = valid_plan()
        plan["resource_changes"][0]["change"]["after_unknown"] = {"template": [{"containers": [{"image": True}]}]}
        with self.assertRaisesRegex(ValueError, "unknown"):
            plan_guard.check(plan, IMAGE, PROJECT, REGION, SERVICE)

    def test_rejects_wrong_runtime_sa_and_notes_bucket(self):
        with self.assertRaisesRegex(ValueError, "runtime service account"):
            plan_guard.check(
                valid_plan(runtime_sa="other@example.invalid"),
                IMAGE,
                PROJECT,
                REGION,
                SERVICE,
                expected_runtime_sa=RUNTIME_SA,
            )
        with self.assertRaisesRegex(ValueError, "notes bucket"):
            plan_guard.check(
                valid_plan(bucket="wrong-bucket"), IMAGE, PROJECT, REGION, SERVICE, expected_notes_bucket=NOTES_BUCKET
            )


class ExecuteApprovalTests(unittest.TestCase):
    def _valid_build(self):
        return {
            "id": "build-1",
            "projectId": PROJECT,
            "buildTriggerId": "11111111-1111-1111-1111-111111111111",
            "serviceAccount": f"projects/{PROJECT}/serviceAccounts/bff-prod-deployer@example.invalid",
            "approval": {
                "state": "APPROVED",
                "config": {"approvalRequired": True},
                "result": {
                    "decision": "APPROVED",
                    "approverAccount": "approver@example.invalid",
                    "approvalTime": "2026-11-21T00:00:00Z",
                },
            },
        }

    def test_verify_approval_accepts_exact_native_approval(self):
        b = self._valid_build()
        execute.verify_approval(
            b,
            PROJECT,
            "build-1",
            b["buildTriggerId"],
            b["serviceAccount"],
            ["approver@example.invalid"],
        )

    def test_verify_approval_rejects_tampered_fields(self):
        base = self._valid_build()
        for mutator, pattern in (
            (lambda d: d["approval"].__setitem__("state", "PENDING"), "native approval"),
            (
                lambda d: d["approval"]["result"].__setitem__("approverAccount", "other@example.invalid"),
                "human evidence",
            ),
            (lambda d: d.__setitem__("buildTriggerId", "22222222-2222-2222-2222-222222222222"), "unexpected trigger"),
            (lambda d: d.__setitem__("serviceAccount", "other"), "unexpected deployer"),
        ):
            mutated = copy.deepcopy(base)
            mutator(mutated)
            with self.assertRaisesRegex(ValueError, pattern):
                execute.verify_approval(
                    mutated,
                    PROJECT,
                    "build-1",
                    base["buildTriggerId"],
                    base["serviceAccount"],
                    ["approver@example.invalid"],
                )

    def test_unpack_rejects_traversal_and_dot_terraform(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            for bad_name in ("../escape.txt", "deploy/gcp/.terraform/plugin"):
                archive = root / "bad.zip"
                with zipfile.ZipFile(archive, "w") as zf:
                    zf.writestr(bad_name, "x")
                target = root / "out"
                target.mkdir(exist_ok=True)
                with self.assertRaisesRegex(ValueError, "unsafe bundle member|provider binaries"):
                    execute.unpack(archive, target)
