package main

import (
	"context"
	"log/slog"
)

type traceHandler struct{ base slog.Handler }

func newTraceHandler(base slog.Handler) *traceHandler { return &traceHandler{base: base} }

func (h *traceHandler) Handle(ctx context.Context, r slog.Record) error {
	if tc, ok := From(ctx); ok {
		attrs := []slog.Attr{
			slog.String("trace_id", tc.TraceID),
			slog.String("span_id", tc.SpanID),
		}
		if tc.ParentSpanID != "" {
			attrs = append(attrs, slog.String("parent_span_id", tc.ParentSpanID))
		}
		r.AddAttrs(attrs...)
	}
	return h.base.Handle(ctx, r)
}

func (h *traceHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &traceHandler{base: h.base.WithAttrs(attrs)}
}
func (h *traceHandler) WithGroup(name string) slog.Handler {
	return &traceHandler{base: h.base.WithGroup(name)}
}
func (h *traceHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return h.base.Enabled(ctx, l)
}
