package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
)

type TraceContext struct {
	TraceID      string
	SpanID       string
	ParentSpanID string
}

type traceKey struct{}

// Start 是请求入口的"根 span"。优先从 header 继承 traceId,没有才新生成。
func Start(ctx context.Context, h http.Header) (context.Context, TraceContext) {
	tc := TraceContext{
		TraceID:      h.Get("X-Trace-Id"),
		SpanID:       newID(),
		ParentSpanID: h.Get("X-Parent-Span-Id"),
	}
	if tc.TraceID == "" {
		tc.TraceID = newID()
	}
	return context.WithValue(ctx, traceKey{}, tc), tc
}

// Span 派生一个新的子 span,traceId 不变,spanId 新生成,parent 指向当前 span。
func Span(ctx context.Context) (context.Context, TraceContext) {
	parent, ok := From(ctx)
	if !ok {
		return Start(ctx, nil)
	}
	child := TraceContext{
		TraceID:      parent.TraceID,
		SpanID:       newID(),
		ParentSpanID: parent.SpanID,
	}
	return context.WithValue(ctx, traceKey{}, child), child
}

// From 从 ctx 取出当前追踪上下文。
func From(ctx context.Context) (TraceContext, bool) {
	tc, ok := ctx.Value(traceKey{}).(TraceContext)
	return tc, ok
}

// Inject 把当前 trace 透传到出站请求的 header。
func Inject(h http.Header, tc TraceContext) {
	h.Set("X-Trace-Id", tc.TraceID)
	h.Set("X-Parent-Span-Id", tc.SpanID)
}

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
