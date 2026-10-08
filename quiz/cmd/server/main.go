// Command server 启动答题评测服务：导入题库、对外提供 HTTP 接口、托管答题页面。
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"quiz/internal/api"
	"quiz/internal/seed"
	"quiz/internal/store"
)

const defaultDSN = "quiz:quiz123@tcp(127.0.0.1:3306)/quiz?parseTime=true&loc=Local&charset=utf8mb4"

func main() {
	addr := flag.String("addr", ":8090", "HTTP 监听地址")
	dsn := flag.String("dsn", defaultDSN, "MySQL 连接串")
	webDir := flag.String("web", "web", "静态资源目录（答题页面所在位置）")
	seedOnly := flag.Bool("seed-only", false, "只导入题库然后退出")
	flag.Parse()

	ctx := context.Background()

	st, err := store.Open(*dsn)
	if err != nil {
		log.Fatalf("连接数据库失败: %v\n提示：先执行 docker compose up -d mysql 起库。", err)
	}
	defer st.Close()

	if err := st.Migrate(ctx); err != nil {
		log.Fatalf("建表失败: %v", err)
	}

	bank := seed.Bank()
	imported, err := st.UpsertQuestions(ctx, bank)
	if err != nil {
		log.Fatalf("导入题库失败: %v", err)
	}
	total, err := st.CountQuestions(ctx)
	if err != nil {
		log.Fatalf("统计题库失败: %v", err)
	}
	topics, err := st.ListTopics(ctx, api.DefaultUser)
	if err != nil {
		log.Fatalf("统计知识点失败: %v", err)
	}
	log.Printf("题库就绪：本次导入 %d 题，库中合计 %d 题，覆盖 %d 个知识点", imported, total, len(topics))

	if *seedOnly {
		return
	}

	srv := &http.Server{
		Addr:              *addr,
		Handler:           api.New(st, *webDir).Routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("答题服务已启动：http://localhost%s （出题 5 道/轮，按知识点）", *addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP 服务异常退出: %v", err)
		}
	}()

	// 收到中断信号后给在途请求留出收尾时间。
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("关闭服务时出错: %v", err)
	}
	log.Print("服务已停止")
}
