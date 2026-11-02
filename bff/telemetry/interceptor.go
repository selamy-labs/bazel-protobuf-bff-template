package telemetry

import (
	connect "connectrpc.com/connect/v2"
	"connectrpc.com/otelconnect"
	"context"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"strings"
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

func (traceParentOnly) Fields() []string { return []string{"traceparent"} }
func (traceParentOnly) Extract(ctx context.Context, c propagation.TextMapCarrier) context.Context {
	return propagation.TraceContext{}.Extract(ctx, propagation.MapCarrier{"traceparent": c.Get("traceparent")})
}
func (traceParentOnly) Inject(ctx context.Context, c propagation.TextMapCarrier) {
	sc := trace.SpanContextFromContext(ctx).WithTraceState(trace.TraceState{})
	propagation.TraceContext{}.Inject(trace.ContextWithSpanContext(ctx, sc), c)
}
