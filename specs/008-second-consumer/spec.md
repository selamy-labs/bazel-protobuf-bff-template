# Spec 008: Second consumer and upgrade (M7)

Parent: spec 001 US5, FR-027, FR-029, SC-008; tasks T071–T075; upstream RFC #30.
No remote, publication, settings or paid work: the consumer stays local, as a separate Git repository over `file://`.

## User stories

**US1 (P1): A separate project adopts the checks by pin.** A neutral project in its own repository consumes `bff_template_quality` at version N, and its
verifier runs N's checks with N's tools, with its own policy and floors.

**US2 (P1): An upgrade is one pin and a reviewable diff.** The consumer moves N to N+1 by changing one chosen pin. Lock and generated effects of that change are regenerated mechanically and shown. A clean consumer needs no manual
edit. Release notes list each added, changed or removed rule, and flag weakening.

**US3 (P2): The base contract holds for a consumer.** The consumer's first reviewed commit anchors its own floors, and an upgrade is judged by the older pin.

## Functional requirements

- **FR-001**: The consumer MUST be a separate repository, in a different neutral domain, with no path into this one.
- **FR-002**: It MUST consume one release of the module by a full commit SHA, through both adapters, against a local `file://` remote: Bazel `git_override` and prek `repo`/`rev`. One value in one place drives both. A copied local wrapper is not remote hook adoption. The pin check reads only the exact form the sync writes, and refuses anything else by line, so a legal spelling it cannot read is never reported as one pin: a second repository, a hook that replaces the remote's entry, or a `git_override` that patches the pinned commit.
- **FR-002a**: The remote-hook witness exercises prek's existing remote-hook interface. It MUST NOT be described as adopting upstream #25's shared stage contract, which stays an unreleased, separate gap.
- **FR-003**: The module MUST NOT assume a host repository's paths. Each host-owned input MUST be named by the host: its policy registry, required tests, floors contract, wrappers. That interface is a small data contract. There is no general generator or expression platform. The one small deterministic step that syncs the two adapter pins is required and allowed.
- **FR-004**: The policy-base rules of spec 007 MUST hold for a consumer, including same-SHA and no-evaluator refusals, using the host's declared paths.
- **FR-005**: The consumer's own `required_tests`, floors and leg assignment MUST be its own, and none is copied from the template.
- **FR-006**: Release notes MUST be generated from a diff of the single rule catalog, and that catalog MUST expose each real rule definition, so the chosen rule change appears. A change hidden behind an unchanged registry entry, such as a new S-code inside `suppression-ban`, does not count until the catalog shows it. Added, changed and removed rules are listed, with weakening flagged. Everything the notes read stays inside the selected tree, so a definition that leaves it by `..`, an absolute path or a symbolic link is refused and never quoted. This compares two tagged registries for notes, and is not a policy gate. Upstream #26 owns detection and authorization.
- **FR-007**: The upgrade MUST change one independently chosen pin, and each mechanical effect, such as a lock, is regenerated and shown. A clean consumer passes at N+1 with no manual edit, and the rule changes needed no second copy. A deliberate defect is caught at N+1, and the restored control passes. If evidence shows a migration is unavoidable, it is called out for review, and the criterion is not weakened in advance.
- **FR-008**: A consumer pinned to a full SHA MUST be unaffected when a tag elsewhere moves. That is a positive integrity witness. Release tooling MUST refuse to change an already recorded tag-to-commit mapping, as a release-procedure check. M7 builds no release issuer, authorization catalog, allowlist or authorization gate.
- **FR-009**: `docs/extending/language-profile.md` MUST give a worked Python design, with no Python code without a consumer.
- **FR-010**: N is a real local tag, made after the consumer interface works. M6's source still has the hosting assumptions, so it is not N. Stable 1.0.0 is declared on the exact candidate that was tested and independently reviewed. Nothing changes in the implementation after that review in order to mark a ledger complete.

## Success criteria

- **SC-001**: A separate clone of the consumer verifies at N with no path into this repository.
- **SC-002**: The upgrade to N+1 is one chosen pin, shown as a real Git diff, with mechanical effects separated from it, and no manual edit on a clean consumer.
- **SC-003**: Every rule change in the upgrade appears in the generated notes.

## Limits

- Local `file://` evidence only. Re-proving against a hosted remote is M8 or later, behind an operator gate.
- The consumer's bare `tools/verify.sh` exits 11, like the template's. That is the agreed contract, not a defect.
