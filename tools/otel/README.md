The local viewer is OpenTelemetry Collector Core v0.164.0 for Linux amd64, pinned
by SHA-256 in view.sh. It prints traces and metrics to the console and exposes
collector counters. All listeners bind to literal localhost addresses. It sends
nothing to a hosted backend. Stop it with Ctrl-C; its temporary executable is removed.

After the telemetry composition patch is adopted, use these five shell lines.
The server still requires its ordinary issuer and exact-origin settings; use real
verified RPCs from the existing client. Terminal A stays attached to the viewer.

```sh
bazel run //tools/otel:view # terminal A
export BFF_OTEL_ENDPOINT=http://127.0.0.1:4318 # terminal B
export BFF_RELEASE_REVISION="$(git rev-parse HEAD)"
bazel run //bff/cmd/bff
curl -fsS http://127.0.0.1:8888/metrics
```

`tools/otel/view.sh --validate` downloads the pinned archive and validates the
configuration without starting listeners. `--help` needs no download. The console
is the viewer; no Jaeger installation, UI service, new infrastructure, or credentials
are needed. The archive is fetched from GitHub over HTTPS and rejected on checksum
mismatch. The debug exporter is a local inspection tool, not a durable backend.

Look for `otelcol_receiver_accepted_spans`, `otelcol_exporter_sent_spans`,
`otelcol_receiver_refused_spans`, and corresponding metric-point counters.
The BFF exports `bff.telemetry.queue_dropped`, `bff.telemetry.export_failures`,
and `bff.telemetry.rejected` when nonzero. Queue drops count the BFF's admission
limit (queued plus in-flight spans), not a guessed SDK internal queue count.
Export failures count trace batches; failed batches are not durably replayed.
After an outage, new batches export once the receiver is reachable again.

The sampler is parent-based trace-ID ratio sampling with fresh public server roots.
With ratio 1, local tests retain all roots despite an incoming unsampled traceparent.
With lower ratios, failures in unsampled traces are not recovered by head sampling.
Operation counters and histograms continue independently of trace sampling.

Provenance: https://github.com/open-telemetry/opentelemetry-collector-releases/releases/tag/v0.164.0
Archive SHA-256: 5db7cf88c2e73a7fdce02b5fa967fcd5dc73bab00b871d947481918dc5093861
Apache-2.0 license: https://github.com/open-telemetry/opentelemetry-collector/blob/v0.164.0/LICENSE
The release tarball contains README.md and otelcol, not a LICENSE file. The license
was separately read at that tag; do not describe it as read from inside the tarball.

## Production-binary live trace smoke

Use the Collector executable from the checksum-verified 0.164.0 archive and the
production binary built from the candidate source:

```sh
node tools/otel/smoke.mjs /absolute/path/to/otelcol /absolute/path/to/bff FULL_SOURCE_SHA
```

The script creates its own loopback issuer, Collector configuration and ephemeral
ports, makes verified CreateNote/GetNote calls, drains the BFF, and checks real
Collector export counters and debug trace output. It refuses invalid arguments,
fails on missing evidence, checks that synthetic bearer/subject/body are absent,
and terminates its children and removes its own temporary directory on exit.
Port acquisition failures are fatal; this is a local witness, not infrastructure
provisioning. The supplied revision identifies the binary's source; a reviewer
running an uncommitted candidate must also record the patch SHA and label it as a
candidate, not claim an exact-commit release receipt.
