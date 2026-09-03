// # 分片与一致性哈希实验（笔记 05）
//
// 对应笔记：notes/distributed/05-复制与分片.md
// 工程实现对照：engineering/consistenthash
//
// 运行：go run ./experiments/ sharding
//
// 实验项：
//
//	第1节：朴素取模 vs 一致性哈希 —— 加节点时的迁移量对比
//	第2节：虚拟节点 —— 少节点场景的均衡性
//	第3节：range 热点 —— 顺序写永远打最后一片
package main

import (
	"crypto/md5"
	"encoding/binary"
	"fmt"
	"sort"
)

// RunShardingExperiments 演示笔记 05 的分片。
func RunShardingExperiments() {
	fmt.Println("========== 第1节: 取模 vs 一致性哈希的迁移量 ==========")
	s1Migration()

	fmt.Println("\n========== 第2节: 虚拟节点的均衡性 ==========")
	s2Vnode()

	fmt.Println("\n========== 第3节: range 分片的热点 ==========")
	s3Hotspot()
}

// ---------- 工具 ----------

// hash32 一致性哈希的标准做法（ketama 同款）：md5 取前 4 字节。
// 注意不能用 fnv32a——对 "node-0"/"node-1" 这类相似短串极度聚集
// （实测 5 节点挤在 1.5% 环长内），分布哈希必须选雪崩特性好的。
func hash32(s string) uint32 {
	sum := md5.Sum([]byte(s))
	return binary.BigEndian.Uint32(sum[:4])
}

// ---------- 第1节：迁移量对比 ----------

func s1Migration() {
	// 造 10000 个 key
	keys := make([]string, 10000)
	for i := range keys {
		keys[i] = fmt.Sprintf("user:%d", i)
	}

	// 朴素取模：N=4 → N=5
	movedMod := 0
	for _, k := range keys {
		before := hash32(k) % 4
		after := hash32(k) % 5
		if before != after {
			movedMod++
		}
	}

	// 一致性哈希：环上 4 → 5 节点（无虚拟节点的纯环）
	ring := func(nodes int) map[string]int { // key -> 节点编号
		assign := map[string]int{}
		nodePos := make([]uint32, nodes)
		for i := 0; i < nodes; i++ {
			nodePos[i] = hash32(fmt.Sprintf("node-%d", i))
		}
		sort.Slice(nodePos, func(a, b int) bool { return nodePos[a] < nodePos[b] })
		for _, k := range keys {
			h := hash32(k)
			// 顺时针找第一个节点
			idx := sort.Search(len(nodePos), func(i int) bool { return nodePos[i] >= h })
			if idx == len(nodePos) {
				idx = 0 // 环回绕
			}
			// 反查这个位置属于哪个节点编号（用位置值匹配）
			assign[k] = nodeIndexOf(nodePos[idx], nodes)
		}
		return assign
	}

	before, after := ring(4), ring(5)
	movedCH := 0
	for _, k := range keys {
		if before[k] != after[k] {
			movedCH++
		}
	}

	fmt.Printf("10000 个 key, 4 节点扩到 5 节点:\n")
	fmt.Printf("  朴素取模 %%N:  迁移 %d (%.0f%%) —— N 一变几乎全部重排\n", movedMod, float64(movedMod)/100)
	fmt.Printf("  一致性哈希:   迁移 %d (%.0f%%) —— 理论期望 1/5=20%%\n", movedCH, float64(movedCH)/100)
}

// nodeIndexOf 反查环位置对应的节点编号。
func nodeIndexOf(pos uint32, nodes int) int {
	for i := 0; i < nodes; i++ {
		if hash32(fmt.Sprintf("node-%d", i)) == pos {
			return i
		}
	}
	return -1
}

// ---------- 第2节：虚拟节点 ----------

type vRing struct {
	pos  []uint32 // 排序后的虚拟节点位置
	own  []int    // 每个位置属于哪个物理节点
}

func newVRing(nodes, vnodes int) *vRing {
	r := &vRing{}
	for i := 0; i < nodes; i++ {
		for v := 0; v < vnodes; v++ {
			r.pos = append(r.pos, hash32(fmt.Sprintf("node-%d#v%d", i, v)))
			r.own = append(r.own, i)
		}
	}
	// 按位置排序（own 跟随）
	idx := make([]int, len(r.pos))
	for i := range idx {
		idx[i] = i
	}
	sort.Slice(idx, func(a, b int) bool { return r.pos[idx[a]] < r.pos[idx[b]] })
	pos2, own2 := make([]uint32, len(idx)), make([]int, len(idx))
	for i, j := range idx {
		pos2[i], own2[i] = r.pos[j], r.own[j]
	}
	r.pos, r.own = pos2, own2
	return r
}

func (r *vRing) owner(k string) int {
	h := hash32(k)
	i := sort.Search(len(r.pos), func(i int) bool { return r.pos[i] >= h })
	if i == len(r.pos) {
		i = 0
	}
	return r.own[i]
}

func s2Vnode() {
	keys := make([]string, 10000)
	for i := range keys {
		keys[i] = fmt.Sprintf("obj:%d", i)
	}

	distribution := func(r *vRing, nodes int) (int, int) {
		cnt := make([]int, nodes)
		for _, k := range keys {
			cnt[r.owner(k)]++
		}
		min, max := cnt[0], cnt[0]
		for _, c := range cnt {
			if c < min {
				min = c
			}
			if c > max {
				max = c
			}
		}
		return min, max
	}

	// 无虚拟节点（每物理节点 1 个点）
	min1, max1 := distribution(newVRing(3, 1), 3)
	// 200 虚拟节点
	min2, max2 := distribution(newVRing(3, 200), 3)

	fmt.Printf("3 节点分 10000 key: 无虚拟节点 min=%d max=%d（不均衡 %.1fx）\n",
		min1, max1, float64(max1)/float64(min1))
	fmt.Printf("                  200 虚拟节点  min=%d max=%d（不均衡 %.1fx）\n",
		min2, max2, float64(max2)/float64(min2))
	fmt.Println("工程值: 100~200 vnodes/节点; 还能按机器权重配 vnode 数")
}

// ---------- 第3节：range 热点 ----------

func s3Hotspot() {
	// range 分片: [0,1M) → 分片0, [1M,2M) → 分片1 ... 自增 ID 写入
	const shards = 4
	const shardSize = 1_000_000
	cnt := make([]int, shards)

	for id := 0; id < 8_000_000; id++ { // 800 万自增 ID
		s := id / shardSize
		if s >= shards {
			s = shards - 1 // 尾片兜住
		}
		cnt[s]++
	}
	fmt.Printf("自增 ID 写 range 分片(4片, 每片容量 1M): 分布 %v\n", cnt)
	fmt.Println("最后 4M 全部落在尾片 —— 顺序写热点; 解法: 预分裂/加盐/HBase region split")

	// 对照: hash 分片
	hcnt := make([]int, shards)
	for id := 0; id < 8_000_000; id++ {
		hcnt[hash32(fmt.Sprint(id))%shards]++
	}
	fmt.Printf("同样的 ID 用 hash 分片:                      分布 %v\n", hcnt)
}
