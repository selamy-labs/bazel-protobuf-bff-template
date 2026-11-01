# bff

## Purpose

This is the Go backend for the browser. It serves `CreateNote` and `GetNote` over the generated
contract, and stores notes behind a port.

## What to read first

- [transport/interceptors.go](transport/interceptors.go) has the error mapping, deadlines and the
  authentication boundary.
- [ports/store.go](ports/store.go) is where a real backend attaches.

## Layout

| Path              | Role                                              |
| ----------------- | ------------------------------------------------- |
| `transport`       | Handlers, validation, error mapping, CORS         |
| `app`             | Use cases, with no transport or generated types   |
| `ports`           | What the application needs from storage           |
| `adapters/memory` | In-memory store. Data does not survive a restart. |
| `identity`        | Authentication boundary                           |
| `server`          | Wiring, start-up refusal, ready event, shutdown   |
| `cmd/bff`         | Production binary                                 |
| `cmd/bff-dev`     | Development binary, `testonly`                    |
| `testing`         | Test identity and fault injection, `testonly`     |

`dependency_rules.tsv` lists the dependency directions, and a test enforces them.

## Identity

This repository ships no identity provider, only the adapter for one: `bff/adapters/identity/oidc` verifies tokens
signed by an issuer that publishes its keys as a JSON Web Key Set, which any OpenID Connect provider does. The
production binary refuses to start until the issuer is named:

```text
bff: no verifier is configured: name the issuer with --verifier-issuer, --verifier-audience and --verifier-jwks-url (or BFF_VERIFIER_ISSUER, BFF_VERIFIER_AUDIENCE, BFF_VERIFIER_JWKS_URL); this repository ships no identity provider
```

To run in production, name your issuer with `--verifier-issuer`, `--verifier-audience` and `--verifier-jwks-url` (or
the `BFF_VERIFIER_*` variables): a token is accepted only when its signature checks against the issuer's current
keys and its issuer, audience and expiry hold, and a rejected token says nothing about why. Run the development
binary locally. It accepts any subject, listens only on loopback, and is labeled `TEST IDENTITY`.

The browser sends a bearer credential in the `Authorization` header, and cookies never authenticate.
CORS allows no credentials, so the server has no CSRF surface. Before you move to cookie sessions,
add `SameSite`, CSRF tokens and origin checks.

## Persistence

The store is the composition's choice: `--store=memory`, the example's bounded store, or `--store=object` with
`--object-bucket`, one object per note in a managed bucket through the bucket service's JSON API with the
runtime's own token ([adapters/store/object](adapters/store/object/store.go)). The local demo uses the
file-backed store through the launcher. Every adapter passes the same contract suite as the memory fake.

## Errors

A store or verifier cannot choose what a caller reads. Only `ports.ErrNotFound`,
`ports.ErrUnavailable`, `identity.ErrUnauthenticated` and context errors keep their meaning across
that boundary. Everything else becomes `internal` with a correlation ID, and the detail goes to the
server log.

## Owner

The template maintainers (see the root README).
