# One new repository and its ruleset, declared and never applied by anything in this repository
# (spec 010 FR-009). No import block, no data source, no second target.

provider "github" {
  owner = var.owner
}

resource "github_repository" "template" {
  name        = var.name
  description = "A generic Bazel/Protobuf BFF template"
  visibility  = "public"
  is_template = true

  has_issues      = true
  has_projects    = false
  has_wiki        = false
  has_discussions = false

  allow_merge_commit     = false
  allow_squash_merge     = true
  allow_rebase_merge     = false
  delete_branch_on_merge = true
}

resource "github_repository_ruleset" "main" {
  name        = "main"
  repository  = github_repository.template.name
  target      = "branch"
  enforcement = var.ruleset_active ? "active" : "disabled"

  conditions {
    ref_name {
      include = ["~DEFAULT_BRANCH"]
      exclude = []
    }
  }

  rules {
    deletion         = true
    non_fast_forward = true

    pull_request {
      required_approving_review_count = 1
      dismiss_stale_reviews_on_push   = true
    }

    required_status_checks {
      strict_required_status_checks_policy = true

      # The aggregate job of .github/workflows/ci.yml, as tools/ci/policy.json names it. governance.sh refuses a
      # context that is not that job, so the protection can never require a check that no run reports.
      required_check {
        context = "gate"
      }
    }
  }
}

output "resources" {
  description = "The resource graph the tests pin."
  value       = ["github_repository.template", "github_repository_ruleset.main"]
}
