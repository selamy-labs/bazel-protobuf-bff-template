# Quickstart (target adopter flow)

The verifier, `//tools/dev` and browser tests work from M4 on. Hook and `//quality:run` commands
arrive in M5. T014 (fresh clone), T042 (browser) and T059 (hooks) witness this flow. `bazel` here means
`tools/bootstrap/bazel.sh`, the pinned Bazel.

Prerequisites: Git, bash, curl and coreutils (GNU `timeout`, `sha256sum`, `tar`). Linux needs unprivileged user namespaces.

```sh
git clone <your copy> my-app && cd my-app
tools/verify.sh            # full canonical verifier (identical to CI)
bazel run //tools/dev          # BFF on 127.0.0.1:8080 (dev mode, test identity) + web on :5173
bazel test //web/e2e:notes_e2e_test   # browser scenarios against the real BFF
bazel run //tools/hooks:install  # prek pre-commit + pre-push hooks
```

Try it:

1. Open http://127.0.0.1:5173, create a note with a title — it appears with its server name.
2. Submit an empty title — the field shows the validation message from the BFF.
3. Look up `notes/does-not-exist` — "Not found".
4. Stop the BFF and retry — "Service unavailable, retry".

Change the contract:

```sh
$EDITOR api/notes/v1/notes.proto     # add an optional field → passes
bazel run //quality:run -- --stage=pre-push
# change a field number → proto-breaking fails naming the field
```

Upgrade shared checks (M7): change the `bff_template_quality` commit in `MODULE.bazel`, read the
release notes' rule-change table, run `tools/verify.sh`.
