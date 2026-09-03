// # 行为型模式实验（笔记 04）
//
// 对应笔记：notes/design-pattern/04-行为型：策略观察者状态机.md
//
// 运行：go run ./experiments/ behavioral
//
// 实验项：
//
//	第1节：策略三层 —— 匿名函数 / 命名函数 / 函数类型当领域概念
//	第2节：订单状态机 —— 转移表 + 纯函数 Next，非法转移显式报错
//	第3节：命令模式 —— Task 闭包 + Retry 加工
package main

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// RunBehavioralExperiments 演示笔记 04 的行为型模式。
func RunBehavioralExperiments() {
	fmt.Println("========== 第1节: 策略三层 ==========")
	b1Strategy()

	fmt.Println("\n========== 第2节: 订单状态机 ==========")
	b2StateMachine()

	fmt.Println("\n========== 第3节: 命令与重试 ==========")
	b3Command()
}

// ---------- 第1节：策略 ----------

type item struct{ name string }

// b1Strategy 策略三层形态。
func b1Strategy() {
	items := []item{{"banana"}, {"apple"}, {"cherry"}}

	// 层次一：匿名函数（一次性）
	out1 := pick(items, func(a, b item) bool { return a.name < b.name })
	fmt.Printf("匿名策略(最短词): %v\n", out1)

	// 层次二：命名函数（可复用可测试）
	out2 := pick(items, longestName)
	fmt.Printf("命名策略(最长词): %v\n", out2)

	// 层次三：函数类型 = 领域概念（EvictPolicy 这类）
	type Rank func(item) int // 排序维度抽象成类型
	byLen := func(i item) int { return len(i.name) }
	out3 := topItem(items, byLen)
	fmt.Printf("类型策略(按长度榜首): %v\n", out3)
}

func pick(items []item, better func(a, b item) bool) item {
	best := items[0]
	for _, it := range items[1:] {
		if better(it, best) {
			best = it
		}
	}
	return best
}

func longestName(a, b item) bool { return len(a.name) > len(b.name) }

func topItem(items []item, rank func(item) int) item {
	best, bestRank := items[0], rank(items[0])
	for _, it := range items[1:] {
		if r := rank(it); r > bestRank {
			best, bestRank = it, r
		}
	}
	return best
}

// ---------- 第2节：状态机 ----------

type oState string

const (
	stCreated   oState = "created"
	stPaid      oState = "paid"
	stShipped   oState = "shipped"
	stClosed    oState = "closed"
	stCancelled oState = "cancelled"
)

type oEvent string

const (
	evPay    oEvent = "pay"
	evShip   oEvent = "ship"
	evClose  oEvent = "close"
	evCancel oEvent = "cancel"
)

// 转移表：全部规则集中一处，加规则只改表。
var transitions = map[oState]map[oEvent]oState{
	stCreated:   {evPay: stPaid, evCancel: stCancelled},
	stPaid:      {evShip: stShipped, evCancel: stCancelled},
	stShipped:   {evClose: stClosed},
	stCancelled: {},
	stClosed:    {},
}

// Next 纯函数：无锁无副作用，单测零 mock。
func Next(s oState, e oEvent) (oState, error) {
	next, ok := transitions[s][e]
	if !ok {
		return "", fmt.Errorf("invalid transition: %s --%s-->", s, e)
	}
	return next, nil
}

type order struct {
	id    string
	state oState
}

func (o *order) fire(e oEvent) error {
	next, err := Next(o.state, e) // 规则判定（纯）
	if err != nil {
		return fmt.Errorf("order %s: %w", o.id, err) // 执行层只加语境
	}
	o.state = next
	return nil
}

func b2StateMachine() {
	o := &order{id: "A-1", state: stCreated}

	for _, e := range []oEvent{evPay, evShip, evClose} {
		if err := o.fire(e); err != nil {
			fmt.Println("出错:", err)
		}
	}
	fmt.Printf("合法链 pay→ship→close: 终态=%s\n", o.state)

	o2 := &order{id: "A-2", state: stCreated}
	_ = o2.fire(evPay)
	if err := o2.fire(evPay); err != nil {
		fmt.Printf("非法转移 paid--pay-->: err=%v（显式报错, 不是静默留在原态）\n", err)
	}

	// 纯函数的可测性：直接对表断言，不需要构造 order
	_, err := Next(stCancelled, evPay)
	fmt.Printf("纯函数直测 Next(cancelled, pay): %v\n", err != nil)
}

// ---------- 第3节：命令 ----------

// Task 闭包即命令。
type Task func(ctx context.Context) error

// Retry 命令加工（命令+装饰器合体）：上限 + 取消优先。
func Retry(t Task, n int) Task {
	return func(ctx context.Context) error {
		var err error
		for i := 0; i < n; i++ {
			if err = t(ctx); err == nil {
				return nil
			}
			if ctx.Err() != nil {
				return ctx.Err() // 取消优先于重试
			}
		}
		return fmt.Errorf("after %d retries: %w", n, err)
	}
}

func b3Command() {
	attempts := 0
	flaky := func(_ context.Context) error { // 前 2 次失败的"网络抖动"
		attempts++
		if attempts <= 2 {
			return errors.New("transient failure")
		}
		return nil
	}

	// 命令排队执行（任务队列的最小骨架）
	queue := []Task{Retry(flaky, 3)}
	for _, t := range queue {
		if err := t(context.Background()); err != nil {
			fmt.Println("task failed:", err)
		}
	}
	fmt.Printf("flaky 任务第 %d 次尝试成功（Retry 加工了原命令）\n", attempts)

	// 取消优先演示：ctx 已取消时不重试
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	always := func(_ context.Context) error { return errors.New("boom") }
	err := Retry(always, 100)(ctx)
	fmt.Printf("ctx 已取消: err=%v（100 次重试被取消拦下, 不是 boom）\n", err)
	_ = time.Now
}
