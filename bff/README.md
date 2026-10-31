# bff

## Purpose

This is the Go backend for the browser. It serves `CreateNote` and `GetNote` over the generated
contract, and stores notes behind a port.

## What to read first

- [transport/interceptors.go](transport/interceptors.go) has the error mapping, deadlines and the
  authentication boundary.
- [ports/store.go](ports/store.go) is where a real backend attaches.

## Layout

| Path | Role |
| --- | --- |
| `transport` | Handlers, validation, error mapping, CORS |
| `app` | Use cases, with no transport or generated types |
| `ports` | What the application needs from storage |
| `adapters/memory` | In-memory store. Data does not survive a restart. |
| `identity` | Authentication boundary |
| `server` | Wiring, start-up refusal, ready event, shutdown |
| `cmd/bff` | Production binary |
| `cmd/bff-dev` | Development binary, `testonly` |
| `testing` | Test identity and fault injection, `testonly` |

`dependency_rules.tsv` lists the dependency directions, and a test enforces them.

## Identity

This repository ships no identity provider, so the production binary refuses to start as shipped:

```text
bff: no verifier is configured; assign newVerifier in bff/cmd/bff/verifier.go
```

To run in production, implement `identity.Verifier` and assign `newVerifier`. Run the development
binary locally. It accepts any subject, listens only on loopback, and is labeled `TEST IDENTITY`.

The browser sends a bearer credential in the `Authorization` header, and cookies never authenticate.
CORS allows no credentials, so the server has no CSRF surface. Before you move to cookie sessions,
add `SameSite`, CSRF tokens and origin checks.

## Errors

A store or verifier cannot choose what a caller reads. Only `ports.ErrNotFound`,
`ports.ErrUnavailable`, `identity.ErrUnauthenticated` and context errors keep their meaning across
that boundary. Everything else becomes `internal` with a correlation ID, and the detail goes to the
server log.

## Owner

The template maintainers (see the root README).
