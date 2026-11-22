# BFF GCP Foundation

Dedicated, non-shared Google Cloud foundation for the BFF hosted rollout:

- **Isolated project**: creates a dedicated project with `auto_create_network = false` and `prevent_destroy = true`.
- **Private OCI registry**: regional Artifact Registry Docker repository for immutable digest releases.
- **Private notes bucket**: uniform bucket-level access, enforced public access prevention, object versioning enabled.
- **Private release handoffs bucket**: separate versioned bucket (uniform bucket-level access, enforced public access prevention, disjoint from notes and state buckets) holding immutable `prepare.sh` release handoff archives consumed by Cloud Build.
- **Separated service accounts**:
  - `bff-release-publisher`: `roles/artifactregistry.writer` on the image repository, read-only `roles/storage.objectViewer` on the `release_handoffs` bucket, and `roles/logging.logWriter` only (no handoff write/delete, no notes access, no deploy rights).
  - `bff-runtime`: `roles/storage.objectCreator` + `roles/storage.objectViewer` on the notes bucket only.
  - `bff-dev-deployer`: `roles/artifactregistry.reader`, `roles/logging.logWriter`, `roles/run.developer` CEL-scoped to the single service (`dev_service_name`, defaulting to `bff` to compose directly with `deploy/gcp/service/main.tf` inside the dedicated isolated dev project), and `google_service_account_iam_member` granting `roles/iam.serviceAccountUser` directly on the `bff-runtime` service account policy only. Production remains isolated in its own dedicated project and `bff-prod-deployer` identity.

## Usage

Backend configuration and concrete variables stay outside the public repository:

```bash
tofu -chdir=deploy/gcp/foundation init -backend-config=/path/to/backend.hcl -lockfile=readonly
tofu -chdir=deploy/gcp/foundation plan -var-file=/path/to/foundation.tfvars -out=/path/to/foundation.tfplan
```

Export `contract_v1` to JSON after operator-approved apply for downstream `deploy/gcp/inputs` validation.
