# Releasing bff_template_quality

A release is one commit of this repository. A consumer pins that commit's full SHA in `MODULE.bazel`, and
its prek configuration follows it. The tag is a name for readers, and a pin never reads it.

1. In one commit, set the same version in `rules.textproto` and `MODULE.bazel`, and write the notes into
   `CHANGES.md`. Copy each tree with `git archive <ref> quality | tar -x -C <dir>`, then run:

   ```bash
   bazel run @bff_template_quality//release:release_bin -- notes <previous tree>/quality <new tree>/quality
   ```

2. Pass the whole gate on that commit, with its policy base.
3. Tag it `quality-v<version>`, and record the tag:

   ```bash
   release record quality/RELEASES.txt quality-v<version> <full SHA>
   release check quality/RELEASES.txt .
   ```

   `record` refuses to change a mapping it holds. `check` reports a recorded tag that moved or went missing.
   Commit the record after the tag. A file cannot name the commit that contains it.

## What the notes show

The notes compare the catalog entry of each rule, and the files the rule names in `definition`, so a change
inside a rule shows when its entry does not change. A file with lines removed or replaced is flagged for
review, because a tool cannot know that a replacement is at least as strict. An added line is not flagged.
Everything the notes read stays inside the selected tree: a definition or catalog that leaves it by `..`, an
absolute path or a symbolic link is refused, and nothing from outside is quoted. A link that stays inside is an
ordinary path. The notes inform a reader and gate nothing. Detecting and authorizing a weakening is upstream #26's.

## Compatibility

1.0.0 is proposed on the commit whose module and catalog carry that version. It is declared only by the tag
`quality-v1.0.0`, on the exact commit that a real consumer has pinned and an independent review has accepted
(spec 008 FR-010). Nothing in the implementation changes to receive the tag.

From that tag, these change only with a major version:

- rule ids and suppression codes, which are never reused
- the `rules { id shard args context targets }` binding, and the `Registry` fields a host sets
- `quality_binary`'s arguments, and the `{contract}` and `{bazel}` expansions
- the exit codes, and the hook ids in `.pre-commit-hooks.yaml`
- the `prek.toml` and `git_override` form that `quality-pin` reads

A minor version may add a rule, and a consumer may then have to fix what the rule finds. The notes show it.
