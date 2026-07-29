package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// User 用户模型
type User struct {
	ID        uint      `gorm:"primarykey"`
	Name      string    `gorm:"type:varchar(64);not null;index"`
	Age       int       `gorm:"default:0"`
	Email     string    `gorm:"type:varchar(128);uniqueIndex"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

func main() {
	// 从环境变量读取数据库配置，提供默认值
	dsn := os.Getenv("MYSQL_DSN")
	if dsn == "" {
		dsn = "root:password@tcp(127.0.0.1:3306)/test?charset=utf8mb4&parseTime=True&loc=Local"
	}

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		log.Fatalf("连接数据库失败: %v", err)
	}

	// 自动迁移
	if err := db.AutoMigrate(&User{}); err != nil {
		log.Fatalf("迁移失败: %v", err)
	}
	fmt.Println("✅ 数据库迁移完成")

	// 创建
	user := User{Name: "张三", Age: 25, Email: "zhangsan@example.com"}
	result := db.Create(&user)
	if result.Error != nil {
		log.Fatalf("创建用户失败: %v", result.Error)
	}
	fmt.Printf("✅ 创建用户成功: ID=%d, Name=%s\n", user.ID, user.Name)

	// 查询
	var found User
	db.First(&found, user.ID)
	fmt.Printf("✅ 查询用户: %+v\n", found)

	// 条件查询
	var users []User
	db.Where("age > ?", 20).Find(&users)
	fmt.Printf("✅ 条件查询: 共%d条\n", len(users))

	// 更新
	db.Model(&found).Update("age", 26)
	fmt.Println("✅ 更新年龄成功")

	// 删除
	// db.Delete(&found)
	// fmt.Println("✅ 删除用户成功")
}
