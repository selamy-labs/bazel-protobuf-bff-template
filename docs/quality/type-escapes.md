# Type escape hatches

Every place the first-party code steps outside its type system, with the reason it is there (spec 011 FR-012,
T003). The inventory was taken over the tracked non-test Go and TypeScript sources at `e82e3bb`; a new escape
hatch joins this table with its reason or does not land.

## TypeScript (`web/app`, `web/client`)

The compiler runs with `strict` and `noUncheckedIndexedAccess`. No `any`, no `as` cast, no `@ts-ignore`,
no `@ts-expect-error` and no `eslint-disable` directive exists in the first-party sources; the suppression ban
refuses the directives, and the lint rules refuse the rest. `exactOptionalPropertyTypes` is not enabled: it is
probed at T003's next step, since enabling it changes how absent and undefined properties are typed across the
generated client.

## Go

No `unsafe`. `reflect` is used once, for `reflect.DeepEqual` in the CI-parity validator, to compare an untyped
workflow fragment with the policy's exact value.

| Site                                                      | Escape                                                                                             | Reason                                                                                                                                                                                                                                                                           |
| --------------------------------------------------------- | -------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `quality/checks/ciparity/parity.go`, `cmd/main.go`        | `map[string]any`, `any`, checked type assertions on `string`, `float64`, `[]any`, `map[string]any` | a workflow file is untyped by nature; the validator reads it as the generic JSON shape and refuses every unexpected key, type or value by name. Typing it as a struct would silently drop unknown keys, which is the opposite of what the check is for                           |
| `deploy/gcp/inputs/inputs.go`                             | `map[string]any`, `any`, checked type assertion on `map[string]any`                                | the presence of each key and the file that gave it are unknowable from a struct decode; each document is walked against the struct's own leaf paths, every key outside them refused by name, and the merged document is then decoded into the struct with unknown fields refused |
| `quality/checks/complexity/complexity.go`                 | checked type assertions on `*ast.FuncDecl`, `*ast.GenDecl`, `*ast.ValueSpec`, `*ast.FuncLit`       | the Go syntax tree is an interface hierarchy; the checked form is the only way to select node kinds                                                                                                                                                                              |
| `quality/release/release.go`, `quality/runner/resolve.go` | `proto.Clone(x).(*T)`                                                                              | the protobuf runtime's clone returns the message interface; the assertion back to the concrete type is the documented use                                                                                                                                                        |
| `bff/identity/identity.go`                                | checked type assertion on a context value                                                          | the context carries the principal under a private key; the checked form returns `ok` false for a missing one, and nothing else reads the key                                                                                                                                     |
| `deploy/gcp/inputs/cmd/validate/main.go`                  | `map[string]any` for the printed summary                                                           | an output map for a JSON print, never read back                                                                                                                                                                                                                                  |
| `quality/checks/ciparity/parity.go`                       | `fail(format string, args ...any)`                                                                 | the variadic formatting signature of the standard library                                                                                                                                                                                                                        |

Nothing above weakens a boundary: every value that enters from outside (a workflow, an inputs file, a token, a
note) is validated by name before it is used, and the generated protocol types never stand in for runtime
validation (spec 003, spec 004).

## OIDC and G09 review update

`bff/adapters/identity/oidc/verifier.go` checks the JWT header's `kid` assertion before use; the JWT library's
key callback necessarily returns `any`. `policy.go` narrows the JWK library's key interface with an explicit
public-key type switch (RSA, approved ECDSA curves, Ed25519); every other key type is refused. These are
boundary checks, not unchecked reinterpretations. The browser's pending-session assertion was removed:
`takePending` now uses a full shape guard, including the token endpoint policy, before a token request.
Test-only fixture assertions remain outside the first-party production inventory. G09 adds no `unsafe` or
unchecked production type assertion.

## Python generator output

`tools/pins/gen_multitool_lock.py` uses `dict[str, Any]` for `out`: the JSON
serialization map contains the schema string alongside tool records with lists
of binary descriptors. Values are constructed from the source-controlled pin
table and passed directly to `json.dump`; this map does not decode external data
or authorize input. The `Any` is an output-shape escape, not validation or proof
of the lock schema. Old/new generator output is byte-identical to the committed
lock. No ignore directive or cast was added; the other two first-party Python
scripts have concrete annotations. A future typed output model can replace this
local escape if consumers begin reading the map.
