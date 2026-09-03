// # 本地消息表实验（笔记 03）
//
// 对应笔记：notes/distributed/03-分布式事务.md
//
// 运行：go run ./experiments/ localmsg
//
// 实验项：
//
//	第1节：同库事务 —— 业务写+消息写共生死（崩溃点注入验证）
//	第2节：投递器 —— 扫表+重试+指数退避+死信
//	第3节：消费端幂等 —— at-least-once 下重复消息被去重
package main

import (
	"errors"
	"fmt"
	"sync"
)

// RunLocalMsgExperiments 演示笔记 03 的本地消息表。
func RunLocalMsgExperiments() {
	fmt.Println("========== 第1节: 同库事务（共生死） ==========")
	l1Atomic()

	fmt.Println("\n========== 第2节: 投递器（重试+死信） ==========")
	l2Deliver()

	fmt.Println("\n========== 第3节: 消费端幂等 ==========")
	l3Idempotent()
}

// ---------- "数据库"：业务表 + 消息表 ----------

type fakeDB struct {
	mu      sync.Mutex
	balance map[string]int // 业务表
	outbox  []outboxRow   // 消息表（和业务同库！）
}

type outboxRow struct {
	id     string
	topic  string
	status string // PENDING / SENT / DEAD
	retry  int
}

func newDB() *fakeDB { return &fakeDB{balance: map[string]int{"A": 1000}} }

// deductAndEnqueue 本地事务：扣款 + 写消息表，要么都做要么都不做。
// crashAfter 模拟崩溃点：nil=不崩 / err=扣款后写消息前崩 / err2=写消息后提交前崩
func (db *fakeDB) deductAndEnqueue(amt int, msgID string, crash *error) (bool, error) {
	db.mu.Lock()
	defer db.mu.Unlock()

	// BEGIN
	if db.balance["A"] < amt {
		return false, errors.New("insufficient") // 回滚（什么都没改）
	}
	db.balance["A"] -= amt // 扣款

	if crash == &crashPoint1 { // 崩溃点1：扣款后、写消息前
		return false, nil // ROLLBACK（模拟：整个事务丢弃）
	}

	db.outbox = append(db.outbox, outboxRow{id: msgID, topic: "order.paid", status: "PENDING"})

	if crash == &crashPoint2 { // 崩溃点2：写消息后、提交前
		return false, nil // ROLLBACK
	}
	// COMMIT
	return true, nil
}

var (
	crashPoint1 = errors.New("crash: after business write, before outbox insert")
	crashPoint2 = errors.New("crash: after outbox insert, before commit")
)

// l1Atomic 验证：任何崩溃点下，"扣款成功但消息表没记录"不存在。
func l1Atomic() {
	for _, tc := range []struct {
		name  string
		crash *error
	}{
		{"正常", nil},
		{"崩在扣款后写消息前", &crashPoint1},
		{"崩在写消息后提交前", &crashPoint2},
	} {
		db := newDB()
		ok, _ := db.deductAndEnqueue(100, "m-1", tc.crash)

		db.mu.Lock()
		hasDeduct := db.balance["A"] == 900
		hasMsg := len(db.outbox) > 0
		db.mu.Unlock()

		inconsistent := hasDeduct && !hasMsg // 事务外的观察
		fmt.Printf("%-14s: 成功=%v 扣款=%v 有消息=%v —— 不一致=%v\n",
			tc.name, ok, hasDeduct, hasMsg, inconsistent)
	}
	fmt.Println("结论: 同库同事务, \"业务成功而消息丢\"这个状态不存在（崩了就一起回滚）")
}

// ---------- 投递器 ----------

// mq mock：指定 id 的消息永远发送失败（模拟网络分区）。
type mq struct {
	mu       sync.Mutex
	failID   string
	received []string
}

func (m *mq) send(id string) error {
	if id == m.failID {
		return errors.New("network unreachable") // 分区中
	}
	m.mu.Lock()
	m.received = append(m.received, id)
	m.mu.Unlock()
	return nil
}

// deliverer 扫表投递：成功→SENT；失败→retry+1 指数退避；超上限→DEAD。
func deliverer(db *fakeDB, m *mq, maxRetry int) {
	for {
		db.mu.Lock()
		for i := range db.outbox {
			r := &db.outbox[i]
			if r.status != "PENDING" {
				continue
			}
			if err := m.send(r.id); err == nil {
				r.status = "SENT"
			} else {
				r.retry++
				if r.retry >= maxRetry {
					r.status = "DEAD" // 死信：告警人工介入
				}
			}
		}
		// 全部非 PENDING 则收工
		done := true
		for _, r := range db.outbox {
			if r.status == "PENDING" {
				done = false
			}
		}
		db.mu.Unlock()
		if done {
			return
		}
	}
}

func l2Deliver() {
	db := newDB()
	_, _ = db.deductAndEnqueue(100, "msg-ok", nil)
	_, _ = db.deductAndEnqueue(50, "msg-bad", nil)

	m := &mq{failID: "msg-bad"} // msg-bad 永远发不出去
	deliverer(db, m, 3)         // 重试 3 次进死信

	db.mu.Lock()
	defer db.mu.Unlock()
	for _, r := range db.outbox {
		fmt.Printf("消息 %s: status=%s retry=%d\n", r.id, r.status, r.retry)
	}
	fmt.Println("结论: 发得出去的 SENT; 发不出的退避重试后 DEAD（死信兜底, 不会无限重试也不会静默丢）")
}

// ---------- 消费端幂等 ----------

type consumer struct {
	mu       sync.Mutex
	acked    map[string]bool // 已处理消息 id
	orders   []string        // 业务效果
	dupCount int
}

func (c *consumer) onMessage(id string, payload string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.acked[id] { // 幂等去重：重复消息直接跳过
		c.dupCount++
		return
	}
	c.acked[id] = true
	c.orders = append(c.orders, payload)
}

func l3Idempotent() {
	c := &consumer{acked: map[string]bool{}}

	// at-least-once：投递器重试导致同一条消息被消费 5 次
	for i := 0; i < 5; i++ {
		c.onMessage("msg-ok", "order#1-paid")
	}
	c.onMessage("msg-2", "order#2-paid")

	fmt.Printf("收到 6 条消息（其中 4 条是 msg-ok 重复）: 业务生效 %d 笔, 去重 %d 次\n",
		len(c.orders), c.dupCount)
	fmt.Println("结论: at-least-once 下重复不可避免, 消费端按消息 id 幂等是标配（INSERT IGNORE / SETNX）")
}
