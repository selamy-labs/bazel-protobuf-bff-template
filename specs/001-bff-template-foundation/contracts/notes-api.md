# Contract: Example Notes API (`template.notes.v1`)

Design-level contract; the `.proto` is written in M2 (task T021) and is the source of truth from
then on. Neutral by construction: no product domain, no external identifiers.

## Service `NoteService`

| RPC | Request | Response | Browser caller | E2E scenarios |
| --- | --- | --- | --- | --- |
| `CreateNote` | `note.title`, `note.body` | `Note` | "New note" form | success; invalid title (empty, >120); body too long; unauthenticated; unavailable |
| `GetNote` | `name` | `Note` | "Open note" lookup | success; not found; other principal's note → not found; deadline exceeded |

Connect protocol over HTTP/1.1 and HTTP/2; JSON and binary codecs both supported by the server;
the browser uses the Connect protocol with JSON for debuggability (binary optional).

## Validation (protovalidate annotations, enforced by BFF)

- `title`: required, trimmed length 1–120.
- `body`: length ≤ 4,000.
- `name`: matches `^notes/[a-z0-9-]{1,64}$`.

## Error mapping (BFF → Connect code)

| Condition | Code | Detail |
| --- | --- | --- |
| validation failure | `invalid_argument` | `buf.validate.Violations` with field paths; browser maps to field messages |
| note absent or owned by another principal | `not_found` | none (no existence leak) |
| missing/invalid identity | `unauthenticated` | none |
| test identity in production mode | `unauthenticated` | none; startup also refuses this configuration |
| storage port unavailable | `unavailable` | retryable hint |
| request deadline exceeded | `deadline_exceeded` | none |
| anything else | `internal` | generic message + correlation id; details only in server log |

## Deadlines

Server default 2 s, maximum 10 s per RPC; the client-supplied `Connect-Timeout-Ms` is honored up to
the maximum; the context propagates into the storage port.

## Compatibility rules

buf `FILE` breaking category against the immutable baseline SHA; field numbers are never reused
(reserved on removal); a wire-incompatible change requires `template.notes.v2`.
