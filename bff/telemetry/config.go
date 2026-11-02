// Package telemetry owns the BFF's bounded, payload-free telemetry contract.
package telemetry

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/exemplar"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"go.opentelemetry.io/otel/trace"
)

// ConfigV1 is resolved at composition. No environment or tuning defaults are read here.
// The only supported protocol is OTLP/HTTP protobuf. Endpoint is an origin, not a path.
// All numeric fields are required; a present zero SampleRatio intentionally means sample nothing; nil is an error.
type Config struct {
	Version                                                    int
	Endpoint, ServiceName, ServiceVersion, Environment         string
	SampleRatio                                                *float64
	QueueSize, BatchSize                                       int
	BatchDelay, ExportTimeout, MetricInterval, ShutdownTimeout time.Duration
	Buckets                                                    []float64
}

var revision = regexp.MustCompile(`^[0-9a-f]{40}$`)

func (c Config) Check() error {
	for _, check := range []func() error{c.checkIdentity, func() error { return checkEndpoint(c.Endpoint) }, c.checkSampling, c.checkQueue, func() error { return checkDurations(c) }, func() error { return checkBuckets(c.Buckets) }} {
		if err := check(); err != nil {
			return err
		}
	}
	return nil
}
func (c Config) checkIdentity() error {
	if c.Version != 1 {
		return errors.New("telemetry.version: expected 1")
	}
	if c.ServiceName != "bff-server" {
		return errors.New("telemetry.service_name: expected bff-server")
	}
	if !revision.MatchString(c.ServiceVersion) {
		return errors.New("telemetry.service_version: required full git SHA")
	}
	if c.Environment != "dev" && c.Environment != "staging" && c.Environment != "prod" {
		return errors.New("telemetry.environment: dev, staging or prod required")
	}
	return nil
}
func (c Config) checkSampling() error {
	if c.SampleRatio == nil || math.IsNaN(*c.SampleRatio) || *c.SampleRatio < 0 || *c.SampleRatio > 1 {
		return errors.New("telemetry.sample_ratio: outside [0,1]")
	}
	return nil
}
func (c Config) checkQueue() error {
	if c.QueueSize < 1 || c.QueueSize > 65536 || c.BatchSize < 1 || c.BatchSize > c.QueueSize {
		return errors.New("telemetry.queue_size/batch_size: required, ordered, at most 65536")
	}
	return nil
}
func checkEndpoint(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || !originOnly(u) {
		return errors.New("telemetry.endpoint: exact origin required")
	}
	ip := net.ParseIP(u.Hostname())
	if u.Scheme != "https" && !(u.Scheme == "http" && ip != nil && ip.IsLoopback()) {
		return errors.New("telemetry.endpoint: HTTPS or literal loopback HTTP required")
	}
	return nil
}
func checkDurations(c Config) error {
	for _, v := range []struct {
		name  string
		value time.Duration
	}{{"batch_delay", c.BatchDelay}, {"export_timeout", c.ExportTimeout}, {"metric_interval", c.MetricInterval}, {"shutdown_timeout", c.ShutdownTimeout}} {
		if v.value <= 0 || v.value > time.Minute {
			return fmt.Errorf("telemetry.%s: required and at most one minute", v.name)
		}
	}
	return nil
}
func checkBuckets(bounds []float64) error {
	if len(bounds) < 1 || len(bounds) > 32 {
		return errors.New("telemetry.buckets: 1..32 boundaries required")
	}
	last := 0.0
	for _, v := range bounds {
		if math.IsNaN(v) || math.IsInf(v, 0) || v <= last {
			return errors.New("telemetry.buckets: finite, positive and increasing required")
		}
		last = v
	}
	return nil
}

// Runtime is process-owned, closed after HTTP drain. Providers are private: callers use
// Interceptor, DecorateStore and LogHandler instead of creating arbitrary instruments.
type Runtime struct {
	cfg                                         Config
	tp                                          *sdktrace.TracerProvider
	mp                                          *sdkmetric.MeterProvider
	tracer                                      trace.Tracer
	latency                                     metric.Float64Histogram
	failures, rejected, exportFailures, dropped metric.Int64Counter
	lifecycle                                   sync.Mutex
	closed                                      bool
	gate                                        *admission
}

