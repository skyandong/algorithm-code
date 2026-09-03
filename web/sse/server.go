package main

import (
	"fmt"
	stdhttp "net/http"
	"os"
	"time"

	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/middleware/recovery"
	"github.com/go-kratos/kratos/v2/transport/http"
)

func main() {
	logger := log.NewStdLogger(os.Stdout)

	// 创建 HTTP 服务
	httpSrv := http.NewServer(
		http.Address(":8080"),
		http.Middleware(recovery.Recovery()),
	)

	// 注册 SSE 路由
	httpSrv.Handle("/sse", stdhttp.HandlerFunc(sseHandler))

	app := kratos.New(
		kratos.Name("sse-server"),
		kratos.Logger(logger),
		kratos.Server(httpSrv),
	)

	fmt.Println("SSE server listening on http://localhost:8080/sse")
	if err := app.Run(); err != nil {
		log.Error(err)
	}
}

func sseHandler(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	// 设置 SSE 响应头
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// 启用流式写入
	flusher, ok := w.(stdhttp.Flusher)
	if !ok {
		stdhttp.Error(w, "Streaming not supported", stdhttp.StatusInternalServerError)
		return
	}

	// 发送初始注释（可选，用于连接检测）
	fmt.Fprintf(w, ": ping\n\n")
	flusher.Flush()

	// 模拟发送事件
	for i := 0; i < 10; i++ {
		// 发送带事件名称的消息
		fmt.Fprintf(w, "event: message\n")
		fmt.Fprintf(w, "data: Hello from server! Count: %d\n", i+1)
		fmt.Fprintf(w, "\n")
		flusher.Flush()

		time.Sleep(1 * time.Second)
	}

	// 发送最终消息
	fmt.Fprintf(w, "event: close\n")
	fmt.Fprintf(w, "data: Server closing connection\n")
	fmt.Fprintf(w, "\n")
	flusher.Flush()
}
