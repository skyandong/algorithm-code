// # 日志体系实验
//
// 日志不能直接"看"，但可以通过行为间接验证：
//
//	Exp1：Redo Log 持久性 — 大批量写入后查询 innodb_os_log_written 观察写入量
//	Exp2：Binlog 格式对比 — 同一条 UPDATE，ROW 和 STATEMENT 格式记录内容差异
//	Exp3：慢查询阈值 — 人为制造慢查询，观察 slow_query_log 是否记录

package main

import (
	"fmt"
	"time"

	"gorm.io/gorm"
)

// LogRecord 用于实验的简单表
type LogRecord struct {
	ID        uint      `gorm:"primarykey"`
	Value     int64     `gorm:"not null"`
	CreatedAt time.Time `gorm:"index"`
}

// RunLogExperiments 运行日志体系实验
func RunLogExperiments(db *gorm.DB) {
	db.AutoMigrate(&LogRecord{})
	db.Where("1 = 1").Delete(&LogRecord{})

	fmt.Println("\n=== 实验1: Redo Log 写入量观察 ===")
	expRedoLogWrite(db)

	fmt.Println("\n=== 实验2: Binlog 格式查看 ===")
	expBinlogFormat(db)

	fmt.Println("\n=== 实验3: 慢查询日志配置查看 ===")
	expSlowQueryLog(db)
}

// expRedoLogWrite 通过批量写入前后对比 innodb_os_log_written，直观感受 Redo Log 写入量
func expRedoLogWrite(db *gorm.DB) {
	var before, after int64

	db.Raw("SHOW GLOBAL STATUS LIKE 'innodb_os_log_written'").Row().Scan(nil, &before)
	fmt.Printf("  写入前 innodb_os_log_written = %d bytes\n", before)

	// 批量写入 1000 行，每行触发一次 Redo Log 写入
	records := make([]LogRecord, 1000)
	for i := range records {
		records[i] = LogRecord{Value: int64(i)}
	}
	db.Create(&records)

	db.Raw("SHOW GLOBAL STATUS LIKE 'innodb_os_log_written'").Row().Scan(nil, &after)
	fmt.Printf("  写入后 innodb_os_log_written = %d bytes\n", after)
	fmt.Printf("  本次写入触发 Redo Log 增量 = %d bytes (%.1f KB)\n", after-before, float64(after-before)/1024)
	fmt.Println("  说明：每次事务提交都追加写 Redo Log（WAL），顺序写极快")
}

// expBinlogFormat 查看当前 Binlog 格式，并展示如何查看最新 Binlog 事件
func expBinlogFormat(db *gorm.DB) {
	var format string
	db.Raw("SHOW VARIABLES LIKE 'binlog_format'").Row().Scan(nil, &format)
	fmt.Printf("  当前 binlog_format = %s\n", format)

	// 写入一行，触发 Binlog 记录
	db.Create(&LogRecord{Value: 9999})
	db.Exec("UPDATE log_records SET value = 8888 WHERE value = 9999")

	// 查看最新 Binlog 文件名
	type BinlogFile struct {
		LogName  string `gorm:"column:Log_name"`
		FileSize int64  `gorm:"column:File_size"`
	}
	var files []BinlogFile
	db.Raw("SHOW BINARY LOGS").Scan(&files)
	if len(files) > 0 {
		latest := files[len(files)-1]
		fmt.Printf("  最新 Binlog 文件: %s (%.1f KB)\n", latest.LogName, float64(latest.FileSize)/1024)
		fmt.Printf("  查看内容: SHOW BINLOG EVENTS IN '%s' LIMIT 10;\n", latest.LogName)
	}

	fmt.Printf("  ROW 格式记录每行变更前后值，STATEMENT 记录 SQL 原文\n")
	fmt.Printf("  生产推荐 ROW：主从一致性有保证，NOW()/UUID() 不会导致从库数据不同\n")
}

// expSlowQueryLog 查看慢查询日志配置，并演示人为触发慢查询
func expSlowQueryLog(db *gorm.DB) {
	type Variable struct {
		VariableName string `gorm:"column:Variable_name"`
		Value        string `gorm:"column:Value"`
	}

	vars := []string{"slow_query_log", "long_query_time", "slow_query_log_file"}
	for _, v := range vars {
		var row Variable
		db.Raw("SHOW VARIABLES LIKE ?", v).Scan(&row)
		fmt.Printf("  %-25s = %s\n", row.VariableName, row.Value)
	}

	fmt.Println("\n  [人为触发慢查询] SELECT SLEEP(2)...")
	start := time.Now()
	db.Exec("SELECT SLEEP(2)")
	fmt.Printf("  耗时 %.1fs，若 long_query_time <= 2 则会被记录到慢查询日志\n", time.Since(start).Seconds())
	fmt.Println("  生产分析工具: mysqldumpslow -s t -t 10 /path/to/slow.log")
}
