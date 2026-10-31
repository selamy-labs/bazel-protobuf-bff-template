# Quickstart (target adopter flow)

This is the flow M1–M5 must make true; it is validated by T014 (fresh clone), T042 (browser),
T059 (hooks). None of these commands work at M0.

Prerequisites: Git, bash, curl and coreutils (GNU `timeout`, `sha256sum`, `tar`). Linux needs unprivileged user namespaces.

```sh
git clone <your copy> my-app && cd my-app
tools/verify.sh            # full canonical verifier (identical to CI)
bazel run //:dev               # BFF on 127.0.0.1:8080 (dev mode, test identity) + web on :5173
bazel test //web/e2e:all       # browser success/failure scenarios against the real BFF
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
