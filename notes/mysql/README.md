# MySQL 学习

GORM vs Ent 对比练习。

```bash
# 设置数据库连接
export MYSQL_DSN="root:password@tcp(127.0.0.1:3306)/test?charset=utf8mb4&parseTime=True&loc=Local"

# 运行 GORM 示例
go run ./gorm/

# 修改 Ent Schema 后重新生成
go generate ./ent/

# 运行 Ent 示例
go run ./ent/cmd/
```

## 目录说明

```
gorm/
  └── main.go          ← GORM 示例，直接 go run

ent/
  ├── schema/          ← 你写的 Schema（改这里）
  ├── cmd/main.go      ← 使用示例
  ├── generate.go      ← go generate 入口
  ├── tools.go         ← 代码生成器依赖
  └── (30+ 生成文件)   ← 从不打开，提交进 git
```
