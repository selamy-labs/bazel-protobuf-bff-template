# web

## Purpose

This is the browser client: one form and one lookup that call the BFF through the generated client.

## What to read first

- [client/errors.ts](client/errors.ts) maps each contract error to what the page shows.
- [client/lifecycle.ts](client/lifecycle.ts) keeps one request in flight: a newer one cancels the older, and a
  superseded result is ignored.
- [e2e/notes.spec.mjs](e2e/notes.spec.mjs) lists every scenario, run in a real browser.
- [e2e/signin.spec.mjs](e2e/signin.spec.mjs) runs the production bundle and the production server against a
  local issuer ([e2e/issuer.mjs](e2e/issuer.mjs)): the sign-in and the verified calls.

## Layout

| Path     | Role                                                                                       |
| -------- | ------------------------------------------------------------------------------------------ |
| `client` | Generated-client transport, credential source, error mapping                               |
| `app`    | React page and the two web builds                                                          |
| `serve`  | Static server for local use and tests, under a required base path, including `config.json` |
| `e2e`    | Playwright specs and the broken-mapping witness                                            |

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

The browser, every library it loads and its fonts are pinned by sha256, for Linux x86_64 with glibc 2.36 or
newer. `tools/verify.sh` runs all of these tests, and its preflight stops early on Linux below that
floor or when it can't confirm glibc. A direct `bazel test` has no preflight. On a host below the
floor, the browser fails at start. Other platforms fail with a message saying so.

## Identity

Only the `testonly` build, `:dist_dev`, contains test identity. The production build, `:dist`, signs in
with the issuer its `config.json` names (`signIn: { issuer, clientId }`), through the authorization code
flow with a proof key in `app/signin/`: the browser is sent to the issuer, comes back with a code, and
the page exchanges it for an identity token, which it sends as the bearer credential. The server
verifies that token (`bff/adapters/identity/oidc`); the page only keeps it for the tab and shows who is
signed in. With no issuer configured, the page says so and every call goes without a credential. Sign
out forgets the tab's session; the issuer's own session is its own. `//web/app:bundle_test` inspects
both bundles: the sign-in present in production, the test identity absent.

The server validates input, so there is one set of rules.

## Owner

The template maintainers (see the root README).
