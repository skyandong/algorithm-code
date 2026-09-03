package main

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// 实验 06：号段双 buffer 发号器
// 模拟笔记 06 §3: 当前段用到低水位时异步预取下一段, 用尽后原子指针切换。
// 关键设定: 低水位的语义是"余量消耗时间 > 取段耗时", 所以本实验带速率控制——
//   阶段 A: 无抖动, 快速发 4 万号（速率控制 400k/s）→ 验证唯一性/切换
//   阶段 B: 开启 DB 抖动 200ms, 以 10k/s 发 1 万号 → 验证抖动被预取吸收
// 锚点: 阶段 B 期间 fallback(同步取段)=0 次, 单号最大延迟 << 抖动时长。

const (
	segStep     = 10000                        // 每段大小
	segLowMark  = 0.3                           // 低水位 30%: 剩 3000 号触发预取
	dbJitter    = 200 * time.Millisecond        // 模拟 DB 抖动耗时
)

// segment: 一个号段 buffer
type segment struct {
	start int64 // 段起始(含)
	end   int64 // 段结束(不含)
	cur   atomic.Int64
}

func (s *segment) remaining() int64 { return s.end - s.cur.Load() }

// segmentAllocator: 模拟 DB 段表（行锁分配不相交区间, 多实例安全）
type segmentAllocator struct {
	mu         sync.Mutex
	max        int64
	fetches    int
	jitterMode atomic.Bool // 开启时 fetch 睡 dbJitter
}

