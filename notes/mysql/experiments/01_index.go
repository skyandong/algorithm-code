// # 索引体系实验
//
// 用真实的 EXPLAIN 输出验证索引原理。
//
// 建表结构：
//
//	orders 表：模拟电商订单，覆盖聚簇索引、联合索引、覆盖索引、回表等场景
//
// 实验项：
//
//	Exp1：EXPLAIN 基础 — 全表扫描(ALL) vs 主键查询(const)
//	Exp2：覆盖索引 — Using index vs 回表(Using where)
//	Exp3：联合索引最左前缀 — 哪些查询能命中索引
//	Exp4：索引失效场景 — 函数运算、隐式转换、左模糊

package main

import (
	"fmt"
	"time"

	"gorm.io/gorm"
)

// Order 电商订单表，刻意设计以覆盖多种索引场景
type Order struct {
	ID         uint      `gorm:"primarykey"`                    // 聚簇索引
	UserID     uint      `gorm:"not null;index:idx_user_status"` // 联合索引前缀
	Status     int8      `gorm:"not null;index:idx_user_status"` // 联合索引后缀
	Amount     int64     `gorm:"not null"`                       // 分，避免浮点
	Phone      string    `gorm:"type:varchar(20);not null"`      // 故意不加索引，演示隐式转换
	Remark     string    `gorm:"type:varchar(256)"`
	CreatedAt  time.Time `gorm:"index:idx_created_status,priority:1"` // 联合索引
	StatusCode int8      `gorm:"index:idx_created_status,priority:2"` // 联合索引
}

// ExplainRow EXPLAIN 输出结构，只取关键字段
type ExplainRow struct {
	ID           int     `gorm:"column:id"`
	SelectType   string  `gorm:"column:select_type"`
	Table        string  `gorm:"column:table"`
	Type         string  `gorm:"column:type"`
	PossibleKeys *string `gorm:"column:possible_keys"`
	Key          *string `gorm:"column:key"`
	KeyLen       *string `gorm:"column:key_len"`
	Rows         int64   `gorm:"column:rows"`
	Filtered     float64 `gorm:"column:filtered"`
	Extra        *string `gorm:"column:Extra"`
}

func printExplain(db *gorm.DB, sql string, args ...interface{}) {
	var rows []ExplainRow
	db.Raw("EXPLAIN "+sql, args...).Scan(&rows)
	for _, r := range rows {
		key := "NULL"
		if r.Key != nil {
			key = *r.Key
		}
		extra := ""
		if r.Extra != nil {
			extra = *r.Extra
		}
		fmt.Printf("  type=%-12s key=%-30s rows=%-8d extra=%s\n", r.Type, key, r.Rows, extra)
	}
}

func printExplainWithKeyLen(db *gorm.DB, sql string, args ...interface{}) {
	var rows []ExplainRow
	db.Raw("EXPLAIN "+sql, args...).Scan(&rows)
	for _, r := range rows {
		key := "NULL"
		if r.Key != nil {
			key = *r.Key
		}
		keyLen := "NULL"
		if r.KeyLen != nil {
			keyLen = *r.KeyLen
		}
		extra := ""
		if r.Extra != nil {
			extra = *r.Extra
		}
		fmt.Printf("  type=%-12s key=%-30s key_len=%-6s rows=%-8d extra=%s\n",
			r.Type, key, keyLen, r.Rows, extra)
	}
}

