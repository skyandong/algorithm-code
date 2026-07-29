package main

import (
	"log"
	"os"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/gin-gonic/gin"
	_ "github.com/go-sql-driver/mysql"

	"github.com/skyandong/go-code/webapp/internal/data/gen"
	"github.com/skyandong/go-code/webapp/internal/handler"
	"github.com/skyandong/go-code/webapp/internal/repository"
	"github.com/skyandong/go-code/webapp/internal/service"
)

func main() {
	// ---- 1. 数据库连接 ----
	dsn := os.Getenv("MYSQL_DSN")
	if dsn == "" {
		dsn = "root:password@tcp(127.0.0.1:3306)/test?charset=utf8mb4&parseTime=True&loc=Local"
	}
	drv, err := entsql.Open(dialect.MySQL, dsn)
	if err != nil {
		log.Fatalf("数据库连接失败: %v", err)
	}
	defer drv.Close()

	// ---- 2. Ent Client ----
	client := gen.NewClient(gen.Driver(drv))
	defer client.Close()

	// 自动迁移
	if err := client.Schema.Create(nil); err != nil {
		log.Printf("⚠️  迁移失败（首次运行可忽略）: %v", err)
	}

	// ---- 3. 依赖注入 ----
	userRepo := repository.NewUserRepo(client)
	postRepo := repository.NewPostRepo(client)

	userSvc := service.NewUserService(userRepo)
	postSvc := service.NewPostService(postRepo, userRepo)

	userH := handler.NewUserHandler(userSvc)
	postH := handler.NewPostHandler(postSvc)

	// ---- 4. 路由注册 ----
	r := gin.Default()

	api := r.Group("/api/v1")
	{
		api.POST("/users", userH.Create)
		api.GET("/users/:id", userH.GetByID)
		api.GET("/users", userH.List)
		api.DELETE("/users/:id", userH.Delete)

		api.POST("/posts", postH.Create)
		api.GET("/posts/:id", postH.GetByID)
		api.GET("/posts", postH.ListWithAuthor)
		api.DELETE("/posts/:id", postH.Delete)
	}

	// ---- 5. 启动 ----
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}
	log.Printf("🚀 启动服务器: %s", addr)
	if err := r.Run(addr); err != nil {
		log.Fatalf("启动失败: %v", err)
	}
}
