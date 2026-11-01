# Plan: M7

Starts from accepted M6 `e16718d`. One writer. Spec Kit files land before code.

1. Spec, research, plan, tasks, matrix. Recheck upstream.
2. Probe: a scratch repository consuming the module by `git_override`, to see each coupling in R1 fail for real.
3. Decouple the module: host-named paths, rule binding, evaluator test by build. Keep this template green throughout, with named controls.
4. Remote hook definitions at the repository root, usable through prek `repo`/`rev`.
5. Tag N in a local bare remote, after the interface works. Release procedure, notes from a registry diff, and the recorded tag mapping check.
6. The smallest consumer, in its own repository, with one canonical pin driving both adapters. Its first commit is its reviewed anchor. Record its exit 11 control.
7. Independent review of the consumer's anchor, then its verifier at N.
8. One real compatible rule improvement, tag N+1, upgrade by the one pin. Show mechanical effects, the clean pass, the deliberate defect caught, and the restored control.
9. Tag integrity: move a tag in the `file://` remote and show the SHA-pinned consumer unaffected.
10. Language profile document.
11. Freeze, gates, independent acceptance. The 1.0.0 declaration and tag attach to that exact reviewed candidate, with no implementation change after it.

`file://` remotes and local Git fixtures are in scope. Closed throughout: this repository's origin, publication, live settings, access, apply, merge, paid runs. No general generator platform, though the small deterministic pin sync is required. No new comparator, and no release issuer or authorization catalog: the rule catalog diff informs readers and gates nothing.

| Risk | Handling |
| --- | --- |
| Decoupling breaks the anchor's `from_base` for this repository | Each change keeps this template's own gate, and a base before the decoupling must still build |
| Rule binding grows into a template language | Keep it a list of ids and paths; stop if it needs conditionals |
| A consumer's evaluator pin predates a schema field | The base's registry parser already ignores unknown fields |
| Contrived N+1 | Use a real, compatible rule improvement that the catalog diff shows |
| Rule change invisible in notes | Rule definitions live in the single diffed catalog, and a notes fixture covers the case |
| Two adapters drift apart | One canonical pin, and the second value written mechanically |
| Root hooks make the tree depend on the module's layout | Keep them thin, calling the pinned runner |
| A second copy of a rule appears in the consumer | Rule binding names ids only; review rejects a copied rule |
