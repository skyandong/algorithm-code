// # DDD 实验（笔记 09）
//
// 对应笔记：notes/design-pattern/09-DDD领域驱动设计.md
//
// 运行：go run ./experiments/ ddd
//
// 实验项：
//
//	第1节：贫血 vs 充血——裸字段谁都能改坏 vs 不变量集中在方法里
//	第2节：值对象 Money——不可变、运算返回新值、相等性
//	第3节：聚合根 Order——私有字段 + 行为守不变量，跨聚合只存 ID
//	第4节：领域事件——事件攒在聚合里，持久化成功后才发布
//	第5节：仓储——消费者侧接口 + 内存实现，应用服务只做编排
package main

import (
	"context"
	"errors"
	"fmt"
)

// RunDDDExperiments 演示笔记 09 的 DDD 战术模式。
func RunDDDExperiments() {
	fmt.Println("========== 第1节: 贫血 vs 充血 ==========")
	d1AnemicVsRich()

	fmt.Println("\n========== 第2节: 值对象 Money ==========")
	d2ValueObject()

	fmt.Println("\n========== 第3节: 聚合根不变量 ==========")
	d3AggregateRoot()

	fmt.Println("\n========== 第4节: 领域事件 ==========")
	d4DomainEvent()

	fmt.Println("\n========== 第5节: 仓储与应用服务 ==========")
	d5Repository()
}

// ---------- 第1节：贫血 vs 充血 ----------

// anemicOrder 贫血模型：全裸字段，不变量没人守。
type anemicOrder struct {
	ID     string
	Status string
	Total  int64
}

// richOrder 充血模型：字段私有，唯一入口是行为方法。
type richOrder struct {
	id     string
	status string
	total  int64
}

func (o *richOrder) pay() error {
	if o.status != "pending" {
		return fmt.Errorf("状态 %s 不可支付", o.status)
	}
	o.status = "paid"
	return nil
}

func d1AnemicVsRich() {
	a := &anemicOrder{ID: "A1", Status: "pending", Total: 100}
	a.Total = -99999 // 贫血：任何代码都能改坏，编译器不管
	a.Status = "cancelled"
	fmt.Printf("贫血: a.Total 被随手改成 %d, a.Status 被改成 %q —— 不变量形同虚设\n", a.Total, a.Status)

	r := &richOrder{id: "R1", status: "pending", total: 100}
	// r.total = -99999 // 编译错误: 字段私有 —— 编译期就挡住
	if err := r.pay(); err != nil {
		fmt.Println("充血: 支付失败:", err)
	}
	fmt.Printf("充血: 支付成功 status=%s；重复支付被拒: %v\n", r.status, r.pay())
}

// ---------- 第2节：值对象 Money ----------

var (
	dddErrNegative      = errors.New("金额不能为负")
	dddErrCurrency      = errors.New("币种不一致")
	dddErrEmptyCurrency = errors.New("币种不能为空")
)

// Money 值对象：不可变，运算返回新值，属性全等即相等。
type Money struct {
	amount   int64 // 最小货币单位（分），float 存钱是事故
	currency string
}

func NewMoney(amount int64, currency string) (Money, error) {
	if amount < 0 {
		return Money{}, dddErrNegative
	}
	if currency == "" {
		return Money{}, dddErrEmptyCurrency
	}
	return Money{amount: amount, currency: currency}, nil
}

// Add 加法：不改自己，返回新值——值对象不可变的落地。
func (m Money) Add(o Money) (Money, error) {
	if m.currency != o.currency {
		return Money{}, fmt.Errorf("%w: %s vs %s", dddErrCurrency, m.currency, o.currency)
	}
	return Money{amount: m.amount + o.amount, currency: m.currency}, nil
}

func (m Money) Equal(o Money) bool { return m == o } // 属性全等即相等

func (m Money) String() string {
	return fmt.Sprintf("%d分%s", m.amount, m.currency)
}

