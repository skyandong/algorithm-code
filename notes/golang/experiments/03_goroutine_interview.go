// # Goroutine 面试题实验
//
// 对应笔记：notes/golang/03-Goroutine面试题集.md（编程手写题 1-6）
//
// 运行：
//
//	go run ./experiments/ interview
//
// 实验项：
//
//	题1：Goroutine + Channel 基础 — 生产者/消费者，发送方负责 close
//	题2：阻塞读并发安全 Map — Out 唤醒所有等待者，Rd 支持超时
//	题3：高并发 IP 限流 — 100 IP × 1000 并发，三分钟窗口，期望 success: 100
//	题4：定时调用 + panic 恢复 — recover 必须在同一 goroutine
//	题5：WaitGroup 支持 WaitTimeout — 超时返回 true 且调用方负责取消
//	题6：多协程查询切片 + context 取消 — 找到即取消其他 worker
package main

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
)

// RunInterviewExperiments 运行笔记 3 的六道编程手写题 + 一道选择题演示。
func RunInterviewExperiments() {
	fmt.Println("===== 题1：Goroutine + Channel 基础 =====")
	q1ProducerConsumer()

	fmt.Println("\n===== 题2：阻塞读并发安全 Map =====")
	q2BlockingMap()

	fmt.Println("\n===== 题3：高并发 IP 限流 =====")
	q3IPRateLimit()

	fmt.Println("\n===== 题4：定时调用 + panic 恢复 =====")
	q4TimerPanicRecover()

	fmt.Println("\n===== 题5：WaitGroup 支持 WaitTimeout =====")
	q5WaitTimeout()

	fmt.Println("\n===== 题6：多协程查询切片 + context 取消 =====")
	q6ContextCancelSearch()

	fmt.Println("\n===== 题19（选择题演示）：闭包与 range 变量 =====")
	q19RangeClosure()
}

// T 题19：range 变量值副本演示 —— Incr 用指针接收者但作用于副本，不改原 slice。
type T struct {
	V int
}

func (t *T) Incr(wg *sync.WaitGroup) {
	defer wg.Done()
	t.V++
}

func (t *T) Print() {
	time.Sleep(1 * time.Second)
	fmt.Print(t.V)
}

// q19RangeClosure 题19：range 变量在 Go 1.22+ 每次迭代独立；
// Incr 是指针接收者但作用于值副本，ts 元素不被修改；
// Print 打印 0~9 各一次，顺序不确定。
func q19RangeClosure() {
	var wg sync.WaitGroup
	wg.Add(10)

	ts := make([]T, 10)
	for i := 0; i < 10; i++ {
		ts[i] = T{i}
	}

	for _, t := range ts {
		go t.Incr(&wg) // 值副本自增，不影响 ts
	}
	wg.Wait()

	for _, t := range ts {
		go t.Print()
	}
	time.Sleep(2 * time.Second)
	fmt.Println("\n（0~9 各出现一次，但顺序不确定；Incr 修改的是值副本，ts 元素未变）")
}

// q1ProducerConsumer 题1：一个 goroutine 产生 5 个随机数，另一个读取打印。
func q1ProducerConsumer() {
	out := make(chan int)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		defer close(out) // 发送方负责关闭
		for i := 0; i < 5; i++ {
			out <- rand.Intn(100)
		}
	}()

	go func() {
		defer wg.Done()
		for v := range out {
			fmt.Println("读取到：", v)
		}
	}()

	wg.Wait()
	fmt.Println("（所有 goroutine 正常退出后主 goroutine 才退出）")
}

// waiter / blockingMap 题2：阻塞读并发安全 Map。
type waiter struct {
	done chan struct{}
}

type blockingMap struct {
	mu      sync.Mutex
	values  map[string]any
	waiters map[string]map[*waiter]struct{}
}

func newBlockingMap() *blockingMap {
	return &blockingMap{
		values:  make(map[string]any),
		waiters: make(map[string]map[*waiter]struct{}),
	}
}

func (m *blockingMap) Out(key string, value any) {
	m.mu.Lock()
	m.values[key] = value
	for w := range m.waiters[key] {
		close(w.done) // 关闭 channel 唤醒所有等待者
	}
	delete(m.waiters, key)
	m.mu.Unlock()
}

