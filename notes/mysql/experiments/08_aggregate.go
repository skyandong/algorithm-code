// 聚合查询练习实验
//
// 把一份 SQL 聚合查询练习题（年级-班级-学生成绩）做成可跑实验：
// 每道题的 SQL 真实执行并打印结果表，找茬题先跑"错误 SQL"再跑"修正 SQL"对比输出，
// 你能直观看到 WHERE 用聚合字段报语法错、COUNT(字段) 漏 NULL、INNER JOIN 丢维度等坑。
//
// schema:
//
//	grade        (grade_id, grade_name)                       年级: 初一/初二
//	class_info   (class_id, grade_id, class_name)             班级: 每年级 2 个班
//	student_score(sid, class_id, student_name, score)         学生成绩, score 可 NULL(缺考)
//
// 实验项:
//
//	基础 5 道: COUNT/SUM/AVG、LEFT JOIN 保留维度、HAVING 过滤聚合、RANK() OVER PARTITION BY、CASE WHEN 分段
//	找茬 5 道: 错误 SQL vs 修正 SQL 对比输出
//	拔高:     ROLLUP 多级汇总、本班平均分关联子查询
package main

import (
	"fmt"
	"os"
	"strings"

	"gorm.io/gorm"
)

// Grade 年级表
type Grade struct {
	GradeID   uint   `gorm:"primarykey;column:grade_id"`
	GradeName string `gorm:"type:varchar(20);not null;column:grade_name"`
}

func (Grade) TableName() string { return "grade" }

// ClassInfo 班级表
type ClassInfo struct {
	ClassID   uint   `gorm:"primarykey;column:class_id"`
	GradeID   uint   `gorm:"not null;column:grade_id"`
	ClassName string `gorm:"type:varchar(20);not null;column:class_name"`
}

func (ClassInfo) TableName() string { return "class_info" }

// StudentScore 学生成绩表, score 可为 NULL(缺考)
type StudentScore struct {
	SID         uint     `gorm:"primarykey;column:sid"`
	ClassID     uint     `gorm:"not null;column:class_id"`
	StudentName string   `gorm:"column:student_name"`
	Score       *float64 `gorm:"type:decimal(5,1);column:score"` // 指针 → NULL 可表达
}

func (StudentScore) TableName() string { return "student_score" }

