package main

import (
	"fmt"
	"math"
)

// 实验 09：手写 rate / increase / irate 与 Counter 重置修正（笔记 09 第 2 节 第 3 节）
// 实现: 区间向量上的速率计算——朴素算法 vs 修正算法, rate vs irate, 窗口不足的失真
// 演示: ① 进程重启导致 Counter 归零时两种算法的结果 ② 窗口 < 4×interval 的失真 ③ 毛刺捕捉能力
// 锚点: ① 重置后朴素算法给出荒谬的负值, 修正算法给出合理正值（09 第 3 节）
//       ② 窗口 10s 配 15s 抓取 → 大量时间点算不出（NaN）或剧烈跳动
//       ③ irate 能捕捉到 15s 的毛刺, rate 把它平滑掉了（所以告警用 rate, 看毛刺用 irate）

// point: 一个样本点（t 为毫秒）
type point struct {
	t int64
	v float64
}

// rateNaive: 朴素算法——直接 (末值 - 首值) / 跨度
// 这是"自己写 (now - before)/300" 会得到的结果, 遇到 Counter 重置就崩
func rateNaive(in []point) float64 {
	if len(in) < 2 {
		return math.NaN()
	}
	first, last := in[0], in[len(in)-1]
	span := float64(last.t-first.t) / 1000
	if span <= 0 {
		return math.NaN()
	}
	return (last.v - first.v) / span
}

// rateCounter: 修正算法——遍历相邻对, 检测到后值 < 前值即判定重置, 把该段当作从 0 开始重新累加
func rateCounter(in []point) float64 {
	if len(in) < 2 {
		return math.NaN()
	}
	delta := 0.0
	for i := 1; i < len(in); i++ {
		d := in[i].v - in[i-1].v
		if d < 0 {
			// Counter 重置（进程重启）: 这一段实际增量就是重置后的当前值
			d = in[i].v
		}
		delta += d
	}
	span := float64(in[len(in)-1].t-in[0].t) / 1000
	if span <= 0 {
		return math.NaN()
	}
	return delta / span
}

// irate: 只取窗口内最后两个样本算瞬时速率
func irate(in []point) float64 {
	if len(in) < 2 {
		return math.NaN()
	}
	a, b := in[len(in)-2], in[len(in)-1]
	span := float64(b.t-a.t) / 1000
	if span <= 0 {
		return math.NaN()
	}
	d := b.v - a.v
	if d < 0 {
		d = b.v // 同样处理重置
	}
	return d / span
}

// increase: 窗口内增量 == rate × 窗口秒数
func increase(in []point, windowSec float64) float64 {
	r := rateCounter(in)
	if math.IsNaN(r) {
		return math.NaN()
	}
	return r * windowSec
}

// windowAt: 取 [end-windowMs, end] 区间内的样本（区间向量的物理含义）
func windowAt(pts []point, end, windowMs int64) []point {
	start := end - windowMs
	var out []point
	for _, p := range pts {
		if p.t > start && p.t <= end {
			out = append(out, p)
		}
	}
	return out
}

// buildSeries: 按每段的 QPS 构造 Counter 序列（intervalMs 抓取一次）
func buildSeries(intervalMs int64, perInterval []float64) []point {
	out := make([]point, 0, len(perInterval)+1)
	v := 0.0
	t := int64(0)
	out = append(out, point{t: t, v: v})
	for _, qps := range perInterval {
		t += intervalMs
		v += qps * float64(intervalMs) / 1000
		out = append(out, point{t: t, v: v})
	}
	return out
}

