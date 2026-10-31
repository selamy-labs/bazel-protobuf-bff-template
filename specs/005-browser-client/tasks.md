# Tasks: Browser Client and End-to-End Tests (M4)

**Input**: [spec.md](spec.md), [plan.md](plan.md), [research.md](research.md),
[acceptance-matrix.md](acceptance-matrix.md)

This ledger, the plan and the matrix were committed before any `web/` code. Each commit names its
tasks. Boxes are ticked only when the named witness passes in the verifier.

| Task | Needs | Work | Spec | Parent | Witness |
| --- | --- | --- | --- | --- | --- |
| [x] T001 | — | Pin Chromium and its library closure | FR-006 | T030, FR-025 | `//web/e2e:browser_smoke_test` |
| [x] T002 | — | Pin React, Vite, connect-web, Playwright | FR-006 | T039 | `lock_fixture.sh` |
| [x] T003 | T002 | `web/client`: transport, credentials, error mapping, tests | FR-001, FR-003, FR-004 | T031, T039, FR-001 | `//web/client:errors_test` |
| [x] T004 | T003 | `web/app`: form, lookup, builds | FR-001, FR-008 | T039, FR-001 | `//web/e2e:notes_e2e_test`, `//web/app:bundle_test` |
| [x] T005 | T004 | `web/serve` static server | FR-005 | T039 | `//web/e2e:notes_e2e_test` |
| [x] T006 | T001, T005 | Specs for every scenario | FR-002, FR-004, FR-005 | T032, FR-008 | `//web/e2e:notes_e2e_test` |
| [x] T007 | T006 | Broken-mapping witness | FR-007 | T041, SC-002 | `//web/e2e:defect_e2e_test` |
| [x] T008 | T005 | `//:dev` runner | FR-009 | T040 | `//tools/dev:dev_test` |
| [x] T009 | T006–T008 | Inventories, docs, provenance, parent ledger | FR-010 | T025, T042 | `test-inventory`, `fresh_clone.sh` |
| [x] T010 | T006 | Review regression: the omitted-cookie scenario gets a same-origin control and requires a successful POST | FR-005 | T032, FR-008 | `the client omits a cookie the browser would send` in `notes_e2e_test` |
| [x] T011 | T009 | Quote tool paths, because Bazel resolves some inside the checkout | FR-010 | T042, FR-025 | `//:verify_body_test`, `fresh_clone.sh` |
| [x] T012 | T001 | Enforce the Linux glibc floor in the preflight; a below-floor or unknown libc fails early | FR-011 | T030, FR-025 | `//:preflight_test` |
| [x] T013 | T001 | Stop detecting a host C compiler; Go builds with cgo off | FR-012 | FR-025 | `compiler-fixture` in `toolchain-fixtures`, `bazelrc-policy` |
| [x] T014 | T001 | Pin the browser's fonts and font configuration in its tree; the smoke paints text | FR-006, FR-013 | T030, FR-025 | `//web/e2e:browser_smoke_test` |

## Execution record

The ledger came first, as planned. Tests were written with the code, not before it. Two later
findings changed the plan:

- Bundling from `index.html` fails under Bazel's sandbox links, so the entry is a script and
  `index.html` is a static file. Plain compiled JavaScript needs no Vite plugin, so
  `@vitejs/plugin-react` was dropped. Chromium's library set gained snapshot mirrors, because the main
  Debian pool drops superseded versions.
- `bundle_test`'s first version read nothing: `grep -r` skips symlinked files. Its positive control
  caught that, and it now uses `grep -R`.

- The fresh-clone witness failed at `25d67de`: `quality/proto/buf.yaml` resolves inside the checkout,
  and a path with parentheses broke the sourced tool list. A test in the verifier now covers this.
- Independent review found that the original no-cookie scenario never seeded a cookie, so it could
  not fail. T010 replaced it. That test earned no credit before then.
- An independent run at `c334119` on a host with glibc 2.35 failed all three browser targets with
  `GLIBC_2.36' not found`, loading the pinned `libexpat`. The other 28 targets passed. The host was
  unsupported, so this is neither a product failure nor a sandbox result. The verifier had let an
  unsupported host run for minutes before failing. T012 makes it stop first. Reading the version
  needs of all 67 pinned ELF files confirmed the floor: `libexpat` and `libstdc++` need 2.36, and
  nothing needs newer. The pins and isolation did not change.
- A qualified run at `902aaf6`, on Debian 12 with glibc 2.36 and no C compiler, stopped in 24 s with
  `Cannot find gcc or CC`, before any test or receipt. This was a real defect, against the documented
  prerequisites. rules_go asks for an optional C++ toolchain on every Go target, which runs Bazel's
  host detection, and `pure` does not stop that. Hosts with `gcc` had hidden it: there the Go binaries
  were built with `CGO_ENABLED=1` and linked to the host's libc, so the result depended on the host.
  T013 turns detection off. They are now static with `CGO_ENABLED=0`. With compilers hidden from
  repository rules, actions and tests, all 32 required tests passed locally.
- A qualified run at `8f602bf` on the same compiler-free image passed 28 of 32 targets. Two failures
  came from that image having no passwd record for its user, which is a test-environment matter. The
  other two were real: `notes_e2e_test` and `defect_e2e_test` crashed in Skia on pages with form
  controls, because the host had no font data. The pinned browser carries its own fontconfig, so no
  shared-library closure could show this. The old smoke only ran JavaScript, and authoring hosts had
  `/etc/fonts`, so nothing caught it. T014 gives the browser a fontconfig sysroot in its own tree.
  `FONTCONFIG_FILE` was not enough: a trace showed the browser still reading host `/etc/fonts`. With
  the sysroot, the same trace touches no host font path.

Credit for the browser scenarios rests on `defect_e2e_test`, which is re-run on every verification.
