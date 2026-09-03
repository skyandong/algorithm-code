// # CAP / quorum 读写实验（笔记 01）
//
// 对应笔记：notes/distributed/01-CAP-BASE-PACELC.md
//
// 运行：go run ./experiments/ cap
//
// 实验项：
//
//	第1节：quorum 可见性 —— W+R>N 时读必见新值；W+R<=N 时能构造出旧读
//	第2节：分区时的选择 —— CP（拒答）vs AP（答旧值）的同一集群两种策略
//	第3节：收敛观测 —— 异步复制下"最终一致"的复制延迟计数
package main

import (
	"fmt"
	"math/rand"
	"sync"
)

// RunCAPExperiments 演示笔记 01 的 CAP 与 quorum。
func RunCAPExperiments() {
	fmt.Println("========== 第1节: quorum 可见性 ==========")
	c1Quorum()

	fmt.Println("\n========== 第2节: 分区时的 C/A 选择 ==========")
	c2Partition()

	fmt.Println("\n========== 第3节: 最终一致的收敛观测 ==========")
	c3Converge()
}

// ---------- 内存集群：N 副本 + quorum 读写 ----------

type replica struct {
	val   int
	ver   int // 版本号，裁决新旧
	alive bool
}

type cluster struct {
	mu   sync.Mutex
	reps []*replica
	n    int
}

func newCluster(n int) *cluster {
	c := &cluster{n: n}
	for i := 0; i < n; i++ {
		c.reps = append(c.reps, &replica{val: 0, ver: 0, alive: true})
	}
	return c
}

// write 向 W 个存活副本写入（随机选，模拟 quorum 写）。
func (c *cluster) write(v, w int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	// 副本随机排列后取前 w 个存活的
	idx := rand.Perm(c.n)
	written := 0
	for _, i := range idx {
		if c.reps[i].alive {
			c.reps[i].val = v
			c.reps[i].ver++
			written++
			if written == w {
				return
			}
		}
	}
}

// read 从 R 个存活副本读，取版本最高的值（quorum 读的标准裁决）。
func (c *cluster) read(r int) (int, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	idx := rand.Perm(c.n)
	best, bestVer, read := 0, -1, 0
	for _, i := range idx {
		rep := c.reps[i]
		if rep.alive {
			if rep.ver > bestVer {
				best, bestVer = rep.val, rep.ver
			}
			read++
			if read == r {
				return best, true
			}
		}
	}
	return 0, false // 可用副本不足 r：拒绝回答（不可用）
}

// c1Quorum W+R>N 交错必然命中新版本；W+R<=N 可以全部读到旧副本。
func c1Quorum() {
	const n = 5
	for _, tc := range []struct {
		w, r int
	}{
		{3, 3}, // W+R=6 > 5：强
		{2, 3}, // W+R=5 = 5：临界（写集合与读集合至少交 0 个——不保证）
		{1, 1}, // W+R=2 < 5：弱
	} {
		stale := 0
		const rounds = 2000
		for i := 0; i < rounds; i++ {
			c := newCluster(n)
			c.write(42, tc.w) // 写入新值 42
			got, ok := c.read(tc.r)
			if !ok || got != 42 {
				stale++ // 读到旧值 0（或拒答）
			}
		}
		fmt.Printf("N=%d W=%d R=%d (W+R%s N): 旧读 %d/%d 次\n",
			n, tc.w, tc.r, map[bool]string{true: ">", false: "≤"}[tc.w+tc.r > n], stale, rounds)
	}
	fmt.Println("结论: W+R>N 时写读集合必相交, 新值必被读到; ≤N 时旧读是常态")
}

// c2Partition 同一集群分区（2/3 分裂）：CP 拒答少数派, AP 答旧值。
func c2Partition() {
	// 5 副本分区：一侧 3（多数派）存活, 一侧 2（少数派）失联
	major := newCluster(3) // 多数派侧的集群抽象
	minor := newCluster(2) // 少数派侧

	major.write(100, 2) // 多数派侧完成一次 quorum 写

	// CP 策略：少数派发现自己联系不上多数（2 < quorum=3），拒答
	if _, ok := minor.read(3); !ok {
		fmt.Println("CP: 少数派(2/5)拒答 read(3) —— 宁可不可用, 不给旧值")
	}

	// AP 策略：少数派本地照答
	v, _ := minor.read(1)
	fmt.Printf("AP: 少数派本地照答 read(1)=%d —— 可用但读到旧值(多数派已写 100)\n", v)

	fmt.Println("同一分区, 两种选择 —— CAP 的全部内容")
}

// c3Converge 异步复制的收敛: 停写后复制轮次内副本追平。
func c3Converge() {
	c := newCluster(5)
	c.write(42, 1) // 只写主副本(异步复制的起点)

	// 模拟异步复制: 每轮把主副本的值搬到一个从副本
	lag := func() int {
		c.mu.Lock()
		defer c.mu.Unlock()
		cnt := 0
		for _, r := range c.reps {
			if r.ver == 0 {
				cnt++
			}
		}
		return cnt
	}
	fmt.Printf("写入后立即: %d/5 副本还是旧值(复制延迟窗口)\n", lag())

	round := 0
	for lag() > 0 { // 复制循环
		round++
		c.mu.Lock()
		for _, r := range c.reps {
			if r.ver == 0 {
				r.val, r.ver = 42, 1
				break // 每轮只追一个
			}
		}
		c.mu.Unlock()
	}
	v, _ := c.read(5)
	fmt.Printf("经过 %d 轮复制: 5/5 收敛到 %d —— 最终一致(收敛轮次可观测可监控)\n", round, v)
}
