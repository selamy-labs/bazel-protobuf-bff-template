# Defaults inventory

Every default in the first-party Go and TypeScript, by its site and its boundary, taken over the tracked non-test
sources at `e6fe228` (spec 011 FR-016, T019). A default is a value the code uses when nothing was given. Each row
is one of:

- **composition**: set once where the program is assembled, overridable from outside, validated there;
- **input**: read from a typed file or the arguments with a schema that refuses unknown, missing, malformed or
  out-of-range values by name, and nothing below it carries a default;
- **contract**: a fixed value the API contract or a check contract names; not configurable by design;
- **scattered**: a default an inner package carries on its own. Each names its owner: T020 moves it to the
  composition or the input boundary, or records it as a fixed bound with its reason.

Test-only packages (`bff/testing/*`, `bff/ports/porttest`, the end-to-end and recording tooling) are outside the
inventory: they are not configurable behavior of the product.

## Production server, `bff/cmd/bff`

| Site                                                                          | Value                                                                                                                                       | Boundary                                              |
| ----------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------- |
| `config.go` `--addr`                                                          | the flag, then `BFF_ADDR`, then `0.0.0.0:$PORT`, then `:8080`                                                                               | composition                                           |
| `config.go` `--allowed-origin`                                                | the flag, then `BFF_ALLOWED_ORIGIN`; no default, refused when empty                                                                         | composition                                           |
| `config.go` `--mode`                                                          | the flag, then `BFF_MODE`, then `production`; only `production` accepted                                                                    | composition                                           |
| `config.go` `--identity`                                                      | the flag, then `BFF_IDENTITY`, then `verifier`; only `verifier` accepted                                                                    | composition                                           |
| `config.go` `--verifier-issuer`, `--verifier-audience`, `--verifier-jwks-url` | the flag, then `BFF_VERIFIER_ISSUER`, `BFF_VERIFIER_AUDIENCE`, `BFF_VERIFIER_JWKS_URL`; no default, all three required, refused by name     | composition                                           |
| `config.go` `--store`, `--object-bucket`                                      | the flag, then `BFF_STORE`, then `memory`; the bucket from the flag or `BFF_OBJECT_BUCKET`, required with the object store, refused by name | composition                                           |
| `config.go` `objectEndpoint`, `objectTokenURL`, `objectPrefix`                | the public bucket API, the metadata server's token endpoint, `notes`                                                                        | composition: declared once, required by `object.Open` |
| `bff/server` `Config.Check`                                                   | mode, authenticator kind, store, exact origin, loopback in dev                                                                              | composition (validation)                              |

## Development server, `bff/cmd/bff-dev`

| Site                         | Value                                  | Boundary                                                |
| ---------------------------- | -------------------------------------- | ------------------------------------------------------- |
| `main.go` `--addr`           | required; refused by name when missing | input: the profile's `bff_port`, passed by the launcher |
| `main.go` `--allowed-origin` | required; refused by name when missing | input: the profile's `web_port`, passed by the launcher |
| `main.go` `--store`          | required; refused by name when missing | input: the profile's `store`, passed by the launcher    |
| `main.go` `--data-dir`       | empty, required with `local`           | composition                                             |

## Local demo profile, `tools/dev/profile.json` and `//tools/dev/profile`

| Site                                                    | Value                                                                                                                                       | Boundary                                                                                                                                                                                                                                                                                     |
| ------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `profile.json`                                          | `version` 1, `bff_port` 8080, `web_port` 5173, `store` `local`, `data_dir` `.local/notes`, `identity` `test-identity`, `seed_subject` `ana` | input: the schema in `tools/dev/profile/main.go` refuses a missing, unknown, malformed or out-of-range value by name; `BFF_PORT`, `WEB_PORT`, `BFF_STORE`, `BFF_DATA_DIR` override one value each under `--env`; nothing below carries a default; the effective value prints with its source |
| `tools/dev/dev.sh` `${BUILD_WORKSPACE_DIRECTORY:-$PWD}` | the data directory's base                                                                                                                   | composition (the launcher resolves a relative `data_dir` against the workspace or the current directory; shell, outside the Go and TypeScript scope, recorded for completeness)                                                                                                              |

## Hosted example inputs, `deploy/gcp/inputs`

| Site                              | Value                                                                                                                         | Boundary                                                                                                                                                                    |
| --------------------------------- | ----------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `inputs.schema.json`, `inputs.go` | no defaults: every value is required, a leaf given by no file is refused by name, and the example holds synthetic values only | input: a base file and an operator's overlays, the last file winning per leaf; every effective value printed with its source; the effective document is what the plan reads |

