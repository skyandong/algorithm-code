// # 时钟与顺序实验（笔记 06）
//
// 对应笔记：notes/distributed/06-时钟与顺序.md
//
// 运行：go run ./experiments/ clock
//
// 实验项：
//
//	第1节：LWW 丢写重演 —— 时钟慢的机器覆盖了因果上更新的写
//	第2节：Lamport 时钟 —— 因果链上计数单调；并发无法判别
//	第3节：向量时钟 —— 判定并发，触发裁决而不是静默丢
//	第4节：雪花 ID 生成器 —— 结构解析 + 时钟回拨的拒绝策略
package main

import (
	"errors"
	"fmt"
	"sync"
)

// RunClockExperiments 演示笔记 06 的时钟与顺序。
func RunClockExperiments() {
	fmt.Println("========== 第1节: LWW 丢写重演 ==========")
	k1LWW()

	fmt.Println("\n========== 第2节: Lamport 时钟 ==========")
	k2Lamport()

	fmt.Println("\n========== 第3节: 向量时钟 ==========")
	k3Vector()

	fmt.Println("\n========== 第4节: 雪花 ID ==========")
	k4Snowflake()
}

// ---------- 第1节：LWW ----------

func k1LWW() {
	// 两个客户端写同一份文档, 机器 X 时钟快 1 分钟
	type write struct {
		who      string
		value    string
		physTime int // 本地物理时间戳
		later    bool // 因果上是否更晚
	}
	writes := []write{
		{who: "X(时钟快1min)", value: "旧内容", physTime: 10_01_01, later: false},
		{who: "Y(时钟准)", value: "新内容", physTime: 10_00_05, later: true},
	}

	// LWW: 存储保留时间戳最大的
	winner := writes[0]
	for _, w := range writes[1:] {
		if w.physTime > winner.physTime {
			winner = w
		}
	}
	fmt.Printf("写入顺序: %s 写 %q → %s 写 %q（因果上后者新）\n",
		writes[0].who, writes[0].value, writes[1].who, writes[1].value)
	fmt.Printf("LWW 裁决: 保留 %q（%s 的时间戳更大）\n", winner.value, winner.who)
	fmt.Println("结果: 因果上更新的写被覆盖 —— 静默丢数据（Cassandra LWW 官方承认的语义）")
}

// ---------- 第2节：Lamport ----------

type lamportNode struct {
	clock int
}

func (n *lamportNode) local() int { n.clock++; return n.clock }

func (n *lamportNode) send() int { return n.local() }

func (n *lamportNode) receive(msgTS int) int {
	if msgTS > n.clock {
		n.clock = msgTS
	}
	n.clock++
	return n.clock
}

func k2Lamport() {
	a, b := &lamportNode{}, &lamportNode{}

	// 因果链: a 本地事件 → a 发消息 → b 收到 → b 本地事件
	e1 := a.local()   // a 的写
	e2 := a.send()    // a 发出（消息携带 e2）
	e3 := b.receive(e2) // b 收到
	e4 := b.local()   // b 基于消息的写
	fmt.Printf("因果链 a.write→send→b.recv→b.write: Lamport %d→%d→%d→%d（单调递增 ✓）\n", e1, e2, e3, e4)

	// 并发事件: 另一个没有消息往来的 c
	c := &lamportNode{}
	c1 := c.local()
	c2 := c.local()
	fmt.Printf("并发节点 c 的两个事件: %d, %d —— 计数小不代表先发生（无法判并发）\n", c1, c2)
	fmt.Println("性质: a→b 则 L(a)<L(b)（因果保序）; 反之不成立（L 小可能并发）")
}

// ---------- 第3节：向量时钟 ----------

type vc []int

func (v vc) merge(other vc) vc {
	out := make(vc, len(v))
	for i := range v {
		out[i] = max(v[i], other[i])
	}
	return out
}

func (v vc) lessEq(o vc) bool {
	for i := range v {
		if v[i] > o[i] {
			return false
		}
	}
	return true
}

