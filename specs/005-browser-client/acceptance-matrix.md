# Acceptance / Evidence Matrix: M4

Classes: **G** deterministic guarantee, **S** review signal. Layers: **off** offline test, **loc**
local verifier, **ci** hosted CI (not run), **hum** human.

| Req | Witness | Defect → expected | Class | Layer |
| --- | --- | --- | --- | --- |
| FR-001 | `//api:compile_gate_test`, `notes_e2e_test` | a wrong caller does not compile | G | off |
| FR-002 | `notes_e2e_test` | each RPC: a success and typed failures in a browser | G | off |
| FR-003 | `errors_test` | each code → one distinct outcome; an unknown code → a general failure | G | off |
| FR-004 | `errors_test`, `notes_e2e_test` | a violation lands on its field; an unknown field stays visible | G | off |
| FR-005 | `notes_e2e_test` | pinned browser, real binary, two origins; a cookie the same-origin control sees is absent from the successful generated-client POST | G | off |
| FR-006 | `browser_smoke_test` | the browser starts with only pinned libraries, and renders with only pinned fonts | G | off |
| FR-007 | `defect_e2e_test` | broken mapping → exactly the not-found scenarios fail | G | off |
| FR-008 | `bundle_test` | test identity in the production bundle → fails | G | off |
| FR-009 | `dev_test` | the runner serves the page and the server, and stops both | G | off |
| FR-010 | `required_tests.txt`, `fresh_clone.sh` | a missing test fails the inventory | G | loc |
| FR-011 | `preflight_test` | glibc below 2.36, or an unknown libc → fails before Bazel starts, also with weak isolation; 2.36 and newer pass; macOS skips it | G | off |
| FR-012 | `compiler-fixture` in `toolchain-fixtures`, `bazelrc-policy` | detection on with compilers hidden → `Cannot find gcc or CC` (control); detection off → every target analyzes; a C toolchain consumer fails on a host that has a compiler | G | loc |
| FR-013 | `browser_smoke_test` | pinned sysroot → text has width, ink and a visible screenshot, in `DejaVu Sans`; the same render with an empty sysroot → `Cannot load default config file` and the `SkFontMgr_FontConfigInterface` `FATAL` | G | off |

| SC | Witness | Status |
| --- | --- | --- |
| SC-001 | `fresh_clone.sh` | See [evidence](../../docs/evidence/m2-m4.md) |
| SC-002 | `notes_e2e_test` | Local, passing |
| SC-003 | `defect_e2e_test` | Local, passing |
| SC-004 | `bundle_test` | Local, passing |

Limits: the pinned browser exists for Linux x86_64 only, and needs glibc 2.36 or newer. The preflight enforces that floor only for the verifier, not for a
direct `bazel test`. Chromium runs
with `--no-sandbox`, in tests only. The production web build has no credential source, so it cannot
complete a call until an adopter adds one.

Hand-run on 2026-10-31: making the client send credentials fails the omitted-cookie scenario. It also
fails most other scenarios, because the browser's CORS check blocks every credentialed call. So this
shows the scenario can fail, but it does not isolate it. The scenario's own same-origin control is
the witness that a cookie would be seen.

Hand-run on 2026-10-31 against `preflight_test`: each mutant failed on its intended case. They were
the floor at 2.35 and at 2.37, an off-by-one comparison, an ignored major version, the check removed,
unknown libc passing, the check run on macOS, no early exit, weak isolation bypassing the check, and
a changed floor in `README.md`.

Hand-run on 2026-11-01 against `compiler-fixture`: the setting removed, a C consumer added to the
checkout, and compilers left on the fixture's `PATH` each failed on their intended step. A separate
cgo probe failed at link, so a cgo target can't silently lose its C sources.

Hand-run on 2026-11-01 against `browser_smoke_test`: launching without the sysroot, a config with no
font directory, fonts left out of the tree, the config left out of the tree, a page with no form
controls, and text left unpainted each failed on the named reason. The empty sysroot reproduces the
qualified host's two lines on a host that has fonts. Without form controls, the same empty sysroot
paints a blank page and exits 0, so the smoke's page must keep them. A trace of a pinned render
touched no host font path. The family-name check can't separate pinned from host DejaVu by itself;
the sysroot check and that trace do.