## Transport and server internals, `bff/transport`, `bff/server`

| Site                                                                                                              | Value                                                                                                                        | Boundary                                                                                                         |
| ----------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------- |
| `bff/cmd/bff/config.go` `--default-deadline` (`BFF_DEFAULT_DEADLINE`), formerly `handlers.go` `DefaultDeadline`   | 2 s for a call that sets no deadline                                                                                         | composition (moved at T020; the development composition declares its own copy in `bff-dev/main.go`)              |
| `bff/cmd/bff/config.go` `--max-deadline` (`BFF_MAX_DEADLINE`), formerly `handlers.go` `MaxDeadline`               | 10 s cap on any client deadline                                                                                              | composition, with the ceiling of one minute an invariant the server's check enforces whatever configuration asks |
| `bff/cmd/bff/config.go` `--max-request-bytes` (`BFF_MAX_REQUEST_BYTES`), formerly `handlers.go` `MaxRequestBytes` | 64 KiB                                                                                                                       | composition, between the floor the schema's largest note needs and a ceiling of 1 MiB, both invariants           |
| `bff/cmd/bff/config.go` `--shutdown-bound` (`BFF_SHUTDOWN_BOUND`), formerly `server.go` `shutdownBound`           | 5 s drain                                                                                                                    | composition, with a ceiling of thirty seconds an invariant                                                       |
| `bff/cmd/bff/config.go` `listener`, formerly `server.go` `http.Server`                                            | `ReadHeaderTimeout` 5 s, `ReadTimeout` 15 s, `WriteTimeout` `MaxDeadline` + 5 s, `IdleTimeout` 60 s, `MaxHeaderBytes` 16 KiB | composition: set once in the composition, required by the server, the write timeout derived from the call bounds |
| `interceptors.go` correlation id                                                                                  | 8 random bytes, hex                                                                                                          | contract (specs/001 contracts/notes-api.md: an internal error carries only a correlation id)                     |

## Application and adapters, `bff/app`, `bff/adapters`