func d2ValueObject() {
	a, _ := NewMoney(1000, "CNY")
	b, _ := NewMoney(2300, "CNY")
	sum, _ := a.Add(b)
	fmt.Printf("值对象: %s + %s = %s（a、b 原值不变: %s, %s）\n", a, b, sum, a, b)

	usd, _ := NewMoney(1000, "USD")
	if _, err := a.Add(usd); err != nil {
		fmt.Printf("值对象: 跨币种相加被拒 —— %v\n", err)
	}
	fmt.Printf("值对象: 相等性按值判断 NewMoney(1000,CNY) == a: %v\n", mustMoney(1000, "CNY") == a)

	if _, err := NewMoney(-1, "CNY"); err != nil {
		fmt.Printf("值对象: 构造即校验 —— %v\n", err)
	}
}

func mustMoney(amount int64, currency string) Money {
	m, err := NewMoney(amount, currency)
	if err != nil {
		panic(err)
	}
	return m
}

// ---------- 第3节：聚合根 ----------

// OrderStatus 状态也是值对象：合法转移集中在聚合根方法里。
type OrderStatus string

const (
	StatusPending   OrderStatus = "pending"
	StatusPaid      OrderStatus = "paid"
	StatusCancelled OrderStatus = "cancelled"
)

type dddOrderItem struct {
	name string
	unit Money
	qty  int
}

// Order 聚合根：外部只引用它；内部明细只能通过方法改——绕过根就绕过了不变量。
type Order struct {
	id      string
	buyerID string // 跨聚合只存 ID，不持有 Buyer 实体
	status  OrderStatus
	items   []dddOrderItem
	events  []any // 领域事件先攒在聚合里（第4节）
}

func NewOrder(id, buyerID string) (*Order, error) {
	if id == "" || buyerID == "" {
		return nil, errors.New("订单 ID 与买家 ID 必填")
	}
	return &Order{id: id, buyerID: buyerID, status: StatusPending}, nil
}

// AddItem 前置校验集中把守：只有待支付订单能加行。
func (o *Order) AddItem(name string, unit Money, qty int) error {
	if o.status != StatusPending {
		return fmt.Errorf("%w: 订单已 %s，不能再加购", dddErrIllegalTransition, o.status)
	}
	if qty <= 0 {
		return errors.New("数量必须为正")
	}
	o.items = append(o.items, dddOrderItem{name: name, unit: unit, qty: qty})
	return nil
}

var (
	dddErrIllegalTransition = errors.New("非法状态转移")
	dddErrAmountMismatch    = errors.New("金额不一致")
)

// Pay 不变量二连：状态机合法转移 + 实付等于应付总额。
func (o *Order) Pay(m Money) error {
	if o.status != StatusPending {
		return fmt.Errorf("%w: 当前 %s 不能支付", dddErrIllegalTransition, o.status)
	}
	total, err := o.Total()
	if err != nil {
		return err
	}
	if !total.Equal(m) {
		return fmt.Errorf("%w: 应付 %s 实付 %s", dddErrAmountMismatch, total, m)
	}
	o.status = StatusPaid
	o.record(OrderPaid{OrderID: o.id, Amount: m})
	return nil
}

// Total 应付总额恒等于明细之和——不变量不在调用方脑子里，在聚合根方法里。
func (o *Order) Total() (Money, error) {
	total := mustMoney(0, "CNY")
	for _, it := range o.items {
		sub, err := it.unit.Add(mustMoney(0, "CNY"))
		if err != nil {
			return Money{}, err
		}
		for i := 1; i < it.qty; i++ {
			sub, err = sub.Add(it.unit)
			if err != nil {
				return Money{}, err
			}
		}
		total, err = total.Add(sub)
		if err != nil {
			return Money{}, err
		}
	}
	return total, nil
}

func (o *Order) Status() OrderStatus { return o.status }

func (o *Order) record(e any) { o.events = append(o.events, e) }

func d3AggregateRoot() {
	o, _ := NewOrder("T1", "buyer-7")
	_ = o.AddItem("键盘", mustMoney(19900, "CNY"), 1)
	_ = o.AddItem("键帽", mustMoney(8900, "CNY"), 2)

	total, _ := o.Total()
	fmt.Printf("聚合根: 下单成功 status=%s, 应付 %s\n", o.Status(), total)

	if err := o.Pay(mustMoney(37600, "CNY")); err != nil { // 19900+8900*2=37700
		fmt.Printf("聚合根: 少付 1 分被拒 —— %v\n", err)
	}
	_ = o.Pay(total)
	fmt.Printf("聚合根: 足额支付 → status=%s, 攒下 %d 个领域事件\n", o.Status(), len(o.events))

	if err := o.AddItem("鼠标", mustMoney(9900, "CNY"), 1); err != nil {
		fmt.Printf("聚合根: 已支付订单加购被拒 —— %v\n", err)
	}
}

