// # 故障检测实验（笔记 07）
//
// 对应笔记：notes/distributed/07-故障检测.md
//
// 运行：go run ./experiments/ phi
//
// 实验项：
//
//	第1节：φ-accrual —— 学分布算怀疑度, 不同偏离程度的 φ 值
//	第2节：固定超时 vs φ 阈值 —— 面对负载变化（分布右移）的误杀对照
//	第3节：gossip 收敛 —— ln(N) 轮全员知情
package main

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
)

// RunPhiExperiments 演示笔记 07 的故障检测。
func RunPhiExperiments() {
	fmt.Println("========== 第1节: φ-accrual 怀疑度 ==========")
	p1Phi()

	fmt.Println("\n========== 第2节: 固定超时 vs φ ==========")
	p2Adaptive()

	fmt.Println("\n========== 第3节: gossip 收敛 ==========")
	p3Gossip()
}

// ---------- 第1节：φ 计算 ----------

// heartbeatStats 收集心跳间隔, 估计分布参数。
type heartbeatStats struct {
	window []float64
}

func (s *heartbeatStats) observe(d float64) {
	s.window = append(s.window, d)
	if len(s.window) > 1000 {
		s.window = s.window[1:]
	}
}

func (s *heartbeatStats) meanSigma() (float64, float64) {
	n := float64(len(s.window))
	sum, sum2 := 0.0, 0.0
	for _, v := range s.window {
		sum += v
		sum2 += v * v
	}
	mean := sum / n
	variance := sum2/n - mean*mean
	return mean, math.Sqrt(variance)
}

// phi 假设正态分布, 计算 P(间隔 > t) 的 -log10。
func (s *heartbeatStats) phi(t float64) float64 {
	mean, sigma := s.meanSigma()
	// P(X > t) 用正态 CDF 补函数: 0.5 * erfc((t-mean)/(sigma*sqrt2))
	z := (t - mean) / (sigma * math.Sqrt2)
	p := 0.5 * math.Erfc(z)
	if p <= 1e-12 {
		p = 1e-12 // 防止 log(0)
	}
	return -math.Log10(p)
}

func p1Phi() {
	// 模拟: 心跳间隔 ~ N(1000ms, 50ms)
	stats := &heartbeatStats{}
	for i := 0; i < 1000; i++ {
		stats.observe(1000 + rand.NormFloat64()*50)
	}
	mean, sigma := stats.meanSigma()
	fmt.Printf("学到的心跳分布: μ=%.0fms σ=%.1fms\n", mean, sigma)

	for _, t := range []float64{1000, 1050, 1100, 1200, 1500} {
		fmt.Printf("  距上次心跳 t=%5.0fms → φ=%5.2f（%s）\n", t, stats.phi(t),
			map[bool]string{true: "正常波动", false: "可疑"}[t < mean+2*sigma])
	}
	fmt.Println("解读: φ≈1 → 10% 概率是正常波动; φ≈3 → 千分之一; φ≈6 → 百万分之一的正常偏离")
	fmt.Println("上层按需取阈值: 低延迟场景 φ=1 动手, 求稳 φ=8")
}

// ---------- 第2节：固定超时 vs φ 阈值 ----------

func p2Adaptive() {
	// 场景: 节点负载升高, 心跳从 N(1000,50) 慢到 N(1400,80)（分布右移）
	const fixedTimeout = 1200.0 // 固定超时: 按"1.2 秒没心跳即死"拍板

	// 检测器 A: 固定超时（不知道分布变了）
	fixedKills := 0
	for i := 0; i < 1000; i++ {
		interval := 1400 + rand.NormFloat64()*80 // 新分布的心跳
		if interval > fixedTimeout {
			fixedKills++ // 误杀: 节点活着, 只是慢
		}
	}

	// 检测器 B: φ-accrual（窗口持续学习新分布）
	stats := &heartbeatStats{}
	for i := 0; i < 500; i++ { // 旧分布打底
		stats.observe(1000 + rand.NormFloat64()*50)
	}
	phiKills := 0
	for i := 0; i < 1000; i++ {
		interval := 1400 + rand.NormFloat64()*80
		stats.observe(interval) // 窗口学习新常态
		if stats.phi(interval) > 3 {
			phiKills++
		}
	}
	fmt.Printf("负载升高（心跳 μ: 1000→1400ms）后 1000 次心跳:\n")
	fmt.Printf("  固定超时 1200ms: 误杀 %d 次（把'变慢'当成'死了'）\n", fixedKills)
	fmt.Printf("  φ-accrual(阈值3): 误判 %d 次（分布右移被窗口吸收, 自动跟上新常态）\n", phiKills)
}

// ---------- 第3节：gossip 收敛 ----------

func p3Gossip() {
	for _, n := range []int{100, 1000, 10000} {
		knows := make([]bool, n)
		knows[0] = true // 节点 0 有个八卦
		rounds := 0
		for informed := 1; informed < n; {
			rounds++
			// 本轮: 每个知情人随机告诉 k=3 个人
			newlyInformed := 0
			knowsNow := make([]bool, n)
			copy(knowsNow, knows)
			for i := 0; i < n; i++ {
				if knows[i] {
					for j := 0; j < 3; j++ {
						target := rand.Intn(n)
						if !knowsNow[target] {
							knowsNow[target] = true
							newlyInformed++
						}
					}
				}
			}
			knows = knowsNow
			informed += newlyInformed
		}
		fmt.Printf("N=%-6d k=3: %d 轮全员知情（理论 ln(N)=%.1f）\n", n, rounds, math.Log(float64(n)))
	}
	fmt.Println("规模无关的对数收敛 —— 万级节点秒级同步成员信息")
	_ = sort.Ints
}
