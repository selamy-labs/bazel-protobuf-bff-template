# Adding a language

A language joins through data and one or two host checks. This page is a worked design for Python. No Python
code ships here, because no consumer needs it yet, and none of the steps below has run. The public discussion
is selamy-labs/relentless-ci-python#30 and #31, and nothing there has been released.

What has run is the same path for Go, in a separate repository: it pins the module, binds rules, and names its
own paths. [quality/README.md](../../quality/README.md) describes that interface, and [m7](../evidence/m7.md) what
was observed.

## What a language needs

| Need | Where it goes |
| --- | --- |
| Tests that Bazel runs | The host's BUILD files, reached by binding `tests` with `targets` |
| Coverage | A profile in the host's contract, with a metric the tool really measures |
| A formatter and a linter | A check in the host's registry, at first |
| Suppressions banned | `suppression-ban`, which reads every text file already |
| A pinned tool | The host's module, passed to `quality_binary` in `tools` |

Nothing in the runner names a language. The contract maps a language to its file extensions, and every tracked
file with one must fall under a profile.

## Worked design: Python

**The toolchain.** The host pins Python and its dependencies in its own module, with locked hashes. The runner
is a Go binary, so a Python host still needs a Go SDK for `quality_binary`, as a build tool only.

**Tests.** Bazel `py_test` targets:

```text
rules { id: "tests" shard: "unit" targets: ["//..."] }
```

**Formatting and linting.** The host adds `FORMAT_PY` to `quality_binary`'s `tools`, and a check of its own:

```text
checks {
  id: "format-py"
  shard: "static"
  tool: "FORMAT_PY"
  selector { include: "\\.py$" }
  stages: [STAGE_PRE_COMMIT, STAGE_FULL]
  classification: CLASSIFICATION_GUARANTEE
  kind: KIND_NATIVE
  timeout_seconds: 300
  command: ["{FORMAT_PY}", "{list_nl}"]
  witness: "//:format_py_test"
}
```

`{list_nl}` is the path of a file that lists the inputs, one per line, so `FORMAT_PY` is a small host script that
reads it and calls the pinned formatter. A second repository
needing the same check is the reason to move it into the catalog, and not before.

**Suppressions.** S007 already bans the common Python forms, because a table entry costs nothing before a
consumer exists. A new tool brings its own forms, which join the table with a paired fixture.

**Coverage.** A profile in the host's contract:

```json
{ "name": "py", "language": "python", "metric": "lines", "include": "^src/", "floor": 90 }
```

The floor is the host's own, set from a measured baseline. A metric states what was measured, so a branch claim
needs a separate profile with real branch data. Whether Bazel's coverage output for Python reaches the merger in
a form it accepts is unverified. It is the first thing a real consumer has to prove, and a report that is empty
must fail rather than pass.

**Policy inputs.** The host adds its tool configuration to `escalate`, and lists its required tests with their legs.

**The base.** `floors-from-base` needs nothing new, because it reads the contract and does not know the language.

## What to prove with the first real consumer

1. A clean clone passes, with a test that really ran.
2. A formatting defect and a banned directive are each rejected, and the restored control passes.
3. Coverage reports a real number, and a lowered floor is refused by the base's evaluator.
4. Nothing from this template's own lists was copied.