// ---------- 第4节：领域事件 ----------

// OrderPaid 过去时：领域里已经发生的事实。
type OrderPaid struct {
	OrderID string
	Amount  Money
}

// EventPublisher 事件出口（消费者侧接口，第5节一起演示）。
type EventPublisher interface {
	Publish(events ...any)
}

type consolePublisher struct{}

func (consolePublisher) Publish(events ...any) {
	for _, e := range events {
		if paid, ok := e.(OrderPaid); ok {
			fmt.Printf("事件总线: OrderPaid{order=%s, amount=%s} → 库存/积分/通知各自订阅\n", paid.OrderID, paid.Amount)
		}
	}
}

func d4DomainEvent() {
	o, _ := NewOrder("E1", "buyer-7")
	_ = o.AddItem("书", mustMoney(4500, "CNY"), 1)
	total, _ := o.Total()
	_ = o.Pay(total)

	pub := consolePublisher{}
	fmt.Printf("领域事件: 支付前聚合里攒着 %d 个事件（还没发生\"发布\"这回事）\n", len(o.events))
	pub.Publish(o.events...) // 持久化成功后才由应用服务发布——事件攒在聚合里的事务语义
}

// ---------- 第5节：仓储与应用服务 ----------

// OrderRepository 仓储接口：领域层定义（消费者侧），基础设施层实现。
type OrderRepository interface {
	ByID(ctx context.Context, id string) (*Order, error)
	Save(ctx context.Context, o *Order) error
}

// memOrderRepo 内存实现：替换成 MySQL/gorm 版，领域层零改动。
type memOrderRepo struct{ m map[string]*Order }

func newMemOrderRepo() *memOrderRepo { return &memOrderRepo{m: map[string]*Order{}} }

func (r *memOrderRepo) ByID(_ context.Context, id string) (*Order, error) {
	o, ok := r.m[id]
	if !ok {
		return nil, fmt.Errorf("订单 %s 不存在", id)
	}
	return o, nil
}

func (r *memOrderRepo) Save(_ context.Context, o *Order) error {
	r.m[o.id] = o
	return nil
}

// OrderApp 应用服务：取聚合 → 调领域方法 → 存回去 → 发布事件。零业务 if。
type OrderApp struct {
	repo OrderRepository
	pub  EventPublisher
}

func (a *OrderApp) PayOrder(ctx context.Context, id string, m Money) error {
	o, err := a.repo.ByID(ctx, id)
	if err != nil {
		return err
	}
	if err := o.Pay(m); err != nil {
		return err
	}
	if err := a.repo.Save(ctx, o); err != nil {
		return err
	}
	a.pub.Publish(o.events...) // 持久化成功才算"发生"
	o.events = nil
	return nil
}

func d5Repository() {
	ctx := context.Background()
	repo := newMemOrderRepo()
	seed, _ := NewOrder("R1", "buyer-7")
	_ = seed.AddItem("显示器", mustMoney(129900, "CNY"), 1)
	_ = repo.Save(ctx, seed)

	app := &OrderApp{repo: repo, pub: consolePublisher{}}

	total, _ := seed.Total()
	if err := app.PayOrder(ctx, "R1", mustMoney(99900, "CNY")); err != nil {
		fmt.Printf("应用服务: 金额不对被领域层拦下 —— %v\n", err)
	}
	if err := app.PayOrder(ctx, "R1", total); err != nil {
		fmt.Println("应用服务: 支付失败:", err)
		return
	}
	saved, _ := repo.ByID(ctx, "R1")
	fmt.Printf("应用服务: 支付落库 → status=%s\n", saved.Status())

	// 仓储接口换 mock：领域层/应用层代码零改动，3 行 stub 完成（01 篇"mock 免费"）
	var _ OrderRepository = newMemOrderRepo()
	fmt.Println("仓储: 接口在领域层定义, 内存/MySQL 实现 可整体替换")
}
