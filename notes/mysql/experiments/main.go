package main

import (
	"fmt"
	"log"
	"os"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func main() {
	dsn := os.Getenv("MYSQL_DSN")
	if dsn == "" {
		dsn = "root:123456@tcp(127.0.0.1:3306)/test?charset=utf8mb4&parseTime=True&loc=Local"
	}

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		log.Fatalf("连接数据库失败: %v", err)
	}

	exp := "index"
	if len(os.Args) > 1 {
		exp = os.Args[1]
	}

	switch exp {
	case "index":
		RunIndexExperiments(db)
	case "transaction":
		RunTransactionExperiments(db)
	case "lock":
		RunLockExperiments(db)
	case "log":
		RunLogExperiments(db)
	case "aggregate":
		RunAggregateExperiments(db)
	default:
		fmt.Println("用法: go run ./experiments/ [index|transaction|lock|log|aggregate]")
	}
}
