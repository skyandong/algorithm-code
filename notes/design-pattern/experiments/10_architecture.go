// # 架构风格实验（笔记 10）
//
// 对应笔记：notes/design-pattern/10-架构风格.md
//
// 运行：go run ./experiments/ arch
//
// 实验项：
//
//	第1节：漏层病——PO 穿透到 API（存储结构变成公共契约）
//	第2节：六边形——被动端口 + 可插拔适配器，换存储核心零改动
//	第3节：CQRS——写模型守不变量，读模型宽表投影
//	第4节：事件溯源——事实重放出状态 + 快照加速
package main

import (
	"context"
	"errors"
	"fmt"
)

// RunArchitectureExperiments 演示笔记 10 的四种架构风格。
func RunArchitectureExperiments() {
	fmt.Println("========== 第1节: 漏层病 ==========")
	a1LeakyLayer()

	fmt.Println("\n========== 第2节: 六边形端口与适配器 ==========")
	a2Hexagonal()

	fmt.Println("\n========== 第3节: CQRS ==========")
	a3CQRS()

	fmt.Println("\n========== 第4节: 事件溯源 ==========")
	a4EventSourcing()
}

// ---------- 第1节：漏层病 ----------

// OrderPO 数据库行结构（dao 层的私有词汇）。
type OrderPO struct {
	ID        string
	Status    string
	Total     int64
	DeletedAt *int64 // DB 加的软删字段——本不该任何人知道
}

