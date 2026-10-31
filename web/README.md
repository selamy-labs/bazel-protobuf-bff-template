# web

## Purpose

This is the browser client: one form and one lookup that call the BFF through the generated client.

## What to read first

- [client/errors.ts](client/errors.ts) maps each contract error to what the page shows.
- [e2e/notes.spec.mjs](e2e/notes.spec.mjs) lists every scenario, run in a real browser.

## Layout

| Path | Role |
| --- | --- |
| `client` | Generated-client transport, credential source, error mapping |
| `app` | React page and the two web builds |
| `serve` | Static server for local use and tests, including `/config.json` |
| `e2e` | Playwright specs and the broken-mapping witness |

## Run it

```sh
tools/bootstrap/bazel.sh run //tools/dev
```

Then open <http://127.0.0.1:5173>. The test identity field chooses the caller. These subjects force
a failure: `fault-unavailable`, `fault-hang`, `fault-secret` and `fault-panic`.

## Test it

```sh
tools/bootstrap/bazel.sh test //web/...
```

The browser and every library it loads are pinned by sha256, for Linux x86_64 with glibc 2.36 or
newer. Other platforms fail with a message saying so. `tools/verify.sh` runs all of these tests.

## Identity

Only the `testonly` build, `:dist_dev`, contains test identity. The production build, `:dist`, has no
credential source and says that sign-in is not configured. Connect yours in `app/main.tsx`.

The server validates input, so there is one set of rules.

## Owner

The template maintainers (see the root README).
