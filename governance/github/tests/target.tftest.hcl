# Offline, with mocked providers: the one target, its name and owner, and the ruleset disabled by default.
mock_provider "github" {}

run "one_public_template_repository" {
  command = plan

  assert {
    condition     = github_repository.template.name == "bazel-protobuf-bff-template" && var.owner == "selamy-labs"
    error_message = "the repository name and owner are fixed"
  }
  assert {
    condition     = github_repository.template.visibility == "public" && github_repository.template.is_template
    error_message = "a public template repository"
  }
  assert {
    condition     = github_repository_ruleset.main.enforcement == "disabled"
    error_message = "the ruleset is disabled until the operator activates it"
  }
  assert {
    condition     = github_repository_ruleset.main.repository == github_repository.template.name
    error_message = "the ruleset binds to the one declared repository"
  }
  assert {
    condition     = length(output.resources) == 2
    error_message = "the graph holds exactly the repository and its ruleset"
  }
  assert {
    condition     = github_repository.template.allow_rebase_merge && !github_repository.template.allow_squash_merge && !github_repository.template.allow_merge_commit
    error_message = "rebase is the only allowed merge method so neutral commit author identity is preserved"
  }
  assert {
    condition     = contains(github_repository_ruleset.main.rules[0].pull_request[0].allowed_merge_methods, "rebase") && length(github_repository_ruleset.main.rules[0].pull_request[0].allowed_merge_methods) == 1
    error_message = "ruleset pull_request allows rebase merge only"
  }
  assert {
    condition     = github_repository_ruleset.main.rules[0].required_linear_history == true
    error_message = "ruleset requires linear history"
  }
  assert {
    condition     = github_repository_ruleset.main.rules[0].committer_email_pattern[0].operator == "regex" && github_repository_ruleset.main.rules[0].committer_email_pattern[0].negate == false && github_repository_ruleset.main.rules[0].committer_email_pattern[0].pattern == "^(noreply@github\\.com|346154065\\+selamy-labs-bot@users\\.noreply\\.github\\.com|claude-cir@selamy\\.dev)$"
    error_message = "ruleset committer_email_pattern must match exact approved bot committer identities"
  }
  assert {
    condition     = github_repository_ruleset.main.rules[0].commit_author_email_pattern[0].operator == "regex" && github_repository_ruleset.main.rules[0].commit_author_email_pattern[0].negate == false && github_repository_ruleset.main.rules[0].commit_author_email_pattern[0].pattern == "^claude-cir@selamy\\.dev$"
    error_message = "ruleset commit_author_email_pattern must match exact approved bot author identities"
  }
}

run "activation_is_the_only_change" {
  command = plan

  variables {
    ruleset_active = true
  }

  assert {
    condition     = github_repository_ruleset.main.enforcement == "active"
    error_message = "the activation input enables the ruleset"
  }
}

run "another_owner_is_refused" {
  command = plan

  variables {
    owner = "someone-else"
  }

  expect_failures = [var.owner]
}

run "another_name_is_refused" {
  command = plan

  variables {
    name = "other-name"
  }

  expect_failures = [var.name]
}
