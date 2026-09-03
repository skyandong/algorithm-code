// # 并发模式实验（笔记 06）
//
// 对应笔记：notes/design-pattern/06-并发模式.md
//
// 运行：go run ./experiments/ concurrency
//
// 实验项：
//
//	第1节：pipeline —— 阶段串联, 每阶段 defer close 自己的输出
//	第2节：fan-out/fan-in —— N 个 worker 消费同一输入, merge 等齐后关总输出
//	第3节：取消传播 —— 消费者跑路必须通知, 否则泄漏
//	第4节：mini errgroup —— 等齐+错误收集+取消+SetLimit 的手写原理版
package main

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// RunConcurrencyExperiments 演示笔记 06 的并发模式。
func RunConcurrencyExperiments() {
	fmt.Println("========== 第1节: pipeline ==========")
	c1Pipeline()

	fmt.Println("\n========== 第2节: fan-out / fan-in ==========")
	c2FanInOut()

	fmt.Println("\n========== 第3节: 取消传播 ==========")
	c3Cancel()

	fmt.Println("\n========== 第4节: mini errgroup ==========")
	c4Errgroup()
}

// ---------- 第1节：pipeline ----------

func gen(ctx context.Context, nums ...int) <-chan int {
	out := make(chan int)
	go func() {
		defer close(out) // 每阶段关自己的输出（发送方关闭规则）
		for _, n := range nums {
			select {
			case out <- n:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}

func square(ctx context.Context, in <-chan int) <-chan int {
	out := make(chan int)
	go func() {
		defer close(out)
		for n := range in { // 上游 close 后 range 自动结束
			select {
			case out <- n * n:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}

func c1Pipeline() {
	ctx := context.Background()
	var results []int

	// square(square(gen()))：三阶段串联，全部流式
	for v := range square(ctx, square(ctx, gen(ctx, 2, 3, 5))) {
		results = append(results, v)
	}
	fmt.Printf("pipeline(2,3,5) 平方两次: %v（期望 16 81 625）\n", results)
}

// ---------- 第2节：fan-out / fan-in ----------

func merge(ctx context.Context, chans ...<-chan int) <-chan int {
	out := make(chan int)
	var wg sync.WaitGroup
	wg.Add(len(chans))
	for _, c := range chans {
		go func(c <-chan int) {
			defer wg.Done()
			for v := range c {
				select {
				case out <- v:
				case <-ctx.Done():
					return
				}
			}
		}(c)
	}
	go func() {
		wg.Wait()  // 所有输入枯竭后
		close(out) // 才关总输出——多路合流的正确关灯方式
	}()
	return out
}

func c2FanInOut() {
	ctx := context.Background()
	in := gen(ctx, 1, 2, 3, 4, 5, 6, 7, 8)

	// fan-out：4 个 square 并发消费同一个 in
	const workers = 4
	outs := make([]<-chan int, workers)
	for i := range outs {
		outs[i] = square(ctx, in)
	}

	sum := 0
	for v := range merge(ctx, outs...) { // fan-in 汇回一条流
		sum += v
	}
	fmt.Printf("8 个数 fan-out 4 worker 平方求和: %d（期望 204）\n", sum)
}

// ---------- 第3节：取消传播 ----------

func c3Cancel() {
	ctx, cancel := context.WithCancel(context.Background())
	out := square(ctx, gen(ctx, makeRange(1, 1000)...))

	got := 0
	for v := range out { // 消费者只取 3 个就跑路
		got++
		if got == 3 {
			break
		}
		_ = v
	}
	cancel() // ← 不调这个 = 泄漏：gen/square 永远阻塞在 send（实验演示三连）
	time.Sleep(50 * time.Millisecond)
	fmt.Printf("消费 3 个后 cancel: goroutine 不再泄漏（pprof goroutine profile 验证法）\n")
	fmt.Println("规则: 从 channel 读的函数返回前, 必须 cancel 或 close(done)——通知生产者停")
}

func makeRange(a, b int) []int {
	r := make([]int, 0, b-a+1)
	for i := a; i <= b; i++ {
		r = append(r, i)
	}
	return r
}

// ---------- 第4节：mini errgroup ----------

// Group 手写迷你版，还原 golang.org/x/sync/errgroup 的核心机制。
// 生产代码直接用标准库 errgroup——这里展示它替你做了什么。
type Group struct {
	wg       sync.WaitGroup
	cancel   context.CancelFunc
	sem      chan struct{} // SetLimit 的信号量
	mu       sync.Mutex
	firstErr error // 只记第一个错误
}

// NewGroup ① 派生 ctx：任一任务出错自动取消全部。
func NewGroup(ctx context.Context, limit int) (*Group, context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	g := &Group{cancel: cancel}
	if limit > 0 {
		g.sem = make(chan struct{}, limit) // ② 并发上限
	}
	return g, ctx
}

func (g *Group) Go(f func() error) {
	if g.sem != nil {
		g.sem <- struct{}{} // 拿名额（满了排队）
	}
	g.wg.Add(1)
	go func() {
		defer g.wg.Done()
		defer func() {
			if g.sem != nil {
				<-g.sem // 还名额
			}
		}()
		if err := f(); err != nil {
			g.mu.Lock()
			if g.firstErr == nil {
				g.firstErr = err // ③ 首错收集
			}
			g.mu.Unlock()
			g.cancel() // 出错即取消其他任务
		}
	}()
}

func (g *Group) Wait() error {
	g.wg.Wait()
	g.cancel()
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.firstErr
}

func c4Errgroup() {
	// 通过一个含失败任务的批次验证：等齐 + 首错 + 取消传播 + 并发上限
	type task struct {
		name string
		fail bool
	}
	tasks := []task{
		{"t1", false}, {"t2", false}, {"t3", true}, {"t4", false},
	}

	g, ctx := NewGroup(context.Background(), 2) // limit=2
	for _, t := range tasks {
		t := t
		g.Go(func() error {
			select {
			case <-ctx.Done():
				return ctx.Err() // 别的任务已失败：后续任务感知取消
			default:
			}
			if t.fail {
				return fmt.Errorf("task %s failed", t.name)
			}
			return nil
		})
	}
	err := g.Wait()
	fmt.Printf("4 任务(1 失败) limit=2: Wait 返回 err=%v（首错收集+取消传播+上限生效）\n", err)
	fmt.Println("生产替代: g, ctx := errgroup.WithContext(ctx); g.SetLimit(2); g.Go(...); g.Wait()")
}