func RunPromQLExperiments() {
	const intervalMs = 15000 // 15s 抓取

	fmt.Println("=== 实验 09: rate / irate / Counter 重置 ===")

	// ---- 锚点 ①: Counter 重置 ----
	// t=0..90s, 前 45s 每秒 10 个请求, t=45 进程重启（Counter 归零）, 之后仍每秒 10 个
	resetSeries := []point{
		{t: 0, v: 1000}, {t: 15000, v: 1150}, {t: 30000, v: 1300},
		{t: 45000, v: 10}, // 重启: 归零后重新累积到 10
		{t: 60000, v: 160}, {t: 75000, v: 310}, {t: 90000, v: 460},
	}
	win := windowAt(resetSeries, 90000, 90000)
	naive := rateNaive(win)
	fixed := rateCounter(win)
	fmt.Printf("重启场景样本: 1000 → 1300 → [重启] 10 → 460（真实 QPS 恒为 10/s）\n")
	fmt.Printf("  朴素 (末-首)/跨度 = %+.2f/s\n", naive)
	fmt.Printf("  修正（检测重置）= %+.2f/s\n", fixed)
	fmt.Printf("%s 锚点① 重置修正: 朴素算法给出负值(%+.2f), 修正算法给出合理正值(%+.2f)\n",
		mark(naive < 0 && fixed > 0), naive, fixed)

	// ---- 锚点 ②: 窗口不足 ----
	// 稳定 100 QPS（带 ±10 的确定性抖动, 否则窗口大小看不出差别）
	per := make([]float64, 40)
	for i := range per {
		per[i] = 100 + float64((i*37)%21) - 10
	}
	steady := buildSeries(intervalMs, per)

	countNaN := func(windowMs int64) int {
		n := 0
		for i := 1; i < len(steady); i++ {
			if math.IsNaN(rateCounter(windowAt(steady, steady[i].t, windowMs))) {
				n++
			}
		}
		return n
	}
	nan10 := countNaN(10000)
	nan60 := countNaN(60000)
	fmt.Printf("\n稳定 100 QPS / 15s 抓取, 共 %d 个抓取点:\n", len(steady))
	fmt.Printf("  窗口 [10s] (< interval): %d/%d 个时间点算不出 rate\n", nan10, len(steady)-1)
	fmt.Printf("  窗口 [60s] (= 4×interval): %d/%d 个时间点算不出 rate\n", nan60, len(steady)-1)

	// 窗口太小时的值跳动（用 30s 窗口 = 2 个点, 与 60s 对比标准差）
	spread := func(windowMs int64) float64 {
		min, max := math.Inf(1), math.Inf(-1)
		for i := 1; i < len(steady); i++ {
			r := rateCounter(windowAt(steady, steady[i].t, windowMs))
			if math.IsNaN(r) {
				continue
			}
			if r < min {
				min = r
			}
			if r > max {
				max = r
			}
		}
		return max - min
	}
	spread30, spread60 := spread(30000), spread(60000)
	fmt.Printf("  窗口 [30s] 波动幅度 = %.2f, 窗口 [60s] 波动幅度 = %.2f\n", spread30, spread60)
	fmt.Printf("%s 锚点② 窗口纪律: [10s] 会导致 %d 处算不出; 且窗口越小波动越大（30s 波幅 %.2f > 60s 波幅 %.2f）\n",
		mark(nan10 > 0 && nan60 == 0 && spread30 > spread60), nan10, spread30, spread60)

	// ---- 锚点 ③: rate 平滑 vs irate 捕捉毛刺 ----
	// 前 19 段 100 QPS, 第 20 段一次 1000 QPS 的毛刺
	spiky := make([]float64, 24)
	for i := range spiky {
		spiky[i] = 100
	}
	spiky[19] = 1000
	spikeSeries := buildSeries(intervalMs, spiky)
	spikeAt := spikeSeries[20].t // 毛刺刚发生的那一刻

	w5m := windowAt(spikeSeries, spikeAt, 300000)
	w30s := windowAt(spikeSeries, spikeAt, 30000)
	r5m := rateCounter(w5m)
	i5m := irate(w5m)
	_ = w30s
	fmt.Printf("\n毛刺场景: 稳定 100 QPS, 某 15s 内突增至 1000 QPS\n")
	fmt.Printf("  rate[5m]  = %.1f/s（20 个样本参与, 毛刺被平滑）\n", r5m)
	fmt.Printf("  irate[5m] = %.1f/s（只取最后两个样本, 毛刺被完整捕捉）\n", i5m)
	fmt.Printf("%s 锚点③ rate 平滑 / irate 敏锐: %.1f vs %.1f——告警用 rate 防翻转, 看毛刺用 irate\n",
		mark(i5m > r5m*3), r5m, i5m)

	// ---- increase == rate × 窗口 ----
	inc := increase(w5m, 300)
	fmt.Printf("  increase[5m] = %.0f 个请求 = rate(%.1f) × 300s %s\n",
		inc, r5m, mark(math.Abs(inc-r5m*300) < 1e-6))
}
