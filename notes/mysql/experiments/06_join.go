// # JOIN 原理实验
//
// 实验项：
//
//	Exp1：Index NLJ — 被驱动表 JOIN 列有二级索引，type=ref
//	Exp2：无索引场景 — 8.0.18+ 自动走 Hash Join
//	Exp3：优化器自动调换驱动表 — 走 class_info 主键 eq_ref
//	Exp4：三表 JOIN 驱动表选择与 STRAIGHT_JOIN 强制顺序
//
// 表结构同时供 07_aggregate.go 复用。

package main

import (
	"fmt"

	"gorm.io/gorm"
)

// Grade 年级表
type Grade struct {
	GradeID   uint   `gorm:"primarykey"`
	GradeName string `gorm:"type:varchar(20);not null"`
}

func (Grade) TableName() string { return "grade" }

// ClassInfo 班级表
type ClassInfo struct {
	ClassID   uint   `gorm:"primarykey"`
	GradeID   uint   `gorm:"not null"`
	ClassName string `gorm:"type:varchar(20);not null"`
}

func (ClassInfo) TableName() string { return "class_info" }

// StudentScore 学生成绩表
type StudentScore struct {
	SID         uint    `gorm:"column:sid;primarykey"` // 显式指定列名，避免 SID 被命名策略映射成 s_id
	ClassID     uint    `gorm:"column:class_id;not null"`
	StudentName string  `gorm:"column:student_name;type:varchar(10)"`
	Score       float64 `gorm:"column:score;type:decimal(5,1)"`
}

func (StudentScore) TableName() string { return "student_score" }

// seedSchoolData 建 grade/class_info/student_score 三张表并写入测试数据
func seedSchoolData(db *gorm.DB) {
	if err := db.AutoMigrate(&Grade{}, &ClassInfo{}, &StudentScore{}); err != nil {
		fmt.Printf("  migrate 失败: %v\n", err)
	}
	db.Where("1 = 1").Delete(&StudentScore{})
	db.Where("1 = 1").Delete(&ClassInfo{})
	db.Where("1 = 1").Delete(&Grade{})

	if err := db.Create(&[]Grade{
		{GradeID: 1, GradeName: "初一"}, {GradeID: 2, GradeName: "初二"},
	}).Error; err != nil {
		fmt.Printf("  seed grade 失败: %v\n", err)
	}
	if err := db.Create(&[]ClassInfo{
		{ClassID: 1, GradeID: 1, ClassName: "初一1班"}, {ClassID: 2, GradeID: 1, ClassName: "初一2班"},
		{ClassID: 3, GradeID: 2, ClassName: "初二1班"}, {ClassID: 4, GradeID: 2, ClassName: "初二2班"},
	}).Error; err != nil {
		fmt.Printf("  seed class_info 失败: %v\n", err)
	}
	if err := db.Create(&[]StudentScore{
		{SID: 1, ClassID: 1, StudentName: "小明", Score: 88},
		{SID: 2, ClassID: 1, StudentName: "小红", Score: 55},
		{SID: 3, ClassID: 1, StudentName: "小刚", Score: 92},
		{SID: 4, ClassID: 2, StudentName: "小李", Score: 76},
		{SID: 5, ClassID: 2, StudentName: "小张", Score: 59},
		{SID: 6, ClassID: 3, StudentName: "小王", Score: 95},
		{SID: 7, ClassID: 3, StudentName: "小刘", Score: 82},
		{SID: 8, ClassID: 4, StudentName: "小陈", Score: 45},
		{SID: 9, ClassID: 4, StudentName: "小赵", Score: 77},
		{SID: 10, ClassID: 4, StudentName: "小孙", Score: 66},
	}).Error; err != nil {
		fmt.Printf("  seed student_score 失败: %v\n", err)
	}
}

