# Local demo

The whole stack, locally, with one command (spec 012 US5). It is the real page, the real server and the real
generated client; the only thing it is not is a deployment.

```sh
tools/bootstrap/bazel.sh run //tools/dev
```

Then open the URL it prints, sign in as any subject (the profile's seed is `ana`), create a note, open it by
name, reload the page and open it again. On a host without namespace sandboxing add `--config=weak-isolation`
after `run`. The first run fetches the pinned toolchains over the network; nothing here claims an offline cold
start.

## The profile

`tools/dev/profile.json` is the one place the demo's values are declared, and the launcher validates it once
before anything starts:

| Field | Meaning | Override |
| --- | --- | --- |
| `bff_port`, `web_port` | loopback ports for the server and the page; distinct, 1 to 65535 | `BFF_PORT`, `WEB_PORT` |
| `store` | `local`: notes live in `data_dir` until a reset; `memory`: notes live until the demo stops | `BFF_STORE` |
| `data_dir` | the local store's directory, relative to the repository root or absolute | `BFF_DATA_DIR` |
| `identity` | `test-identity`, the development identity on loopback: any caller may claim any subject | none |
| `seed_subject` | the synthetic subject the demo and its recording use | none |

The launcher prints each effective value with its source (`bff_port="8080" # from profile.json`). A missing,
unknown or invalid value, a port already in use, or an unknown argument is refused by name, and nothing starts.

## Persistence, restart, reset

With `store: local`, a note survives a restart of the demo: the server writes one file per note under
`data_dir`, atomically, with owners and ids encoded so that nothing can leave that directory. `--reset` removes
that directory and nothing else before starting:

```sh
tools/bootstrap/bazel.sh run //tools/dev -- --reset
```

With `store: memory`, notes live until the demo stops. Neither is a durability claim beyond the host's file
system. Both adapters pass the same contract suite as any store an adopter brings (`bff/ports/porttest`).

## Stop

Ctrl-C stops the launcher, which stops the server and the page it started and nothing else. If one of them
exits on its own, the launcher reports its status, stops the other, and exits nonzero.

## The recording

`docs/media/local-demo.gif` is the README's animation: `tools/dev/record.sh` starts the real stack through the one
command with a scratch data directory, drives the real page in the pinned browser and captures seven frames
(open, sign in, type, create, reload, type the name, open), and assembles them with the hash-locked encoder under
the pinned uv Python. `docs/media/local-demo.json` names the revision it was recorded at, the frames, the size and
the digest; `//tools/dev/record:media_test` checks that the GIF and the record agree. Regenerate it after a
material change to the page; the raw frames and logs stay outside the repository. Linux x86_64 only, because the
pinned browser is.

## Limitations

- The development identity is for loopback only; the production server refuses to start without a real
  verifier, and the test identity is absent from the production bundle. Nothing here weakens that.
- The supported host is the Linux x86_64 profile the verifier qualifies. The launcher needs only bash 3.2 and
  POSIX tools, and an independent run on macOS ARM built the target and reached the launcher at `34e1bbc`; a
  native macOS result is recorded only when that independent run records it.
- The demo's own witness is `//tools/dev:dev_test`: the page reaches the server, the profile's refusals, a busy
  port, persistence across a restart, reset, the first child's exit and SIGTERM. The browser journeys of spec 005
  cover the page against the same server.
