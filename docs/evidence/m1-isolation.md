# M1 Isolation Evidence

This record is the witness for specs/002-hermetic-skeleton US3 / SC-003. It was produced on the
authoring host on 2026-10-29.

**Host**: Ubuntu 24.04.5, kernel 6.8, 20 CPUs, with
`kernel.apparmor_restrict_unprivileged_userns=1`.

## Default run: fails closed

```text
$ tools/verify.sh
preflight isolation: Bazel's linux-sandbox cannot create user/mount namespaces (kernel.apparmor_restrict_unprivileged_userns=1)
  fix: [admin] add an AppArmor profile granting 'userns,' to $HOME/.cache/bazel/_bazel_*/install/*/linux-sandbox (narrowest; ...)
  fix: [admin] sysctl -w kernel.apparmor_restrict_unprivileged_userns=0 (host-wide; weaker)
  fix: run in CI or a container where unprivileged user namespaces are allowed
  fix: [no admin] tools/verify.sh --weak-isolation (results are labeled 'isolation: weakened')
verify: preflight failed; nothing was verified      (exit 1)
```

Running Bazel directly on this host also fails rather than falling back, because `.bazelrc` names
`linux-sandbox` explicitly.

**Root cause.** The sandbox helper reports `linux-sandbox-pid1.cc:205: "mount": Permission
denied`, and `unshare --user --map-root-user` cannot write `uid_map`. The host was not modified:
changing host security settings is outside this work's authority.

## Explicit opt-in: passes, labeled

```text
$ tools/verify.sh --weak-isolation
PASS inventory … PASS speckit-provenance          (13 checks)
verify: PASSED at <revision> (isolation: weakened)
```

The receipt records `"isolation": "weakened"`.

## Not yet witnessed

A run at `isolation: full` has not been witnessed on any host. See
[m1-remediation.md](m1-remediation.md) for the exact-revision evidence and its limits.
