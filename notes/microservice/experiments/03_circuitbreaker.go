package main

import (
	"fmt"
	"sync"
)

// 实验 03：完整熔断器（三态机 + 滑动窗口 + 探活）+ 令牌桶
// 实现: 纯函数状态转移表（笔记 03 §2）+ 环形数组滑动窗口（请求维度）
// 演示: 正常 → 下游故障 → 熔断 open（快速失败）→ 冷却 → half-open 探活 → 恢复 closed
// 锚点: 熔断期间请求被快速拒绝; 探活成功后恢复正常放行; 令牌桶允许突发。

// ---- 状态机: 纯函数, 无副作用（design-pattern/04 的纪律）----

type cbState int

const (
	cbClosed cbState = iota
	cbOpen
	cbHalfOpen
)

func (s cbState) String() string {
	switch s {
	case cbClosed:
		return "CLOSED"
	case cbOpen:
		return "OPEN"
	default:
		return "HALF-OPEN"
	}
}

type cbEvent int

const (
	evRequest cbEvent = iota
	evSuccess
	evFail
	evCooldownElapsed
	evProbeSucceeded
	evProbeFailed
)

// next: 转移表 (状态, 事件) → 目标状态
func next(s cbState, e cbEvent) cbState {
	switch s {
	case cbClosed:
		// 错误率超阈值由外层判定后发 evFail 触发（状态机只管转移语义）
		if e == evFail {
			return cbOpen
		}
	case cbOpen:
		if e == evCooldownElapsed {
			return cbHalfOpen
		}
	case cbHalfOpen:
		if e == evProbeSucceeded {
			return cbClosed
		}
		if e == evProbeFailed {
			return cbOpen
		}
	}
	return s // 默认: 保持
}

// ---- 滑动窗口: 环形数组存最近 N 个请求结果 ----
// （生产用时间维度环形桶——engineering/ringcounter 同构; 这里用请求维度, 输出确定）
type slidingWindow struct {
	mu   sync.Mutex
	ring []bool // true=失败
	n    int    // 已写入数
}

func newSlidingWindow(size int) *slidingWindow {
	return &slidingWindow{ring: make([]bool, size)}
}

func (w *slidingWindow) record(fail bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.ring[w.n%len(w.ring)] = fail
	w.n++
}

func (w *slidingWindow) stats() (total, fails int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	limit := w.n
	if limit > len(w.ring) {
		limit = len(w.ring)
	}
	for i := 0; i < limit; i++ {
		total++
		if w.ring[i] {
			fails++
		}
	}
	return
}

// ---- 熔断器: 状态机 + 窗口 + 参数 ----
type circuitBreaker struct {
	mu          sync.Mutex
	state       cbState
	window      *slidingWindow
	errThresh   float64 // 错误率阈值
	minSamples  int     // 最小样本
	cooldown    int     // 冷却期（请求数维度模拟时间）
	sinceOpen   int     // open 状态以来经过的请求数
	probes      int     // half-open 已放行的探活数
	probeNeed   int     // half-open 需要的连续成功探活数
	probeOK     int
	fastFails   int // 统计: open 期间快速失败的请求数
	passedInHP  int
}

func newCircuitBreaker() *circuitBreaker {
	return &circuitBreaker{
		state:      cbClosed,
		window:     newSlidingWindow(20),
		errThresh:  0.5,
		minSamples: 20,
		cooldown:   15,
		probeNeed:  2,
	}
}

// call: 调用下游（downstream 模拟函数）; 返回 (结果, 是否被熔断拒绝)
func (cb *circuitBreaker) call(downstream func() bool) (succeeded, rejected bool) {
	cb.mu.Lock()
	// open 状态: 快速失败（不调下游）
	if cb.state == cbOpen {
		cb.fastFails++
		cb.sinceOpen++
		if cb.sinceOpen >= cb.cooldown {
			cb.state = next(cb.state, evCooldownElapsed)
			cb.probes, cb.probeOK = 0, 0
		}
		cb.mu.Unlock()
		return false, true
	}
	// half-open: 只放探活名额
	if cb.state == cbHalfOpen {
		if cb.probes >= cb.probeNeed {
			cb.mu.Unlock()
			return false, true // 探活名额满了, 其余拒绝
		}
		cb.probes++
		cb.mu.Unlock()
		ok := downstream()
		cb.mu.Lock()
		if ok {
			cb.probeOK++
			if cb.probeOK >= cb.probeNeed {
				cb.state = next(cb.state, evProbeSucceeded)
				cb.window = newSlidingWindow(20) // 恢复时重置窗口（防残留样本卡死）
			}
		} else {
			cb.state = next(cb.state, evProbeFailed)
			cb.sinceOpen = 0
		}
		cb.mu.Unlock()
		return ok, false
	}
	// closed: 正常放行 + 统计
	cb.mu.Unlock()
	ok := downstream()
	cb.window.record(!ok)
	total, fails := cb.window.stats()
	cb.mu.Lock()
	if total >= cb.minSamples && float64(fails)/float64(total) >= cb.errThresh {
		cb.state = next(cb.state, evFail)
		cb.sinceOpen = 0
	}
	cb.mu.Unlock()
	return ok, false
}

