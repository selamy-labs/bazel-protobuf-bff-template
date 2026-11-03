package telemetry

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

func TestCloudLoggingTraceCorrelation(t *testing.T) {
	traceIDHex := "4bf92f3577b34da6a3ce929d0e0e4736"
	spanIDHex := "00f067cc0dd902c7"
	tid, err := trace.TraceIDFromHex(traceIDHex)
	if err != nil {
		t.Fatal(err)
	}
	sid, err := trace.SpanIDFromHex(spanIDHex)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name        string
		projectID   string
		sampled     bool
		wantTrace   string
		wantSpanID  string
		wantSampled bool
	}{
		{
			name:        "without project ID, sampled",
			projectID:   "",
			sampled:     true,
			wantTrace:   traceIDHex,
			wantSpanID:  spanIDHex,
			wantSampled: true,
		},
		{
			name:        "without project ID, unsampled",
			projectID:   "",
			sampled:     false,
			wantTrace:   traceIDHex,
			wantSpanID:  spanIDHex,
			wantSampled: false,
		},
		{
			name:        "with project ID, sampled",
			projectID:   "example-gcp-project",
			sampled:     true,
			wantTrace:   "projects/example-gcp-project/traces/" + traceIDHex,
			wantSpanID:  spanIDHex,
			wantSampled: true,
		},
		{
			name:        "with project ID, unsampled",
			projectID:   "my-prod-project",
			sampled:     false,
			wantTrace:   "projects/my-prod-project/traces/" + traceIDHex,
			wantSpanID:  spanIDHex,
			wantSampled: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ratio := 1.0
			cfg := Config{
				Version:         1,
				Endpoint:        "http://127.0.0.1:4318",
				ServiceName:     "bff-server",
				ServiceVersion:  strings.Repeat("a", 40),
				Environment:     "dev",
				ProjectID:       tc.projectID,
				SampleRatio:     &ratio,
				QueueSize:       64,
				BatchSize:       32,
				BatchDelay:      time.Minute,
				ExportTimeout:   time.Second,
				MetricInterval:  time.Minute,
				ShutdownTimeout: 3 * time.Second,
				Buckets:         []float64{.001, .005, .01, .05, .1, .5, 1, 5},
			}
			r, err := New(context.Background(), cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = r.Shutdown(context.Background()) }()

			var buf bytes.Buffer
			handler := r.LogHandler(slog.NewJSONHandler(&buf, nil))
			logger := slog.New(handler)

			var flags trace.TraceFlags
			if tc.sampled {
				flags = trace.FlagsSampled
			}
			sc := trace.NewSpanContext(trace.SpanContextConfig{
				TraceID:    tid,
				SpanID:     sid,
				TraceFlags: flags,
			})
			ctx := trace.ContextWithSpanContext(context.Background(), sc)

			logger.InfoContext(ctx, "store complete", "rpc.status_code", "OK")

			var parsed map[string]any
			if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
				t.Fatalf("failed to parse log JSON: %v, raw: %s", err, buf.String())
			}

			// Original trace_id and span_id must remain
			if got, ok := parsed["trace_id"].(string); !ok || got != traceIDHex {
				t.Errorf("trace_id = %v, want %v", parsed["trace_id"], traceIDHex)
			}
			if got, ok := parsed["span_id"].(string); !ok || got != spanIDHex {
				t.Errorf("span_id = %v, want %v", parsed["span_id"], spanIDHex)
			}

			// Cloud Logging attributes
			if got, ok := parsed["logging.googleapis.com/trace"].(string); !ok || got != tc.wantTrace {
				t.Errorf("logging.googleapis.com/trace = %v, want %v", parsed["logging.googleapis.com/trace"], tc.wantTrace)
			}
			if got, ok := parsed["logging.googleapis.com/spanId"].(string); !ok || got != tc.wantSpanID {
				t.Errorf("logging.googleapis.com/spanId = %v, want %v", parsed["logging.googleapis.com/spanId"], tc.wantSpanID)
			}
			if got, ok := parsed["logging.googleapis.com/trace_sampled"].(bool); !ok || got != tc.wantSampled {
				t.Errorf("logging.googleapis.com/trace_sampled = %v, want %v", parsed["logging.googleapis.com/trace_sampled"], tc.wantSampled)
			}

			// rpc.status_code attribute allowed
			if got, ok := parsed["rpc.status_code"].(string); !ok || got != "OK" {
				t.Errorf("rpc.status_code = %v, want OK", parsed["rpc.status_code"])
			}
		})
	}
}

