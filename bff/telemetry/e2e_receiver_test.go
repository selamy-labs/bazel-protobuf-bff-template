package telemetry

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	connect "connectrpc.com/connect/v2"
	"connectrpc.com/connect/v2/connecthttp"
	"connectrpc.com/otelconnect"
	notesv1 "example.com/bfftemplate/api/template/notes/v1"
	"example.com/bfftemplate/api/template/notes/v1/notesv1connect"
	"example.com/bfftemplate/bff/adapters/memory"
	"example.com/bfftemplate/bff/app"
	"example.com/bfftemplate/bff/identity"
	"example.com/bfftemplate/bff/ports"
	"example.com/bfftemplate/bff/testing/faults"
	"example.com/bfftemplate/bff/transport"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	collectmetric "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	collecttrace "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricpb "go.opentelemetry.io/proto/otlp/metrics/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"
)

// receiver decodes the real exporter's protobuf, acknowledges only valid requests,
// and can fail or block traces without replacing any exporter implementation.
type receiver struct {
	server   *httptest.Server
	mu       sync.Mutex
	traces   []*collecttrace.ExportTraceServiceRequest
	metrics  []*collectmetric.ExportMetricsServiceRequest
	raw      [][]byte
	failed   atomic.Bool
	requests atomic.Int64
	block    chan struct{}
	entered  chan struct{}
}

func newReceiver(t *testing.T) *receiver {
	t.Helper()
	r := &receiver{}
	r.server = httptest.NewServer(http.HandlerFunc(r.serve))
	t.Cleanup(func() { r.server.Close() })
	return r
}
func (r *receiver) serve(w http.ResponseWriter, q *http.Request) {
	r.requests.Add(1)
	if q.Method != "POST" || q.Header.Get("Content-Type") != "application/x-protobuf" {
		http.Error(w, "protocol", 400)
		return
	}
	if q.Header.Get("Authorization") != "" {
		http.Error(w, "credentials", 400)
		return
	}
	if r.failed.Load() {
		http.Error(w, "unavailable", 503)
		return
	}
	if q.URL.Path == "/v1/traces" && r.block != nil {
		select {
		case r.entered <- struct{}{}:
		default:
		}
		select {
		case <-r.block:
		case <-q.Context().Done():
			return
		}
	}
	data, err := io.ReadAll(io.LimitReader(q.Body, 4<<20))
	if err != nil {
		http.Error(w, "body", 400)
		return
	}
	var response proto.Message
	r.mu.Lock()
	defer r.mu.Unlock()
	switch q.URL.Path {
	case "/v1/traces":
		msg := new(collecttrace.ExportTraceServiceRequest)
		if proto.Unmarshal(data, msg) != nil {
			http.Error(w, "protobuf", 400)
			return
		}
		r.traces = append(r.traces, msg)
		response = &collecttrace.ExportTraceServiceResponse{}
	case "/v1/metrics":
		msg := new(collectmetric.ExportMetricsServiceRequest)
		if proto.Unmarshal(data, msg) != nil {
			http.Error(w, "protobuf", 400)
			return
		}
		r.metrics = append(r.metrics, msg)
		response = &collectmetric.ExportMetricsServiceResponse{}
	default:
		http.NotFound(w, q)
		return
	}
	r.raw = append(r.raw, bytes.Clone(data))
	body, _ := proto.Marshal(response)
	w.Header().Set("Content-Type", "application/x-protobuf")
	_, _ = w.Write(body)
}
func (r *receiver) spans() []*tracepb.Span {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*tracepb.Span
	for _, request := range r.traces {
		for _, resource := range request.ResourceSpans {
			for _, scope := range resource.ScopeSpans {
				out = append(out, scope.Spans...)
			}
		}
	}
	return out
}
func (r *receiver) metric(name string) []*metricpb.Metric {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*metricpb.Metric
	for _, request := range r.metrics {
		for _, resource := range request.ResourceMetrics {
			for _, scope := range resource.ScopeMetrics {
				for _, m := range scope.Metrics {
					if m.Name == name {
						out = append(out, m)
					}
				}
			}
		}
	}
	return out
}
func (r *receiver) assertNoSecrets(t *testing.T, secrets ...string) {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, data := range r.raw {
		for _, s := range secrets {
			if bytes.Contains(data, []byte(s)) {
				t.Fatalf("secret reached receiver: %q", s)
			}
		}
	}
}
func testConfig(endpoint string) Config {
	ratio := 1.0
	return Config{Version: 1, Endpoint: endpoint, ServiceName: "bff-server", ServiceVersion: strings.Repeat("a", 40), Environment: "dev", SampleRatio: &ratio, QueueSize: 64, BatchSize: 32, BatchDelay: time.Minute, ExportTimeout: time.Second, MetricInterval: time.Minute, ShutdownTimeout: 3 * time.Second, Buckets: []float64{.001, .005, .01, .05, .1, .5, 1, 5}}
}
func newRuntime(t *testing.T, c Config) *Runtime {
	t.Helper()
	r, err := New(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := r.Shutdown(context.Background()); err != nil {
			t.Error(err)
		}
	})
	return r
}
func flush(t *testing.T, r *Runtime) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := r.ForceFlush(ctx); err != nil {
		t.Fatal(err)
	}
}

