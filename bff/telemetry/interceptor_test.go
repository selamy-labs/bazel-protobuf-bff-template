package telemetry

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

func TestParseCloudSpanID_DecimalAndHex(t *testing.T) {
	tests := []struct {
		name      string
		raw       string
		wantOK    bool
		wantHex   string
		forbidHex string
	}{
		{
			name:      "16-digit decimal span ID parses as decimal not hex",
			raw:       "1234567890123456",
			wantOK:    true,
			wantHex:   "000462d53c8abac0",
			forbidHex: "1234567890123456",
		},
		{
			name:    "16-char lowercase hex with letters parses as hex",
			raw:     "00f067cc0dd902c7",
			wantOK:  true,
			wantHex: "00f067cc0dd902c7",
		},
		{
			name:    "short decimal span ID",
			raw:     "123456789",
			wantOK:  true,
			wantHex: "00000000075bcd15",
		},
		{
			name:    "max uint64 decimal span ID",
			raw:     "18446744073709551615",
			wantOK:  true,
			wantHex: "ffffffffffffffff",
		},
		{
			name:   "zero decimal span ID is rejected",
			raw:    "0",
			wantOK: false,
		},
		{
			name:   "all zeros 16-digit decimal span ID is rejected",
			raw:    "0000000000000000",
			wantOK: false,
		},
		{
			name:   "overflow decimal span ID is rejected",
			raw:    "18446744073709551616",
			wantOK: false,
		},
		{
			name:   "empty span ID is rejected",
			raw:    "",
			wantOK: false,
		},
		{
			name:   "uppercase hex with letters is rejected",
			raw:    "00F067CC0DD902C7",
			wantOK: false,
		},
		{
			name:   "invalid non-hex string is rejected",
			raw:    "not-a-valid-span",
			wantOK: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sid, ok := parseCloudSpanID(tc.raw)
			if ok != tc.wantOK {
				t.Fatalf("parseCloudSpanID(%q) ok=%v, want %v", tc.raw, ok, tc.wantOK)
			}
			if tc.wantOK {
				gotHex := sid.String()
				if gotHex != tc.wantHex {
					t.Errorf("parseCloudSpanID(%q) = %s, want %s", tc.raw, gotHex, tc.wantHex)
				}
				if tc.forbidHex != "" && gotHex == tc.forbidHex {
					t.Errorf("parseCloudSpanID(%q) erroneously matched forbidden hex %s", tc.raw, tc.forbidHex)
				}
			}
		})
	}
}

func TestParseCloudTraceContext_ExactAdversarialCases(t *testing.T) {
	traceHex := "105445aa7843bc8bf206b12000100000"

	t.Run("16-digit decimal header parses as decimal span", func(t *testing.T) {
		header := traceHex + "/1234567890123456;o=1"
		sc, ok := parseCloudTraceContext(header)
		if !ok || !sc.IsValid() {
			t.Fatalf("parseCloudTraceContext(%q) failed", header)
		}
		if sc.TraceID().String() != traceHex {
			t.Errorf("traceID = %s, want %s", sc.TraceID().String(), traceHex)
		}
		if sc.SpanID().String() != "000462d53c8abac0" {
			t.Errorf("spanID = %s, want 000462d53c8abac0 (decimal)", sc.SpanID().String())
		}
		if sc.SpanID().String() == "1234567890123456" {
			t.Errorf("spanID was erroneously parsed as hex 1234567890123456")
		}
		if !sc.IsSampled() {
			t.Errorf("expected sampled=true")
		}
	})

	t.Run("16-char hex with letters parses as hex span", func(t *testing.T) {
		header := traceHex + "/00f067cc0dd902c7;o=1"
		sc, ok := parseCloudTraceContext(header)
		if !ok || !sc.IsValid() {
			t.Fatalf("parseCloudTraceContext(%q) failed", header)
		}
		if sc.TraceID().String() != traceHex {
			t.Errorf("traceID = %s, want %s", sc.TraceID().String(), traceHex)
		}
		if sc.SpanID().String() != "00f067cc0dd902c7" {
			t.Errorf("spanID = %s, want 00f067cc0dd902c7 (hex)", sc.SpanID().String())
		}
		if !sc.IsSampled() {
			t.Errorf("expected sampled=true")
		}
	})
}

func TestTraceParentOnly_ExtractPrecedence(t *testing.T) {
	p := traceParentOnly{}

	t.Run("traceparent takes precedence over x-cloud-trace-context", func(t *testing.T) {
		carrier := propagation.MapCarrier{
			"traceparent":           "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067cc0dd902c7-01",
			"x-cloud-trace-context": "105445aa7843bc8bf206b12000100000/1234567890123456;o=1",
		}
		ctx := p.Extract(context.Background(), carrier)
		sc := trace.SpanContextFromContext(ctx)
		if !sc.IsValid() {
			t.Fatal("expected valid span context")
		}
		if sc.TraceID().String() != "4bf92f3577b34da6a3ce929d0e0e4736" {
			t.Errorf("got trace %s, want traceparent trace", sc.TraceID().String())
		}
		if sc.SpanID().String() != "00f067cc0dd902c7" {
			t.Errorf("got span %s, want traceparent span", sc.SpanID().String())
		}
	})

	t.Run("fallback extracts 16-digit decimal span correctly", func(t *testing.T) {
		carrier := propagation.MapCarrier{
			"x-cloud-trace-context": "105445aa7843bc8bf206b12000100000/1234567890123456;o=1",
		}
		ctx := p.Extract(context.Background(), carrier)
		sc := trace.SpanContextFromContext(ctx)
		if !sc.IsValid() {
			t.Fatal("expected valid span context")
		}
		if sc.TraceID().String() != "105445aa7843bc8bf206b12000100000" {
			t.Errorf("got trace %s", sc.TraceID().String())
		}
		if sc.SpanID().String() != "000462d53c8abac0" {
			t.Errorf("got span %s, want decimal 000462d53c8abac0", sc.SpanID().String())
		}
	})
}
