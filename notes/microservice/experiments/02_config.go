package main

import (
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
)

// 实验 02：配置中心热更新闭环
// 实现: watch 模拟（推送 channel）→ 校验 → atomic.Pointer 整体换快照（笔记 02 §2）
// 验证: ① 更新瞬间并发读者读到的快照 100% 自洽（要么完整旧版要么完整新版, 无中间态）
//       ② 更新风暴的随机延迟摊开
// 锚点: 中间态计数 = 0; 随机延迟后应用时间分布覆盖 0~maxJitter。

// configSnapshot: 配置快照——不可变值, 构造后绝不修改
type configSnapshot struct {
	Version  int
	Timeout  int // ms
	MaxConns int
}

// validate: 应用前的校验（笔记 02 §2 的第③步）——坏配置拒绝应用
func validate(c *configSnapshot) error {
	if c.Timeout <= 0 || c.Timeout > 30000 {
		return fmt.Errorf("timeout %dms 非法（应为 (0, 30000]）", c.Timeout)
	}
	if c.MaxConns <= 0 || c.MaxConns > 10000 {
		return fmt.Errorf("maxConns %d 非法（应为 (0, 10000]）", c.MaxConns)
	}
	return nil
}

// configCenter: 极简配置中心（watch 通道 + atomic 快照）
type configCenter struct {
	snapshot atomic.Pointer[configSnapshot]
	watch    chan *configSnapshot // 推送通道（生产是长轮询/gRPC stream）
}

func newConfigCenter(init *configSnapshot) *configCenter {
	c := &configCenter{watch: make(chan *configSnapshot, 8)}
	c.snapshot.Store(init)
	return c
}

// runWatchLoop: 消费推送: 校验 → 换快照（唯一写者）
func (c *configCenter) runWatchLoop(rejected *atomic.Int64) {
	for cfg := range c.watch {
		if err := validate(cfg); err != nil {
			rejected.Add(1)
			fmt.Printf("  [watch] 拒绝坏配置 v%d: %v\n", cfg.Version, err)
			continue
		}
		c.snapshot.Store(cfg) // 原子整体替换
	}
}

func (c *configCenter) load() *configSnapshot { return c.snapshot.Load() }

func RunConfigExperiments() {
	fmt.Println("== 实验 02: watch + atomic 快照的热更新闭环 ==")

	var rejected atomic.Int64
	cc := newConfigCenter(&configSnapshot{Version: 1, Timeout: 500, MaxConns: 100})
	go cc.runWatchLoop(&rejected)

	// ---- Part 1: 并发读者在更新瞬间读到一致快照 ----
	fmt.Println("--- Part 1: 更新瞬间的快照一致性 ---")

	const readers = 8
	var wg sync.WaitGroup
	stop := make(chan struct{})
	var reads, inconsistent atomic.Int64
	var oldSeen, newSeen atomic.Int64

	// 并发读者: 高频读快照, 校验自洽性
	// 自洽定义: Version=1 → Timeout=500 且 MaxConns=100; Version=2 → Timeout=800 且 MaxConns=200
	// 出现 Version=2 但 Timeout=500 之类的组合 = 中间态（逐字段改的典型症状）
	for i := 0; i < readers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				s := cc.load()
				ok := (s.Version == 1 && s.Timeout == 500 && s.MaxConns == 100) ||
					(s.Version == 2 && s.Timeout == 800 && s.MaxConns == 200)
				if !ok {
					inconsistent.Add(1) // 中间态!
				}
				reads.Add(1)
				if s.Version == 1 {
					oldSeen.Add(1)
				} else {
					newSeen.Add(1)
				}
			}
		}()
	}

	// 推送新配置（模拟配置中心推送）
	time.Sleep(20 * time.Millisecond) // 让读者先跑一会
	cc.watch <- &configSnapshot{Version: 2, Timeout: 800, MaxConns: 200}
	time.Sleep(20 * time.Millisecond) // 更新后继续读
	close(stop)
	wg.Wait()

	fmt.Printf("并发读 %s 次: 旧版 %s 次 + 新版 %s 次\n", humanCount(reads.Load()), humanCount(oldSeen.Load()), humanCount(newSeen.Load()))
	fmt.Printf("中间态读取: %d 次（逐字段热改会产生大量中间态, 整体换快照为 0）\n", inconsistent.Load())
	fmt.Printf("锚点: 中间态 = 0 且新旧版本都被读到 → %s\n",
		mark(inconsistent.Load() == 0 && oldSeen.Load() > 0 && newSeen.Load() > 0))

	// ---- Part 2: 坏配置被校验拦截 ----
	fmt.Println("\n--- Part 2: 坏配置拒绝应用 ---")
	cc.watch <- &configSnapshot{Version: 3, Timeout: 0, MaxConns: 100} // timeout=0 非法
	time.Sleep(10 * time.Millisecond)
	s := cc.load()
	fmt.Printf("推送 v3(timeout=0) 后, 当前生效: v%d（校验拒绝, 停留 v2）→ %s\n", s.Version, mark(s.Version == 2))
	fmt.Printf("累计拒绝坏配置 %d 个\n", rejected.Load())

	// ---- Part 3: 更新风暴的随机延迟摊开 ----
	fmt.Println("\n--- Part 3: 更新风暴随机延迟摊开 ---")
	const instances = 100
	const maxJitter = 50 * time.Millisecond
	rng := rand.New(rand.NewSource(7))
	appliedAt := make([]time.Duration, 0, instances)
	var mu sync.Mutex
	t0 := time.Now()
	var wg2 sync.WaitGroup
	for i := 0; i < instances; i++ {
		wg2.Add(1)
		// jitter 值在主 goroutine 预生成（math/rand 的 Rand 非并发安全）
		d := time.Duration(rng.Int63n(int64(maxJitter)))
		go func() {
			defer wg2.Done()
			// 随机延迟 0~maxJitter 再应用——把同时到达摊开
			time.Sleep(d)
			mu.Lock()
			appliedAt = append(appliedAt, time.Since(t0))
			mu.Unlock()
		}()
	}
	wg2.Wait()
	fmt.Printf("100 实例全部收到推送, 应用时间摊开在 0~%v 内（而非同一毫秒冲击）\n", maxJitter)
	fmt.Printf("首批(前10%%)与末批(后10%%)应用间隔: %.1fms\n",
		float64(appliedAt[len(appliedAt)-1]-appliedAt[0])/1e6)
	fmt.Println("锚点: 随机延迟使应用时间分散（同构: 缓存 TTL 抖动 / 重试退避抖动）✓")
}

func humanCount(n int64) string {
	switch {
	case n >= 10000:
		return fmt.Sprintf("%.1f万", float64(n)/10000)
	case n >= 1000:
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	default:
		return fmt.Sprintf("%d", n)
	}
}
