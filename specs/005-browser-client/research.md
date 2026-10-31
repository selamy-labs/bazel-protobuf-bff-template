# Research: Browser Client and End-to-End Tests (M4)

**Date**: 2026-10-31.

## R1 A hermetic browser (parent T030)

**Probe** (2026-10-31, outside the repository): Playwright 1.64.1 pins `chrome-headless-shell`
156.0.8078.4. It downloads from `cdn.playwright.dev` with a stable URL. On the authoring host it
fails at start with `libnspr4.so: cannot open shared object file`; `libnss3`, `libnssutil3` and
`libasound` are also missing. A clean host would miss many more.

**Decision**: pin the browser archive and its full shared-library closure by sha256.

- The libraries are Debian 12 packages. Bookworm's glibc 2.36 sets the floor, and hosts need only
  glibc at that version or newer. Ubuntu 24.04 has 2.39. The verifier's preflight enforces this
  floor on Linux, and fails when it can't confirm glibc.
- `tools/pins/gen_browser_libs.py` resolves the closure from Debian's package index and writes
  `tools/browser/debs.bzl`. A repository rule fetches and flattens them.
- Tests point Playwright at the pinned executable with `LD_LIBRARY_PATH` set only for the browser.

**Rejected**:

- Documenting system packages as a prerequisite breaks constitution III, and could not run here.
- rules_playwright pins browsers but leaves host libraries to the host.
- A community Lambda build of Chromium would swap a pinned official browser for an unofficial one.

**Scope**: Linux x86_64 only. Other platforms fail with a clear message instead of skipping.

**Chromium's own sandbox**: Playwright launches Chromium with `--no-sandbox` by default. That stays,
because user namespaces are unavailable to it on restricted hosts and the browser loads only
this test's local pages. It is a test-only choice, and it is not a claim about production browsers.

## R2 Serving the page

The page reads the server URL from `/config.json`, so one build works at any address. A small static
server, `web/serve/serve.mjs`, serves the bundle and that file. Tests start it first on port 0, then
start the server with that exact origin. This needs no fixed ports, which keeps parallel test runs
safe.

## R3 Two web builds

`vite build` runs in a production mode and a `testonly` dev mode. Only the dev mode defines the test
identity constant, so the bundler removes that code from production. A test checks the production
bundle for it.

## R4 Client tests

Mapping tests use `node:test` on compiled TypeScript instead of Vitest. That adds no dependency and
revises parent T031. Rendered behavior is covered in the browser.

## R5 The defect witness (parent T041)

A testonly genrule applies one `sed` edit to `bff/transport/interceptors.go`, mapping `not_found` to
`internal`. A small target chain compiles that into a variant dev binary. The same specs run against
it, and the witness requires that exactly the not-found scenarios fail. An edit that changes nothing
fails the genrule.

## R6 Pins

| Item | Pin | Notes |
| --- | --- | --- |
| React, React DOM | 19.3.1 | |
| Vite | 8.3.4 | 8.3.5 is 10 days old |
| @vitejs/plugin-react | 6.1.2 | |
| @connectrpc/connect-web | 2.2.0 | |
| @playwright/test | 1.64.1 | |
| chrome-headless-shell | 156.0.8078.4 | Playwright's pin; sha256 in `MODULE.bazel` |
