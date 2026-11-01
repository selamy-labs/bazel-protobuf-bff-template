# Contract: host profile

FR-035. Spec 008's consumer and its pins are unchanged by this contract.

The qualified profile is a Debian 12 container on Linux x86_64, at UID 1000, with a full Linux sandbox, 2 CPUs, 6 GiB and a fresh `HOME`. Results hold for that profile and the revisions the evidence records name. `tools/bootstrap/preflight.sh` is necessary and is not qualification: any other host qualifies only by the unchanged gates below. Support is Linux only. macOS stays unverified.

An optional NixOS host is a future producer's offering. No template user needs it, it does not build images, and it owns no toolchain. A host configuration belongs to its producer, and this repository holds none.

## What the template needs from any host

These are measured requirements, not an implementation. A producer may meet them however it chooses.

| Concern | Requirement | Measured or stated by |
| --- | --- | --- |
| Pins | The template's pins are used unchanged: Bazel by sha256, tools by lockfile, Go in `MODULE.bazel`, Spec Kit by commit | `tools/bootstrap/bazel.pins`, `multitool.lock.json` |
| Loader and glibc | The pinned prebuilt binaries run unmodified. glibc reports 2.36 or newer, a floor measured from the browser closure | `preflight.sh` |
| Browser | The pinned `chrome-headless-shell` and its pinned Debian 12 library closure load and run | `web` end-to-end tests |
| Fonts | Tests use the pinned font set through `tools/browser/fonts.conf`. A qualified run once crashed without font data, so a host's fonts are not relied on | `docs/evidence/m2-m4.md` |
| UID | Measured at UID 1000. A producer states its own, and private writable state must belong to it | Qualified runs |
| Sandbox | `linux-sandbox` works, with unprivileged user and mount namespaces. A qualifying run reports `isolation: full`. `--weak-isolation` is labeled, and does not qualify | `preflight.sh`, receipts |
| Credentials | The app, browser and quality checks need none, and a qualifying run has none | Qualified runs |
| Consumers | Immutable: full SHA pins, bundles, `file://` fixtures. A host rewrites no pin and injects no override | spec 008 |

## State and caches

- **Writable state is private** to a user or a run: `$XDG_CACHE_HOME/bff-template`, Bazel's output root, prek's cache, `.verify/`.
- **A shared cache is read-only**, and only if the producer supports that.
- **A qualification run uses a fresh `HOME`.** That describes the run. It does not forbid a developer's own caches.
- **Retention and disk are the producer's**, in its descriptor. This contract sets no universal number, because none has been measured for such a host.

## Producer descriptor

Required before any pilot, versioned and reviewed:

- operating system and architecture
- the immutable configuration revision, and the descriptor's version
- measured capacity requirements
- retention limits, and who owns cleanup
- what happens on exhaustion

Numbers are the producer's own measurements.

## Qualifying a host

The existing commands, unchanged, with primary receipts:

1. `tools/bootstrap/preflight.sh`
2. `tools/verify.sh --base=<commit>`, including the browser tests
3. A consumer's own verifier at its pinned commit

Provisioning a host proves nothing here, and no support is promoted without these gates passing unchanged.

## Status

Queued, as T092. It needs a reviewed immutable producer revision and a complete descriptor. After that, one disposable pilot, and no wider migration.

## Ownership

| Concern | Owner |
| --- | --- |
| Infrastructure lifecycle | OpenTofu |
| NixOS on self-managed hosts | Shared host operations |
| App builds, tests, toolchain pins | Bazel |
| App images | rules_oci, through Bazel |
| Workloads | GitOps, where it applies |
| Managed runtimes | The provider, as supported |
