// # 聚合查询实验
//
// 对应 08-聚合查询练习.md 的建表/数据/习题，验证 SQL 真实行为。
//
// 实验项：
//
//	Exp1：基础聚合 — LEFT JOIN + GROUP BY + 多聚合函数
//	Exp2：窗口函数 — 分组排名（必须包子查询再过滤 rn）
//	Exp3：Error 1054 — WHERE 里用聚合别名，真实报错
//	Exp4：Error 1055 — select * + 一对多 JOIN + GROUP BY，真实报错
//	Exp5：ROLLUP 多级汇总 — IFNULL 兜底

package main

import (
	"fmt"

	"gorm.io/gorm"
)

// printRows 执行 SQL 并以 KV 形式打印结果
func printRows(db *gorm.DB, sql string, args ...interface{}) {
	var rows []map[string]interface{}
	if err := db.Raw(sql, args...).Scan(&rows).Error; err != nil {
		fmt.Printf("  ❌ 报错: %v\n", err)
		return
	}
	for _, r := range rows {
		fmt.Printf("  %v\n", r)
	}
	if len(rows) == 0 {
		fmt.Println("  (无结果)")
	}
}

// RunAggregateExperiments 运行聚合查询实验
func RunAggregateExperiments(db *gorm.DB) {
	seedSchoolData(db)

	fmt.Println("\n=== 实验1: 基础聚合（习题1）===")
	fmt.Println("每个班级：总人数/总分/平均分/不及格人数 — COUNT(*) 数人，IFNULL 兜底")
	printRows(db, `SELECT c.class_name,
		COUNT(*) AS total_num,
		IFNULL(SUM(s.score),0) AS total_score,
		ROUND(IFNULL(AVG(s.score),0),2) AS avg_score,
		SUM(IF(s.score<60,1,0)) AS fail_count
		FROM class_info c
		LEFT JOIN student_score s ON c.class_id = s.class_id
		GROUP BY c.class_id, c.class_name`)

	fmt.Println("\n=== 实验2: 窗口函数分组排名（习题4）===")
	fmt.Println("RANK() + PARTITION BY 分年级排名，外层过滤前 2 名")
	printRows(db, `SELECT * FROM (
		SELECT g.grade_name, c.class_name, ROUND(AVG(s.score),2) AS class_avg,
			RANK() OVER(PARTITION BY g.grade_id ORDER BY AVG(s.score) DESC) AS rank_no
		FROM grade g
		JOIN class_info c ON g.grade_id = c.grade_id
		LEFT JOIN student_score s ON c.class_id = s.class_id
		GROUP BY g.grade_id, g.grade_name, c.class_id, c.class_name
	) t WHERE t.rank_no <= 2`)

	fmt.Println("\n=== 实验3: WHERE 里用聚合别名 — Error 1054 ===")
	fmt.Println("WHERE avg_score >= 70（avg_score 是 SELECT 别名）")
	printRows(db, `SELECT c.class_name, ROUND(AVG(s.score),2) AS avg_score
		FROM class_info c
		LEFT JOIN student_score s ON c.class_id = s.class_id
		WHERE avg_score >= 70
		GROUP BY c.class_id, c.class_name`)

	fmt.Println("\n=== 实验4: select * + 一对多 JOIN + GROUP BY — Error 1055 ===")
	fmt.Println("GROUP BY c.class_id 但 select * 含 s 表列（一个班多行，决定不了）")
	printRows(db, `SELECT * FROM class_info c
		LEFT JOIN student_score s ON s.class_id = c.class_id
		GROUP BY c.class_id`)

	fmt.Println("\n=== 实验5: ROLLUP 多级汇总 ===")
	fmt.Println("GROUP BY ... WITH ROLLUP 生成年级小计/全校合计，IFNULL 美化 NULL 行")
	printRows(db, `SELECT
		IFNULL(g.grade_name,'全校总计') AS grade_name,
		IFNULL(c.class_name,'年级小计') AS class_name,
		COUNT(*) AS total_people,
		ROUND(IFNULL(AVG(s.score),0),2) AS avg_score
		FROM grade g
		JOIN class_info c ON g.grade_id = c.grade_id
		LEFT JOIN student_score s ON c.class_id = s.class_id
		GROUP BY g.grade_name, c.class_name WITH ROLLUP`)

	fmt.Println("\n=== 聚合实验结束 ===")
	fmt.Println("关键结论：")
	fmt.Println("  1. 数人用 COUNT(*)，COUNT(字段) 忽略 NULL")
	fmt.Println("  2. 聚合过滤用 HAVING；WHERE 用别名报 Error 1054")
	fmt.Println("  3. 窗口函数排名必加 PARTITION BY，外层过滤必须包子查询")
	fmt.Println("  4. 功能依赖仅限 GROUP BY 列唯一决定 SELECT 列；一对多 + select * 报 Error 1055")
	fmt.Println("  5. ROLLUP 汇总行配合 IFNULL 兜底")
}
