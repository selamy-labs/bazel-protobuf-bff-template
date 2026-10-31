# Implementation Plan: Browser Client and End-to-End Tests (M4)

**Branch**: `005-browser-client` | **Date**: 2026-10-31 | **Spec**: [spec.md](spec.md)

## Summary

A minimal React page calls the BFF through the generated client. Playwright drives a pinned Chromium
under `bazel test` against the real dev binary, across two origins. A variant binary with a broken
mapping proves the specs can fail for the right reason. `//:dev` runs the same pieces locally.

## Technical Context

| Item | Choice |
| --- | --- |
| Languages | TypeScript 6.0.3, React 19.3.1, Node 24.22.0 |
| New tools | Vite 8.3.4, connect-web 2.2.0, Playwright 1.64.1, pinned Chromium and Debian libraries |
| Testing | `js_test` with `node:test`; Playwright under `js_test`; `sh_test` for bundle and dev runner checks |
| Constraints | Sandbox network off; loopback only; no fixed ports in tests |

## Constitution Check

| Principle | Status | Notes |
| --- | --- | --- |
| I. Real slice | PASS | This milestone closes it: browser, generated client, real server. |
| II. Contracts first | PASS | Only generated clients call the server. |
| III. Hermetic, pinned | PASS | The browser and its libraries are pinned by sha256 (research R1). |
| IV. One policy | PASS | Browser tests run in `bazel test`, so local and CI are the same. |
| V. Cannot weaken itself | PASS (scoped) | M6 limits still apply. |
| VI. Exact-revision evidence | PASS | New tests join the required inventory. |
| VII. Guarantees vs signals | PASS | Scenarios are guarantees. UI quality is review. |
| VIII. Bounded, actionable | PASS | Test timeouts; child processes stopped on exit. |
| IX. Public, neutral | PASS | Provenance records Debian packages and Chromium. |
| X. Smallest route | PASS | No Vitest, no CSS framework, no router. |
| Security boundaries | PASS | No credentialed CORS; test identity fenced from the production bundle. |

## Project Structure

```text
web/
├── client/      # transport, errors.ts (Connect error → Outcome), credentials
├── app/         # React components, index.html, vite config
├── serve/       # static server and /config.json
└── e2e/         # Playwright config, specs, defect witness
tools/browser/   # debs.bzl, repository rule
tools/dev/       # //:dev runner
```

## Complexity Tracking

| Exception | Why | Owner | Expiry |
| --- | --- | --- | --- |
| Debian library closure | A browser cannot run hermetically without it, and no smaller pinned option exists | Template maintainers | Review at each Playwright upgrade |