// relation 判定两事件关系。
func relation(a, b vc) string {
	if a.lessEq(b) && !b.lessEq(a) {
		return "a → b（因果）"
	}
	if b.lessEq(a) && !a.lessEq(b) {
		return "b → a（因果）"
	}
	return "并发"
}

func k3Vector() {
	// 三进程向量时钟
	// 场景1: 因果——P0 写 → 发给 P1 → P1 基于它再写
	p0Write := vc{1, 0, 0}
	p1Write := p0Write.merge(vc{0, 1, 0}) // P1 收到后逐维取 max → {1,1,0}
	p1Write[1]++                          // 本地事件自增 → {1,2,0}
	fmt.Printf("P0写 → P1基于它再写: %s（中间态: merge({1,0,0},{0,1,0}) 后自增）\n", relation(p0Write, p1Write))

	// 场景2: 并发——P1 和 P2 各自本地写, 无消息往来
	p1Local := vc{0, 1, 0}
	p2Local := vc{0, 0, 1}
	fmt.Printf("P1 本地写 vs P2 本地写:  %s\n", relation(p1Local, p2Local))
	fmt.Println("价值: 检测到并发 → 触发裁决（合并/人工/保留双版本）——而不是 LWW 悄悄丢一个")
}

// ---------- 第4节：雪花 ID ----------

type snowflake struct {
	mu        sync.Mutex
	lastTS    int64 // 上次发号的时间戳（毫秒）
	seq       int64 // 同毫秒序列
	workerID  int64
	maxBackms int64 // 回拨容忍阈值
}

var errBackward = errors.New("clock moved backwards, refuse to issue")

func newSnowflake(worker int64) *snowflake { return &snowflake{workerID: worker, maxBackms: 5} }

func (s *snowflake) now() int64 { return fakeWallClock } // 可注入的墙钟

var fakeWallClock int64 = 1000000

// next 生成一个 64 位 ID：1 符号 + 41 时间戳 + 10 机器 + 12 序列。
func (s *snowflake) next() (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ts := s.now()
	if ts < s.lastTS {
		if s.lastTS-ts <= s.maxBackms {
			// 小回拨：借用上一毫秒的序列空间继续（简化：等待墙钟追上, 这里直接顺延 lastTS）
			ts = s.lastTS
		} else {
			return 0, fmt.Errorf("%w: %dms", errBackward, s.lastTS-ts) // 大回拨：拒绝发号
		}
	}

	if ts == s.lastTS {
		s.seq++
		if s.seq >= 1<<12 { // 序列压满 4096：等下一毫秒
			ts++
			for ts <= s.lastTS { // 模拟等待
				ts = s.now() + s.lastTS + 1 - s.now() + ts // 简化：直接跳到 lastTS+1
			}
			s.seq = 0
		}
	} else {
		s.seq = 0
	}
	s.lastTS = ts

	// 1 + 41 + 10 + 12 = 64
	id := (ts << 22) | (s.workerID << 12) | s.seq
	return id, nil
}

func k4Snowflake() {
	sf := newSnowflake(7)

	// 正常连续发号（同毫秒：序列递增）
	ids := make([]int64, 0, 5)
	for i := 0; i < 5; i++ {
		id, err := sf.next()
		if err != nil {
			fmt.Println("err:", err)
			continue
		}
		ids = append(ids, id)
	}
	fmt.Printf("同毫秒连发 5 个: %v\n", ids)
	fmt.Printf("解析首号: ts=%d worker=%d seq=%d\n", ids[0]>>22, (ids[0]>>12)&0x3FF, ids[0]&0xFFF)

	// 时钟回拨 50ms：超过阈值 → 拒绝
	fakeWallClock -= 50
	_, err := sf.next()
	fmt.Printf("时钟回拨 50ms(>阈值5ms): err=%v\n", err != nil)
	fmt.Println("策略: 小回拨等待/借用序列, 大回拨拒服务 —— 唯一性优先于可用性")
	fmt.Println("（Leaf-snowflake 用 ZK 对账时钟; 号段模式则完全时钟无关）")
}
