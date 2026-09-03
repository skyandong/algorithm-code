package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
)

// 自动版 demo:业务代码零 trace 调用。入站走 TraceMiddleware,出站用 tracedClient。
// 如需在非 HTTP 场景手动控制 span,可调用 Start()/Span()/Inject()——见 trace.go。

var client = newTracedClient()

func main() {
	slog.SetDefault(slog.New(newTraceHandler(slog.NewJSONHandler(os.Stdout, nil))))
	fmt.Println("--- trace chain demo ---")

	entry := TraceMiddleware(http.HandlerFunc(handleExplain))
	req := httptest.NewRequest(http.MethodGet, "/lesson/explain?lessonId=42", nil)
	entry.ServeHTTP(httptest.NewRecorder(), req)
}

func handleExplain(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	slog.InfoContext(ctx, "service: handle lesson explain", "lessonId", r.URL.Query().Get("lessonId"))

	lesson, err := fetchLesson(ctx, 42)
	if err != nil {
		slog.ErrorContext(ctx, "service: failed", "err", err)
		return
	}
	slog.InfoContext(ctx, "service: got lesson", "title", lesson.Title)
	_ = json.NewEncoder(w).Encode(lesson)
}

func fetchLesson(ctx context.Context, id int) (*Lesson, error) {
	slog.InfoContext(ctx, "biz: fetch lesson from data layer")
	return callContentService(ctx, id)
}

func callContentService(ctx context.Context, id int) (*Lesson, error) {
	slog.InfoContext(ctx, "data: calling downstream content service")

	downstream := httptest.NewServer(TraceMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		slog.InfoContext(r.Context(), "downstream: querying content", "id", r.URL.Query().Get("id"))
		_ = json.NewEncoder(w).Encode(Lesson{ID: id, Title: "围棋入门"})
	})))
	defer downstream.Close()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/?id=%d", downstream.URL, id), nil)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var lesson Lesson
	if err := json.NewDecoder(resp.Body).Decode(&lesson); err != nil {
		return nil, err
	}
	return &lesson, nil
}

type Lesson struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
}