// These are test verifiers implementing the real identity port, never production credentials.
type verifier struct{}

func (verifier) Verify(_ context.Context, token string) (string, error) {
	switch token {
	case "PRIVATE_TOKEN":
		return "PRIVATE_OWNER", nil
	case "fault-unavailable":
		return faults.Unavailable, nil
	case "fault-secret":
		return faults.Secret, nil
	default:
		return "", identity.ErrUnauthenticated
	}
}

type captureStore struct {
	next ports.Store
	log  *slog.Logger
	mu   sync.Mutex
	sc   trace.SpanContext
}

func (s *captureStore) Put(ctx context.Context, n ports.Note) error {
	if labeler, ok := otelconnect.LabelerFromContext(ctx); ok {
		labeler.Add(attribute.String("owner_id", "PRIVATE_METRIC_OWNER"))
	}
	s.mu.Lock()
	s.sc = trace.SpanContextFromContext(ctx)
	s.mu.Unlock()
	s.log.InfoContext(ctx, "store complete", "owner_id", n.Owner, "body", n.Body, "error.type", "OK")
	return s.next.Put(ctx, n)
}
func (s *captureStore) Get(ctx context.Context, o, id string) (ports.Note, error) {
	return s.next.Get(ctx, o, id)
}

type synchronizedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *synchronizedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}
func (b *synchronizedBuffer) String() string { b.mu.Lock(); defer b.mu.Unlock(); return b.b.String() }
func harness(t *testing.T, r *Runtime) (notesv1connect.NoteServiceClient, *captureStore, *synchronizedBuffer) {
	t.Helper()
	buf := &synchronizedBuffer{}
	logger := slog.New(r.LogHandler(slog.NewJSONHandler(buf, nil)))
	capture := &captureStore{next: faults.Store{Next: memory.New(100)}, log: logger}
	decorated, err := r.DecorateStore(capture)
	if err != nil {
		t.Fatal(err)
	}
	auth, err := identity.NewBearer(verifier{})
	if err != nil {
		t.Fatal(err)
	}
	interceptor, err := r.Interceptor()
	if err != nil {
		t.Fatal(err)
	}
	h := transport.NewHandler(transport.Options{Telemetry: interceptor, Notes: app.New(decorated), Authenticator: auth, AllowedOrigin: "https://app.example.invalid", Logger: logger, Limits: transport.Limits{DefaultDeadline: time.Second, MaxDeadline: 2 * time.Second, MaxRequestBytes: 64 << 10}})
	// Simulate an outer HTTP middleware already carrying an untrusted parent.
	injected, _ := trace.TraceIDFromHex(parentTrace)
	injectedSpan, _ := trace.SpanIDFromHex(parentSpan)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		ctx := trace.ContextWithSpanContext(req.Context(), trace.NewSpanContext(trace.SpanContextConfig{TraceID: injected, SpanID: injectedSpan, TraceFlags: trace.FlagsSampled}))
		h.ServeHTTP(w, req.WithContext(ctx))
	}))
	t.Cleanup(server.Close)
	client := notesv1connect.NewNoteServiceClient(connect.NewClient(connecthttp.NewTransport(server.Client(), server.URL)))
	return client, capture, buf
}

