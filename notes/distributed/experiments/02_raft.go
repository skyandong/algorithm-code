// # Raft 选主模拟实验（笔记 02）
//
// 对应笔记：notes/distributed/02-共识：Raft.md
//
// 运行：go run ./experiments/ raft
//
// 实验项：
//
//	第1节：随机超时打散 —— 并发起选, 几乎总有人先醒当选（活锁免疫）
//	第2节：任期逻辑时钟 —— 旧 Leader 复活自动退位
//	第3节：选举限制 —— 日志落后的 Candidate 拿不到票, 已提交日志不丢
//	第4节：分区恢复 —— 少数派 Candidate 的 term 不影响多数派 Leader
package main

import (
	"fmt"
	"math/rand"
	"sort"
)

// RunRaftExperiments 演示笔记 02 的 Raft 选主机制。
func RunRaftExperiments() {
	fmt.Println("========== 第1节: 随机超时打散竞选 ==========")
	r1Timeout()

	fmt.Println("\n========== 第2节: 任期逻辑时钟 ==========")
	r2Term()

	fmt.Println("\n========== 第3节: 选举限制 ==========")
	r3ElectionRestriction()

	fmt.Println("\n========== 第4节: 分区恢复 ==========")
	r4Heal()
}

// ---------- 节点抽象 ----------

type node struct {
	id       int
	term     int // 当前任期（逻辑时钟）
	votedFor int // 本任期投给谁（-1 未投）
	lastLog  int // 最后日志 index（选举限制的判据）
}

// r1Timeout 五个 Follower 同时开始等随机超时, 先醒者赢。
func r1Timeout() {
	const n = 5
	splitCnt, okCnt := 0, 0
	const rounds = 3000

	for i := 0; i < rounds; i++ {
		// 每人抽一个 [150, 300) 的超时
		timeouts := make([]int, n)
		for j := range timeouts {
			timeouts[j] = 150 + rand.Intn(150)
		}
		// 最小者胜出；若最小值有并列（同 ms 醒）→ 本轮竞选分裂
		sorted := append([]int(nil), timeouts...)
		sort.Ints(sorted)
		if sorted[0] == sorted[1] {
			splitCnt++ // 两人同时醒 → 都拉不到过半
			continue
		}
		okCnt++
	}
	fmt.Printf("5 节点随机超时[%d轮]: 当选 %d, 分裂 %d（随机性几乎消灭同时醒）\n", rounds, okCnt, splitCnt)
	fmt.Println("若无随机（所有人同刻醒）: 每轮必然分裂 —— 活锁")
}

// r2Term 旧 Leader 复活: 收到更大 term 自动退位。
func r2Term() {
	oldLeader := &node{id: 1, term: 5, votedFor: 1, lastLog: 10} // 旧 Leader(term=5)
	newLeader := &node{id: 3, term: 6, votedFor: 3, lastLog: 9}  // 分区期当选的新 Leader

	// 旧 Leader 分区恢复后向 Follower 发号施令
	// Follower 已经见过 term=6, 拒绝 term=5 的指令
	follower := &node{id: 2, term: 6, votedFor: 3}
	reject := oldLeader.term < follower.term
	fmt.Printf("旧Leader(term=%d)指令 → 已见过 term=%d 的 Follower: 拒绝=%v\n",
		oldLeader.term, follower.term, reject)

	// 旧 Leader 收到拒绝（附带 term=6）→ 5 < 6 → 自贬
	if newLeader.term > oldLeader.term {
		oldLeader.term, oldLeader.votedFor = newLeader.term, -1
		fmt.Printf("旧Leader 收到更大 term: 退位为 Follower(term=%d) —— 无需显式换届仪式\n", oldLeader.term)
	}
}

// r3ElectionRestriction 投票前检查 Candidate 的日志新旧。
func r3ElectionRestriction() {
	// 5 节点, 已提交日志到 index=10（过半持有）
	committed := 10
	nodes := []*node{
		{0, 1, -1, 10}, // 有完整日志
		{1, 1, -1, 10},
		{2, 1, -1, 10},
		{3, 1, -1, 9},  // 落后
		{4, 1, -1, 3},  // 严重落后
	}

	candidates := []*node{
		{id: 6, term: 2, lastLog: 7},  // 日志落后于已提交 → 必然拿不到过半
		{id: 7, term: 2, lastLog: 10}, // 日志完整 → 过半
	}

	for _, cand := range candidates {
		votes := 0
		for _, f := range nodes {
			// 选举限制: Candidate 的 lastLog 必须 >= 投票者的 lastLog
			// （实现是 (term,index) 字典序比较, 这里简化为 index）
			if cand.lastLog >= f.lastLog {
				votes++
			}
		}
		fmt.Printf("Candidate#%d(lastLog=%d, 已提交=%d): 得票 %d/5 —— %s\n",
			cand.id, cand.lastLog, committed, votes,
			map[bool]string{true: "当选, 已提交日志安全", false: "落选, 不缺已提交日志"}[votes > 2])
	}
	fmt.Println("不变量: 已提交日志过半持有 ∧ 当选者集合与之相交 → 新Leader必持有全部已提交日志")
}

// r4Heal 分区期的少数派 Candidate 疯狂涨 term, 恢复后不打扰多数派（pre-vote 的动机）。
func r4Heal() {
	// 多数派侧: 正常 Leader term=10
	leaderTerm := 10

	// 少数派节点: 分区内选不出主, 反复竞选 term 疯涨
	isolated := &node{id: 9, term: 10}
	for i := 0; i < 5; i++ { // 5 轮竞选全部失败（凑不够过半）
		isolated.term++
	}
	fmt.Printf("少数派节点分区期 5 次竞选失败: term 涨到 %d（高于多数派 Leader 的 %d）\n",
		isolated.term, leaderTerm)

	// 恢复后按原版 Raft: 大 term 会把正常 Leader 打下台（无谓扰动）
	fmt.Println("原版 Raft: 恢复后大 term 打下正常 Leader → 一次不必要的重新选主")
	// pre-vote: 先探询"大家要不要新 Leader", 多数派不理 → 不涨 term, 不扰动
	fmt.Println("pre-vote 补丁: 拉票前先探询, 无人响应就不涨 term —— 工程标配")
}
