package main

import (
	"fmt"
	"math"
)

// 实验 12：高基数爆炸演示（笔记 07 第 4 节、笔记 12 第 5 节）
// 实现: 按维度乘积计算 series 数, 估算内存与抓取体积, 演示路径归一化的收敛效果
// 演示: 给一个看似无害的指标加上 user_id 标签会发生什么
// 锚点: ① series 数是各 label 取值数的乘积——加一个高基数维度直接爆炸
//       ② 路径归一化把 1000 个实际路径收敛成 3 个路由模板
//       ③ 按 3KB/series 估算内存, 千万级 series = 数十 GB（打挂 Prometheus）
//       ④ 治理手段: 丢弃 label / 用 exemplar 挂 trace_id 而不是做成 label

type dim struct {
	name string
	card int // 该 label 的取值数
}

// seriesCount: series 数 = 各维度取值数的乘积（乘上实例数）
func seriesCount(dims []dim, instances int) float64 {
	n := float64(instances)
	for _, d := range dims {
		n *= float64(d.card)
	}
	return n
}

const (
	bytesPerSeries = 3 * 1024 // Prometheus 里一条 series 的常驻内存经验值
	bytesPerLine   = 80       // exposition 文本里一行样本的均值
)

func humanBytes(b float64) string {
	units := []string{"B", "KB", "MB", "GB", "TB", "PB"}
	i := 0
	for b >= 1024 && i < len(units)-1 {
		b /= 1024
		i++
	}
	return fmt.Sprintf("%.1f %s", b, units[i])
}

func RunCardinalityExperiments() {
	fmt.Println("=== 实验 12: 高基数（cardinality）爆炸 ===")

	const instances = 100

	// 健康的设计: 全部是低基数枚举维度
	healthy := []dim{{"route", 50}, {"method", 5}, {"status", 10}}
	nHealthy := seriesCount(healthy, instances)
	fmt.Printf("健康设计  route(50) × method(5) × status(10) × 实例(%d) = %.0f series\n",
		instances, nHealthy)
	fmt.Printf("          内存 ≈ %s, 单次抓取 ≈ %s\n\n",
		humanBytes(nHealthy*bytesPerSeries), humanBytes(nHealthy*bytesPerLine))

	// 灾难: 加上 user_id
	boom := []dim{{"route", 50}, {"method", 5}, {"status", 10}, {"user_id", 1_000_000}}
	nBoom := seriesCount(boom, instances)
	fmt.Printf("加 user_id  route × method × status × user_id(100万) × 实例(%d) = %.3e series\n", instances, nBoom)
	fmt.Printf("          内存 ≈ %s, 单次抓取 ≈ %s\n",
		humanBytes(nBoom*bytesPerSeries), humanBytes(nBoom*bytesPerLine))
	growth := nBoom / nHealthy
	fmt.Printf("%s 锚点① 基数乘法效应: 加一个维度放大 %.0f 倍（%.0f → %.3e）\n",
		mark(growth > 1e5), growth, nHealthy, nBoom)

	// 锚点 ②: 路径归一化
	rawPaths := make([]string, 0, 1000)
	for i := 0; i < 1000; i++ {
		rawPaths = append(rawPaths, fmt.Sprintf("/user/%d", i))
	}
	patterns := map[string]int{}
	for _, p := range rawPaths {
		patterns[routePattern(p)]++
	}
	fmt.Printf("\n1000 个真实路径 /user/{0..999} 归一化后 → %d 个路由模板: %v\n", len(patterns), patterns)
	fmt.Printf("%s 锚点② 路径归一化: 1000 → %d（不归一就是 1000 条 series, 且随用户量无限增长）\n",
		mark(len(patterns) == 1), len(patterns))

	// 锚点 ③: 什么时候算"打挂"
	fmt.Println()
	for _, n := range []float64{1e5, 1e6, 1e7, 1e8} {
		fmt.Printf("  %.0e series → 内存 ≈ %-8s 抓取 ≈ %-8s\n",
			n, humanBytes(n*bytesPerSeries), humanBytes(n*bytesPerLine))
	}
	fmt.Printf("%s 锚点③ 量级直觉: 千万级 series 已需 %s 内存, 单次抓取 %s——这正是'打挂 Prometheus'的含义\n",
		mark(true), humanBytes(1e7*bytesPerSeries), humanBytes(1e7*bytesPerLine))

	// 锚点 ④: 治理——丢弃 label 后的收敛效果
	// 模拟 metric_relabel_configs drop user_id
	afterDrop := []dim{{"route", 50}, {"method", 5}, {"status", 10}}
	nAfter := seriesCount(afterDrop, instances)
	fmt.Printf("\n治理: metric_relabel_configs 丢弃 user_id → 回到 %.0f series（降 %.0f 倍）\n",
		nAfter, nBoom/nAfter)
	fmt.Printf("%s 锚点④ 治理有效: 抓取侧丢弃 label 是唯一'不用改业务代码'的兜底手段\n",
		mark(math.Abs(nAfter-nHealthy) < 1e-9))
	fmt.Println("  另一条路: 明细（trace_id/订单号）走 exemplar 挂在样本上——不产生新 series（12 第 7 节）")
}
