package main

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
)

// 实验 08：手写分桶与 histogram_quantile（笔记 07 §3、09 §5）
// 实现: 指数分桶 + Prometheus 的线性插值分位算法 + 精确分位数（对照）
// 演示: ① 细桶 vs 粗桶的 P99 误差 ② 多实例桶相加 → 全局 P99 正确 ③ 分位数不可加
// 锚点: ① 细桶下插值 P99 与精确 P99 误差 < 桶宽
//       ② 粗桶下误差放大到桶宽量级（"P99 不准"的真凶是桶宽不是函数）
//       ③ 各实例桶相加后算出的全局 P99 ≈ 精确全局 P99（桶可加）
//       ④ 各实例 P99 的算术平均 ≠ 全局 P99（分位数不可加 → Summary 的死穴）

type bucket struct {
	upper float64 // 上界 le
	count float64 // 累计计数（不是本桶独有）
}

// histogramQuantile: Prometheus histogram_quantile 的核心算法
// 输入必须是累计桶（升序, 最后一个是 +Inf）
func histogramQuantile(q float64, buckets []bucket) float64 {
	if len(buckets) == 0 || math.IsNaN(q) || q < 0 || q > 1 {
		return math.NaN()
	}
	total := buckets[len(buckets)-1].count
	if total == 0 {
		return math.NaN()
	}
	rank := q * total

	for i, b := range buckets {
		if b.count < rank {
			continue
		}
		// 落在第 i 个桶里
		if i == 0 {
			// 第一个桶: 假设下界是 0（Prometheus 的做法）
			if b.upper <= 0 {
				return b.upper
			}
			return b.upper * (rank / b.count)
		}
		prev := buckets[i-1]
		lo, hi := prev.upper, b.upper
		prevCount := prev.count
		if math.IsInf(hi, 1) {
			return lo // +Inf 桶无法插值, 返回上一个上界
		}
		// 桶内线性插值: lo + 桶宽 × 桶内排名占比
		return lo + (hi-lo)*(rank-prevCount)/(b.count-prevCount)
	}
	return math.NaN()
}

// observeInto: 把样本灌进桶（累计语义: 所有 le >= v 的桶都 +1）
func observeInto(buckets []float64, samples []float64) []bucket {
	counts := make([]float64, len(buckets))
	for _, v := range samples {
		for i, ub := range buckets {
			if v <= ub {
				counts[i]++
			}
		}
	}
	out := make([]bucket, len(buckets))
	for i := range buckets {
		out[i] = bucket{upper: buckets[i], count: counts[i]}
	}
	return out
}

// exactQuantile: 从原始样本算精确分位数（桶插值的对照组）
func exactQuantile(q float64, samples []float64) float64 {
	if len(samples) == 0 {
		return math.NaN()
	}
	sorted := append([]float64(nil), samples...)
	sort.Float64s(sorted)
	idx := int(math.Ceil(q*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

// genLatencies: 确定性生成双峰延迟（70% 快请求 ~50ms, 30% 慢请求 ~600ms）
func genLatencies(n int, seed int64) []float64 {
	rng := rand.New(rand.NewSource(seed))
	out := make([]float64, 0, n)
	for i := 0; i < n; i++ {
		if rng.Float64() < 0.7 {
			out = append(out, 0.02+rng.Float64()*0.06) // 20~80ms
		} else {
			out = append(out, 0.4+rng.Float64()*0.4) // 400~800ms
		}
	}
	return out
}

func RunHistogramExperiments() {
	fmt.Println("=== 实验 08: 分桶与 histogram_quantile ===")

	const n = 10000
	samples := genLatencies(n, 42)
	exact := exactQuantile(0.99, samples)
	fmt.Printf("样本 %d 条（双峰: 70%% 快 ~50ms / 30%% 慢 ~600ms）, 精确 P99 = %.4fs\n", n, exact)

	// 锚点 ①/②: 细桶 vs 粗桶
	fine := []float64{0.05, 0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0, plusInf}
	coarse := []float64{0.1, 1.0, plusInf}

	pFine := histogramQuantile(0.99, observeInto(fine, samples))
	pCoarse := histogramQuantile(0.99, observeInto(coarse, samples))
	errFine := math.Abs(pFine-exact) / exact
	errCoarse := math.Abs(pCoarse-exact) / exact

	fmt.Printf("  细桶(12 个, 宽 0.1) 插值 P99 = %.4fs  相对误差 %.2f%%\n", pFine, errFine*100)
	fmt.Printf("  粗桶(3 个,  宽 0.9) 插值 P99 = %.4fs  相对误差 %.2f%%\n", pCoarse, errCoarse*100)
	fmt.Printf("%s 锚点① 细桶误差 < 5%%: %.2f%%\n", mark(errFine < 0.05), errFine*100)
	fmt.Printf("%s 锚点② 粗桶误差显著放大（%.0f 倍）: %.2f%%——桶宽决定误差, 不是函数不准\n",
		mark(errCoarse > errFine*3), errCoarse/max(errFine, 1e-9), errCoarse*100)

	// 锚点 ③: 桶可加 → 两个实例桶相加后算全局 P99 正确
	a := samples[:n/2]
	b := samples[n/2:]
	bA := observeInto(fine, a)
	bB := observeInto(fine, b)
	merged := make([]bucket, len(fine))
	for i := range fine {
		merged[i] = bucket{upper: fine[i], count: bA[i].count + bB[i].count}
	}
	pMerged := histogramQuantile(0.99, merged)
	errMerged := math.Abs(pMerged-exact) / exact
	fmt.Printf("  实例A 桶 + 实例B 桶 → 全局 P99 = %.4fs（精确 %.4fs, 误差 %.2f%%）\n", pMerged, exact, errMerged*100)
	fmt.Printf("%s 锚点③ 桶可加: 相加后算出的全局 P99 与精确值一致（<5%%）\n", mark(errMerged < 0.05))

	// 锚点 ④: 分位数不可加 → Summary 的死穴
	fast := make([]float64, 5000)
	slow := make([]float64, 5000)
	for i := range fast {
		fast[i] = 0.1
	}
	for i := range slow {
		slow[i] = 1.0
	}
	qFast := exactQuantile(0.99, fast)
	qSlow := exactQuantile(0.99, slow)
	avgOfQuantiles := (qFast + qSlow) / 2
	globalQ := exactQuantile(0.99, append(append([]float64{}, fast...), slow...))
	fmt.Printf("  实例A P99 = %.2fs, 实例B P99 = %.2fs\n", qFast, qSlow)
	fmt.Printf("  两者算术平均 = %.2fs,   真实全局 P99 = %.2fs\n", avgOfQuantiles, globalQ)
	fmt.Printf("%s 锚点④ 分位数不可加: 平均 %.2f ≠ 真实 %.2f（差 %.0f%%）→ Summary 无法算全局分位\n",
		mark(math.Abs(avgOfQuantiles-globalQ) > 0.01), avgOfQuantiles, globalQ,
		math.Abs(avgOfQuantiles-globalQ)/globalQ*100)
}
