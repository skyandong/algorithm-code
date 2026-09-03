package main

import (
	"fmt"
	"math/rand"
	"sort"
)

// 实验 05：IM 投递可靠性——seqId + ack + 补洞重组
// 模拟笔记 05 §3 的铁三角:
//   1. 服务端为会话分配单调 seqId (1..N)
//   2. 网络投递乱序 + 部分丢失（未 ack 的消息）
//   3. 客户端检测断档 → 触发补洞拉取 → 缓冲重组 → 按序上抛
// 锚点: 乱序+丢包后, 客户端最终收到完整有序无重复的 1..100。

// imServer: 模拟服务端（会话 seqId 分配 + 消息存储 + 补洞拉取）
type imServer struct {
	messages map[int]string // seqId → 内容
	maxSeq   int
}

func (s *imServer) nextSeq() int {
	s.maxSeq++
	return s.maxSeq
}

// fetchRange: 补洞——客户端拉取 [from, to] 区间的消息
func (s *imServer) fetchRange(from, to int) map[int]string {
	out := make(map[int]string)
	for i := from; i <= to; i++ {
		if m, ok := s.messages[i]; ok {
			out[i] = m
		}
	}
	return out
}

// imClient: 模拟客户端（缓冲重组 + 幂等去重 + 断档检测）
type imClient struct {
	recv        map[int]string // 已收（缓冲区, 未上抛）
	ackedUpTo   int            // 已连续上抛的最大 seq
	seen        map[int]bool   // messageId 幂等去重
	delivered   []int          // 最终上抛给 UI 的顺序
	fetchCount  int            // 补洞拉取次数
	dupFiltered int            // 幂等过滤掉的重复
}

func newIMClient() *imClient {
	return &imClient{recv: map[int]string{}, seen: map[int]bool{}}
}

// onMessage: 收到一条消息（可能乱序、可能重复）
func (c *imClient) onMessage(seq int, content string) {
	if c.seen[seq] {
		c.dupFiltered++ // 幂等: 重传的重复消息直接丢弃
		return
	}
	c.seen[seq] = true
	c.recv[seq] = content
}

// flush: 断档检测 + 补洞 + 依序上抛（gap = 从 ackedUpTo+1 开始的连续段）
func (c *imClient) flush(server *imServer) {
	// 找到缓冲区里从 ackedUpTo+1 开始的连续段
	for {
		next := c.ackedUpTo + 1
		if _, ok := c.recv[next]; !ok {
			break
		}
		c.delivered = append(c.delivered, next)
		c.ackedUpTo = next
	}
	// 断档补洞: 服务器还有 > ackedUpTo 的消息, 但缓冲区里 next 缺失
	if server.maxSeq > c.ackedUpTo {
		if _, ok := c.recv[c.ackedUpTo+1]; !ok {
			c.fetchCount++
			holes := server.fetchRange(c.ackedUpTo+1, server.maxSeq)
			// 只接收缓冲区里还没有的（真实实现: 客户端带上已缓冲位点,
			// 服务端只补缺失区间; 这里简化为拉回后本地判重跳过）
			for seq, m := range holes {
				if _, buffered := c.recv[seq]; !buffered {
					c.onMessage(seq, m)
				}
			}
			// 补洞后继续上抛
			for {
				next := c.ackedUpTo + 1
				if _, ok := c.recv[next]; !ok {
					break
				}
				c.delivered = append(c.delivered, next)
				c.ackedUpTo = next
			}
		}
	}
}

func RunIMExperiments() {
	fmt.Println("== 实验 05: seqId + ack + 补洞的投递模拟 ==")

	const msgCount = 100

	// ---- 服务端: 分配 seqId 并"发送" ----
	server := &imServer{messages: map[int]string{}}
	client := newIMClient()

	deliveries := make([]struct {
		seq  int
		drop bool // 模拟丢包（未 ack 触发补洞场景）
	}, 0, msgCount)

	rng := rand.New(rand.NewSource(42)) // 固定种子, 3 次运行输出一致
	for i := 1; i <= msgCount; i++ {
		seq := server.nextSeq()
		server.messages[seq] = fmt.Sprintf("msg-%d", seq)
		if seq%17 == 0 {
			// 该消息"投递失败", 客户端不会收到（drop 字段标 true, 消费侧跳过）
			deliveries = append(deliveries, struct {
				seq  int
				drop bool
			}{seq, true})
			continue
		}
		// 模拟重复投递: seq%23==0 的消息会收到两次
		times := 1
		if seq%23 == 0 {
			times = 2
		}
		for t := 0; t < times; t++ {
			deliveries = append(deliveries, struct {
				seq  int
				drop bool
			}{seq, false})
		}
	}

	// ---- 打乱投递顺序（乱序到达, 用局部反转模拟网络乱序）----
	for i := 0; i < len(deliveries); i += 5 {
		j := min(i+5, len(deliveries))
		for a, b := i, j-1; a < b; a, b = a+1, b-1 {
			deliveries[a], deliveries[b] = deliveries[b], deliveries[a]
		}
	}
	_ = rng // 种子保留备用（本实验打乱用确定性分组反转）

	// ---- 客户端逐条接收, 每 10 条尝试 flush 一次 ----
	for idx, d := range deliveries {
		if d.drop {
			continue // 丢包: 客户端什么都收不到
		}
		client.onMessage(d.seq, server.messages[d.seq])
		if idx%10 == 9 {
			client.flush(server)
		}
	}
	client.flush(server) // 最终 flush

	// ---- 验收 ----
	fmt.Printf("消息总数      : %d (含 %d 条丢失、%d 条重复投递)\n",
		msgCount, msgCount/17, countDup(deliveries))

	fmt.Println("\n--- 验收 ---")
	// 1. 完整: 上抛了全部 100 条
	fmt.Printf("  不丢: 上抛 %d 条 == %d → %s\n", len(client.delivered), msgCount, mark(len(client.delivered) == msgCount))
	// 2. 有序: 上抛序列严格递增
	sorted := sort.IntsAreSorted(client.delivered)
	fmt.Printf("  有序: 上抛序列严格递增 → %s\n", mark(sorted))
	// 3. 无重复
	dup := 0
	seen := map[int]bool{}
	for _, s := range client.delivered {
		if seen[s] {
			dup++
		}
		seen[s] = true
	}
	fmt.Printf("  不重: 重复上抛 %d 条 → %s\n", dup, mark(dup == 0))
	// 4. 精确等于 1..100
	exact := len(client.delivered) == msgCount
	for i, s := range client.delivered {
		if s != i+1 {
			exact = false
			break
		}
	}
	fmt.Printf("  精确还原: 上抛序列 == [1..100] → %s\n", mark(exact))

	fmt.Printf("\n统计: 补洞拉取 %d 次, 幂等过滤重复投递 %d 条\n", client.fetchCount, client.dupFiltered)
	if exact && dup == 0 {
		fmt.Println("→ 结论: 乱序+丢包+重复下, 铁三角保证会话完整、有序、无重复 ✓")
	} else {
		fmt.Println("→ 结论: 重组失败 ✗")
	}
}

func countDup(deliveries []struct {
	seq  int
	drop bool
}) int {
	c := 0
	for _, d := range deliveries {
		if !d.drop && d.seq%23 == 0 {
			c++
		}
	}
	return c
}
