package telemetry

import (
	"context"
	"go.opentelemetry.io/otel/trace"
	"log/slog"
	"regexp"
)

// LogHandler is a deliberately restrictive wrapper over the composition's sink.
// It flattens groups and drops unapproved keys/messages, including raw "error".
// It must be the outer handler: later logger.With calls must pass through it.
type logHandler struct {
	next    slog.Handler
	attrs   []slog.Attr
	runtime *Runtime
}

func (r *Runtime) LogHandler(next slog.Handler) slog.Handler {
	return &logHandler{next: next, runtime: r}
}
func (h *logHandler) Enabled(ctx context.Context, l slog.Level) bool { return h.next.Enabled(ctx, l) }
func (h *logHandler) WithGroup(string) slog.Handler                  { return h.WithAttrs(nil) }
func (h *logHandler) WithAttrs(a []slog.Attr) slog.Handler {
	c := *h
	c.attrs = append(append([]slog.Attr(nil), h.attrs...), a...)
	return &c
}
func (h *logHandler) Handle(ctx context.Context, r slog.Record) error {
	message := "bff event"
	if r.Message == "internal error" || r.Message == "store complete" {
		message = r.Message
	}
	out := slog.NewRecord(r.Time, r.Level, message, 0)
	count := 0
	add := func(a slog.Attr) bool {
		if count < 30 && safeLogAttr(a) {
			count++
			out.AddAttrs(a)
		} else {
			h.runtime.rejected.Add(ctx, 1)
		}
		return true
	}
	for _, a := range h.attrs {
		add(a)
	}
	r.Attrs(add)
	sc := trace.SpanContextFromContext(ctx)
	if sc.IsValid() {
		out.AddAttrs(slog.String("trace_id", sc.TraceID().String()), slog.String("span_id", sc.SpanID().String()))
	}
	return h.next.Handle(ctx, out)
}

var correlationID = regexp.MustCompile(`^[0-9a-f]{16}$`)

func safeLogAttr(a slog.Attr) bool {
	if a.Value.Kind() != slog.KindString {
		return false
	}
	switch a.Key {
	case "procedure":
		return a.Value.String() == createProcedure || a.Value.String() == getProcedure
	case "correlation_id":
		return correlationID.MatchString(a.Value.String())
	case "error.type":
		return validOutcome(a.Value.String())
	default:
		return false
	}
}