func New(ctx context.Context, c Config) (*Runtime, error) {
	if err := c.Check(); err != nil {
		return nil, err
	}
	c.Buckets = append([]float64(nil), c.Buckets...)
	ratio := *c.SampleRatio
	c.SampleRatio = &ratio
	r := &Runtime{cfg: c}
	res := resource.NewWithAttributes(semconv.SchemaURL, semconv.ServiceName(c.ServiceName), semconv.ServiceVersion(c.ServiceVersion), attribute.String("deployment.environment.name", c.Environment))
	// A dedicated client ignores proxy environment, forbids redirects, and contains no
	// application credentials. Explicit options override OTEL_EXPORTER_* environment.
	client := &http.Client{Transport: &http.Transport{Proxy: nil}, Timeout: c.ExportTimeout, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("telemetry redirects forbidden") }}
	mx, err := otlpmetrichttp.New(ctx, otlpmetrichttp.WithEndpointURL(c.Endpoint+"/v1/metrics"), otlpmetrichttp.WithHTTPClient(client), otlpmetrichttp.WithHeaders(map[string]string{}), otlpmetrichttp.WithCompression(otlpmetrichttp.NoCompression), otlpmetrichttp.WithTimeout(c.ExportTimeout), otlpmetrichttp.WithRetry(otlpmetrichttp.RetryConfig{Enabled: false}))
	if err != nil {
		return nil, err
	}
	r.mp = sdkmetric.NewMeterProvider(sdkmetric.WithResource(res), sdkmetric.WithExemplarFilter(exemplar.AlwaysOffFilter), sdkmetric.WithView(sdkmetric.NewView(sdkmetric.Instrument{Name: "*"}, sdkmetric.Stream{AttributeFilter: allowedAttribute})), sdkmetric.WithReader(sdkmetric.NewPeriodicReader(mx, sdkmetric.WithInterval(c.MetricInterval), sdkmetric.WithTimeout(c.ExportTimeout))))
	if err = r.instruments(); err != nil {
		_ = r.mp.Shutdown(ctx)
		return nil, err
	}
	tx, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(c.Endpoint+"/v1/traces"), otlptracehttp.WithHTTPClient(client), otlptracehttp.WithHeaders(map[string]string{}), otlptracehttp.WithCompression(otlptracehttp.NoCompression), otlptracehttp.WithEncoding(otlptracehttp.EncodingProtobuf), otlptracehttp.WithTimeout(c.ExportTimeout), otlptracehttp.WithRetry(otlptracehttp.RetryConfig{Enabled: false}))
	if err != nil {
		_ = r.mp.Shutdown(ctx)
		return nil, err
	}
	r.gate = &admission{slots: make(chan struct{}, c.QueueSize), rejected: r.rejected, dropped: r.dropped}
	counted := &countingExporter{SpanExporter: tx, slots: r.gate.slots, failures: r.exportFailures}
	// One extra SDK queue slot is reserved for the single serialized ForceFlush marker.
	// Admission bounds queued + in-flight spans, so SDK enqueue cannot drop accepted spans.
	r.gate.next = sdktrace.NewBatchSpanProcessor(counted, sdktrace.WithMaxQueueSize(c.QueueSize+1), sdktrace.WithMaxExportBatchSize(c.BatchSize), sdktrace.WithBatchTimeout(c.BatchDelay), sdktrace.WithExportTimeout(c.ExportTimeout))
	r.tp = sdktrace.NewTracerProvider(sdktrace.WithResource(res), sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(*c.SampleRatio))), sdktrace.WithRawSpanLimits(sdktrace.SpanLimits{AttributeCountLimit: 32, AttributeValueLengthLimit: 256, EventCountLimit: 0, LinkCountLimit: 1, AttributePerEventCountLimit: 0, AttributePerLinkCountLimit: 0}), sdktrace.WithSpanProcessor(r.gate))
	r.tracer = r.tp.Tracer("example.com/bfftemplate/bff/telemetry", trace.WithInstrumentationVersion(c.ServiceVersion), trace.WithSchemaURL(semconv.SchemaURL))
	return r, nil
}
func (r *Runtime) instruments() error {
	m := r.mp.Meter("example.com/bfftemplate/bff/telemetry", metric.WithSchemaURL(semconv.SchemaURL))
	var errs [5]error
	r.latency, errs[0] = m.Float64Histogram("bff.store.duration", metric.WithUnit("s"), metric.WithExplicitBucketBoundaries(r.cfg.Buckets...))
	r.failures, errs[1] = m.Int64Counter("bff.operation.errors", metric.WithUnit("{error}"))
	r.rejected, errs[2] = m.Int64Counter("bff.telemetry.rejected", metric.WithUnit("{item}"))
	r.exportFailures, errs[3] = m.Int64Counter("bff.telemetry.export_failures", metric.WithUnit("{batch}"))
	r.dropped, errs[4] = m.Int64Counter("bff.telemetry.queue_dropped", metric.WithUnit("{span}"))
	return errors.Join(errs[:]...)
}
func (r *Runtime) ForceFlush(ctx context.Context) error {
	r.lifecycle.Lock()
	defer r.lifecycle.Unlock()
	if r.closed {
		return errors.New("telemetry closed")
	}
	// Trace export failures are observable via export_failures even when an earlier
	// background batch failed; ForceFlush cannot resurrect that batch.
	return errors.Join(r.tp.ForceFlush(ctx), r.mp.ForceFlush(ctx))
}
func (r *Runtime) Shutdown(ctx context.Context) error {
	r.lifecycle.Lock()
	defer r.lifecycle.Unlock()
	if r.closed {
		return nil
	}
	r.closed = true
	ctx, cancel := context.WithTimeout(ctx, r.cfg.ShutdownTimeout)
	defer cancel()
	// Flush traces first so final rejection/drop/export-failure counters reach metrics.
	return errors.Join(r.tp.Shutdown(ctx), r.mp.Shutdown(ctx))
}

