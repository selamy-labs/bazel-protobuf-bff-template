# Generic Bazel/Protobuf BFF Template

## Purpose

This is a neutral starter for contract-first web applications. A browser client and a
backend-for-frontend talk only through generated Protocol Buffer/Connect clients, and Bazel owns the
toolchains, code generation and tests. The aim is shared, versioned quality checks that run the same
way in Git hooks, in the local verifier and in CI. The local hooks and the one check registry exist, and a
separate repository consumes the checks at `quality-v1.0.0` by one pin; hosted GitHub Actions CI runs on pull requests and recorded green gates on PR #1 (runs 41536411991 at f510f8e and 41542211806 at e82bc8f), while ruleset `main` (`id: 26410174`) is active on the default branch (requiring pull requests and `build-test`, `e2e`, `policy`, and `gate`), and post-merge `main` neutrality was restored on 0254b7a5 by PR #5 (repairing the historical `d9f97dd9` commit metadata regression), while any tagged release or package publication remains pending.

**Status: M1 to M8 are implemented and independently accepted at their revisions, locally; M8a's image is built,
tested and executed independently; the repository is public at `2d0ecf9`, PR #1 (`feat/012-hosted-telemetry-wiring`) passed all 4 hosted GitHub Actions CI jobs (`build-test`, `e2e`, `policy`, `gate`) at `f510f8e` (run `41536411991`) and `e82bc8f` (run `41542211806`) and merged to `main` at `d9f97dd`, while post-merge `main` run `41544265549` passed `build-test` and `e2e` but failed `policy` (`neutrality`) because the GitHub squash-merge commit `d9f97dd` recorded commit metadata outside `tools/release/neutrality.allow` (subsequently repaired and restored green on main at `0254b7a5` via PR #5), and ruleset `main` (`id: 26410174`) is active since `2026-11-03`, requiring pull requests and `build-test`, `e2e`, `policy`, and `gate` (strict). Linux local-demo acceptance (M10 at `30498f4`/`2d0ecf9`) and Linux `file://` second-consumer adoption (M7 at `e982dd7`) are verified locally, while current-revision native macOS local-demo acceptance and native macOS template-adoption evidence remain unverified. The quality baseline (M9) is
open: the Go complexity, Go and TypeScript duplication, Go dead-code and Python lint gates are enforced from this
tree's measurements, property tests run for Go and TypeScript, mutation is probed but not bound, Go branch outcomes
are an open blocking gap, and every control is at its measured state in the [baseline](docs/quality/baseline.md).
The local whole-stack demo (M10) works on Linux, and the production server's telemetry is witnessed locally; the
hosted example exists only as offline tests and a reviewable plan, nothing is deployed, and no hosted claim is
made.** The app works:
`CreateNote` and `GetNote` run from a React page through a Go server, tested in a pinned browser. The
[specification index](specs/README.md) is the one authority on each milestone's status, the evidence
pages under [docs/evidence](docs/evidence/) list what has and has not been observed at which revision,
and [capability states](docs/quality/capability-states.md) has the summary. Current-revision native macOS remains unverified. The production server and its container image refuse to start without an issuer and its telemetry inputs ([bff/README.md](bff/README.md), [spec 009](specs/009-bff-image/contracts/image.md)), and a healthy production deployment remains unsupported until a real issuer exists. The hosted deployment input funnel ([deploy/gcp/inputs/telemetry.go](deploy/gcp/inputs/telemetry.go) at `e82bc8f`) additionally rejects browser origins with IPv6 zone indices (%), private/loopback/link-local IP literals, and non-ASCII hosts before provisioning.

## How to use and test

Prerequisites: Git, bash, curl, coreutils (GNU `timeout`, `sha256sum`, `tar`) and util-linux (`flock`), on a Linux x86_64
host with glibc 2.36 or newer, which the verifier's preflight checks first. No C compiler is needed. Bazel itself, the Go SDK, Node, Python, a browser
and every check tool are fetched at pinned, checksummed versions; `tools/bootstrap/bazel.sh` verifies
the Bazel binary on every call. The verifier discards the caller's environment, so rc files, tool options and PATH helpers cannot
change a verdict. It does trust its own caches under `$HOME/.cache`: Bazel does not re-hash an
extracted install base.