func (a *segmentAllocator) fetch() (int64, int64) {
	if a.jitterMode.Load() {
		time.Sleep(dbJitter) // 模拟 DB 抖动
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.max += segStep
	return a.max - segStep, a.max
}

// segmentIDGen: 双 buffer 发号器
type segmentIDGen struct {
	alloc      *segmentAllocator
	cur        atomic.Pointer[segment]
	nextSeg    atomic.Pointer[segment] // 预取备胎
	fetching   atomic.Bool
	switches   atomic.Int64
	preFetches atomic.Int64
	fallbacks  atomic.Int64 // 同步兜底取段次数（预取来不及时的正确性兜底）
}

func newSegmentIDGen(a *segmentAllocator) *segmentIDGen {
	g := &segmentIDGen{alloc: a}
	start, end := a.fetch()
	s := &segment{start: start, end: end}
	s.cur.Store(start)
	g.cur.Store(s)
	return g
}

// next: 发一个号。当前段用尽时切换备胎; 备胎也没有时同步兜底取段。
func (g *segmentIDGen) next() int64 {
	for {
		s := g.cur.Load()
		id := s.cur.Add(1) - 1
		if id < s.end {
			// 发号成功。到达低水位 → 异步预取下一段
			if float64(s.remaining()) < segLowMark*float64(segStep) && g.nextSeg.Load() == nil {
				if g.fetching.CompareAndSwap(false, true) {
					g.preFetches.Add(1)
					go func() {
						defer g.fetching.Store(false)
						start, end := g.alloc.fetch()
						ns := &segment{start: start, end: end}
						ns.cur.Store(start)
						g.nextSeg.Store(ns)
					}()
				}
			}
			return id
		}
		// 当前段用尽 → 原子切到备胎
		if ns := g.nextSeg.Load(); ns != nil {
			if g.cur.CompareAndSwap(s, ns) {
				g.switches.Add(1)
				g.nextSeg.Store(nil)
				continue
			}
			continue // 别的线程已切换, 重试
		}
		// 备胎也没有 → 同步兜底（正确性优先, 牺牲这一次的延迟）
		g.fallbacks.Add(1)
		start, end := g.alloc.fetch()
		ns := &segment{start: start, end: end}
		ns.cur.Store(start)
		g.cur.Store(ns)
	}
}

func RunSegmentExperiments() {
	fmt.Println("== 实验 06: 双 buffer 号段发号器（DB 抖动期间不中断） ==")
	fmt.Printf("段大小 %d, 低水位 %.0f%%（剩 %d 号时预取, 以 10k/s 速率可撑 %.1fs > 抖动 %v）\n",
		segStep, segLowMark*100, int64(segLowMark*float64(segStep)),
		float64(int64(segLowMark*float64(segStep)))/10000.0, dbJitter)

	alloc := &segmentAllocator{}
	gen := newSegmentIDGen(alloc)

	var mu sync.Mutex
	ids := make([]int64, 0, 50000)
	var maxLatA, maxLatB atomic.Int64

	// ---- 阶段 A: 无抖动, 4 goroutine × 10000 号, 每 100 号 sleep 1ms（合计 ~400k/s）----
	var wg sync.WaitGroup
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			local := make([]int64, 0, 10000)
			for i := 0; i < 10000; i++ {
				t0 := time.Now()
				id := gen.next()
				if l := time.Since(t0).Microseconds(); l > maxLatA.Load() {
					maxLatA.Store(l)
				}
				local = append(local, id)
				if i%100 == 99 {
					time.Sleep(time.Millisecond) // 速率控制
				}
			}
			mu.Lock()
			ids = append(ids, local...)
			mu.Unlock()
		}()
	}
	wg.Wait()
	nA := len(ids)

	// ---- 阶段 B: 开启抖动, 单 goroutine × 10000 号, 每 10 号 sleep 1ms（10k/s）----
	alloc.jitterMode.Store(true)
	fallbackBefore := gen.fallbacks.Load()
	for i := 0; i < 10000; i++ {
		t0 := time.Now()
		id := gen.next()
		if l := time.Since(t0).Microseconds(); l > maxLatB.Load() {
			maxLatB.Store(l)
		}
		mu.Lock()
		ids = append(ids, id)
		mu.Unlock()
		if i%10 == 9 {
			time.Sleep(time.Millisecond) // 10k/s
		}
	}
	fallbackDuringB := gen.fallbacks.Load() - fallbackBefore

	// ---- 验收 ----
	total := len(ids)
	fmt.Printf("阶段 A: 无抖动发号 %d（速率 ~400k/s）, 阶段 B: 抖动 %v 下发号 %d（速率 10k/s）\n",
		nA, dbJitter, total-nA)
	fmt.Printf("取段 %d 次, 预取触发 %d 次, 原子切换 %d 次\n",
		alloc.fetches, gen.preFetches.Load(), gen.switches.Load())
	fmt.Printf("阶段 B 同步兜底次数: %d（预期 0——抖动被异步预取吸收）\n", fallbackDuringB)

	fmt.Println("\n--- 验收 ---")
	seen := map[int64]bool{}
	dup := 0
	for _, id := range ids {
		if seen[id] {
			dup++
		}
		seen[id] = true
	}
	fmt.Printf("  无重复          : %d 个重复 → %s\n", dup, mark(dup == 0))
	fmt.Printf("  总量正确        : 去重后 %d == %d → %s\n", len(seen), total, mark(len(seen) == total))

	latA := time.Duration(maxLatA.Load()) * time.Microsecond
	latB := time.Duration(maxLatB.Load()) * time.Microsecond
	fmt.Printf("  阶段A最大发号延迟: %v（无抖动, 微秒级）→ %s\n", latA, mark(latA < 10*time.Millisecond))
	fmt.Printf("  阶段B最大发号延迟: %v << 抖动 %v → %s\n", latB, dbJitter, mark(latB < dbJitter))

	ok := dup == 0 && len(seen) == total && fallbackDuringB == 0 && latB < dbJitter
	if ok {
		fmt.Println("\n→ 结论: DB 抖动 200ms 期间发号不中断——抖动发生在预取窗口内, 被异步吸收 ✓")
		fmt.Println("  原理: 低水位的本质是『余量消耗时间 ≥ 取段耗时』, 速率感知是号段方案的隐含前提")
	} else {
		fmt.Println("\n→ 结论: 存在问题 ✗")
	}
}