// RunAggregateExperiments 运行聚合查询实验
func RunAggregateExperiments(db *gorm.DB) {
	if err := db.AutoMigrate(&Grade{}, &ClassInfo{}, &StudentScore{}); err != nil {
		fmt.Printf("migrate error: %v\n", err)
		return
	}
	seedAggregate(db)

	fmt.Println("\n=== schema & 数据概览 ===")
	printSection("grade")
	runQuery(db, "SELECT * FROM grade ORDER BY grade_id")
	printSection("class_info")
	runQuery(db, "SELECT * FROM class_info ORDER BY class_id")
	printSection("student_score")
	runQuery(db, "SELECT * FROM student_score ORDER BY sid")

	// ============ 第一部分: 基础练习题 ============
	fmt.Println("\n" + strings.Repeat("=", 70))
	fmt.Println("第一部分: 基础练习题")
	fmt.Println(strings.Repeat("=", 70))

	fmt.Println("\n--- 习题 1: 每班 总人数/总分/平均分/不及格人数, 没学生的班级也要展示 ---")
	runQuery(db, `
SELECT
    c.class_name,
    COUNT(*) AS total_num,
    IFNULL(SUM(s.score),0) total_score,
    ROUND(IFNULL(AVG(s.score),0),2) avg_score,
    SUM(IF(s.score<60,1,0)) fail_count
FROM class_info c
LEFT JOIN student_score s ON c.class_id = s.class_id
GROUP BY c.class_id, c.class_name
ORDER BY c.class_id`)

	fmt.Println("\n--- 习题 2: 班级平均分>=70 的班级, 附带年级名 ---")
	runQuery(db, `
SELECT
    g.grade_name,
    c.class_name,
    COUNT(*) student_count,
    ROUND(AVG(s.score),2) avg_score
FROM grade g
JOIN class_info c ON g.grade_id = c.grade_id
LEFT JOIN student_score s ON c.class_id = s.class_id
GROUP BY g.grade_id, g.grade_name, c.class_id, c.class_name
HAVING avg_score >= 70
ORDER BY c.class_id`)

	fmt.Println("\n--- 习题 3: 每年级 总人数/平均分/不及格总人数/不及格占比 ---")
	runQuery(db, `
SELECT
    g.grade_name,
    COUNT(s.sid) grade_total_stu,
    ROUND(AVG(s.score),2) grade_avg,
    SUM(IF(s.score<60,1,0)) grade_fail_num,
    ROUND( SUM(IF(s.score<60,1,0)) / COUNT(s.sid)*100 ,1 ) fail_rate
FROM grade g
JOIN class_info c ON g.grade_id = c.grade_id
LEFT JOIN student_score s ON c.class_id = s.class_id
GROUP BY g.grade_id, g.grade_name
ORDER BY g.grade_id`)

	fmt.Println("\n--- 习题 4: 年级内按班级平均分 RANK, 只展示年级前 2 ---")
	runQuery(db, `
SELECT * FROM (
    SELECT
        g.grade_name,
        c.class_name,
        ROUND(AVG(s.score),2) class_avg,
        RANK() OVER(PARTITION BY g.grade_id ORDER BY AVG(s.score) DESC) rank_no
    FROM grade g
    JOIN class_info c ON g.grade_id = c.grade_id
    LEFT JOIN student_score s ON c.class_id = s.class_id
    GROUP BY g.grade_id, g.grade_name, c.class_id, c.class_name
) t
WHERE t.rank_no <= 2
ORDER BY t.grade_name, t.rank_no`)

	fmt.Println("\n--- 习题 5: 每班分段统计 <60 / 60-79 / 80-89 / >=90 ---")
	runQuery(db, `
SELECT
    c.class_name,
    SUM(CASE WHEN s.score <60 THEN 1 ELSE 0 END) less60,
    SUM(CASE WHEN s.score BETWEEN 60 AND 79 THEN 1 ELSE 0 END) score60_79,
    SUM(CASE WHEN s.score BETWEEN 80 AND 89 THEN 1 ELSE 0 END) score80_89,
    SUM(CASE WHEN s.score >=90 THEN 1 ELSE 0 END) more90
FROM class_info c
LEFT JOIN student_score s ON c.class_id = s.class_id
GROUP BY c.class_id, c.class_name
ORDER BY c.class_id`)

	// ============ 第二部分: 找茬错题训练 ============
	fmt.Println("\n" + strings.Repeat("=", 70))
	fmt.Println("第二部分: 找茬错题训练 (错误 SQL vs 修正 SQL 对比)")
	fmt.Println(strings.Repeat("=", 70))

	fmt.Println("\n--- 错题 1: WHERE 用聚合字段(应改 HAVING) ---")
	fmt.Println("[错误 SQL] WHERE avg_score >= 70 — 期望: 语法错误(未知列 avg_score)")
	runQuery(db, `
SELECT
    g.grade_name,
    c.class_name,
    COUNT(s.score) AS student_num,
    ROUND(AVG(s.score),2) avg_score
FROM grade g
JOIN class_info c ON g.grade_id = c.grade_id
LEFT JOIN student_score s ON c.class_id = s.class_id
WHERE avg_score >= 70
GROUP BY g.grade_name, c.class_name`)
	fmt.Println("[修正 SQL] WHERE → HAVING, COUNT(s.score) → COUNT(*), GROUP BY 补主键")
	runQuery(db, `
SELECT
    g.grade_name,
    c.class_name,
    COUNT(*) AS student_num,
    ROUND(AVG(s.score),2) avg_score
FROM grade g
JOIN class_info c ON g.grade_id = c.grade_id
LEFT JOIN student_score s ON c.class_id = s.class_id
GROUP BY g.grade_id, g.grade_name, c.class_id, c.class_name
HAVING avg_score >= 70
ORDER BY c.class_id`)

	fmt.Println("\n--- 错题 2: INNER JOIN 丢维度 + 除零风险 ---")
	fmt.Println("[错误 SQL] INNER JOIN 丢无学生班级, COUNT(s.sid) 漏 NULL, 除零风险")
	runQuery(db, `
SELECT
    g.grade_name,
    COUNT(s.sid) total_stu,
    SUM(IF(score<60,1,0)) fail_num,
    ROUND(SUM(IF(score<60,1,0)) / COUNT(s.sid)*100,1) fail_rate
FROM grade g
INNER JOIN class_info c ON g.grade_id=c.grade_id
INNER JOIN student_score s ON c.class_id=s.class_id
GROUP BY g.grade_name
ORDER BY g.grade_id`)
	fmt.Println("[修正 SQL] LEFT JOIN 保留维度, COUNT(*) 统计人数")
	runQuery(db, `
SELECT
    g.grade_name,
    COUNT(*) total_stu,
    SUM(IF(s.score<60,1,0)) fail_num,
    ROUND(SUM(IF(s.score<60,1,0)) / COUNT(s.sid)*100,1) fail_rate
FROM grade g
JOIN class_info c ON g.grade_id=c.grade_id
LEFT JOIN student_score s ON c.class_id=s.class_id
GROUP BY g.grade_id, g.grade_name
ORDER BY g.grade_id`)

	fmt.Println("\n--- 错题 3: 窗口函数缺 PARTITION BY 变全校排名 ---")
	fmt.Println("[错误 SQL] RANK() OVER(ORDER BY ...) 缺 PARTITION BY — 变全校排名")
	runQuery(db, `
SELECT
    g.grade_name,
    c.class_name,
    ROUND(AVG(s.score),2) avg_score,
    RANK() OVER(ORDER BY AVG(s.score) DESC) rn
FROM grade g
JOIN class_info c ON g.grade_id=c.grade_id
LEFT JOIN student_score s ON c.class_id=s.class_id
GROUP BY g.grade_id, g.grade_name, c.class_id, c.class_name
ORDER BY g.grade_id, c.class_id`)
	fmt.Println("[修正 SQL] 加 PARTITION BY g.grade_id — 分年级排名")
	runQuery(db, `
SELECT
    g.grade_name,
    c.class_name,
    ROUND(AVG(s.score),2) avg_score,
    RANK() OVER(PARTITION BY g.grade_id ORDER BY AVG(s.score) DESC) rn
FROM grade g
JOIN class_info c ON g.grade_id=c.grade_id
LEFT JOIN student_score s ON c.class_id=s.class_id
GROUP BY g.grade_id, g.grade_name, c.class_id, c.class_name
ORDER BY g.grade_id, rn`)

	fmt.Println("\n--- 错题 4: ROLLUP 汇总行 NULL 没兜底 + COUNT(s.sid) 不准 ---")
	fmt.Println("[错误 SQL] 汇总行 grade_name/class_name 显示 NULL 不友好")
	runQuery(db, `
SELECT
    g.grade_name,
    c.class_name,
    COUNT(s.sid) people_num,
    ROUND(AVG(s.score), 2) avg_score
FROM grade g
LEFT JOIN class_info c ON g.grade_id = c.grade_id
LEFT JOIN student_score s ON c.class_id = s.class_id
GROUP BY g.grade_name, c.class_name WITH ROLLUP`)
	fmt.Println("[修正 SQL] IFNULL 兜底汇总行名称 + COUNT(*)")
	runQuery(db, `
SELECT
    IFNULL(g.grade_name, '全校汇总') AS grade_name,
    IFNULL(c.class_name, '年级小计') AS class_name,
    COUNT(*) AS people_num,
    ROUND(IFNULL(AVG(s.score), 0), 2) AS avg_score
FROM grade g
LEFT JOIN class_info c ON g.grade_id = c.grade_id
LEFT JOIN student_score s ON c.class_id = s.class_id
GROUP BY g.grade_name, c.class_name WITH ROLLUP`)

	fmt.Println("\n--- 错题 5: 子查询嵌套, 缺考 NULL 影响全校平均分 ---")
	fmt.Println("[错误 SQL] NULL 分数拉低全校平均, INNER JOIN 丢无数据年级")
	runQuery(db, `
SELECT
    g.grade_name,
    COUNT(s.sid) grade_people,
    AVG(s.score) grade_avg
FROM grade g
JOIN class_info c ON g.grade_id = c.grade_id
JOIN student_score s ON c.class_id = s.class_id
GROUP BY g.grade_name
HAVING grade_avg > (
    SELECT AVG(score) FROM student_score
)
ORDER BY g.grade_id`)
	fmt.Println("[修正 SQL] LEFT JOIN 保留 + NULL 兜底")
	runQuery(db, `
SELECT
    g.grade_name,
    COUNT(*) AS grade_people,
    ROUND(IFNULL(AVG(s.score), 0), 2) AS grade_avg
FROM grade g
JOIN class_info c ON g.grade_id = c.grade_id
LEFT JOIN student_score s ON c.class_id = s.class_id
GROUP BY g.grade_id, g.grade_name
HAVING grade_avg > (
    SELECT ROUND(AVG(IFNULL(score, 0)),2) FROM student_score
)
ORDER BY g.grade_id`)

	// ============ 拔高题 ============
	fmt.Println("\n" + strings.Repeat("=", 70))
	fmt.Println("拔高题")
	fmt.Println(strings.Repeat("=", 70))

	fmt.Println("\n--- ROLLUP 多级汇总: 各班明细 + 年级小计 + 全校总计 ---")
	runQuery(db, `
SELECT
    IFNULL(g.grade_name,'全校总计') grade_name,
    IFNULL(c.class_name,'年级小计') class_name,
    COUNT(*) total_people,
    ROUND(IFNULL(AVG(s.score),0),2) avg_score
FROM grade g
JOIN class_info c ON g.grade_id = c.grade_id
LEFT JOIN student_score s ON c.class_id = s.class_id
GROUP BY g.grade_name, c.class_name WITH ROLLUP`)

	fmt.Println("\n--- 拔高思考题: 高于本班平均分的学生 ---")
	runQuery(db, `
SELECT
    c.class_name,
    s.student_name,
    s.score,
    class_avg.class_avg_score
FROM student_score s
JOIN class_info c ON s.class_id = c.class_id
JOIN (
    SELECT class_id, ROUND(AVG(score),2) class_avg_score
    FROM student_score
    GROUP BY class_id
) class_avg ON s.class_id = class_avg.class_id
WHERE s.score > class_avg.class_avg_score
ORDER BY c.class_id, s.score DESC`)

	fmt.Println("\n=== 聚合查询实验结束 ===")
	fmt.Println("避坑总结:")
	fmt.Println("  1. 统计人数优先 COUNT(*), COUNT(字段) 忽略 NULL")
	fmt.Println("  2. 保留维度数据(年级/班级)用 LEFT JOIN, INNER JOIN 会丢无数据维度")
	fmt.Println("  3. WHERE 过滤原始行, HAVING 过滤聚合结果")
	fmt.Println("  4. GROUP BY 严格模式: SELECT 非聚合字段都要进 GROUP BY(建议带主键)")
	fmt.Println("  5. NULL 数值计算用 IFNULL(字段,0) 兜底")
	fmt.Println("  6. 窗口函数分组排名必须加 PARTITION BY")
	fmt.Println("  7. ROLLUP 汇总行配合 IFNULL 美化")
}

