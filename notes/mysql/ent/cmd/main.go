package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "github.com/go-sql-driver/mysql"

	"github.com/skyandong/go-code/notes/mysql/ent"
	"github.com/skyandong/go-code/notes/mysql/ent/post"
	"github.com/skyandong/go-code/notes/mysql/ent/user"
)

func main() {
	ctx := context.Background()

	dsn := os.Getenv("MYSQL_DSN")
	if dsn == "" {
		dsn = "root:password@tcp(127.0.0.1:3306)/test?charset=utf8mb4&parseTime=True&loc=Local"
	}

	drv, err := entsql.Open(dialect.MySQL, dsn)
	if err != nil {
		log.Fatalf("连接数据库失败: %v", err)
	}
	defer drv.Close()

	client := ent.NewClient(ent.Driver(drv))
	defer client.Close()

	// ---- 1. 自动迁移 ----
	if err := client.Schema.Create(ctx); err != nil {
		log.Fatalf("迁移失败: %v", err)
	}
	fmt.Println("✅ 迁移完成")

	// ---- 2. 创建关联数据 ----
	// 创建作者
	author, err := client.User.Create().
		SetUsername("张三").
		SetEmail("zhangsan@example.com").
		SetAge(28).
		Save(ctx)
	if err != nil {
		log.Fatalf("创建用户失败: %v", err)
	}
	fmt.Printf("✅ 创建用户: %s (ID=%d)\n", author.Username, author.ID)

	// 创建标签
	tagGo, _ := client.Tag.Create().SetName("Go").Save(ctx)
	tagEnt, _ := client.Tag.Create().SetName("Ent").Save(ctx)
	fmt.Printf("✅ 创建标签: %s, %s\n", tagGo.Name, tagEnt.Name)

	// 创建文章（关联作者和标签）
	article, err := client.Post.Create().
		SetTitle("Ent 入门指南").
		SetContent("Ent 是 Facebook 开源的 Go 实体框架...").
		SetCreatedAt(time.Now()).
		SetAuthor(author).              // 关联作者
		AddTags(tagGo, tagEnt).         // 关联标签
		Save(ctx)
	if err != nil {
		log.Fatalf("创建文章失败: %v", err)
	}
	fmt.Printf("✅ 创建文章: %s (ID=%d)\n", article.Title, article.ID)

	// ---- 3. 查询 + 关联加载（Eager Loading） ----
	// 查文章，同时把作者和标签一起查出来（避免 N+1）
	articles, err := client.Post.Query().
		WithAuthor().              // 预加载作者
		WithTags().                // 预加载标签
		Where(post.HasAuthor()).   // 有作者的文章
		Order(ent.Desc(post.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		log.Fatalf("查询文章失败: %v", err)
	}

	for _, p := range articles {
		fmt.Printf("\n📝 %s\n", p.Title)
		fmt.Printf("   作者: %s\n", p.Edges.Author.Username)
		fmt.Printf("   标签: ")
		for _, t := range p.Edges.Tags {
			fmt.Printf("#%s ", t.Name)
		}
		fmt.Println()
	}

	// ---- 4. 图遍历：从作者查他的所有文章 ----
	posts, err := client.User.Query().
		Where(user.UsernameEQ("张三")).
		QueryPosts().              // 沿着 Edge 导航到 Post
		All(ctx)
	if err != nil {
		log.Fatalf("查询用户文章失败: %v", err)
	}
	fmt.Printf("\n📊 张三共写了 %d 篇文章\n", len(posts))

	// ---- 5. 更新 ----
	client.Post.UpdateOneID(article.ID).
		SetTitle("Ent 入门指南（已更新）").
		Exec(ctx)
	fmt.Println("✅ 文章标题已更新")

	// ---- 6. 删除 ----
	// client.Post.DeleteOne(article).Exec(ctx)
	// fmt.Println("✅ 文章已删除")
}
