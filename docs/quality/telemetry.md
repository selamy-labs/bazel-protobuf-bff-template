# BFF telemetry, contract v1

The production composition requires `BFF_OTEL_ENDPOINT` (an HTTPS origin or literal
loopback HTTP origin) and `BFF_RELEASE_REVISION` (the source commit's full SHA).
`bff/cmd/bff/telemetry.go` resolves the remaining values once and prints effective
configuration with its source. A missing issuer still produces the existing exact
startup refusal before telemetry is configured.

Defaults at composition: environment `prod`, sample ratio 1, queue 2048 spans,
batch 512 spans, batch delay 1s, export timeout 2s, metric interval 10s, shutdown
bound 5s, histogram seconds `.001,.005,.01,.05,.1,.5,1,5`. Each has a corresponding
`BFF_OTEL_*` override in `resolveTelemetry`; a present sample ratio zero is valid.
The typed adapter refuses absent, invalid or out-of-range fields, without fallback.

Privacy invariants: public trace context becomes a link on a new trace, never a
trusted parent; baggage and tracestate are discarded. Span names and attribute
keys/values are closed sets. Owners, note text, bearer tokens and raw error strings
are not exported. Span and log attribute budgets are fixed (32 attributes, bounded
values); exported span events are suppressed. A metric SDK view filters the final
attributes, including attributes a Connect labeler appends, and exemplars are off.
The slog bridge accepts only its bounded contract and injects real trace/span IDs.

OTLP HTTP/protobuf exporters use explicit endpoints, empty headers, no proxy,
no redirects, no compression and no retries. Standard OTEL environment variables
cannot replace these choices. The bounded span queue never blocks an RPC. Rejected
items, queue drops and export failures have separate counters. A failed batch is
not promised later replay. Store spans, duration histograms and operation error
counters wrap the configured store, including the managed object adapter.

The server drains before telemetry shuts down with a fresh bounded context. The
in-process protobuf receiver test checks successful Create/Get, unavailable store,
trace links, redaction, real metrics, receiver outage/recovery, queue pressure and
shutdown flush. `//bff/telemetry:e2e_receiver_test` is required in the inventory.
The browser production witness explicitly uses a loopback unavailable collector,
sampling zero, synthetic revision and dev environment; its purpose is browser
identity behavior, not a live telemetry assertion.

## The hosted service

`deploy/gcp/service` passes `BFF_OTEL_ENDPOINT`, `BFF_RELEASE_REVISION`, `BFF_OTEL_ENVIRONMENT` and
`BFF_OTEL_SAMPLE_RATIO` from the input funnel. The revision is the served digest's, so a rollback reports the older
revision. `//deploy/gcp/inputs:inputs_test` starts the real production binary with the module's environment and
reads back what it resolved, and shows it refusing without the endpoint or the revision. This is offline: no hosted
export has been observed.

A hosted run must still settle three things. None is decided here, and each is a gate input of spec 012 T017:

- **Where the export goes.** The exporter sends no credentials, so a backend that requires them needs a collector
  in front of it. A collector beside the service would be loopback http, which the binary accepts and the funnel
  refuses until the module declares one, so that no endpoint exports to nothing.
- **CPU between requests.** The service scales to zero under request-based billing, where the platform allocates
  CPU only during requests. Batches and metric intervals can therefore wait for the next request, and metrics
  from an idle instance may arrive late or not at all. Instance-based billing changes this and changes the cost.
- **The shutdown window.** The platform allows 10 seconds between SIGTERM and SIGKILL. The server's drain bound and
  the telemetry shutdown bound are 5 seconds each and run in turn, so the worst case uses the whole window, and
  a slow collector can lose the final batch. Failed batches are never replayed, so this loses data, not requests.

## The local viewer

`tools/otel/view.sh` runs the checksum-pinned Collector 0.164.0 locally.
`tools/otel/smoke.mjs` drives the production binary through a synthetic local issuer,
then verifies real Collector trace/metric export and absence of synthetic secrets.
No cloud telemetry endpoint, identity, resource or paid backend is provisioned.
Neither standalone tests nor a local Collector receipt replace the eventual
integrated revision's Bazel/full verifier and hosted rollout gates.
