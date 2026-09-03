// # 分布式锁正确性实验（笔记 04）
//
// 对应笔记：notes/distributed/04-锁与协调实践.md
//
// 运行：go run ./experiments/ locks
//
// 实验项：
//
//	第1节：TTL 攻击时间线 —— 进程停顿后"自以为持锁"，两个临界区重叠
//	第2节：fencing token —— 旧持锁者的写入被资源侧拒绝
//	第3节：revision 排队锁 vs 抢占锁 —— 公平性与精确唤醒
package main

import (
	"fmt"
	"sort"
	"sync"
)

// RunLocksExperiments 演示笔记 04 的锁正确性。
func RunLocksExperiments() {
	fmt.Println("========== 第1节: TTL 攻击时间线 ==========")
	l1TTLAttack()

	fmt.Println("\n========== 第2节: fencing token ==========")
	l2Fencing()

	fmt.Println("\n========== 第3节: 排队锁 vs 抢占锁 ==========")
	l3QueueVsRace()
}

// ---------- 第1节：进程停顿攻击（对所有 TTL 锁成立） ----------

func l1TTLAttack() {
	type event struct {
		t    int // 逻辑时间 ms
		who  string
		what string
	}
	// 重演笔记里的攻击时间线
	events := []event{
		{0, "A", "拿到锁 TTL=10s"},
		{10000, "锁", "TTL 到期自动开放（A 还没干完）"},
		{10001, "B", "拿到锁进入临界区"},
		{10050, "A", "从 15s 停顿中醒来, 继续按\"我持锁\"干活"},
	}
	for _, e := range events {
		fmt.Printf("  t=%5dms  [%s] %s\n", e.t, e.who, e.what)
	}
	overlapped := true // A 和 B 的临界区在 [10001, ...] 重叠
	fmt.Printf("结果: A/B 临界区重叠=%v（进程停顿攻击对所有基于超时的锁成立）\n", overlapped)
	fmt.Println("缓解: 临界区尽短 + fencing token（见第2节）+ 存储侧强制")
}

// ---------- 第2节：fencing token ----------

type fencedStore struct {
	mu           sync.Mutex
	maxTokenSeen int
	writes       []string // 接受的写
}

// write 资源侧强制校验：token 必须是见过的最大值（旧持锁者被拒）。
func (s *fencedStore) write(token int, who string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if token < s.maxTokenSeen {
		return false // 迟到的旧持锁者
	}
	s.maxTokenSeen = token
	s.writes = append(s.writes, who)
	return true
}

func l2Fencing() {
	store := &fencedStore{}

	// A 拿锁 token=33, B 拿锁 token=34（锁服务单调递增）
	// A 停顿醒来后写入（带 33）, B 已写入过（34）
	okB := store.write(34, "B")
	okA := store.write(33, "A") // A 迟到的写

	fmt.Printf("B 写入(token=34): 接受=%v\n", okB)
	fmt.Printf("A 迟到写入(token=33): 接受=%v（旧 token 被资源侧拒绝）\n", okA)
	fmt.Printf("最终生效: %v —— 互斥的执行点从客户端自觉移到资源侧强制\n", store.writes)
}

// ---------- 第3节：排队锁（etcd/ZK 式） vs 抢占锁（Redis 式） ----------

type queueLock struct {
	mu   sync.Mutex
	next int // revision 分配器
}

// acquire 排队：按到达顺序分配 revision, 最小者持锁。
// 返回 (revision, 是否队头)。
func (q *queueLock) acquire() (int, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	rev := q.next
	q.next++
	return rev, rev == 0 // 队列里只有自己是队头（简化：无释放）
}

func l3QueueVsRace() {
	// 排队锁：5 个竞争者, revision 0..4, 依次持锁
	q := &queueLock{}
	type waiter struct {
		name string
		rev  int
	}
	var waiters []waiter
	for _, name := range []string{"A", "B", "C", "D", "E"} {
		rev, _ := q.acquire()
		waiters = append(waiters, waiter{name, rev})
	}
	sort.Slice(waiters, func(i, j int) bool { return waiters[i].rev < waiters[j].rev })

	fmt.Print("排队锁(etcd revision 序): ")
	for _, w := range waiters {
		fmt.Printf("%s(rev=%d) ", w.name, w.rev)
	}
	fmt.Println("—— 依次持锁, 天然公平 + revision 即 fencing")

	// 抢占锁：同一时刻重试, 谁抢到随机
	pickOwner := func(tries int) map[string]int {
		wins := map[string]int{}
		for i := 0; i < tries; i++ { // 100 轮模拟"重新抢"
			wins[[...]string{"A", "B", "C", "D", "E"}[i%5]]++
			_ = tries
		}
		return wins
	}
	_ = pickOwner

	// 更贴切的模拟：同时起跑一次, 谁先 SET NX 谁赢（确定性演示用随机）
	raceWinner := [...]string{"A", "B", "C", "D", "E"}[2]
	fmt.Printf("抢占锁(Redis SET NX): 本轮 %s 抢到 —— 竞争激烈时全部客户端反复重试（无排队语义）\n", raceWinner)

	fmt.Println("结论: 排队式 watch 前驱精确唤醒; 抢占式失败者轮询风暴; 但排队式延迟=队列长度, 高频短临界区都不合适")
}
