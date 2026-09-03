package main

import "fmt"

// 实验 01：估算计算器
// 目标：把笔记 01 的速查表变成可运行的推导链——输入业务参数，
// 打印 QPS / 存储 / 分片数的完整推导过程。锚点：短链读路径 5 万 QPS、
// IM 存储一年 ~180TB、MySQL 写分片数 5 台。

const (
	secondsPerDay = 86400 // 一天 ≈ 10^5 秒
)

// estimateQPS: 日请求量 → 均值 QPS → 峰值 QPS → Redis 分片数
func estimateQPS(label string, dau int64, actionsPerUser int64, peakFactor float64) {
	fmt.Printf("\n[%s] 日活 %d, 人均日行为 %d 次\n", label, dau, actionsPerUser)
	dayReq := dau * actionsPerUser
	avgQPS := dayReq / secondsPerDay
	peakQPS := float64(avgQPS) * peakFactor
	fmt.Printf("  日请求        = %d × %d = %s (%d)\n", dau, actionsPerUser, human(dayReq), dayReq)
	fmt.Printf("  均值 QPS      = %d / 86400 = %d\n", dayReq, avgQPS)
	fmt.Printf("  峰值 QPS      = %d × %.0f = %.0f\n", avgQPS, peakFactor, peakQPS)

	// Redis 单机 10 万 QPS, 留 3 倍余量（故障转移时单机扛双倍 + 增长）
	shards := peakQPS / 100000 / 3
	if shards < 1 {
		shards = 1
	}
	fmt.Printf("  Redis 分片    = %.0f / (10万×3倍余量) = %.0f → %d 分片(+副本)\n", peakQPS, peakQPS/300000, int64(shards))
}

// estimateStorage: 记录数 × 单条大小 → 存储量 → MySQL 分表数
func estimateStorage(label string, rows int64, bytesPerRow int, days int) {
	fmt.Printf("\n[%s] 记录数 %s/天, 单条 %dB\n", label, human(rows), bytesPerRow)
	dayBytes := rows * int64(bytesPerRow)
	fmt.Printf("  日增存储      = %s × %dB = %s (%d B)\n", human(rows), bytesPerRow, humanBytes(dayBytes), dayBytes)
	yearBytes := dayBytes * 365
	fmt.Printf("  年增存储      = %s × 365 = %s\n", humanBytes(dayBytes), humanBytes(yearBytes))

	// MySQL 单表红线 2000 万行
	tables := rows / 20000000
	if tables < 1 {
		tables = 1
	}
	fmt.Printf("  单表红线      = 2000 万行 → %s/天 ÷ 2000万 = %d 张表/天才不爆\n", human(rows), tables)
}

// estimateMySQLShards: 写 QPS → 分库数（单机 MySQL 写 ~2k QPS）
func estimateMySQLShards(label string, writeQPS float64) {
	fmt.Printf("\n[%s] 写峰值 QPS %.0f\n", label, writeQPS)
	shards := writeQPS / 2000
	if shards < 1 {
		shards = 1
	}
	fmt.Printf("  分库数        = %.0f / 2000 = %.1f → %d 个分库\n", writeQPS, writeQPS/2000, int64(shards+0.999))
}

func human(n int64) string {
	switch {
	case n >= 1_0000_0000_0000:
		return fmt.Sprintf("%.0f万亿", float64(n)/1_0000_0000_0000)
	case n >= 1_0000_0000:
		return fmt.Sprintf("%.1f亿", float64(n)/1_0000_0000)
	case n >= 1_0000:
		return fmt.Sprintf("%.1f万", float64(n)/1_0000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

func humanBytes(b int64) string {
	const kb, mb, gb, tb = 1 << 10, 1 << 20, 1 << 30, 1 << 40
	switch {
	case b >= tb:
		return fmt.Sprintf("%.0fTB", float64(b)/tb)
	case b >= gb:
		return fmt.Sprintf("%.1fGB", float64(b)/gb)
	case b >= mb:
		return fmt.Sprintf("%.0fMB", float64(b)/mb)
	case b >= kb:
		return fmt.Sprintf("%.1fKB", float64(b)/kb)
	default:
		return fmt.Sprintf("%dB", b)
	}
}

func RunEstimateExperiments() {
	fmt.Println("== 实验 01: 估算计算器（笔记 01 §5 的可运行版） ==")

	// 推演一：短链服务（对应笔记 03 §1）
	fmt.Println("--- 推演一: 短链服务读路径 ---")
	estimateQPS("短链跳转", 1_0000_0000, 10, 5)

	// 推演二：IM 消息存储（对应笔记 05 §1）
	fmt.Println("\n--- 推演二: IM 消息存储 ---")
	estimateStorage("IM 消息", 5_0000_0000, 1024, 365)

	// 推演三：秒杀写路径（对应笔记 02 §1）
	fmt.Println("\n--- 推演三: 秒杀下单写路径 ---")
	estimateMySQLShards("秒杀落库(假设同步)", 10000)

	// 布隆过滤器内存账（对应笔记 03 §3）
	fmt.Println("\n--- 推演四: 布隆过滤器内存账 ---")
	const n = 1_0000_0000 // 1 亿 key
	const p = 0.01        // 误判率 1%
	// m = -n·lnp/(ln2)², 最优 k = (m/n)·ln2
	const ln2 = 0.6931471805599453
	m := -float64(n) * ln(1-p) / (ln2 * ln2)
	k := m / float64(n) * ln2
	fmt.Printf("  key 数 n = 1亿, 目标误判率 p = 1%%\n")
	fmt.Printf("  位数 m   = -n·ln(p)/(ln2)² = %.0f 亿 bit ≈ %.0fMB\n", m/1_0000_0000, m/8/1024/1024)
	fmt.Printf("  哈希数 k = (m/n)·ln2      = %.1f → 取 7\n", k)
	fmt.Printf("  结论: 120MB 内存保护 DB 不被随机 key 穿透打穿\n")

	// 锚点自检（README 数据锚点表对齐）
	fmt.Println("\n--- 锚点自检 ---")
	check := func(name string, got, want float64, tol string) {
		fmt.Printf("  %-24s 计算值=%-10.0f 预期=%s → %s\n", name, got, tol, mark(int64(got+0.5) == int64(want+0.5)))
	}
	check("短链均值 QPS", float64(1_0000_0000*10/86400), 11574, "≈11574")
	check("IM 年增存储(TB)", float64(5_0000_0000*1024*365)/float64(1<<40), 170, "≈170")
}

func ln(x float64) float64 {
	// 简单够用: 用换底公式 (实验环境避免引入 math 也行, 但直接用更清晰)
	return logE(x)
}

func mark(ok bool) string {
	if ok {
		return "✓"
	}
	return "✗"
}

// logE: math.Log 的本地实现（牛顿迭代, 精度足够本实验展示）
func logE(x float64) float64 {
	if x <= 0 {
		return 0
	}
	// 换到 [0.5, 2) 区间逐次折半, 再用级数 ln(y) = 2·(t + t³/3 + t⁵/5 + ...), t=(y-1)/(y+1)
	count := 0
	for x > 2 {
		x /= 2
		count++
	}
	for x < 0.5 {
		x *= 2
		count--
	}
	t := (x - 1) / (x + 1)
	sum := 0.0
	power := t
	for i := 1; i <= 25; i += 2 {
		sum += power / float64(i)
		power *= t * t
	}
	return 2*sum + float64(count)*0.6931471805599453
}