const parentTrace = "0102030405060708090a0b0c0d0e0f10"
const parentSpan = "0102030405060708"

func callContext(token string) context.Context {
	ctx, info := connect.NewClientContext(context.Background())
	info.RequestHeader().Set("Authorization", "Bearer "+token)
	info.RequestHeader().Set("traceparent", "00-"+parentTrace+"-"+parentSpan+"-00")
	info.RequestHeader().Set("tracestate", "private=PRIVATE_TRACE_STATE")
	info.RequestHeader().Set("baggage", "owner=PRIVATE_BAGGAGE")
	return ctx
}
func create(t *testing.T, c notesv1connect.NoteServiceClient, token string) (*notesv1.CreateNoteResponse, error) {
	t.Helper()
	return c.CreateNote(callContext(token), &notesv1.CreateNoteRequest{Note: &notesv1.Note{Title: "PRIVATE_TITLE", Body: "PRIVATE_BODY"}})
}
func attr(attrs []*commonpb.KeyValue, key string) string {
	for _, a := range attrs {
		if a.Key == key {
			return a.Value.GetStringValue()
		}
	}
	return ""
}
func TestOTLPSuccessFailureRedactionAndMetrics(t *testing.T) {
	recv := newReceiver(t)
	r := newRuntime(t, testConfig(recv.server.URL))
	client, capture, logs := harness(t, r)
	made, err := create(t, client, "PRIVATE_TOKEN")
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.GetNote(callContext("PRIVATE_TOKEN"), &notesv1.GetNoteRequest{Name: made.GetNote().GetName()})
	if err != nil || got.GetNote().GetBody() != "PRIVATE_BODY" {
		t.Fatalf("get=%v err=%v", got, err)
	}
	_, err = create(t, client, "fault-unavailable")
	if connect.CodeOf(err) != connect.CodeUnavailable {
		t.Fatalf("fault=%v", err)
	}
	_, err = create(t, client, "fault-secret")
	if connect.CodeOf(err) != connect.CodeInternal {
		t.Fatalf("secret fault=%v", err)
	}
	// Bypass the safe public API from this same-package negative control.
	_, bad := r.tracer.Start(context.Background(), "store.get")
	bad.SetAttributes(attribute.String("owner_id", "PRIVATE_ATTRIBUTE"))
	bad.End()
	flush(t, r)
	spans := recv.spans()
	if len(spans) != 8 {
		t.Fatalf("spans=%d want 8 (four RPC + four store); rejected must be absent", len(spans))
	}
	roots := map[string]*tracepb.Span{}
	storeErrors := 0
	rpcErrors := 0
	for _, s := range spans {
		if s.Kind == tracepb.Span_SPAN_KIND_SERVER {
			if len(s.ParentSpanId) != 0 || hex.EncodeToString(s.TraceId) == parentTrace || len(s.Links) != 1 {
				t.Fatalf("untrusted parent became trusted: %v", s)
			}
			if hex.EncodeToString(s.Links[0].TraceId) != parentTrace || hex.EncodeToString(s.Links[0].SpanId) != parentSpan || s.Links[0].TraceState != "" {
				t.Fatalf("bad link: %v", s.Links)
			}
			if attr(s.Attributes, "rpc.system.name") != "connectrpc" || attr(s.Attributes, "rpc.method") == "" {
				t.Fatalf("missing semconv: %v", s)
			}
			roots[string(s.SpanId)] = s
			if s.Status.GetCode() == tracepb.Status_STATUS_CODE_ERROR {
				rpcErrors++
			}
		} else {
			if s.Status.GetCode() == tracepb.Status_STATUS_CODE_ERROR {
				storeErrors++
			}
		}
		if s.Status.GetMessage() != "" || len(s.Events) != 0 {
			t.Fatal("raw error text/events exported")
		}
	}
	if len(roots) != 4 || storeErrors != 2 || rpcErrors != 2 {
		t.Fatalf("roots=%d store errors=%d rpc errors=%d", len(roots), storeErrors, rpcErrors)
	}
	for _, s := range spans {
		if s.Kind != tracepb.Span_SPAN_KIND_SERVER {
			parent := roots[string(s.ParentSpanId)]
			if parent == nil || !bytes.Equal(parent.TraceId, s.TraceId) {
				t.Fatalf("orphan store span: %v", s)
			}
		}
	}
	capture.mu.Lock()
	sc := capture.sc
	capture.mu.Unlock()
	if !strings.Contains(logs.String(), sc.TraceID().String()) || !strings.Contains(logs.String(), sc.SpanID().String()) {
		t.Fatalf("log correlation absent: %s", logs.String())
	}
	if !strings.Contains(logs.String(), "logging.googleapis.com/trace") ||
		!strings.Contains(logs.String(), "logging.googleapis.com/spanId") ||
		!strings.Contains(logs.String(), "logging.googleapis.com/trace_sampled") {
		t.Fatalf("cloud logging correlation absent: %s", logs.String())
	}
	for _, secret := range []string{"PRIVATE_OWNER", "PRIVATE_BODY", faults.PlantedSecret} {
		if strings.Contains(logs.String(), secret) {
			t.Fatalf("log leak: %s", logs.String())
		}
	}
	recv.assertNoSecrets(t, "PRIVATE_TOKEN", "PRIVATE_OWNER", "PRIVATE_TITLE", "PRIVATE_BODY", "PRIVATE_TRACE_STATE", "PRIVATE_BAGGAGE", "PRIVATE_ATTRIBUTE", "PRIVATE_METRIC_OWNER", faults.PlantedSecret)
	assertHistogram(t, recv, "bff.store.duration", 4, r.cfg.Buckets)
	assertHistogram(t, recv, "rpc.server.call.duration", 4, r.cfg.Buckets)
	assertSum(t, recv, "bff.operation.errors", 4)
	if sum(recv.metric("bff.telemetry.rejected")) < 1 {
		t.Fatal("redaction not counted")
	}
	recv.mu.Lock()
	defer recv.mu.Unlock()
	for _, request := range recv.traces {
		for _, rs := range request.ResourceSpans {
			if attr(rs.Resource.Attributes, "service.name") != "bff-server" || attr(rs.Resource.Attributes, "service.version") != strings.Repeat("a", 40) || attr(rs.Resource.Attributes, "deployment.environment.name") != "dev" {
				t.Fatalf("bad resource: %v", rs.Resource)
			}
		}
	}
}
func sum(ms []*metricpb.Metric) int64 {
	var total int64
	for _, m := range ms {
		var n int64
		for _, p := range m.GetSum().GetDataPoints() {
			n += p.GetAsInt()
		}
		if n > total {
			total = n
		}
	}
	return total
}
func assertSum(t *testing.T, r *receiver, name string, want int64) {
	t.Helper()
	if got := sum(r.metric(name)); got != want {
		t.Fatalf("%s=%d want %d", name, got, want)
	}
}
func assertHistogram(t *testing.T, r *receiver, name string, want uint64, bounds []float64) {
	t.Helper()
	var count uint64
	for _, m := range r.metric(name) {
		var n uint64
		for _, p := range m.GetHistogram().GetDataPoints() {
			n += p.Count
			if len(p.ExplicitBounds) != len(bounds) {
				t.Fatalf("%s boundaries=%v", name, p.ExplicitBounds)
			}
			for i, b := range bounds {
				if p.ExplicitBounds[i] != b {
					t.Fatalf("%s boundaries=%v", name, p.ExplicitBounds)
				}
			}
		}
		if n > count {
			count = n
		}
	}
	if count != want {
		t.Fatalf("%s count=%d want %d", name, count, want)
	}
}
func TestReceiverOutageRecoveryAndShutdownFlush(t *testing.T) {
	recv := newReceiver(t)
	c := testConfig(recv.server.URL)
	c.BatchSize = 1
	r := newRuntime(t, c)
	client, _, _ := harness(t, r)
	recv.failed.Store(true)
	if _, err := create(t, client, "PRIVATE_TOKEN"); err != nil {
		t.Fatalf("export outage broke RPC: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	_ = r.ForceFlush(ctx)
	cancel()
	if recv.requests.Load() == 0 {
		t.Fatal("outage never exercised exporter")
	}
	recv.failed.Store(false)
	if _, err := create(t, client, "PRIVATE_TOKEN"); err != nil {
		t.Fatal(err)
	}
	flush(t, r)
	if len(recv.spans()) < 2 {
		t.Fatal("no successful export after recovery")
	}
	if sum(recv.metric("bff.telemetry.export_failures")) < 1 {
		t.Fatal("failed exports not counted")
	}
	// End one span into a delayed, non-full batch: only Shutdown can export it.
	recv2 := newReceiver(t)
	r2 := newRuntime(t, testConfig(recv2.server.URL))
	_, pending := r2.tracer.Start(context.Background(), "store.put")
	pending.End()
	if len(recv2.spans()) != 0 {
		t.Fatal("span was not pending")
	}
	start := time.Now()
	if err := r2.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > r2.cfg.ShutdownTimeout || len(recv2.spans()) != 1 {
		t.Fatal("shutdown did not flush within deadline")
	}
}
func TestQueuePressureIsCountedAndDoesNotBlockCalls(t *testing.T) {
	recv := newReceiver(t)
	recv.block = make(chan struct{})
	recv.entered = make(chan struct{}, 1)
	defer close(recv.block)
	c := testConfig(recv.server.URL)
	c.QueueSize = 2
	c.BatchSize = 1
	c.ExportTimeout = 5 * time.Second
	c.ShutdownTimeout = 10 * time.Second
	r := newRuntime(t, c)
	client, _, _ := harness(t, r)
	_, first := r.tracer.Start(context.Background(), "store.put")
	first.End()
	select {
	case <-recv.entered:
	case <-time.After(time.Second):
		t.Fatal("export never entered")
	}
	// Keep one in flight, fill remaining admission slot, reject the rest immediately.
	for i := 0; i < 10; i++ {
		_, s := r.tracer.Start(context.Background(), "store.get")
		s.End()
	}
	if _, err := create(t, client, "PRIVATE_TOKEN"); err != nil {
		t.Fatalf("backpressure blocked RPC: %v", err)
	}
	// Release before cleanup and flush. Replace channel only after handlers have exited.
	recv.block <- struct{}{}
	recv.block <- struct{}{}
	flush(t, r)
	assertSum(t, recv, "bff.telemetry.queue_dropped", 11)
}
func TestConfigAndAttributeRefusals(t *testing.T) {
	base := testConfig("http://127.0.0.1:4318")
	if err := base.Check(); err != nil {
		t.Fatal(err)
	}
	cases := []func(*Config){func(c *Config) { c.Version = 0 }, func(c *Config) { c.SampleRatio = nil }, func(c *Config) { c.Endpoint = "http://example.com" }, func(c *Config) { c.Endpoint = "https://user:password@example.com" }, func(c *Config) { c.QueueSize = 0 }, func(c *Config) { c.BatchSize = c.QueueSize + 1 }, func(c *Config) { c.ExportTimeout = 0 }, func(c *Config) { ratio := 2.0; c.SampleRatio = &ratio }, func(c *Config) { c.Buckets = []float64{1, 1} }, func(c *Config) { c.ServiceVersion = "main" }}
	for i, f := range cases {
		c := base
		f(&c)
		if c.Check() == nil {
			t.Errorf("case %d accepted", i)
		}
	}
	if allowedAttribute(attribute.String("rpc.method", "PRIVATE_OWNER")) || allowedAttribute(attribute.String("owner_id", "x")) {
		t.Fatal("allowlist weakened")
	}
	recv := newReceiver(t)
	r := newRuntime(t, testConfig(recv.server.URL))
	if _, err := r.DecorateStore(nil); err == nil {
		t.Fatal("nil store accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s, _ := r.DecorateStore(memory.New(1))
	if !errors.Is(s.Put(ctx, ports.Note{}), context.Canceled) {
		t.Fatal("context/error identity changed")
	}
}

func TestTCPReceiverRestartAndEnvironmentIsolation(t *testing.T) {
	trap := newReceiver(t)
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", trap.server.URL)
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", trap.server.URL+"/leak")
	t.Setenv("OTEL_EXPORTER_OTLP_METRICS_ENDPOINT", trap.server.URL+"/leak")
	t.Setenv("OTEL_EXPORTER_OTLP_HEADERS", "Authorization=PRIVATE_ENV_TOKEN")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_HEADERS", "Authorization=PRIVATE_ENV_TOKEN")
	t.Setenv("OTEL_EXPORTER_OTLP_METRICS_HEADERS", "Authorization=PRIVATE_ENV_TOKEN")
	recv := newReceiver(t)
	r := newRuntime(t, testConfig(recv.server.URL))
	client, _, _ := harness(t, r)
	addr := recv.server.Listener.Addr().String()
	recv.server.Close()
	if _, err := create(t, client, "PRIVATE_TOKEN"); err != nil {
		t.Fatalf("TCP outage broke RPC: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	err := r.ForceFlush(ctx)
	cancel()
	if err == nil {
		t.Fatal("outage flush falsely succeeded")
	}
	restarted := httptest.NewUnstartedServer(http.HandlerFunc(recv.serve))
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	_ = restarted.Listener.Close()
	restarted.Listener = listener
	restarted.Start()
	recv.server = restarted
	if _, err := create(t, client, "PRIVATE_TOKEN"); err != nil {
		t.Fatal(err)
	}
	flush(t, r)
	if len(recv.spans()) != 2 {
		t.Fatalf("recovery spans=%d", len(recv.spans()))
	}
	if trap.requests.Load() != 0 {
		t.Fatal("exporter escaped configured endpoint")
	}
	recv.assertNoSecrets(t, "PRIVATE_ENV_TOKEN")
}

func TestRedactionCoversValuesNamesEventsAndBoundLogger(t *testing.T) {
	recv := newReceiver(t)
	r := newRuntime(t, testConfig(recv.server.URL))
	_, badValue := r.tracer.Start(context.Background(), "store.get")
	badValue.SetAttributes(attribute.String("rpc.method", "PRIVATE_VALUE"))
	badValue.End()
	_, badName := r.tracer.Start(context.Background(), "PRIVATE_NAME")
	badName.End()
	ctx, span := r.tracer.Start(context.Background(), "store.get")
	span.AddEvent("PRIVATE_EVENT", trace.WithAttributes(attribute.String("body", "PRIVATE_EVENT_BODY")))
	span.SetStatus(codes.Error, "PRIVATE_STATUS")
	buffer := &synchronizedBuffer{}
	log := slog.New(r.LogHandler(slog.NewJSONHandler(buffer, nil))).With("Authorization", "PRIVATE_LOG_TOKEN").WithGroup("PRIVATE_GROUP")
	log.InfoContext(ctx, "PRIVATE_MESSAGE", "trace_id", "PRIVATE_SPOOF", "error", errors.New("PRIVATE_LOG_ERROR"))
	span.End()
	flush(t, r)
	if len(recv.spans()) != 1 || sum(recv.metric("bff.telemetry.rejected")) < 2 {
		t.Fatal("planted bad spans were not rejected")
	}
	recv.assertNoSecrets(t, "PRIVATE_VALUE", "PRIVATE_NAME", "PRIVATE_EVENT", "PRIVATE_EVENT_BODY", "PRIVATE_STATUS")
	if strings.Contains(buffer.String(), "PRIVATE_") {
		t.Fatalf("log leak: %s", buffer.String())
	}
}

func TestXCloudTraceContextPropagationAndPrecedenceE2E(t *testing.T) {
	recv := newReceiver(t)
	r := newRuntime(t, testConfig(recv.server.URL))
	client, _, _ := harness(t, r)

	xctcTrace := "4bf92f3577b34da6a3ce929d0e0e4736"
	xctcSpan := "00f067cc0dd902c7"

	// 1. Fallback when traceparent is absent
	ctxFallback, info := connect.NewClientContext(context.Background())
	info.RequestHeader().Set("Authorization", "Bearer PRIVATE_TOKEN")
	info.RequestHeader().Set("x-cloud-trace-context", xctcTrace+"/"+xctcSpan+";o=1")
	_, err := client.CreateNote(ctxFallback, &notesv1.CreateNoteRequest{Note: &notesv1.Note{Title: "PRIVATE_TITLE", Body: "PRIVATE_BODY"}})
	if err != nil {
		t.Fatal(err)
	}
	flush(t, r)
	spans := recv.spans()
	var serverSpan *tracepb.Span
	for _, s := range spans {
		if s.Kind == tracepb.Span_SPAN_KIND_SERVER {
			serverSpan = s
			break
		}
	}
	if serverSpan == nil {
		t.Fatal("no server span found")
	}
	if len(serverSpan.Links) != 1 {
		t.Fatalf("links=%d want 1", len(serverSpan.Links))
	}
	if hex.EncodeToString(serverSpan.Links[0].TraceId) != xctcTrace || hex.EncodeToString(serverSpan.Links[0].SpanId) != xctcSpan {
		t.Fatalf("bad link from x-cloud-trace-context: trace=%s span=%s",
			hex.EncodeToString(serverSpan.Links[0].TraceId), hex.EncodeToString(serverSpan.Links[0].SpanId))
	}

	// 1a. Fallback with 16-digit decimal span ID parses as decimal (1234567890123456 -> 000462d53c8abac0)
	recv.mu.Lock()
	recv.traces = nil
	recv.mu.Unlock()

	decTrace := "105445aa7843bc8bf206b12000100000"
	ctxDec, infoDec := connect.NewClientContext(context.Background())
	infoDec.RequestHeader().Set("Authorization", "Bearer PRIVATE_TOKEN")
	infoDec.RequestHeader().Set("x-cloud-trace-context", decTrace+"/1234567890123456;o=1")
	_, err = client.CreateNote(ctxDec, &notesv1.CreateNoteRequest{Note: &notesv1.Note{Title: "PRIVATE_TITLE", Body: "PRIVATE_BODY"}})
	if err != nil {
		t.Fatal(err)
	}
	flush(t, r)
	spans = recv.spans()
	serverSpan = nil
	for _, s := range spans {
		if s.Kind == tracepb.Span_SPAN_KIND_SERVER {
			serverSpan = s
			break
		}
	}
	if serverSpan == nil {
		t.Fatal("no server span found")
	}
	if len(serverSpan.Links) != 1 {
		t.Fatalf("links=%d want 1", len(serverSpan.Links))
	}
	gotTrace := hex.EncodeToString(serverSpan.Links[0].TraceId)
	gotSpan := hex.EncodeToString(serverSpan.Links[0].SpanId)
	if gotTrace != decTrace || gotSpan != "000462d53c8abac0" {
		t.Fatalf("bad link from 16-digit decimal x-cloud-trace-context: got trace=%s span=%s, want trace=%s span=000462d53c8abac0",
			gotTrace, gotSpan, decTrace)
	}
	if gotSpan == "1234567890123456" {
		t.Fatalf("16-digit decimal span ID was erroneously parsed as hex 1234567890123456")
	}

	// 1b. Fallback with 16-char hex with letters (00f067cc0dd902c7)
	recv.mu.Lock()
	recv.traces = nil
	recv.mu.Unlock()

	ctxHex, infoHex := connect.NewClientContext(context.Background())
	infoHex.RequestHeader().Set("Authorization", "Bearer PRIVATE_TOKEN")
	infoHex.RequestHeader().Set("x-cloud-trace-context", decTrace+"/00f067cc0dd902c7;o=1")
	_, err = client.CreateNote(ctxHex, &notesv1.CreateNoteRequest{Note: &notesv1.Note{Title: "PRIVATE_TITLE", Body: "PRIVATE_BODY"}})
	if err != nil {
		t.Fatal(err)
	}
	flush(t, r)
	spans = recv.spans()
	serverSpan = nil
	for _, s := range spans {
		if s.Kind == tracepb.Span_SPAN_KIND_SERVER {
			serverSpan = s
			break
		}
	}
	if serverSpan == nil {
		t.Fatal("no server span found")
	}
	if len(serverSpan.Links) != 1 {
		t.Fatalf("links=%d want 1", len(serverSpan.Links))
	}
	gotTrace = hex.EncodeToString(serverSpan.Links[0].TraceId)
	gotSpan = hex.EncodeToString(serverSpan.Links[0].SpanId)
	if gotTrace != decTrace || gotSpan != "00f067cc0dd902c7" {
		t.Fatalf("bad link from hex x-cloud-trace-context: got trace=%s span=%s, want trace=%s span=00f067cc0dd902c7",
			gotTrace, gotSpan, decTrace)
	}

	// 2. Precedence when both are present
	recv.mu.Lock()
	recv.traces = nil
	recv.mu.Unlock()

	ctxBoth, infoBoth := connect.NewClientContext(context.Background())
	infoBoth.RequestHeader().Set("Authorization", "Bearer PRIVATE_TOKEN")
	infoBoth.RequestHeader().Set("traceparent", "00-"+parentTrace+"-"+parentSpan+"-01")
	infoBoth.RequestHeader().Set("x-cloud-trace-context", xctcTrace+"/"+xctcSpan+";o=1")
	_, err = client.CreateNote(ctxBoth, &notesv1.CreateNoteRequest{Note: &notesv1.Note{Title: "PRIVATE_TITLE", Body: "PRIVATE_BODY"}})
	if err != nil {
		t.Fatal(err)
	}
	flush(t, r)
	spans = recv.spans()
	serverSpan = nil
	for _, s := range spans {
		if s.Kind == tracepb.Span_SPAN_KIND_SERVER {
			serverSpan = s
			break
		}
	}
	if serverSpan == nil {
		t.Fatal("no server span found")
	}
	if len(serverSpan.Links) != 1 {
		t.Fatalf("links=%d want 1", len(serverSpan.Links))
	}
	if hex.EncodeToString(serverSpan.Links[0].TraceId) != parentTrace || hex.EncodeToString(serverSpan.Links[0].SpanId) != parentSpan {
		t.Fatalf("traceparent precedence violated: got trace=%s span=%s",
			hex.EncodeToString(serverSpan.Links[0].TraceId), hex.EncodeToString(serverSpan.Links[0].SpanId))
	}

	// 3. Malformed x-cloud-trace-context has no link
	recv.mu.Lock()
	recv.traces = nil
	recv.mu.Unlock()

	ctxBad, infoBad := connect.NewClientContext(context.Background())
	infoBad.RequestHeader().Set("Authorization", "Bearer PRIVATE_TOKEN")
	infoBad.RequestHeader().Set("x-cloud-trace-context", "malformed-header-value")
	_, err = client.CreateNote(ctxBad, &notesv1.CreateNoteRequest{Note: &notesv1.Note{Title: "PRIVATE_TITLE", Body: "PRIVATE_BODY"}})
	if err != nil {
		t.Fatal(err)
	}
	flush(t, r)
	spans = recv.spans()
	serverSpan = nil
	for _, s := range spans {
		if s.Kind == tracepb.Span_SPAN_KIND_SERVER {
			serverSpan = s
			break
		}
	}
	if serverSpan == nil {
		t.Fatal("no server span found")
	}
	if len(serverSpan.Links) != 0 {
		t.Fatalf("malformed header produced link: %v", serverSpan.Links)
	}
}
