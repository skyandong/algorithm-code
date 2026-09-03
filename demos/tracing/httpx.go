package main

import (
	"log/slog"
	"net/http"
)

// TraceMiddleware 是入站 HTTP 边界——从 header 续上/新建根 span 并塞进 ctx。
// 业务 handler 拿到的 ctx 自带 trace,无需显式调用 Start/Span。
func TraceMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, _ := Start(r.Context(), r.Header)
		slog.InfoContext(ctx, "request received", "path", r.URL.Path)
		next.ServeHTTP(w, r.WithContext(ctx))
		slog.InfoContext(ctx, "request done")
	})
}

// tracedTransport 包装 http.RoundTripper,出站时自动派生 span + 注入 trace header。
type tracedTransport struct{ base http.RoundTripper }

func newTracedClient() *http.Client {
	return &http.Client{Transport: &tracedTransport{base: http.DefaultTransport}}
}

func (t *tracedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx, tc := Span(req.Context())
	Inject(req.Header, tc)
	req = req.WithContext(ctx)
	slog.InfoContext(ctx, "outbound http: sending", "url", req.URL.String())
	resp, err := t.base.RoundTrip(req)
	if err != nil {
		slog.ErrorContext(ctx, "outbound http: failed", "err", err)
		return nil, err
	}
	slog.InfoContext(ctx, "outbound http: got response", "status", resp.Status)
	return resp, nil
}
