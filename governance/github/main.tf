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
  allow_squash_merge     = false
  allow_rebase_merge     = true
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
    deletion                = true
    non_fast_forward        = true
    required_linear_history = true

    pull_request {
      required_approving_review_count = 1
      dismiss_stale_reviews_on_push   = true
      allowed_merge_methods           = ["rebase"]
    }

    required_status_checks {
      strict_required_status_checks_policy = true

      # The aggregate job of .github/workflows/ci.yml, as tools/ci/policy.json names it. governance.sh refuses a
      # context that is not that job, so the protection can never require a check that no run reports.
      required_check {
        context = "gate"
      }
    }

    committer_email_pattern {
      operator = "regex"
      negate   = false
      pattern  = "^(noreply@github\\.com|346154065\\+selamy-labs-bot@users\\.noreply\\.github\\.com|claude-cir@selamy\\.dev)$"
    }

    commit_author_email_pattern {
      operator = "regex"
      negate   = false
      pattern  = "^claude-cir@selamy\\.dev$"
    }
  }
}

output "resources" {
  description = "The resource graph the tests pin."
  value       = ["github_repository.template", "github_repository_ruleset.main"]
}