type admission struct {
	next              sdktrace.SpanProcessor
	slots             chan struct{}
	mu                sync.RWMutex
	closed            bool
	rejected, dropped metric.Int64Counter
}

func (*admission) OnStart(context.Context, sdktrace.ReadWriteSpan) {}
func (a *admission) OnEnd(s sdktrace.ReadOnlySpan) {
	if !s.SpanContext().IsSampled() {
		return
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.closed {
		return
	}
	if !safeSpan(s) {
		a.rejected.Add(context.Background(), 1)
		return
	}
	select {
	case a.slots <- struct{}{}:
		a.next.OnEnd(cleanSpan{s})
	default:
		a.dropped.Add(context.Background(), 1)
	}
}
func (a *admission) ForceFlush(ctx context.Context) error { return a.next.ForceFlush(ctx) }
func (a *admission) Shutdown(ctx context.Context) error {
	a.mu.Lock()
	a.closed = true
	a.mu.Unlock()
	return a.next.Shutdown(ctx)
}

type countingExporter struct {
	sdktrace.SpanExporter
	slots    chan struct{}
	failures metric.Int64Counter
}

func (e *countingExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	defer func() {
		for range spans {
			<-e.slots
		}
	}()
	err := e.SpanExporter.ExportSpans(ctx, spans)
	if err != nil {
		e.failures.Add(context.Background(), 1)
	}
	return err
}

// Error text and events never cross this boundary. Error code and enum attributes
// carry diagnostics without request bodies, JWTs, downstream errors or trace state.
type cleanSpan struct{ sdktrace.ReadOnlySpan }

func (s cleanSpan) Status() sdktrace.Status {
	v := s.ReadOnlySpan.Status()
	v.Description = ""
	return v
}
func (s cleanSpan) Events() []sdktrace.Event { return nil }
func (s cleanSpan) Links() []sdktrace.Link {
	out := make([]sdktrace.Link, 0, 1)
	for _, l := range s.ReadOnlySpan.Links() {
		l.Attributes = nil
		l.SpanContext = l.SpanContext.WithTraceState(trace.TraceState{})
		out = append(out, l)
	}
	return out
}
func safeSpan(s sdktrace.ReadOnlySpan) bool {
	if !allowedSpanName(s.Name()) {
		return false
	}
	for _, a := range s.Attributes() {
		if !allowedAttribute(a) {
			return false
		}
	}
	return true
}
func allowedSpanName(name string) bool {
	return name == "store.put" || name == "store.get" || name == createProcedure[1:] || name == getProcedure[1:]
}
func allowedAttribute(a attribute.KeyValue) bool {
	if a.Value.Type() != attribute.STRING {
		return false
	}
	value := a.Value.AsString()
	switch string(a.Key) {
	case "rpc.system.name":
		return value == "connectrpc"
	case "rpc.method":
		return value == createProcedure[1:] || value == getProcedure[1:]
	case "rpc.response.status_code", "error.type":
		return validOutcome(value)
	case "bff.operation":
		return value == "put" || value == "get"
	default:
		return false
	}
}
func validOutcome(s string) bool {
	switch s {
	case "OK", "CANCELED", "UNKNOWN", "INVALID_ARGUMENT", "DEADLINE_EXCEEDED", "NOT_FOUND", "ALREADY_EXISTS", "PERMISSION_DENIED", "RESOURCE_EXHAUSTED", "FAILED_PRECONDITION", "ABORTED", "OUT_OF_RANGE", "UNIMPLEMENTED", "INTERNAL", "UNAVAILABLE", "DATA_LOSS", "UNAUTHENTICATED":
		return true
	}
	return false
}

func originOnly(u *url.URL) bool {
	return u.Hostname() != "" && u.User == nil && u.RawQuery == "" && u.Fragment == "" && u.Path == ""
}