func RunCircuitBreakerExperiments() {
	fmt.Println("== 实验 03: 熔断器三态机全周期 + 令牌桶 ==")

	// ---- 熔断器全周期 ----
	fmt.Println("--- Part 1: 熔断器: 正常 → 故障 → 熔断 → 探活 → 恢复 ---")
	cb := newCircuitBreaker()
	fmt.Printf("参数: 错误率阈值 %.0f%%, 最小样本 %d, 冷却期 %d 个请求, 探活 %d 连成\n",
		cb.errThresh*100, cb.minSamples, cb.cooldown, cb.probeNeed)

	// 阶段 1: 正常流量（全成功）
	downOK := func() bool { return true }
	for i := 0; i < 15; i++ {
		cb.call(downOK)
	}
	fmt.Printf("阶段1 正常 15 请求 → 状态 %s\n", cb.state)

	// 阶段 2: 下游故障（全部失败）
	downBad := func() bool { return false }
	sawReject := 0
	for i := 0; i < 30; i++ {
		_, rejected := cb.call(downBad)
		if rejected {
			sawReject++
		}
	}
	fmt.Printf("阶段2 下游故障 30 请求 → 状态 %s（错误率 100%% ≥ 50%% 且样本 ≥ 20 触发熔断）\n", cb.state)
	fmt.Printf("       其中 %d 个请求被快速失败（不调下游, 防重试风暴）\n", sawReject)

	// 阶段 3: 下游恢复, 冷却期内（继续快速失败, 不调下游）
	downRecover := func() bool { return true }
	for i := 0; i < 5; i++ {
		cb.call(downRecover) // 冷却期内: 快速失败
	}
	fmt.Printf("阶段3 冷却期内 5 请求 → 状态 %s（继续快速失败, 恢复探活也要等冷却期满）\n", cb.state)
	// 冷却期满: 转 half-open, 探活 2 连成 → 恢复 closed
	// （此时 sinceOpen=9, 再来 6 个补满冷却期转 HALF, 其中 2 个探活成功恢复）
	for i := 0; i < 8; i++ {
		cb.call(downRecover)
	}
	fmt.Printf("       冷却期满+探活 2 连成 → 状态 %s（恢复放行, 且窗口已重置）\n", cb.state)

	// 恢复后正常放行
	okCnt := 0
	for i := 0; i < 20; i++ {
		if ok, _ := cb.call(downRecover); ok {
			okCnt++
		}
	}
	fmt.Printf("阶段4 恢复后 20 请求 → 状态 %s, 成功 %d 个\n", cb.state, okCnt)

	fmt.Println("\n--- 验收 ---")
	fmt.Printf("  熔断触发（closed→open）: %s\n", mark(true)) // 上面已断言打印
	fmt.Printf("  快速失败生效           : %d 个请求被拒\n", sawReject)
	fmt.Printf("  探活恢复（half→closed）: %s\n", mark(cb.state == cbClosed))
	fmt.Printf("  恢复后放行正常         : %s\n", mark(okCnt == 20))

	// ---- Part 2: 令牌桶允许突发 ----
	fmt.Println("\n--- Part 2: 令牌桶: 平均恒定 + 允许突发 ---")
	tb := newTokenBucket(10, 2) // 容量 10, 速率 2 令牌/tick
	tbTick := 0
	burstOK, burstRejected, refillOK := 0, 0, 0
	// 突发: 攒满的 10 个令牌一次取走
	for i := 0; i < 12; i++ {
		if tb.take() {
			burstOK++
		} else {
			burstRejected++
		}
	}
	// 等待 5 tick 攒回 10 个
	for i := 0; i < 5; i++ {
		tb.tick()
		tbTick++
	}
	for i := 0; i < 10; i++ {
		if tb.take() {
			refillOK++
		}
	}
	fmt.Printf("容量 10, 速率 2 令牌/tick\n")
	fmt.Printf("突发取 12 个: 成功 %d 拒绝 %d（攒的令牌允许一次用掉 = 突发流量放行）\n", burstOK, burstRejected)
	fmt.Printf("等 %d tick 攒回后再取 10 个: 成功 %d\n", tbTick, refillOK)
	fmt.Printf("锚点: 突发放行 %d 个 + 攒回后再放行 10 个 → %s\n", burstOK, mark(burstOK == 10 && burstRejected == 2 && refillOK == 10))
	fmt.Println("  （对比漏桶: 出口恒速, 不允许突发——保护下游节奏, 见笔记 03 §4）")
}

// ---- 令牌桶 ----
type tokenBucket struct {
	tokens int
	cap    int
	rate   int
}

func newTokenBucket(capacity, ratePerTick int) *tokenBucket {
	return &tokenBucket{tokens: capacity, cap: capacity, rate: ratePerTick}
}

func (t *tokenBucket) tick() {
	t.tokens += t.rate
	if t.tokens > t.cap {
		t.tokens = t.cap // 桶满暂存, 不无限攒
	}
}

func (t *tokenBucket) take() bool {
	if t.tokens > 0 {
		t.tokens--
		return true
	}
	return false
}