// RunIndexExperiments 运行所有索引实验
func RunIndexExperiments(db *gorm.DB) {
	// 建表
	if err := db.AutoMigrate(&Order{}); err != nil {
		fmt.Printf("migrate error: %v\n", err)
		return
	}

	// 插入测试数据（幂等：先清空）
	db.Where("1 = 1").Delete(&Order{})
	seedOrders(db)

	fmt.Println("\n=== 实验1: 全表扫描(ALL) vs 主键查询(const) ===")
	// 全表扫描：无条件，type=ALL
	fmt.Println("[全表扫描] SELECT * FROM orders")
	printExplain(db, "SELECT * FROM orders")

	// 主键等值：type=const，一次 IO 直接定位
	fmt.Println("[主键查询] SELECT * FROM orders WHERE id = 1")
	printExplain(db, "SELECT * FROM orders WHERE id = ?", 1)

	fmt.Println("\n=== 实验2: 覆盖索引 vs 回表 ===")
	// 回表：查询 amount 不在联合索引内，需回表
	// type=ref，extra 无 Using index → 需回表
	fmt.Println("[回表] SELECT id, amount FROM orders WHERE user_id = 1")
	printExplain(db, "SELECT id, amount FROM orders WHERE user_id = ?", 1)

	// 覆盖索引：只查 user_id、status，这两列都在联合索引 idx_user_status 内
	// extra = Using index → 无需回表
	fmt.Println("[覆盖索引] SELECT user_id, status FROM orders WHERE user_id = 1")
	printExplain(db, "SELECT user_id, status FROM orders WHERE user_id = ?", 1)

	fmt.Println("\n=== 实验3: 联合索引最左前缀 ===")
	// idx_user_status(user_id, status)
	fmt.Println("[命中全部] WHERE user_id=1 AND status=1  — 期望 type=ref, key=idx_user_status")
	printExplain(db, "SELECT id FROM orders WHERE user_id = ? AND status = ?", 1, 1)

	fmt.Println("[命中前缀] WHERE user_id=1  — 期望 type=ref, key=idx_user_status")
	printExplain(db, "SELECT id FROM orders WHERE user_id = ?", 1)

	fmt.Println("[跳过前缀] WHERE status=1  — 期望 type=ALL（跳过 user_id，索引失效）")
	printExplain(db, "SELECT id FROM orders WHERE status = ?", 1)

	fmt.Println("\n=== 实验4: 索引失效场景 ===")
	// 函数运算：对 created_at 做 DATE() 函数，索引失效
	fmt.Println("[函数运算] WHERE DATE(created_at) = '2024-01-01'  — 期望 type=ALL")
	printExplain(db, "SELECT id FROM orders WHERE DATE(created_at) = '2024-01-01'")

	// 正确写法：范围条件，索引生效
	fmt.Println("[正确范围] WHERE created_at BETWEEN '2024-01-01' AND '2024-01-02'  — 期望 type=range")
	printExplain(db, "SELECT id FROM orders WHERE created_at BETWEEN ? AND ?",
		"2024-01-01 00:00:00", "2024-01-02 00:00:00")

	// 隐式类型转换
	// phone 是 varchar，先加索引，再分别传字符串和数字
	db.Exec("ALTER TABLE orders ADD INDEX idx_phone(phone)")
	fmt.Println("[隐式转换-失效] phone(varchar) 传数字 → MySQL CAST 整列，索引失效 — 期望 key=NULL")
	printExplain(db, "SELECT id FROM orders WHERE phone = ?", 13800138001) // int，触发列 CAST
	fmt.Println("[正确-生效]    phone(varchar) 传字符串 → 无转换，索引正常 — 期望 key=idx_phone")
	printExplain(db, "SELECT id FROM orders WHERE phone = ?", "13800130001") // string，正常
	db.Exec("ALTER TABLE orders DROP INDEX idx_phone")

	// user_id 是 uint，传字符串 '1' → MySQL CAST 常量，列不变，索引照常
	fmt.Println("[int列传字符串] user_id(uint) 传 '1' → CAST 常量，索引正常 — 期望 key=idx_user_status")
	printExplain(db, "SELECT id FROM orders WHERE user_id = '1'")

	// 左模糊：LIKE '%xxx'，索引无法确定起始键
	// 先给 remark 加临时索引演示（实际项目中 remark 一般不加索引）
	db.Exec("ALTER TABLE orders ADD INDEX idx_remark(remark(64))")
	fmt.Println("[左模糊] WHERE remark LIKE '%test'  (期望 type=ALL)")
	printExplain(db, "SELECT id FROM orders WHERE remark LIKE '%test'")
	fmt.Println("[右前缀] WHERE remark LIKE 'test%'  (期望 type=range，前缀可以走索引)")
	printExplain(db, "SELECT id FROM orders WHERE remark LIKE 'test%'")
	db.Exec("ALTER TABLE orders DROP INDEX idx_remark") // 清理

	fmt.Println("\n=== 实验5: 最左前缀 key_len 验证 ===")
	// idx_created_status(created_at, status_code)
	// created_at: datetime = 5 字节；status_code: tinyint = 1 字节
	// key_len=5  → 只用了 created_at
	// key_len=6  → 用了 created_at + status_code
	fmt.Println("[只用前缀] WHERE created_at BETWEEN ... — 期望 key_len=5")
	printExplainWithKeyLen(db, "SELECT id FROM orders WHERE created_at BETWEEN ? AND ?",
		"2024-01-01", "2024-01-10")

	fmt.Println("[用全部列] WHERE created_at BETWEEN ... AND status_code=1 — 期望 key_len=6")
	printExplainWithKeyLen(db, "SELECT id FROM orders WHERE created_at BETWEEN ? AND ? AND status_code = ?",
		"2024-01-01", "2024-01-10", 1)

	fmt.Println("[跳过前缀] WHERE status_code=1 — 期望 key=NULL（索引失效）")
	printExplainWithKeyLen(db, "SELECT id FROM orders WHERE status_code = ?", 1)

	fmt.Println("\n=== 实验6: 回表 vs 覆盖索引（扩展索引消灭回表）===")
	fmt.Println("[回表] SELECT user_id, status, amount WHERE user_id=1（amount 不在索引内）")
	printExplain(db, "SELECT user_id, status, amount FROM orders WHERE user_id = ?", 1)

	db.Exec("ALTER TABLE orders ADD INDEX idx_user_status_amount(user_id, status, amount)")
	fmt.Println("[覆盖索引] 加了 idx_user_status_amount 后，同样查询无需回表 — 期望 Extra=Using index")
	printExplain(db, "SELECT user_id, status, amount FROM orders WHERE user_id = ?", 1)
	db.Exec("ALTER TABLE orders DROP INDEX idx_user_status_amount")

	fmt.Println("\n=== 实验7: 联合索引列顺序对 key_len 的影响 ===")
	// 验证：等值列在前 vs 范围列在前，key_len 差异
	// 查询：WHERE user_id=1 AND created_at > '2024-01-01'

	// 顺序一：(user_id, created_at) — 等值在前
	// user_id uint=4B, created_at datetime=5B, key_len 期望=9
	db.Exec("ALTER TABLE orders ADD INDEX idx_uid_cat(user_id, created_at)")
	fmt.Println("[等值在前] idx(user_id, created_at)，WHERE user_id=1 AND created_at>... — 期望 key_len=9")
	printExplainWithKeyLen(db, "SELECT id FROM orders WHERE user_id=? AND created_at>?", 1, "2024-01-01")
	db.Exec("ALTER TABLE orders DROP INDEX idx_uid_cat")

	// 顺序二：(created_at, user_id) — 范围在前，user_id 被截断
	// key_len 期望=5（只用到 created_at）
	db.Exec("ALTER TABLE orders ADD INDEX idx_cat_uid(created_at, user_id)")
	fmt.Println("[范围在前] idx(created_at, user_id)，WHERE user_id=1 AND created_at>... — 期望 key_len=5")
	printExplainWithKeyLen(db, "SELECT id FROM orders WHERE user_id=? AND created_at>?", 1, "2024-01-01")
	db.Exec("ALTER TABLE orders DROP INDEX idx_cat_uid")

	fmt.Println("\n=== 实验9: 索引选择性（Cardinality）===")
	// 选择性 = COUNT(DISTINCT col) / COUNT(*)，越接近 1 越值得建索引
	// status 只有 0/1/2，选择性极低；user_id 有 10 个值；id 唯一，选择性=1
	type CardRow struct {
		Col         string  `gorm:"column:col"`
		Cardinality float64 `gorm:"column:cardinality"`
	}
	var cards []CardRow
	db.Raw(`
		SELECT 'id'      AS col, COUNT(DISTINCT id)      / COUNT(*) AS cardinality FROM orders
		UNION ALL
		SELECT 'user_id' AS col, COUNT(DISTINCT user_id) / COUNT(*) AS cardinality FROM orders
		UNION ALL
		SELECT 'status'  AS col, COUNT(DISTINCT status)  / COUNT(*) AS cardinality FROM orders
	`).Scan(&cards)
	fmt.Println("列选择性（越接近 1 越适合建索引）：")
	for _, c := range cards {
		bar := ""
		for i := 0; i < int(c.Cardinality*20); i++ {
			bar += "█"
		}
		fmt.Printf("  %-10s %.4f  %s\n", c.Col, c.Cardinality, bar)
	}
	fmt.Println("  → status 选择性极低，建索引意义不大；id 唯一，选择性=1，最适合")

	fmt.Println("\n=== 索引实验结束 ===")
	fmt.Println("关键结论：")
	fmt.Println("  1. type 从好到坏: const > eq_ref > ref > range > index > ALL")
	fmt.Println("  2. Extra=Using index 表示覆盖索引，无回表")
	fmt.Println("  3. 联合索引必须从最左列开始连续使用，跳过则失效")
	fmt.Println("  4. 函数/隐式转换/左模糊 会让索引失效")
	fmt.Println("  5. 选择性 COUNT(DISTINCT col)/COUNT(*) < 0.1 的列，建索引性价比低")
}

func seedOrders(db *gorm.DB) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	orders := make([]Order, 0, 100)
	for i := 1; i <= 100; i++ {
		orders = append(orders, Order{
			UserID:     uint(i%10 + 1), // user_id 1-10
			Status:     int8(i % 3),
			Amount:     int64(i * 100),
			Phone:      fmt.Sprintf("1380013%04d", i),
			Remark:     fmt.Sprintf("test-order-%d", i),
			CreatedAt:  now.Add(time.Duration(i) * time.Hour),
			StatusCode: int8(i % 3),
		})
	}
	db.Create(&orders)
}
