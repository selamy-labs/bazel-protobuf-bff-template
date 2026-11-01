# Acceptance matrix: BFF image

Layers: off, loc, hum, run (a real runtime on an authorized host). Hosted and deployed are never claimed. Each negative needs its positive first.

| Req | Witness | Valid | Defect → expected | Class | Layer |
| --- | --- | --- | --- | --- | --- |
| FR-001 | Lock, review | one builder, pinned | a Dockerfile or a second builder → review rejects | S | hum |
| FR-002 | `image_test`; Bazel analysis | exactly `/bff` | a second file → `image_test` fails; `bff-dev` → refused at analysis, because it is `testonly` | G | off |
| FR-003 | `image_test` | linux/amd64, numeric non-root, absolute entrypoint | uid 0, a user name, a wrong entrypoint → fails | G | off |
| FR-004 | `image_test` | packaged ELF has no `PT_INTERP` and no `DT_NEEDED` | either present → fails | G | off |
| FR-005 | `image_test`; real runtime run, bound to the same content digest | exit 1, exact refusal, nothing served | a healthy start, or a changed message → fails | G | off, run |
| FR-006 | Repeatability script | one digest from two paths | a timestamp or path in a layer → digests differ | G | loc |
| FR-007 | Verifier, `image_test` | required, with no runtime; descriptor closure checked from content | the test unlisted → `test-inventory` fails; a blob that does not match its digest or size → fails | G | off |
| FR-008 | Review here; the inventory check at spec 010 T004 | image contents listed | an unlisted runtime dependency → review rejects here, and the check fails at 010 | S, then G | hum, off |
| FR-009, FR-010 | Review | one builder, limit stated | a second builder, or a health claim → review rejects | S | hum |
| FR-011 | Review | nothing pushed | a push step or registry → review rejects | S | hum |