func TestSafeLogAttr(t *testing.T) {
	tests := []struct {
		name string
		attr slog.Attr
		want bool
	}{
		// procedure
		{name: "procedure create", attr: slog.String("procedure", createProcedure), want: true},
		{name: "procedure get", attr: slog.String("procedure", getProcedure), want: true},
		{name: "procedure unknown", attr: slog.String("procedure", "/other.Service/Method"), want: false},

		// correlation_id
		{name: "correlation_id valid", attr: slog.String("correlation_id", "0123456789abcdef"), want: true},
		{name: "correlation_id uppercase", attr: slog.String("correlation_id", "0123456789ABCDEF"), want: false},
		{name: "correlation_id too short", attr: slog.String("correlation_id", "0123456789"), want: false},
		{name: "correlation_id non-hex", attr: slog.String("correlation_id", "0123456789abcdefg"), want: false},

		// error.type
		{name: "error.type OK", attr: slog.String("error.type", "OK"), want: true},
		{name: "error.type INTERNAL", attr: slog.String("error.type", "INTERNAL"), want: true},
		{name: "error.type UNAVAILABLE", attr: slog.String("error.type", "UNAVAILABLE"), want: true},
		{name: "error.type invalid", attr: slog.String("error.type", "CUSTOM_ERROR"), want: false},

		// rpc.status_code
		{name: "rpc.status_code OK", attr: slog.String("rpc.status_code", "OK"), want: true},
		{name: "rpc.status_code canceled", attr: slog.String("rpc.status_code", "canceled"), want: true},
		{name: "rpc.status_code CANCELED", attr: slog.String("rpc.status_code", "CANCELED"), want: true},
		{name: "rpc.status_code invalid_argument", attr: slog.String("rpc.status_code", "invalid_argument"), want: true},
		{name: "rpc.status_code deadline_exceeded", attr: slog.String("rpc.status_code", "deadline_exceeded"), want: true},
		{name: "rpc.status_code not_found", attr: slog.String("rpc.status_code", "not_found"), want: true},
		{name: "rpc.status_code already_exists", attr: slog.String("rpc.status_code", "already_exists"), want: true},
		{name: "rpc.status_code permission_denied", attr: slog.String("rpc.status_code", "permission_denied"), want: true},
		{name: "rpc.status_code resource_exhausted", attr: slog.String("rpc.status_code", "resource_exhausted"), want: true},
		{name: "rpc.status_code failed_precondition", attr: slog.String("rpc.status_code", "failed_precondition"), want: true},
		{name: "rpc.status_code aborted", attr: slog.String("rpc.status_code", "aborted"), want: true},
		{name: "rpc.status_code out_of_range", attr: slog.String("rpc.status_code", "out_of_range"), want: true},
		{name: "rpc.status_code unimplemented", attr: slog.String("rpc.status_code", "unimplemented"), want: true},
		{name: "rpc.status_code internal", attr: slog.String("rpc.status_code", "internal"), want: true},
		{name: "rpc.status_code unavailable", attr: slog.String("rpc.status_code", "unavailable"), want: true},
		{name: "rpc.status_code data_loss", attr: slog.String("rpc.status_code", "data_loss"), want: true},
		{name: "rpc.status_code unauthenticated", attr: slog.String("rpc.status_code", "unauthenticated"), want: true},
		{name: "rpc.status_code invalid string", attr: slog.String("rpc.status_code", "unknown_code"), want: false},
		{name: "rpc.status_code http 200", attr: slog.String("rpc.status_code", "200"), want: false},

		// unapproved attributes
		{name: "unapproved secret", attr: slog.String("secret", "PRIVATE_VALUE"), want: false},
		{name: "unapproved token", attr: slog.String("token", "my-token"), want: false},
		{name: "unapproved body", attr: slog.String("body", "note content"), want: false},
		{name: "unapproved trace_id in user attrs", attr: slog.String("trace_id", "spoofed"), want: false},
		{name: "unapproved span_id in user attrs", attr: slog.String("span_id", "spoofed"), want: false},

		// non-string types
		{name: "rpc.status_code int", attr: slog.Int("rpc.status_code", 0), want: false},
		{name: "rpc.status_code bool", attr: slog.Bool("rpc.status_code", true), want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := safeLogAttr(tc.attr); got != tc.want {
				t.Errorf("safeLogAttr(%v) = %v, want %v", tc.attr, got, tc.want)
			}
		})
	}
}

