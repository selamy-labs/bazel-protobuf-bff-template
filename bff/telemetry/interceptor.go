package telemetry

import (
	"context"
	"encoding/binary"
	"strconv"
	"strings"

	connect "connectrpc.com/connect/v2"
	"connectrpc.com/otelconnect"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

const createProcedure = "/template.notes.v1.NoteService/CreateNote"
const getProcedure = "/template.notes.v1.NoteService/GetNote"

// Interceptor must be outside transport masking, so it sees the mapped public
// outcome. It starts a fresh local root and links public remote context.
func (r *Runtime) Interceptor() (connect.ServerInterceptor, error) {
	traced, err := otelconnect.NewServerInterceptor(otelconnect.WithTracerProvider(r.tp), otelconnect.WithMeterProvider(r.mp), otelconnect.WithRPCSystem(otelconnect.ConnectRPCSystem), otelconnect.WithPropagator(traceParentOnly{}), otelconnect.WithFilter(func(_ context.Context, s connect.Spec) bool {
		return s.Procedure == createProcedure || s.Procedure == getProcedure
	}), otelconnect.WithAttributeFilter(func(_ connect.Spec, a attribute.KeyValue) bool { return allowedAttribute(a) }), otelconnect.WithDurationHistogramOptions(metric.WithExplicitBucketBoundaries(r.cfg.Buckets...)))
	if err != nil {
		return nil, err
	}
	return func(next connect.ServerFunc) connect.ServerFunc {
		measured := traced(func(ctx context.Context, s connect.Spec, stream connect.ServerStream) error {
			err := next(ctx, s, stream)
			if err != nil {
				r.failures.Add(ctx, 1, metric.WithAttributes(attribute.String("rpc.method", s.Procedure[1:]), attribute.String("error.type", strings.ToUpper(connect.CodeOf(err).String()))))
			}
			return err
		})
		return func(ctx context.Context, s connect.Spec, stream connect.ServerStream) error {
			// Clear even a parent injected by outer HTTP middleware. otelconnect's default
			// link rule alone does not clear a preexisting span context.
			if s.Procedure != createProcedure && s.Procedure != getProcedure {
				return next(ctx, s, stream)
			}
			return measured(trace.ContextWithSpanContext(ctx, trace.SpanContext{}), s, stream)
		}
	}, nil
}

// No baggage or tracestate is extracted, even into links.
type traceParentOnly struct{}

func (traceParentOnly) Fields() []string { return []string{"traceparent", "x-cloud-trace-context"} }
func (traceParentOnly) Extract(ctx context.Context, c propagation.TextMapCarrier) context.Context {
	tp := c.Get("traceparent")
	if strings.TrimSpace(tp) != "" {
		return propagation.TraceContext{}.Extract(ctx, propagation.MapCarrier{"traceparent": tp})
	}
	header := c.Get("x-cloud-trace-context")
	if header == "" {
		header = c.Get("X-Cloud-Trace-Context")
	}
	if strings.TrimSpace(header) == "" {
		return ctx
	}
	sc, ok := parseCloudTraceContext(header)
	if !ok || !sc.IsValid() {
		return ctx
	}
	return trace.ContextWithRemoteSpanContext(ctx, sc)
}
func (traceParentOnly) Inject(ctx context.Context, c propagation.TextMapCarrier) {
	sc := trace.SpanContextFromContext(ctx).WithTraceState(trace.TraceState{})
	propagation.TraceContext{}.Inject(trace.ContextWithSpanContext(ctx, sc), c)
}

func parseCloudTraceContext(header string) (trace.SpanContext, bool) {
	mainPart, optPart, hasOpts := strings.Cut(header, ";")
	traceIDStr, spanIDStr, hasSlash := strings.Cut(mainPart, "/")
	if !hasSlash || len(traceIDStr) != 32 || len(spanIDStr) == 0 {
		return trace.SpanContext{}, false
	}
	for i := 0; i < len(traceIDStr); i++ {
		c := traceIDStr[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return trace.SpanContext{}, false
		}
	}
	traceID, err := trace.TraceIDFromHex(traceIDStr)
	if err != nil || !traceID.IsValid() {
		return trace.SpanContext{}, false
	}

	spanID, ok := parseCloudSpanID(spanIDStr)
	if !ok || !spanID.IsValid() {
		return trace.SpanContext{}, false
	}

	flags := trace.TraceFlags(0)
	if hasOpts {
		f, ok := parseCloudTraceOptions(optPart)
		if !ok {
			return trace.SpanContext{}, false
		}
		flags = f
	}

	return trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: flags,
		Remote:     true,
	}), true
}

func parseCloudSpanID(s string) (trace.SpanID, bool) {
	if len(s) == 0 {
		return trace.SpanID{}, false
	}
	val, err := strconv.ParseUint(s, 10, 64)
	if err == nil {
		if val == 0 {
			return trace.SpanID{}, false
		}
		var spanID trace.SpanID
		binary.BigEndian.PutUint64(spanID[:], val)
		return spanID, true
	}
	if len(s) == 16 && isLowerHex(s) {
		sid, err := trace.SpanIDFromHex(s)
		if err == nil && sid.IsValid() {
			return sid, true
		}
	}
	return trace.SpanID{}, false
}

func isLowerHex(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

func parseCloudTraceOptions(opt string) (trace.TraceFlags, bool) {
	if !strings.HasPrefix(opt, "o=") {
		return 0, false
	}
	val := opt[2:]
	if val == "1" {
		return trace.FlagsSampled, true
	}
	if val == "0" {
		return 0, true
	}
	num, err := strconv.ParseUint(val, 10, 64)
	if err != nil {
		return 0, false
	}
	if num&1 != 0 {
		return trace.FlagsSampled, true
	}
	return 0, true
}