// runQuery 执行一条 SQL 并以表格形式打印结果, 执行失败打印错误
func runQuery(db *gorm.DB, sql string) {
	rows, err := db.Raw(sql).Rows()
	if err != nil {
		fmt.Fprintf(os.Stdout, "  ❌ 执行失败: %v\n", err)
		return
	}
	defer rows.Close()

	cols, _ := rows.Columns()
	if len(cols) == 0 {
		fmt.Println("  (无列)")
		return
	}

	// 收集所有行
	var table [][]string
	header := make([]string, len(cols))
	copy(header, cols)
	table = append(table, header)

	for rows.Next() {
		raw := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range raw {
			ptrs[i] = &raw[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			fmt.Fprintf(os.Stdout, "  ❌ 扫描失败: %v\n", err)
			return
		}
		row := make([]string, len(cols))
		for i, v := range raw {
			row[i] = cellString(v)
		}
		table = append(table, row)
	}

	printTable(table)
}

// cellString 把任意 cell 转成字符串, nil → "NULL"
func cellString(v any) string {
	if v == nil {
		return "NULL"
	}
	switch b := v.(type) {
	case []byte:
		return string(b)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// printTable 按列对齐打印二维字符串表
func printTable(table [][]string) {
	if len(table) == 0 {
		fmt.Println("  (空)")
		return
	}
	width := make([]int, len(table[0]))
	for _, row := range table {
		for i, cell := range row {
			if w := len(cell); w > width[i] {
				width[i] = w
			}
		}
	}
	for r, row := range table {
		var sb strings.Builder
		sb.WriteString("  ")
		for i, cell := range row {
			sb.WriteString(cell)
			if i < len(row)-1 {
				sb.WriteString(strings.Repeat(" ", width[i]-len(cell)+2))
			}
		}
		fmt.Println(sb.String())
		if r == 0 {
			var sep strings.Builder
			sep.WriteString("  ")
			for i, w := range width {
				sep.WriteString(strings.Repeat("-", w))
				if i < len(width)-1 {
					sep.WriteString("  ")
				}
			}
			fmt.Println(sep.String())
		}
	}
}

// printSection 打印小节标题
func printSection(title string) {
	fmt.Printf("\n[%s]\n", title)
}

func seedAggregate(db *gorm.DB) {
	// 幂等清空, 避免 seed 重复
	db.Exec("DELETE FROM student_score")
	db.Exec("DELETE FROM class_info")
	db.Exec("DELETE FROM grade")
	db.Exec("ALTER TABLE student_score AUTO_INCREMENT = 1")
	db.Exec("ALTER TABLE class_info AUTO_INCREMENT = 1")
	db.Exec("ALTER TABLE grade AUTO_INCREMENT = 1")

	// 年级
	db.Create([]*Grade{
		{GradeName: "初一"},
		{GradeName: "初二"},
	})

	// 班级: 初一1/2班, 初二1/2班
	db.Create([]*ClassInfo{
		{GradeID: 1, ClassName: "初一1班"},
		{GradeID: 1, ClassName: "初一2班"},
		{GradeID: 2, ClassName: "初二1班"},
		{GradeID: 2, ClassName: "初二2班"},
	})

	// 学生成绩, 照原文档测试数据
	f := func(v float64) *float64 { return &v }
	db.Create([]*StudentScore{
		{ClassID: 1, StudentName: "小明", Score: f(88)},
		{ClassID: 1, StudentName: "小红", Score: f(55)},
		{ClassID: 1, StudentName: "小刚", Score: f(92)},
		{ClassID: 2, StudentName: "小李", Score: f(76)},
		{ClassID: 2, StudentName: "小张", Score: f(59)},
		{ClassID: 3, StudentName: "小王", Score: f(95)},
		{ClassID: 3, StudentName: "小刘", Score: f(82)},
		{ClassID: 4, StudentName: "小陈", Score: f(45)},
		{ClassID: 4, StudentName: "小赵", Score: f(77)},
		{ClassID: 4, StudentName: "小孙", Score: f(66)},
	})
}