func (m *blockingMap) Rd(key string, timeout time.Duration) any {
	m.mu.Lock()
	if value, ok := m.values[key]; ok {
		m.mu.Unlock()
		return value
	}

	w := &waiter{done: make(chan struct{})}
	if m.waiters[key] == nil {
		m.waiters[key] = make(map[*waiter]struct{})
	}
	m.waiters[key][w] = struct{}{}
	m.mu.Unlock()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-w.done:
		m.mu.Lock()
		value := m.values[key]
		m.mu.Unlock()
		return value
	case <-timer.C:
		m.mu.Lock()
		if _, waiting := m.waiters[key][w]; waiting {
			delete(m.waiters[key], w)
			if len(m.waiters[key]) == 0 {
				delete(m.waiters, key)
			}
			m.mu.Unlock()
			return nil
		}
		value := m.values[key]
		m.mu.Unlock()
		return value
	}
}

func q2BlockingMap() {
	m := newBlockingMap()

	// 场景 1：key 已存在，立即返回
	m.Out("k1", "v1")
	fmt.Println("Rd(k1)：", m.Rd("k1", time.Second), "（已存在，立即返回）")

	// 场景 2：key 不存在，多个等待者被 Out 唤醒
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			fmt.Printf("等待者%d Rd(k2)：%v\n", id, m.Rd("k2", 2*time.Second))
		}(i)
	}
	time.Sleep(100 * time.Millisecond)
	m.Out("k2", "v2") // 唤醒所有等待 k2 的 goroutine
	wg.Wait()

	// 场景 3：key 不存在且超时，返回 nil
	fmt.Println("Rd(missing)：", m.Rd("missing", 300*time.Millisecond), "（超时返回 nil）")
}

// Ban 题3：高并发 IP 限流。
type Ban struct {
	mu       sync.Mutex
	visitIPs map[string]time.Time
}

func NewBan() *Ban {
	return &Ban{visitIPs: make(map[string]time.Time)}
}

// visit 返回 true 表示本次访问被限制，false 表示成功。
func (b *Ban) visit(ip string, now time.Time) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	if last, ok := b.visitIPs[ip]; ok && now.Sub(last) < 3*time.Minute {
		return true
	}
	b.visitIPs[ip] = now
	return false
}

func q3IPRateLimit() {
	ban := NewBan()
	var success atomic.Int64

	var wg sync.WaitGroup
	wg.Add(100 * 1000)

	for i := 0; i < 1000; i++ {
		for j := 0; j < 100; j++ {
			ip := fmt.Sprintf("192.168.1.%d", j)
			go func(ip string) {
				defer wg.Done()
				if !ban.visit(ip, time.Now()) {
					success.Add(1)
				}
			}(ip)
		}
	}

	wg.Wait()
	fmt.Println("success:", success.Load(), "（每个 IP 三分钟窗口内只允许一次 → 期望 100）")
}

// q4TimerPanicRecover 题4：每秒调用一次 proc，panic 也不能让程序退出。
func q4TimerPanicRecover() {
	proc := func() { panic("ok") }

	callSafely := func(f func()) {
		defer func() {
			if value := recover(); value != nil {
				fmt.Println("recovered:", value)
			}
		}()
		f()
	}

	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	// 只跑 3 个 tick，演示后退出（笔记中是常驻循环）
	done := time.After(650 * time.Millisecond)
	for {
		select {
		case <-ticker.C:
			callSafely(proc)
		case <-done:
			fmt.Println("（演示结束：panic 被 recover，程序未退出）")
			return
		}
	}
}

// WaitTimeout 题5：WaitGroup 支持超时等待。
func WaitTimeout(wg *sync.WaitGroup, timeout time.Duration) bool {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-done:
		return false // 自然结束
	case <-timer.C:
		return true // 超时
	}
}

