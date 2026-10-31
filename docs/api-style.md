# API Style Ledger

`buf lint` with the `STANDARD` category and `buf breaking` with `FILE` are the enforced rules. This
ledger records the Google AIP rules a reviewer applies by hand, which are a review signal. There are
no lint exceptions: exclusions by comment or configuration are not honored.

| Rule | Decision | `CreateNote` | `GetNote` |
| --- | --- | --- | --- |
| AIP-122 resource names | Applied: `notes/{note_id}`. The `note_id` is server-assigned. | response | request |
| AIP-131 standard get | Applied, except for the response wrapper. | — | `name` field |
| AIP-133 standard create | Applied, except for the response wrapper and the `parent` field: notes are scoped to the principal, so there is no parent. | `note` field | — |
| AIP-203 field behavior | Written in comments, because the template does not import `google/api`. `name` and `create_time` are output only. | yes | yes |
| AIP-193 errors | Applied through the mapping in specs/001 `contracts/notes-api.md`. | yes | yes |

## Justified exceptions

| Exception | Why |
| --- | --- |
| Response wrappers instead of returning `Note` | `STANDARD` lint requires `<Rpc>Response` names, and a wrapper can gain fields compatibly. |
| No `google.api` annotations | They would add a dependency with no consumer. The only transport is Connect. |

## Review checklist for a new RPC

1. It has a production browser caller and a browser test, for a success and a failure.
2. Each error it can return appears in the mapping.
3. Every field has a constraint, or a comment saying why it has none.
4. This ledger has a row for it.

The upstream contract analyzer (selamy-labs/relentless-ci-python#28) is not released. Until it is,
step 1 is enforced only through the end-to-end tests.
