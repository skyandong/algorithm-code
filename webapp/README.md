# Go Web 项目实战备忘

> 代码生成是止痛药，不是维生素。没病别吃。

## 项目结构

```text
webapp/
├── cmd/server/main.go           # 入口（依赖Inject？手写5行就够了）
├── internal/
│   ├── handler/                 # HTTP 层（Gin）
│   ├── service/                 # 业务逻辑层
│   ├── repository/              # 数据访问层
│   └── data/                    # ★ 只有这里跟 ORM 有关
│       ├── schema/              # 你写的 Schema（3 个文件）
│       └── gen/                 # Ent 生成的代码（从不打开）
├── go.mod
└── go.sum
```

核心原则：**handler → service → repository**，三层，别整 DDD。

## ORM 怎么选

| 框架 | 类型安全 | 代码生成 | 学习曲线 | 适合场景 |
|------|---------|---------|---------|---------|
| **sqlx** | ❌ | ❌ | 低 | 手写 SQL，零黑魔法，**最推荐** |
| **sqlc** | ✅ | ✅ | 中 | SQL 写查询 → 生成类型安全代码 |
| **GORM** | ❌ | ❌ | 低 | 上手快，但运行时反射，坑多 |
| **Ent** | ✅ | ✅ 生成30+文件 | 中高 | 复杂关联、团队协作、类型安全强迫症 |

### Ent 代码生成真相

```text
你写的（3 个文件）：
  schema/user.go
  schema/post.go
  schema/tag.go

生成的（30+ 个文件，从不打开）：
  gen/user.go, gen/user_create.go, gen/user_query.go ...
  gen/post.go, gen/post_create.go, gen/post_query.go ...
  gen/tag.go, gen/tag_create.go, gen/tag_query.go ...
```

**那 30 个生成文件 = GORM 里 `gorm:"type:varchar(64)"` 那些 tag 的编译期等价物。** 习惯就好，你 99% 的时间只碰 `schema/`。

生成代码**提交进 git**，同事 checkout 后 IDE 直接能找到引用，不需要手动生成。

## 联表查询对比

| 操作 | GORM | Ent |
|------|------|-----|
| 联表查文章+作者 | `Preload("Author")` | `WithAuthor()` |
| 查作者时带出文章 | `Preload("Posts")` | `WithPosts()` |
| 从用户跳到文章 | 先查用户，再 for 循环 | `QueryPosts()` 一步到位 |
| 查到的数据 | 运行时反射 map | 编译期 `p.Edges.Author.Username` |

**Preload / WithXxx 就是帮你做 JOIN**，而且自动处理数据去重和 N+1 问题。

## 依赖注入

```go
// 手写 5 行，别上 Wire
func main() {
    db := initDB()
    svc := service.NewUserService(repo.NewUserRepo(db))
    handler.NewUserHandler(svc).Register(r)
    r.Run()
}
```

等你的 `main()` 有 50 行 `NewXxx` 的时候，再考虑 Wire。

## 一句话哲学

> 代码生成是**止痛药**，不是**维生素**。
> 没病别吃。先用手写 SQL + sqlx + Gin 写出项目来，
> 等到痛了（手写 JOIN 太烦 / main 函数太长 / 字段名写错运行时才报错），
> 再决定要不要上 Ent 或 Wire。
