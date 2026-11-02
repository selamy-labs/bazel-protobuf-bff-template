package telemetry

import (
	"context"
	"errors"
	"example.com/bfftemplate/bff/ports"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	"time"
)

type store struct {
	next    ports.Store
	runtime *Runtime
}

func (r *Runtime) DecorateStore(next ports.Store) (ports.Store, error) {
	if next == nil {
		return nil, errors.New("telemetry.store: required")
	}
	return &store{next: next, runtime: r}, nil
}
func (s *store) Put(ctx context.Context, n ports.Note) (err error) {
	ctx, end := s.start(ctx, "put")
	defer func() { end(err) }()
	return s.next.Put(ctx, n)
}
func (s *store) Get(ctx context.Context, owner, id string) (n ports.Note, err error) {
	ctx, end := s.start(ctx, "get")
	defer func() { end(err) }()
	return s.next.Get(ctx, owner, id)
}
func (s *store) start(ctx context.Context, operation string) (context.Context, func(error)) {
	start := time.Now()
	a := attribute.String("bff.operation", operation)
	ctx, span := s.runtime.tracer.Start(ctx, "store."+operation, trace.WithSpanKind(trace.SpanKindInternal), trace.WithAttributes(a))
	return ctx, func(err error) {
		defer span.End()
		outcome := storeOutcome(err)
		s.runtime.latency.Record(ctx, time.Since(start).Seconds(), metric.WithAttributes(a))
		if err != nil {
			kind := attribute.String("error.type", outcome)
			span.SetAttributes(kind)
			span.SetStatus(codes.Error, "")
			s.runtime.failures.Add(ctx, 1, metric.WithAttributes(a, kind))
		}
	}
}
func storeOutcome(err error) string {
	switch {
	case err == nil:
		return "OK"
	case errors.Is(err, ports.ErrUnavailable):
		return "UNAVAILABLE"
	case errors.Is(err, ports.ErrNotFound):
		return "NOT_FOUND"
	case errors.Is(err, context.Canceled):
		return "CANCELED"
	case errors.Is(err, context.DeadlineExceeded):
		return "DEADLINE_EXCEEDED"
	default:
		return "INTERNAL"
	}
}