```sh
tools/verify.sh --base=e3b3b348587483ed1da10957766e32615aaabb01
```

`--base` names the policy base: the commit whose evaluator judges this tree's coverage floors and
suppressions, so a change cannot approve its own weakening. Here it is the first independently reviewed
checker commit. Naming a commit proves nothing about its review, so name one you trust. Without `--base` the
verifier still runs every other check, then exits 11 and says so. A commit cannot be its own base. See
[hooks](docs/quality/hooks.md) for the setting a push uses, and [evidence](docs/quality/evidence.md) for what a
passing run does and does not claim.

The command fails closed if the host cannot provide namespace sandboxing, for example on
Ubuntu 24.04 with the default AppArmor policy. The preflight prints the fixes. If you can't apply
them, opt in to reduced isolation explicitly; the result is labeled `isolation: weakened`:

```sh
tools/verify.sh --weak-isolation --base=e3b3b348587483ed1da10957766e32615aaabb01
```

To bring the whole stack up locally and demo it (the real page, the real server, the generated client), then
open the URL it prints:

```sh
tools/bootstrap/bazel.sh run //tools/dev
```

On a host that needs reduced isolation, add `--config=weak-isolation` after `run`. The command uses the
development server, which accepts any test identity and listens only on loopback; `tools/dev/profile.json` is
the one place its ports, store and seed are declared, notes persist in a local directory until
`-- --reset`, and a busy port or an invalid value is refused by name before anything starts. See
[docs/local-demo.md](docs/local-demo.md) for the profile, persistence, restart, reset and the limitations.

![Local demo: the real page against the real local server, recorded from that one command with the test identity; sign-in, create, reload, open](docs/media/local-demo.gif)

_Local demo, recorded from the running stack by `tools/dev/record.sh` at the revision named in
[docs/media/local-demo.json](docs/media/local-demo.json). It shows the development identity on loopback; nothing
hosted is claimed._

Other entry points:

| Command                                              | What it does                                                                                                                                          |
| ---------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------- |
| `tools/bootstrap/bazel.sh build //bff/cmd/bff:image` | The production server as an OCI layout, built only here; `:image_tarball` loads it into a runtime elsewhere                                           |
| `tools/bootstrap/fresh_clone.sh`                     | Fresh-clone witness; `--hostile` also plants rc files and a real defect                                                                               |
| `tools/bootstrap/bazel.sh run //tools/hooks:install` | Installs the pinned Git hooks; see [hooks](docs/quality/hooks.md)                                                                                     |
| `tools/bench/measure.sh`                             | Feedback-cost measurement of bootstrap, first verify, warm and edited runs, or of the hook stages with `--stages-only`; results go to `docs/budgets/` |

`bazel` on your PATH is not used by the verifier. To run Bazel yourself with the same pin and rc
isolation, use `tools/bootstrap/bazel.sh`.

## What to read first

1. [Design for review](specs/001-bff-template-foundation/design.md)
2. [Constitution](.specify/memory/constitution.md)
3. [Spec of specs and milestone map](specs/001-bff-template-foundation/spec.md)
4. [Research, including the reference inspection](specs/001-bff-template-foundation/research.md)
5. [Tasks](specs/001-bff-template-foundation/tasks.md),
   [acceptance matrix](specs/001-bff-template-foundation/acceptance-matrix.md) and
   [issue map](specs/001-bff-template-foundation/issue-map.md)
6. [Provenance](docs/provenance.md) and [public-release review](docs/public-release-review.md)

## Spec Kit

Spec Kit is pinned to `github/spec-kit` v1.2.3 (commit `5ea8b1cf`) and run through a pinned uv with hash-locked dependencies. To check
that the vendored scaffold matches that release:

```sh
tools/speckit/bootstrap.sh verify
```

## Owner

Proposed: Sélamy Labs, pending the operator's approval of the public identity, recorded in the external
release packet. The public source repository is published at 2d0ecf9 after the publication and neutrality checks; tagged releases, package publication and hosted deployment remain behind their operator gates. See research R-14.
