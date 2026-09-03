package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
)

func main() {
	// 发起 SSE 请求
	resp, err := http.Get("http://localhost:8080/sse")
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer resp.Body.Close()

	// 检查响应头
	contentType := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "text/event-stream") {
		log.Fatalf("Expected text/event-stream, got: %s", contentType)
	}

	fmt.Println("Connected to SSE server")
	fmt.Println("------------------------")

	// 读取 SSE 流
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()

		// 解析 SSE 格式
		if strings.HasPrefix(line, "event: ") {
			eventType := strings.TrimPrefix(line, "event: ")
			fmt.Printf("[Event] %s\n", eventType)
		} else if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			fmt.Printf("[Data] %s\n", data)
		} else if line == "" {
			// 空行表示事件结束
			fmt.Println("------------------------")
		} else if strings.HasPrefix(line, ":") {
			// 注释行，通常用于心跳
			fmt.Printf("[Comment] %s\n", line)
		} else {
			fmt.Printf("[Unknown] %s\n", line)
		}
	}

	if err := scanner.Err(); err != nil {
		if err == io.EOF {
			fmt.Println("Server closed connection")
		} else {
			log.Fatalf("Error reading SSE: %v", err)
		}
	}
}