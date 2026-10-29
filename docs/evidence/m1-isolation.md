# M1 Isolation Evidence

This record is the witness for specs/002-hermetic-skeleton US3 / SC-003. It was produced on the
authoring host on 2026-10-29.

**Host**: Ubuntu 24.04.5, kernel 6.8, 20 CPUs, with
`kernel.apparmor_restrict_unprivileged_userns=1`.

## Default run: fails closed

This is the default command:

```text
$ bazel run //:verify
ERROR: 'linux-sandbox' was requested for explicit default strategies but no strategy with that
identifier was registered. Valid values are: [dynamic_worker, processwrapper-sandbox, standalone, ...]
ERROR: Build did NOT complete successfully          (exit 2)
```

Running the script directly reaches the preflight, which fails with remediations:

```text
$ tools/verify.sh
preflight isolation: Bazel's linux-sandbox cannot create user/mount namespaces (kernel.apparmor_restrict_unprivileged_userns=1)
  fix: [admin] add an AppArmor profile granting 'userns,' to $HOME/.cache/bazel/_bazel_*/install/*/linux-sandbox (narrowest; ...)
  fix: [admin] sysctl -w kernel.apparmor_restrict_unprivileged_userns=0 (host-wide; weaker)
  fix: run in CI or a container where unprivileged user namespaces are allowed
  fix: [no admin] bazel run --config=weak-isolation //:verify -- --weak-isolation (results are labeled 'isolation: weakened')
verify: preflight failed; nothing was verified      (exit 1)
```

**Root cause.** The sandbox helper reports `linux-sandbox-pid1.cc:205: "mount": Permission
denied`, and `unshare --user --map-root-user` cannot write `uid_map`. The host was not modified:
changing host security settings is outside this work's authority.

## Explicit opt-in: passes, labeled

```text
$ bazel run --config=weak-isolation //:verify -- --weak-isolation
PASS bazel-test … PASS speckit-provenance          (11 checks)
verify: PASSED at <revision> (isolation: weakened)
```

The receipt records `"isolation": "weakened"`.

## Not yet witnessed

A run at `isolation: full` has not been witnessed yet. The CI workflow lifts the restriction on
its disposable runner and lets the preflight confirm full isolation. That remains unverified until
a hosted run exists (spec A2, M8).

## M1 exit evidence (T022)

| Claim | Revision | Result | Layer |
| --- | --- | --- | --- |
| Fresh clone (empty HOME, path with a space, no credentials) passes `//:verify`, all 11 checks | `e953809929b19bdcc6d90353cbb6efc116c011d7` | PASSED, `isolation: weakened` | local |
| Budgets (cold n=3, warm n=5, no-network n=1) | `e98c55e4e6902ca2029bdaceef940c807e46c416` | `docs/budgets/linux-x86_64-20c.json` | local |
| Full isolation in hosted CI | — | not yet run (no remote; M8) | hosted-ci |
