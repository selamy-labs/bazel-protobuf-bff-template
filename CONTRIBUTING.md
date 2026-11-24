# Contributing

Thank you for contributing to the Generic Bazel/Protobuf BFF Template. Every change in this
repository follows the [Constitution](.specify/memory/constitution.md) and runs through one
canonical, hermetic verification path.

## Prerequisites and Hermetic Toolchain

- Linux `x86_64` host with glibc 2.36+, Git, `bash`, `curl`, coreutils (`timeout`, `sha256sum`,
  `tar`), and `flock`.
- Always invoke Bazel through `tools/bootstrap/bazel.sh` rather than a host `bazel` binary so the
  pinned Bazel version and rc isolation match CI.
- Install the pinned local Git hooks once per clone:

```sh
tools/bootstrap/bazel.sh run //tools/hooks:install
```

See [docs/quality/hooks.md](docs/quality/hooks.md) for hook stages and policy-base configuration.

## Verification Before Opening a Pull Request

Run the canonical verifier against a resolved trusted `main` commit SHA before requesting review:

```sh
base="$(git rev-parse main)"
tools/verify.sh --base="$base"
```

On hosts without unprivileged user namespaces, add `--weak-isolation`; receipts explicitly label
that mode `isolation: weakened`, while hosted GitHub Actions CI runs under full isolation. See
[docs/quality/evidence.md](docs/quality/evidence.md) for what each receipt proves and does not
claim.

## Pull Request Hygiene

- Keep pull requests small, single-concern, and independently revertible.
- Pull request branches MUST contain a single commit atop `main` with neutral author/committer
  identity; protected `main` uses rebase-only integration (never squash or merge commits).
- Never bypass hooks (`--no-verify`), never weaken policy floors or complexity ceilings, and never
  introduce linter/formatter suppressions (`suppression-ban`).
- Keep all commits, code, docs, and public comments free of private paths, credentials, internal
  identifiers, or unverified live claims.

## Testing Standard

All new tests and test retrofits follow the canonical standard in
[docs/quality/test-standard.md](docs/quality/test-standard.md) and
[Constitution § Testing Standard](.specify/memory/constitution.md#testing-standard):

1. **AAA and one assertion per test case**: visibly separate Arrange, Act, and Assert, with one
   assertion per named case (or per table-driven subtest).
2. **Property-based testing by default**: use the language's pinned property library (`testing/quick`
   in Go, `fast-check` in TypeScript, `Hypothesis` in Python once pinned) with reproducible seeds
   wherever behavior defines an invariant.
3. **All mutants killed with a ratcheting floor**: target 100% mutation kill score with a floor
   that only ratchets upward; only proven equivalent mutants may survive, listed in a reviewed
   equivalents file with ID, location, and one-line proof.
4. **Retrofit slices**: retrofit one module at a time in small, behavior-preserving PRs never mixed
   with feature work, reporting before/after mutation scores and floor changes once mutation
   tooling is configured for that module.

## Key References

- [Constitution](.specify/memory/constitution.md)
- [Specification index](specs/README.md)
- [Quality baseline](docs/quality/baseline.md)
- [Canonical testing standard](docs/quality/test-standard.md)
- [Hooks](docs/quality/hooks.md) and [Evidence](docs/quality/evidence.md)