func TestTraceParentOnlyExtract(t *testing.T) {
	prop := traceParentOnly{}

	t.Run("fields includes traceparent and x-cloud-trace-context", func(t *testing.T) {
		fields := prop.Fields()
		hasTP := false
		hasXCTC := false
		for _, f := range fields {
			if f == "traceparent" {
				hasTP = true
			}
			if f == "x-cloud-trace-context" {
				hasXCTC = true
			}
		}
		if !hasTP || !hasXCTC {
			t.Errorf("Fields() = %v, want traceparent and x-cloud-trace-context", fields)
		}
	})

	t.Run("fallback with 16-hex span ID sampled", func(t *testing.T) {
		carrier := propagation.MapCarrier{
			"x-cloud-trace-context": "4bf92f3577b34da6a3ce929d0e0e4736/00f067cc0dd902c7;o=1",
		}
		ctx := prop.Extract(context.Background(), carrier)
		sc := trace.SpanContextFromContext(ctx)
		if !sc.IsValid() {
			t.Fatal("expected valid SpanContext")
		}
		if sc.TraceID().String() != "4bf92f3577b34da6a3ce929d0e0e4736" {
			t.Errorf("TraceID = %s, want 4bf92f3577b34da6a3ce929d0e0e4736", sc.TraceID().String())
		}
		if sc.SpanID().String() != "00f067cc0dd902c7" {
			t.Errorf("SpanID = %s, want 00f067cc0dd902c7", sc.SpanID().String())
		}
		if !sc.IsSampled() {
			t.Errorf("IsSampled = false, want true")
		}
		if !sc.IsRemote() {
			t.Errorf("IsRemote = false, want true")
		}
	})

	t.Run("fallback with 16-hex span ID unsampled", func(t *testing.T) {
		carrier := propagation.MapCarrier{
			"x-cloud-trace-context": "4bf92f3577b34da6a3ce929d0e0e4736/00f067cc0dd902c7;o=0",
		}
		ctx := prop.Extract(context.Background(), carrier)
		sc := trace.SpanContextFromContext(ctx)
		if !sc.IsValid() {
			t.Fatal("expected valid SpanContext")
		}
		if sc.IsSampled() {
			t.Errorf("IsSampled = true, want false")
		}
	})

	t.Run("fallback with no options unsampled", func(t *testing.T) {
		carrier := propagation.MapCarrier{
			"x-cloud-trace-context": "4bf92f3577b34da6a3ce929d0e0e4736/00f067cc0dd902c7",
		}
		ctx := prop.Extract(context.Background(), carrier)
		sc := trace.SpanContextFromContext(ctx)
		if !sc.IsValid() {
			t.Fatal("expected valid SpanContext")
		}
		if sc.IsSampled() {
			t.Errorf("IsSampled = true, want false")
		}
	})

	t.Run("fallback with decimal uint64 span ID", func(t *testing.T) {
		// 123456789 in hex is 0x00000000075bcd15
		var expectedSpanBuf [8]byte
		binary.BigEndian.PutUint64(expectedSpanBuf[:], 123456789)
		expectedSpanHex := hex.EncodeToString(expectedSpanBuf[:])

		carrier := propagation.MapCarrier{
			"x-cloud-trace-context": "4bf92f3577b34da6a3ce929d0e0e4736/123456789;o=1",
		}
		ctx := prop.Extract(context.Background(), carrier)
		sc := trace.SpanContextFromContext(ctx)
		if !sc.IsValid() {
			t.Fatal("expected valid SpanContext")
		}
		if sc.SpanID().String() != expectedSpanHex {
			t.Errorf("SpanID = %s, want %s", sc.SpanID().String(), expectedSpanHex)
		}
		if !sc.IsSampled() {
			t.Errorf("IsSampled = false, want true")
		}
	})

	t.Run("fallback with max decimal uint64 span ID", func(t *testing.T) {
		carrier := propagation.MapCarrier{
			"x-cloud-trace-context": "4bf92f3577b34da6a3ce929d0e0e4736/18446744073709551615;o=1",
		}
		ctx := prop.Extract(context.Background(), carrier)
		sc := trace.SpanContextFromContext(ctx)
		if !sc.IsValid() {
			t.Fatal("expected valid SpanContext")
		}
		if sc.SpanID().String() != "ffffffffffffffff" {
			t.Errorf("SpanID = %s, want ffffffffffffffff", sc.SpanID().String())
		}
	})

	t.Run("case insensitive header name", func(t *testing.T) {
		carrier := propagation.MapCarrier{
			"X-Cloud-Trace-Context": "4bf92f3577b34da6a3ce929d0e0e4736/00f067cc0dd902c7;o=1",
		}
		ctx := prop.Extract(context.Background(), carrier)
		sc := trace.SpanContextFromContext(ctx)
		if !sc.IsValid() {
			t.Fatal("expected valid SpanContext")
		}
		if sc.TraceID().String() != "4bf92f3577b34da6a3ce929d0e0e4736" {
			t.Errorf("TraceID = %s, want 4bf92f3577b34da6a3ce929d0e0e4736", sc.TraceID().String())
		}
	})

	t.Run("traceparent precedence when both are present", func(t *testing.T) {
		tpTraceID := "0102030405060708090a0b0c0d0e0f10"
		tpSpanID := "0102030405060708"
		xctcTraceID := "4bf92f3577b34da6a3ce929d0e0e4736"
		xctcSpanID := "00f067cc0dd902c7"

		carrier := propagation.MapCarrier{
			"traceparent":           "00-" + tpTraceID + "-" + tpSpanID + "-01",
			"x-cloud-trace-context": xctcTraceID + "/" + xctcSpanID + ";o=1",
		}
		ctx := prop.Extract(context.Background(), carrier)
		sc := trace.SpanContextFromContext(ctx)
		if !sc.IsValid() {
			t.Fatal("expected valid SpanContext")
		}
		if sc.TraceID().String() != tpTraceID {
			t.Errorf("TraceID = %s, want %s (traceparent precedence)", sc.TraceID().String(), tpTraceID)
		}
		if sc.SpanID().String() != tpSpanID {
			t.Errorf("SpanID = %s, want %s (traceparent precedence)", sc.SpanID().String(), tpSpanID)
		}
	})

	malformedCases := []struct {
		name   string
		header string
	}{
		{name: "empty header", header: ""},
		{name: "whitespace header", header: "   "},
		{name: "no slash", header: "4bf92f3577b34da6a3ce929d0e0e4736"},
		{name: "empty trace id", header: "/00f067cc0dd902c7;o=1"},
		{name: "empty span id", header: "4bf92f3577b34da6a3ce929d0e0e4736/;o=1"},
		{name: "uppercase trace id", header: "4BF92F3577B34DA6A3CE929D0E0E4736/00f067cc0dd902c7;o=1"},
		{name: "all-zero trace id", header: "00000000000000000000000000000000/00f067cc0dd902c7;o=1"},
		{name: "short trace id", header: "4bf92f3577b34da6a3ce929d0e0e473/00f067cc0dd902c7;o=1"},
		{name: "long trace id", header: "4bf92f3577b34da6a3ce929d0e0e473600/00f067cc0dd902c7;o=1"},
		{name: "non-hex trace id", header: "4bf92f3577b34da6a3ce929d0e0e473g/00f067cc0dd902c7;o=1"},
		{name: "all-zero decimal span id", header: "4bf92f3577b34da6a3ce929d0e0e4736/0;o=1"},
		{name: "all-zero hex span id", header: "4bf92f3577b34da6a3ce929d0e0e4736/0000000000000000;o=1"},
		{name: "non-hex non-decimal span id", header: "4bf92f3577b34da6a3ce929d0e0e4736/xyz;o=1"},
		{name: "overflow decimal span id", header: "4bf92f3577b34da6a3ce929d0e0e4736/18446744073709551616;o=1"},
		{name: "negative span id", header: "4bf92f3577b34da6a3ce929d0e0e4736/-1;o=1"},
		{name: "empty options value", header: "4bf92f3577b34da6a3ce929d0e0e4736/00f067cc0dd902c7;o="},
		{name: "non-o option", header: "4bf92f3577b34da6a3ce929d0e0e4736/00f067cc0dd902c7;invalid=1"},
		{name: "non-numeric option", header: "4bf92f3577b34da6a3ce929d0e0e4736/00f067cc0dd902c7;o=abc"},
	}

	for _, tc := range malformedCases {
		t.Run("reject malformed: "+tc.name, func(t *testing.T) {
			carrier := propagation.MapCarrier{
				"x-cloud-trace-context": tc.header,
			}
			ctx := prop.Extract(context.Background(), carrier)
			sc := trace.SpanContextFromContext(ctx)
			if sc.IsValid() {
				t.Errorf("expected invalid SpanContext for header %q, got %v", tc.header, sc)
			}
		})
	}
}

