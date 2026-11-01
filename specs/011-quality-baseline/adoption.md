# Adoption reconciliation

A portfolio adoption matrix, held outside this repository (2026-11-01, primary sources cited there), was reconciled
against the canonical map. Each applicable row either reuses an existing enforced control, with its witness, or
becomes one bounded task with an owner, a command, a threshold source and exact-revision evidence; a row that does
not apply says why. Nothing here adds a framework, an upgrade, a remote generation service, a flag service or a
controller. Rows I01 to I03 are optional host contracts (spec 001 host-profile contract) and bind nothing here.

| Row | Here | Reused control, or bounded task |
| --- | --- | --- |
| G01 configuration and defaults | spec 011 FR-016, US4 | T019 to T021. Protocol zero values are contracts of the schema, never application configuration defaults (A01); the server's `Config.Check` already refuses an unset authenticator, store or origin at the boundary (`bff/server`, `server_test`) |
| G02 runtime flags | spec 011 FR-016 | T021: OpenFeature only where runtime evaluation fits, probed before any pin, with the configured fallback and the evaluation reason recorded; files for stable configuration. No flag service |
| G03 strong types | spec 011 FR-012 | TypeScript already runs `strict` and `noUncheckedIndexedAccess` (`tsconfig.json`); T003 inventories the escape hatches and probes `exactOptionalPropertyTypes`; T022 owns the Python scripts' typing |
| G04 coverage | spec 011 FR-008 | T007. Lines and branch outcomes measured separately; Go's cover tool does not instrument short-circuit interiors; the gobco probe covers `if` branches only and an uncalled `select` is not counted even with `-branch`, so it is at most a named sub-metric. Python coverage for the three scripts is T022, with branch measurement separate from lines |
| G05 property and mutation | spec 011 FR-009, FR-010 | T008, T009; survivors reported by population (killed, surviving, invalid, timed out, excluded); Python property tests through Hypothesis in T022 |
| G06 complexity and deletion | spec 011 FR-005 to FR-007 | The Go complexity gate is enforced (`complexity`, `complexity_test`, evidence m9); `dupl` candidates need semantic review (T004); dead-code detection needs a planted violation and declared entry points (T005). Python complexity through the pinned uv route is T022 |
| G07 templates, README, minimalism | spec 010 FR-012, T018 | The organization's public `.github` defaults are inherited; no local template directory, so the inherited forms are not suppressed; README accuracy is T018 |
| G08 specs and evidence | spec 001 and this map | Reconciled through the installed Spec Kit workflow; `/speckit-analyze` is read-only; checklist marks are reviewer judgments, not test results |
| G09 observability and security | spec 004, spec 005; spec 011 | Startup failure is witnessed by the image's exact refusal (spec 009) and `server_test`; bounded shutdown exists in `bff/server`. Reachable-vulnerability analysis needs a database fetch and is T023, a probe at the hosted boundary, never a substitute for the inventory |
| B01 hermetic builds | spec 002 | Pinned toolchains, lockfile error mode and the sandbox preflight exist (`tools/verify.sh`, `toolchain-fixtures`); a sandbox is not hostile-fork isolation, which B05 covers |
| B02 remote credentials and cache | spec 010 T021 | No remote cache, no BES, no credential helper; forks receive nothing. Stays that way unless a reviewed plan says otherwise |
| B03 trusted receipts | spec 007 | The receipt names the tested tree, the base, the policy digest and the image digest (receipt v5); hosted receipts are uploaded as artifacts and never trusted by label (`ci_parity_test`) |
| B04 RBE performance | not applicable | No remote execution in scope (spec 001) |
| B05 public CI trust | spec 007, spec 010 | Ephemeral hosted runners, `persist-credentials: false`, minimum permissions, pinned action SHAs (`.github/workflows/ci.yml`, `zizmor`, `actionlint`); trusted publishing stays a separate path with no shared credentials |
| B06 required CI | spec 010 FR-009b | The gate job rejects any non-success result and the exact leg count; the ruleset's required check is related to it (`governance_test`) |
| B07 hosted resources | spec 010 T021 | Measured from real runs after publication; no private cache, no larger runners |
| B08 OCI reproducibility | spec 009 research R-adoption | Base-less static image pinned by content; one declared platform; `image_index` and signing stay out until qualified at the pinned rules_oci 2.3.0 |
| B09 disclosure, licenses, provenance | spec 010 FR-002 to FR-004 | The inventory by scope with the image's measured linked set; unresolved findings block publication and are reviewed one by one (T019) |
| B10 schema compatibility | spec 003 FR-006 to FR-008 | `proto-breaking` runs over the whole module against the declared baseline commit; the schema probes already delete a field (`FIELD_NO_DELETE`) and an RPC (`RPC_NO_DELETE`) against the declared baseline commit; generators are pinned. T024 added the whole-message deletion probe (`MESSAGE_NO_DELETE`), matched as the rule's JSON type |
| B11 GitHub App identity | not applicable | No App is installed or planned (spec 010 closed list) |
| A01 protocol presence | spec 003; spec 011 FR-016 | Explicit presence where needed; zero values are not configuration defaults |
| A02 RPC and browser security | spec 004, spec 005 | Server-side authentication and ownership (`identity`, `server_test`), exact origin, loopback-only dev mode; generated types do not validate runtime input, so protovalidate runs server-side |
| A03 React and Connect state | spec 005; spec 011 T025 | The client has no request cancellation and no error boundary today: T025 adds obsolete-request cancellation with a slow-A/fast-B witness, unmount cleanup, deadline behavior and a render error boundary |
| A04 accessibility and browser tests | spec 005; spec 011 T026 | Labels, `aria-live` and `aria-describedby` exist (`web/app/App.tsx`); T026 adds keyboard access, visible focus, focus recovery after an error and role-based locators in the Playwright journeys; automated scans are not full accessibility |
| A05 runtime health | spec 004, spec 009 | Startup and bind failures are refused and witnessed; process survival is not health, and no health endpoint is claimed |
| A06 fakes and wire contracts | spec 011 FR-011 | T010: the contract suite over the memory fake; in-process transport in `transport_test`, a real HTTP path in the browser journeys |
| I01 to I03 host contracts | spec 001 host-profile contract | Optional; NixOS is never mandatory for template users |