func q5WaitTimeout() {
	// 场景 1：worker 不退出，WaitTimeout 超时返回 true，调用方负责取消
	var wg sync.WaitGroup
	stop := make(chan struct{})
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case <-stop:
			case <-time.After(10 * time.Second):
			}
		}()
	}

	if WaitTimeout(&wg, 300*time.Millisecond) {
		fmt.Println("场景1：超时（true），调用方 close(stop) 取消 worker")
		close(stop)
		wg.Wait()
		fmt.Println("场景1：所有 worker 已退出")
	}

	// 场景 2：任务快速完成，WaitTimeout 返回 false
	var wg2 sync.WaitGroup
	wg2.Add(3)
	for i := 0; i < 3; i++ {
		go func() {
			defer wg2.Done()
			time.Sleep(50 * time.Millisecond)
		}()
	}
	fmt.Println("场景2：WaitTimeout 返回", WaitTimeout(&wg2, 2*time.Second), "（false = 自然结束）")
}

// findWithTimeout 题6：多协程查找目标值，找到即取消其他 worker。
func findWithTimeout(values []int, target, workers int, timeout time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	found := make(chan struct{}, 1)
	var wg sync.WaitGroup

	if workers > len(values) {
		workers = len(values)
	}
	if workers == 0 {
		fmt.Println("Timeout! Not Found")
		return
	}

	for i := 0; i < workers; i++ {
		start := len(values) * i / workers
		end := len(values) * (i + 1) / workers
		part := values[start:end]

		wg.Go(func() {
			for _, value := range part {
				select {
				case <-ctx.Done():
					return
				default:
				}

				if value == target {
					select {
					case found <- struct{}{}:
						cancel()
					default:
					}
					return
				}
			}
		})
	}

	finished := make(chan struct{})
	go func() {
		wg.Wait()
		close(finished)
	}()

	select {
	case <-found:
		fmt.Println("Found it!")
		cancel()
		<-finished
	case <-finished:
		select {
		case <-found:
			fmt.Println("Found it!")
		default:
			if ctx.Err() == context.DeadlineExceeded {
				fmt.Println("Timeout! Not Found")
			} else {
				fmt.Println("Not Found")
			}
		}
	}
}

func q6ContextCancelSearch() {
	// 场景 1：目标存在 → Found it!
	big := make([]int, 1_000_000)
	for i := range big {
		big[i] = i
	}
	fmt.Print("场景1（目标 777777 存在）：")
	findWithTimeout(big, 777777, 8, 5*time.Second)

	// 场景 2：目标不存在且扫描很快完成 → Not Found（未超时，worker 自然跑完）
	fmt.Print("场景2（目标不存在，扫描完成）：")
	findWithTimeout(big, -1, 8, 5*time.Second)

	// 场景 3：目标不存在 + 每次元素检查较慢 → 超时取消 → Timeout! Not Found
	// 用 sleep 模拟"每个元素耗时较长的查询"（如内存/网络查找），
	// 让扫描必然超过 timeout，确定性演示 ctx.Done() 取消路径。
	fmt.Print("场景3（目标不存在，慢查询触发超时）：")
	findSlowWithTimeout(big, -1, 2, 500*time.Millisecond)
}

// findSlowWithTimeout 与 findWithTimeout 逻辑一致，只是每个元素检查前加微小延时，
// 用于确定性地触发 context 超时取消路径。
func findSlowWithTimeout(values []int, target, workers int, timeout time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	found := make(chan struct{}, 1)
	var wg sync.WaitGroup

	if workers > len(values) {
		workers = len(values)
	}
	if workers == 0 {
		fmt.Println("Timeout! Not Found")
		return
	}

	for i := 0; i < workers; i++ {
		start := len(values) * i / workers
		end := len(values) * (i + 1) / workers
		part := values[start:end]

		wg.Go(func() {
			for _, value := range part {
				select {
				case <-ctx.Done():
					return
				default:
				}
				time.Sleep(2 * time.Microsecond) // 模拟慢查询

				if value == target {
					select {
					case found <- struct{}{}:
						cancel()
					default:
					}
					return
				}
			}
		})
	}

	finished := make(chan struct{})
	go func() {
		wg.Wait()
		close(finished)
	}()

	select {
	case <-found:
		fmt.Println("Found it!")
		cancel()
		<-finished
	case <-finished:
		select {
		case <-found:
			fmt.Println("Found it!")
		default:
			if ctx.Err() == context.DeadlineExceeded {
				fmt.Println("Timeout! Not Found")
			} else {
				fmt.Println("Not Found")
			}
		}
	}
}
