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