func TestConfigProjectIDValidation(t *testing.T) {
	baseCfg := func() Config {
		ratio := 1.0
		return Config{
			Version:         1,
			Endpoint:        "http://127.0.0.1:4318",
			ServiceName:     "bff-server",
			ServiceVersion:  strings.Repeat("a", 40),
			Environment:     "dev",
			SampleRatio:     &ratio,
			QueueSize:       64,
			BatchSize:       32,
			BatchDelay:      time.Second,
			ExportTimeout:   time.Second,
			MetricInterval:  time.Second,
			ShutdownTimeout: time.Second,
			Buckets:         []float64{0.1, 0.5, 1.0},
		}
	}

	t.Run("empty ProjectID is valid", func(t *testing.T) {
		cfg := baseCfg()
		cfg.ProjectID = ""
		if err := cfg.Check(); err != nil {
			t.Fatalf("expected valid config, got: %v", err)
		}
	})

	t.Run("valid ProjectID is accepted", func(t *testing.T) {
		cfg := baseCfg()
		cfg.ProjectID = "my-gcp-project-123"
		if err := cfg.Check(); err != nil {
			t.Fatalf("expected valid config, got: %v", err)
		}
	})

	t.Run("invalid ProjectID is refused", func(t *testing.T) {
		cfg := baseCfg()
		cfg.ProjectID = "INVALID_PROJECT"
		if err := cfg.Check(); err == nil {
			t.Fatal("expected error for invalid ProjectID, got nil")
		}
	})
}