// printExplainJoin 打印多表 JOIN 的 EXPLAIN（带 table 列，看清谁驱动谁）
func printExplainJoin(db *gorm.DB, sql string) {
	var rows []ExplainRow
	db.Raw("EXPLAIN " + sql).Scan(&rows)
	for _, r := range rows {
		key := "NULL"
		if r.Key != nil {
			key = *r.Key
		}
		extra := ""
		if r.Extra != nil {
			extra = *r.Extra
		}
		fmt.Printf("  table=%-14s type=%-8s key=%-16s rows=%-5d extra=%s\n", r.Table, r.Type, key, r.Rows, extra)
	}
}

// RunJoinExperiments 运行 JOIN 实验
func RunJoinExperiments(db *gorm.DB) {
	seedSchoolData(db)

	fmt.Println("\n=== 实验1: Index NLJ（被驱动表 JOIN 列有二级索引）===")
	// 手动建索引，名字可控，便于后面 drop
	db.Exec("CREATE INDEX idx_score_class ON student_score(class_id)")
	// STRAIGHT_JOIN 固定 class_info 当驱动表，防止优化器调换驱动表（走 class_info 主键）
	fmt.Println("[有索引] STRAIGHT_JOIN class_info JOIN student_score — 期望被驱动表 type=ref, key=idx_score_class")
	printExplainJoin(db, "SELECT STRAIGHT_JOIN c.class_name, s.student_name FROM class_info c JOIN student_score s ON s.class_id = c.class_id")

	fmt.Println("\n=== 实验2: 无索引场景 Hash Join（8.0.18+）===")
	db.Exec("DROP INDEX idx_score_class ON student_score")
	fmt.Println("[无索引] 同样查询 — 期望被驱动表 Extra=Using join buffer (hash join)")
	printExplainJoin(db, "SELECT STRAIGHT_JOIN c.class_name, s.student_name FROM class_info c JOIN student_score s ON s.class_id = c.class_id")
	db.Exec("CREATE INDEX idx_score_class ON student_score(class_id)")

	fmt.Println("\n=== 实验3: 优化器自动调换驱动表 ===")
	// 不强制顺序时，优化器发现 class_id 是 class_info 的主键，
	// 反过来让 student_score 当驱动表、class_info 走 PRIMARY（type=eq_ref）更优
	fmt.Println("[普通 JOIN] 不加 STRAIGHT_JOIN — 期望 student_score 变驱动表，class_info 走 PRIMARY eq_ref")
	printExplainJoin(db, "SELECT c.class_name, s.student_name FROM class_info c JOIN student_score s ON s.class_id = c.class_id")

	fmt.Println("\n=== 实验4: 驱动表选择 ===")
	fmt.Println("[三表 JOIN] grade 只有 2 行 → 优化器选 grade 当驱动表（从上到下依次驱动）")
	printExplainJoin(db, `SELECT g.grade_name, c.class_name, s.student_name
		FROM grade g
		JOIN class_info c ON c.grade_id = g.grade_id
		JOIN student_score s ON s.class_id = c.class_id`)

	fmt.Println("[STRAIGHT_JOIN] 强制按书写顺序 — class_info 无 grade_id 索引时退化为 hash join")
	printExplainJoin(db, `SELECT STRAIGHT_JOIN g.grade_name, c.class_name, s.student_name
		FROM grade g
		JOIN class_info c ON c.grade_id = g.grade_id
		JOIN student_score s ON s.class_id = c.class_id`)

	fmt.Println("\n=== JOIN 实验结束 ===")
	fmt.Println("关键结论：")
	fmt.Println("  1. 被驱动表 JOIN 列有索引 → Index NLJ（type=ref），最理想")
	fmt.Println("  2. 无索引时 8.0.18+ 自动 Hash Join，替代 BNLJ")
	fmt.Println("  3. 优化器不只选驱动表，还会调换连接方向找最优访问路径（实验3）")
	fmt.Println("  4. STRAIGHT_JOIN 可强制顺序，代价是可能放弃更优计划")
}