| Site                                                                                                    | Value                          | Boundary                                                                                                                                 |
| ------------------------------------------------------------------------------------------------------- | ------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------- |
| `app/notes.go` `namePrefix`, `idPattern`, id of 16 random bytes                                         | `notes/` and `[a-z0-9-]{1,64}` | contract (the resource name in the schema's validation rule)                                                                             |
| `bff/cmd/bff/config.go` `--max-notes` (`BFF_MAX_NOTES`), formerly `adapters/memory/store.go` `MaxNotes` | 10000 notes per process        | composition: `memory.New(limit)` requires it, the development composition declares its own, and the ceiling of a million is an invariant |
| `adapters/store/local/store.go` directory mode                                                          | `0o700`                        | contract: an invariant of the file-backed store, never configurable                                                                      |

## Browser client, `web/app`, `web/client`, `web/serve`

| Site                                            | Value                                                                         | Boundary                                                                                                                                                                                         |
| ----------------------------------------------- | ----------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `app/main.tsx` `deadlineMs`                     | 2000 ms, passed to every client                                               | composition                                                                                                                                                                                      |
| `app/main.tsx` `/config.json` `bffUrl`          | read at start                                                                 | input: the static server's `--bff-url`                                                                                                                                                           |
| `app/main.tsx` `/config.json` `signIn`          | read at start; absent means sign-in is not configured and the page says so    | input: the static server's `setSignIn` locally, the published site's configuration when hosted (the issuer and the client id are the verifier's inputs, declared once)                           |
| `client/transport.ts` `deadlineMs`              | required option, no default                                                   | composition (required inner parameter)                                                                                                                                                           |
| `serve/serve.mjs` `serve(root, port, basePath)` | an ephemeral port on `127.0.0.1`; the content types of `.html`, `.js`, `.css` | input: the port and the base path are required, the profile's `web_port` and the launcher's `/` for the local demo, or an ephemeral port and the hosted example's base path where a test says so |
| `app/main.tsx` `plantedRenderError`             | `#planted-render-error`                                                       | contract of the development build's witness, removed from production                                                                                                                             |

## Quality runner and checks, `quality/`, `tools/`

| Site                                                                                                                                                                                                                                                                                                                               | Value                                                                                                                                                                                           | Boundary                                                                                                                                                             |
| ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `runner/cmd/main.go` flags                                                                                                                                                                                                                                                                                                         | `--isolation` `full`, `--jobs` `DefaultJobs()`, `--slots` 2, `--deadline` 0 then the stage table (pre-commit 2 min, commit-msg 1 min, pre-push 30 min, full 38 min), `--base-source` `argument` | composition: the verifier and the hooks pass them                                                                                                                    |
| `runner/cmd/main.go` environment                                                                                                                                                                                                                                                                                                   | `BUILD_WORKSPACE_DIRECTORY`, `XDG_CACHE_HOME` then `$HOME/.cache`, `GITHUB_*` for the producer                                                                                                  | composition                                                                                                                                                          |
| `registry.textproto` `timeout_seconds` per check                                                                                                                                                                                                                                                                                   | per check                                                                                                                                                                                       | input (the registry is validated by the runner)                                                                                                                      |
| `runner/bazel.go` `clientGrace` 10 s, `serverGrace` 5 s; `supervise.go` `termGrace` 2 s; `stage.go` `bookkeepingBudget` 30 s, `maxRetainedLog` 1 MiB, `MaxCapturedStderr` 2048; `report.go` `MaxDiagnostics` 50, `MaxMeasurements` 32, `MaxMeasurementLen` 200, `keepLogRuns` 5; polls of 100 ms and 50 ms; a 1 MiB scanner buffer | the runner's bounds                                                                                                                                                                             | contract: each is a fixed bound in `specs/001-bff-template-foundation/contracts/check-registry.md` with its reason, and a change to one is a change to that contract |
| `runner/report.go` states and exit codes                                                                                                                                                                                                                                                                                           | `PASSED` … `NOT_RUN`; 0, 1, 2, 10, 11, 12                                                                                                                                                       | contract (`contracts/check-registry.md`)                                                                                                                             |
| `checks/complexity` `--budgets`                                                                                                                                                                                                                                                                                                    | `quality/complexity_budgets.json`                                                                                                                                                               | composition: the path convention, overridable by flag; the budgets file is the input boundary                                                                        |
| `checks/coverage/cmd/floors` `--contract`                                                                                                                                                                                                                                                                                          | `quality/coverage_floors.json`; `vendored` `tools/speckit/scaffold.sha256`                                                                                                                      | composition: path conventions, overridable by flag                                                                                                                   |
| `checks/suppression` `--root` `.`, `rerun`, `patternTable`                                                                                                                                                                                                                                                                         | path conventions                                                                                                                                                                                | composition                                                                                                                                                          |
| `proto/baseline.go` `BootstrapMarker`                                                                                                                                                                                                                                                                                              | `bootstrap`                                                                                                                                                                                     | contract (spec 003 FR-007)                                                                                                                                           |
| `release/release.go` `Catalog`, `maxLines` 20                                                                                                                                                                                                                                                                                      | the catalog's name; the diagnostic bound                                                                                                                                                        | contract: the catalog's name, and the diagnostic bound recorded in the check-registry contract                                                                       |
| `checks/coverage/config.go` metrics                                                                                                                                                                                                                                                                                                | the four metric names                                                                                                                                                                           | contract                                                                                                                                                             |

## Count

| Boundary    | Rows |
| ----------- | ---- |
| composition | 23   |
| input       | 9    |
| contract    | 10   |
| scattered   | 0    |

No scattered default remains at `8d92022` (SC-006): each was moved to a composition or an input boundary, or is a contract constant with its reason recorded. A new default joins this inventory with its boundary or does not land.

## Configuration-only witnesses and bypass controls

One witness per surface shows that configuration alone changes the behavior, and one control per surface shows
that a setting which would weaken a security, quality, spend or invariant contract is refused (spec 011 FR-016,
T021).

| Surface               | Configuration alone changes the behavior                                                                                                                                                                            | A weakening setting is refused                                                                                                                                                                                      |
| --------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Production server     | `TestTheBoundsAreConfiguration`: a tighter maximum caps a client asking for a minute, a tighter body bound refuses before decoding; `TestListenAddressPrecedence` and `TestBoundsPrecedence`: flag, variable, value | dev mode and test identity refused by flag and by variable; a deadline beyond a minute, a request beyond 1 MiB, a drain beyond thirty seconds, a store beyond a million notes refused by name whatever source asked |
| Local demo profile    | `//tools/dev:dev_test`: `BFF_STORE=local` makes a note survive a restart; `BFF_PORT` moves the server, printed with its source; `TestTheFileIsTheOneDefaultSiteAndTheEnvironmentOverridesByName`                    | an identity other than the development one, an unknown store, a shared port, a bad port value, each refused by name; the development server refuses any non-loopback address and any wildcard origin                |
| Hosted example inputs | `the_origin_port_and_bounds_come_from_the_funnel` in the service module's mock tests, and `TestAnOverlayWinsPerLeafAndEveryLeafNamesItsSource`: an operator's overlay changes the plan's bounds                     | a wildcard origin, an origin with a path, an `http` issuer, more than ten instances, a timeout beyond five minutes, a zero budget, a digest that is neither the image's nor the previous one, each refused by name  |
| Browser client        | the lifecycle test: a 20 ms deadline ends a hanging call as a deadline failure; the composition's 2 s is what the page uses                                                                                         | the production bundle carries configured sign-in and no test identity (`//web/app:bundle_test`); the server, not the page, decides what a credential is worth                                                       |
| Quality runner        | `--isolation=weakened` labels every result weakened; a stage deadline override ends a hung tree at its bound (`TestHungTreeIsRemovedAtItsDeadline`)                                                                 | a lowered coverage floor or a loosened budget is refused by comparison with the base (`floors-from-base`, `complexity compare`); a policy base the event did not name is refused (exit 11)                          |

Runtime evaluation: no behavior is evaluated at run time, so OpenFeature is not adopted (research R10).

## Object adapter bounds and browser session policy (review follow-up)

`bff/cmd/bff/config.go` declares objectHTTPTimeout (10 s), objectTokenMaxBytes (64 KiB),
objectMaxBytes (1 MiB), and objectTokenRefreshSkew (1 minute) once. The adapter requires
both byte limits and accepts an explicit zero refresh skew; it supplies no fallback.
Safety ceilings of 1 MiB token response, 16 MiB object and 1 hour skew are adapter
invariants, not defaults. Redirects are refused to keep the metadata/token and storage
trust boundaries fixed. Put retains the port's replacement semantics, without optimistic
concurrency; it must not be described as a compare-and-swap operation.

Browser sessions are scoped to the configured issuer and client. An unconfigured page
emits no credential. sessionStorage remains readable to same-origin JavaScript: this is
not an HttpOnly session or an XSS containment boundary. Sign-out invalidates any token
exchange already in flight. Browser expiry checks are presentation guards; the server
remains responsible for authenticating each RPC.

## Verifier policy and telemetry composition

`bff/cmd/bff/verifier.go` declares required `oidc.Policy`: leeway 30s, HTTP timeout
5s, refresh 1h, unknown-key refresh rate 5m with a 100ms wait, JWKS 64KiB and 32 keys.
The adapter does not invent these values. Leeway zero is explicit, other bounds
must be positive; absolute safety ceilings are validated by `Policy.Check`.
The process signal context owns initial fetch and refresh; failed construction
cancels its derived context. Key IDs are required. Public asymmetric signing keys
must match declared purpose, operations and algorithm; redirects and URL userinfo
are refused, responses are complete bounded JSON, duplicate key IDs are refused.
A key set with no eligible signing key cannot start the verifier.

Telemetry defaults, required fields, source precedence, privacy invariants and
negative controls are recorded in `docs/quality/telemetry.md`. All adjustable
telemetry defaults live in `bff/cmd/bff/telemetry.go`, never in the runtime adapter. A hosted run sets the
endpoint, the revision, the environment and the sample ratio from the input funnel, and keeps the others' composition
values.

## TypeScript duplication scan contract

`tools/checks/duplication_ts.sh` takes its token threshold from the registry (50).
Its clone minimum is five lines and its explicit per-file ceiling is 1 MiB; larger
selected files refuse with exit 11 rather than disappearing from the measurement.
It scans tracked TS, TSX and MJS in one owned snapshot, with jscpd's `mild` token
mode and an explicit empty config. Ambient config, ignore files and baselines
cannot change the scan. Test/spec files, generated `_pb.ts` and `testdata` are
excluded before copying. Untracked files are counted separately.

The measurement uses the report's analyzed-source count, not the number selected:
files below the token minimum cannot contribute a threshold-sized clone. An empty
selection is explicitly zero; a nonempty selection with zero analyzed sources
refuses. Nonzero tool status, unexpected stderr, absent/malformed report, impossible
counts or invalid clone locations refuse. The only accepted stderr text is the
pinned tool's exact announcement of the supplied config path.