func a1LeakyLayer() {
	// 漏层：handler 直接把 PO 当 API 响应返回——存储结构成了公共契约
	po := &OrderPO{ID: "o1", Status: "paid", Total: 9900}
	resp := map[string]any{"id": po.ID, "status": po.Status, "total": po.Total, "deleted_at": po.DeletedAt}
	fmt.Printf("漏层: PO 直接出 API, deleted_at=%v 也被带出去了 —— 改列名/加字段, 前端契约跟着崩\n", resp["deleted_at"])

	// 治理：接口层只收 DTO，翻译在边界完成（六边形端口的消费者侧视角）
	type OrderDTO struct { // 对外词汇，与表结构解耦
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	_ = OrderDTO{ID: po.ID, Status: po.Status}
	fmt.Println("治理: handler 只收 DTO, PO 封死在 dao 层 —— 存储随便改, 契约不动")
}

// ---------- 第2节：六边形 ----------

// archOrder 核心域聚合（09 篇的聚合根，字段私有守不变量）。
type archOrder struct {
	id     string
	status string
	total  int64
}

// OrderStore 被动端口：核心定义，外部实现（消费者侧接口）。
type OrderStore interface {
	ByID(ctx context.Context, id string) (*archOrder, error)
	Save(ctx context.Context, o *archOrder) error
}

// PlaceOrder 核心用例：只认识端口，不认识任何具体存储。
type PlaceOrder struct{ store OrderStore }

func (u *PlaceOrder) Exec(ctx context.Context, id string, cents int64) error {
	o := &archOrder{id: id, status: "pending", total: cents}
	if cents <= 0 {
		return errors.New("金额必须为正")
	}
	return u.store.Save(ctx, o)
}

// memStore 适配器一：内存实现（测试/压测用）。
type memStore struct{ m map[string]*archOrder }

func (s *memStore) ByID(_ context.Context, id string) (*archOrder, error) {
	o, ok := s.m[id]
	if !ok {
		return nil, fmt.Errorf("订单 %s 不存在", id)
	}
	return o, nil
}
func (s *memStore) Save(_ context.Context, o *archOrder) error { s.m[o.id] = o; return nil }

// fakeStore 适配器二：注入故障的假库（失败路径测试用）。
type fakeStore struct{ err error }

func (s *fakeStore) ByID(context.Context, string) (*archOrder, error) { return nil, s.err }
func (s *fakeStore) Save(context.Context, *archOrder) error           { return s.err }

func a2Hexagonal() {
	ctx := context.Background()
	// 同一个核心用例，插上不同适配器——核心代码 diff 为零
	ok := &PlaceOrder{store: &memStore{m: map[string]*archOrder{}}}
	if err := ok.Exec(ctx, "o1", 9900); err != nil {
		fmt.Println("六边形: 意外失败:", err)
		return
	}
	fmt.Println("六边形: 内存适配器下用例成功 —— 测试不连任何真库")

	broken := &PlaceOrder{store: &fakeStore{err: errors.New("连接超时")}}
	if err := broken.Exec(ctx, "o2", 100); err != nil {
		fmt.Printf("六边形: 故障注入适配器下同一核心报错 —— %v\n", err)
	}
	fmt.Println("六边形: 验收 = 核心包的 go list -deps 里看不到 sql/http/proto")
}

// ---------- 第3节：CQRS ----------

// WriteOrder 写模型：聚合 + 不变量（规范化）。
type WriteOrder struct {
	id    string
	items int64
	paid  bool
}

func (o *WriteOrder) Pay(cents int64) error {
	if o.paid {
		return errors.New("已支付")
	}
	if cents != o.items {
		return fmt.Errorf("应付 %d 实付 %d", o.items, cents)
	}
	o.paid = true
	return nil
}

// OrderRow 读模型：宽表 DTO（反规范化），查询直接返回，不碰聚合。
type OrderRow struct {
	ID      string
	Status  string
	Cents   int64
	BuyerNM string // 读侧连买家名字都拍平进来了——写模型根本不认识 BuyerNM
}

// project 写后投影：把写侧事实翻译成读侧形状。
func project(w *WriteOrder, buyer string) OrderRow {
	status := "pending"
	if w.paid {
		status = "paid"
	}
	return OrderRow{ID: w.id, Status: status, Cents: w.items, BuyerNM: buyer}
}

func a3CQRS() {
	w := &WriteOrder{id: "o1", items: 9900}
	if err := w.Pay(100); err != nil { // 写侧：不变量把守
		fmt.Printf("CQRS 写侧: 金额不对被拦 —— %v\n", err)
	}
	_ = w.Pay(9900)

	row := project(w, "王工") // 读侧：宽表投影，查询零装配
	fmt.Printf("CQRS 读侧: 查询直接返回宽表 %+v —— 不用装配合聚合\n", row)
	fmt.Println("CQRS 判据: 读形状(宽表+买家名) ≠ 写形状(纯聚合) → 值得分开; 同形 CRUD 别分")
}

// ---------- 第4节：事件溯源 ----------

// OrderState 状态只是重放出来的缓存视图。
type OrderState struct {
	ID     string
	Status string
	Total  int64
}

// Event 每个事实知道怎么改状态（ApplyTo）。
type Event interface{ ApplyTo(*OrderState) }

type archOrderCreated struct{ id string }

func (e archOrderCreated) ApplyTo(s *OrderState) { s.ID, s.Status = e.id, "pending" }

type archItemAdded struct{ cents int64 }

func (e archItemAdded) ApplyTo(s *OrderState) { s.Total += e.cents }

type archOrderPaid struct{ cents int64 }

func (e archOrderPaid) ApplyTo(s *OrderState) { s.Status, s.Total = "paid", e.cents }

// replay 从头重放事实序列，得到任意时刻的状态。
func replay(events []Event) *OrderState {
	s := &OrderState{}
	for _, e := range events {
		e.ApplyTo(s)
	}
	return s
}

// snapshot 定期快照：从第 N 号事实的状态出发，只重放增量。
func replayFrom(snap *OrderState, after []Event) *OrderState {
	s := *snap // 拷贝快照
	for _, e := range after {
		e.ApplyTo(&s)
	}
	return &s
}

func a4EventSourcing() {
	events := []Event{
		archOrderCreated{id: "o1"},
		archItemAdded{cents: 4900},
		archItemAdded{cents: 5000},
		archOrderPaid{cents: 9900},
	}
	s := replay(events)
	fmt.Printf("事件溯源: 重放 %d 个事实 → 状态 %+v\n", len(events), s)

	old := replay(events[:2]) // 时间旅行：只重放到第 2 个事实
	fmt.Printf("事件溯源: 重放到第 2 个事实 → 状态 %+v （调试\"昨晚它为什么这样\"是原生能力）\n", old)

	snap := replay(events[:3]) // 第 3 号事实时做快照
	fast := replayFrom(snap, events[3:])
	fmt.Printf("事件溯源: 快照+增量重放 → 状态 %+v （与全量重放一致, 千次事件不用每次从头算）\n", fast)
	fmt.Println("事件溯源 vs 审计日志: 日志是旁路记录可删可漏; 事实序列是唯一真相, 状态只是缓存视图")
}
