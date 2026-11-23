# Reproducible Playwright Video Evidence for BFF Web Flows

This template provides one reusable, hermetic Playwright recording recipe for representative web smoke checks and comparative visible-flow before/after evidence (`#27`, spec `005-browser-client` FR-014..FR-016). All recording code lives strictly in `testonly` tooling under `web/e2e/` and `tools/browser/`; production web bundles (`//web/app:dist`) and production server images (`//bff/cmd/bff:image`) never include or execute recording code.

## Single Command

Record and verify a representative smoke capture of the Notes reference flow at the current committed revision (default CI mode):

```sh
tools/checks/web_video_evidence.sh
```

By default this stages capture and validation in a temporary directory and publishes bounded evidence to `.verify/web-video-evidence/` (or `$BFF_VIDEO_EVIDENCE_OUTPUTS` / `--out <dir>`) **only after validation succeeds**:

- `manifest.json` — binds the exact 40-hex Git revision (`revision`), flow name, mode (`representative-smoke`), synthetic seed state, viewport dimensions, codec, byte sizes, SHA-256 digests, and non-production evidence boundary.
- `desktop/smoke/flow.webm` + `desktop/smoke/*.png` (`1280x720`).
- `mobile/smoke/flow.webm` + `mobile/smoke/*.png` (`390x844`).
- `video-evidence-command.log` + `command-result.json` — always preserved, including on failure (when zero unvalidated media enters the output directory).

For visible web flow changes that require matched comparative before/after evidence against a baseline revision, pass `--mode before-after --before-revision <40-hex-baseline-sha>`. The script ALWAYS checks out that exact baseline commit in a clean detached `git worktree`, builds `//web/app:dist_dev` from it, writes a SHA-256 `bff-web-dist-provenance/v1` manifest binding the baseline commit, git tree, Bazel target, and built tree digest (`distSha256`), and verifies the served baseline bytes against that provenance before recording:

```sh
tools/checks/web_video_evidence.sh \
  --mode before-after \
  --before-revision <40-hex-baseline-sha>
```

`tools/checks/web_video_evidence.sh` does not accept an arbitrary caller-supplied baseline directory. At the lower-level `record-notes-video.mjs` CLI boundary, `--before-web-dist` requires matching `--before-revision`, `--before-tree`, and `--before-provenance`; an unrelated or tampered distribution paired with the expected baseline SHA is refused before recording. Comparative `before-after` mode binds both `beforeRevision` + `beforeDistSha256` and candidate `revision` + `distSha256` in `manifest.json` and per-phase captures, at matching desktop (`1280x720`) and mobile (`390x844`) viewports, identical synthetic seed state, and matching screenshot steps. For local uncommitted previews (never used in CI), pass `--allow-dirty`.

## Hermetic Browser and Video Encoder

`//tools/browser:browser` (`testonly = True`) bundles the SHA-256-pinned `chrome-headless-shell` binary, its Debian shared libraries and fonts, and the SHA-256-pinned static Playwright FFmpeg build (`playwright_ffmpeg`, revision `1013` matching `@playwright/test 1.64.1`). No host browser or host `ffmpeg` is required.

## Conforming Stateful Connect Fake

`web/e2e/connect-notes-fake.mjs` implements a stateful fake for `template.notes.v1.NoteService` using the generated Protobuf schemas (`//api:notes_ts`) and Connect JSON/binary wire encodings:

- Real state transitions (`CreateNote` -> `GetNote`) and disk persistence across reloads and server restarts.
- Real `buf.validate.Violations` protobuf error details using Unicode code-point lengths (`<= 120` title, `<= 4000` body) matching `api/template/notes/v1/notes.proto`.
- Real `unauthenticated` (missing credential), cross-subject `not_found` denial, and `unavailable` failure behavior.

`//web/e2e:connect_fake_test` verifies wire-level and Unicode-boundary conformance directly against the generated `createNotesClient` (`//web/client`).

## Boundary: Deterministic UI Evidence vs. Hosted Acceptance

- **Deterministic UI evidence (`evidenceBoundary: "deterministic-ui-fake"`)**: Uses synthetic seed data and the conforming stateful Connect fake to produce reproducible representative smoke or dual-revision before/after videos, matched screenshots, and exact-revision manifests in local runs and CI. It proves UI layout, accessibility, state transitions, reload persistence, validation, denial, and error rendering without secrets or external providers.
- **Hosted sign-in, provider, and live release acceptance**: Remains a separate real-browser and live-environment responsibility. Production OIDC/provider sign-in flows (`//web/e2e:signin_e2e_test`, `//web/e2e:same_origin_e2e_test`, actual-container `tools/checks/image_runtime.sh`) and hosted Cloud Run release smoke (`#20`) are never replaced by fake-backed recordings.

## CI Artifacts and Fail-Closed Verification

- The `e2e` job in `.github/workflows/ci.yml` runs `//web/e2e:connect_fake_test` and `//web/e2e:video_evidence_test` (including automated negative checks rejecting missing manifests, wrong candidate or baseline revisions, missing or non-WebM videos, wrong-dimension WebM video tracks even with consistent manifest digests, mismatched before/after starting states or PNG viewport dimensions, missing mobile captures when required, oversized artifacts, secret markers, and false hosted-acceptance claims).
- `//:web_video_evidence_script_test` verifies staged-then-publish discipline: failed runs preserve logs and `command-result.json` (`"published":false`) while publishing zero unvalidated media.
- The `e2e` job runs `tools/checks/web_video_evidence.sh` (`representative-smoke`) into `$artifact_path/web-video-evidence`, publishing validated reviewer-accessible artifacts inside `verify-receipt-e2e-<attempt>` (14-day retention).

## Adopting in Derivative Apps and Future Reference Clients (`#29`)

Derivative applications reuse `web/e2e/playwright.video.config.mjs` and `web/e2e/video-evidence.mjs` (`createVideoPlaywrightConfig`, `recordWebFlowEvidence`, `validateVideoEvidence`, `parseWebmVideoTrack`, `VIEWPORTS`) directly instead of copying a second recording framework:

1. Pass your app's `flow`, synthetic `seedState`, `viewports`, explicit `revision` (and `beforeRevision` when comparing two builds), and `runFlow` callback to `recordWebFlowEvidence`.
2. Verify the emitted directory with `validateVideoEvidence` against the exact Git revision(s) and mode (`representative-smoke` or `before-after`).
3. Keep all recording helpers strictly `testonly = True` and synthetic-only. Downstream `#29` CLI/web reference-client acceptance stays tracked separately under `#29`.
