package consistenthashcache

import (
	"fmt"
	"math"
	"testing"
)

// 跑法：
//
//	go test -bench=. -benchmem -run=^$ ./engineering/consistenthash/
//
// 这里最有价值的不是「快慢」，而是两个面试必问却很少有人给数字的问题：
//  1. 虚拟节点数 replicas 取多少合适？（看分布不平衡度随 replicas 的收益递减）
//  2. 扩容真的只迁移 1/N 的 key 吗？（看迁移比例是否收敛到理论值）
// 所以用 ReportMetric 报告自定义指标，而不只看 ns/op。

var benchNodes = []string{"redis-1", "redis-2", "redis-3"}
var benchNodesPlus1 = []string{"redis-1", "redis-2", "redis-3", "redis-4"}

// benchKeys 预生成一批 key，质数步长避免与 hash 函数产生共振
var benchKeys = func() []string {
	keys := make([]string, 10000)
	for i := range keys {
		keys[i] = "user:" + itoa(i*7919)
	}
	return keys
}()

// BenchmarkBuildRing 构建开销随 replicas 线性增长（排序是 O(n log n)，n = 节点数 × replicas）。
// 结论用途：replicas 不是越大越好，构建发生在扩容瞬间，太大会拉长扩容耗时。
func BenchmarkBuildRing(b *testing.B) {
	for _, replicas := range []int{10, 50, 150, 500, 1000} {
		b.Run(fmt.Sprintf("replicas=%d", replicas), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				BuildRing(benchNodes, replicas)
			}
		})
	}
}

// BenchmarkRingLookup 查找是二分，replicas 翻 10 倍只多约 3 次比较，
// 所以 replicas 增大几乎不损害读性能——这是「可以放心加大 replicas」的依据。
func BenchmarkRingLookup(b *testing.B) {
	for _, replicas := range []int{10, 50, 150, 500, 1000} {
		r := BuildRing(benchNodes, replicas)
		b.Run(fmt.Sprintf("replicas=%d", replicas), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				r.Lookup(benchKeys[i%len(benchKeys)])
			}
		})
	}
}

// BenchmarkRingDistribution 核心指标：分布不平衡度。
//
// 定义：各节点命中数相对理想均值的变异系数（标准差 / 均值），单位是百分比。
// 0% 表示完全均匀。理想一致性哈希应该随 replicas 增大单调下降并收敛。
//
// 看这个 benchmark 要看「收益递减拐点」：从 10 到 150 改善巨大，
// 从 500 到 1000 基本没变化——这就是生产上常取 100~200 的量化依据。
func BenchmarkRingDistribution(b *testing.B) {
	for _, replicas := range []int{1, 10, 50, 100, 150, 200, 500, 1000} {
		r := BuildRing(benchNodes, replicas)
		b.Run(fmt.Sprintf("replicas=%d", replicas), func(b *testing.B) {
			counts := make(map[string]int, len(benchNodes))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				counts[r.Lookup(benchKeys[i%len(benchKeys)])]++
			}
			b.StopTimer()

			mean := float64(b.N) / float64(len(benchNodes))
			var sumSq float64
			for _, node := range benchNodes {
				diff := float64(counts[node]) - mean
				sumSq += diff * diff
			}
			stddev := math.Sqrt(sumSq / float64(len(benchNodes)))
			cv := stddev / mean * 100 // 变异系数，百分比

			b.ReportMetric(cv, "不平衡度%")
			maxSkew := 0.0
			for _, node := range benchNodes {
				skew := math.Abs(float64(counts[node])-mean) / mean * 100
				if skew > maxSkew {
					maxSkew = skew
				}
			}
			b.ReportMetric(maxSkew, "最大偏离%")
		})
	}
}

// BenchmarkRingReshare 验证一致性哈希的核心承诺：节点数 N → N+1 只迁移约 1/(N+1) 的 key。
// 3 → 4 的理论迁移率是 25%，实测越接近 25% 说明实现越正确。
// 对比组是普通取模 hash（迁移率约 75%），差距就是一致性哈希的价值。
func BenchmarkRingReshare(b *testing.B) {
	oldRing := BuildRing(benchNodes, 150)
	newRing := BuildRing(benchNodesPlus1, 150)

	b.Run("一致性哈希/3->4节点", func(b *testing.B) {
		moved := 0
		for i := 0; i < b.N; i++ {
			k := benchKeys[i%len(benchKeys)]
			if oldRing.Lookup(k) != newRing.Lookup(k) {
				moved++
			}
		}
		b.ReportMetric(float64(moved)/float64(b.N)*100, "迁移%")
	})

	// 对照组：直接取模，扩容时几乎全部 key 要重分布
	b.Run("取模hash/3->4节点", func(b *testing.B) {
		moved := 0
		for i := 0; i < b.N; i++ {
			k := benchKeys[i%len(benchKeys)]
			oldIdx := int(hash(k) % 3)
			newIdx := int(hash(k) % 4)
			if oldIdx != newIdx {
				moved++
			}
		}
		b.ReportMetric(float64(moved)/float64(b.N)*100, "迁移%")
	})
}

// 精准失效 vs 全量清空的「缓存保留率」对比不适合用 benchmark 表达（两者耗时都
// 微秒级，差别在正确性而非性能），改由 cache_test.go 的 TestInvalidateMovedKeys_*
// 用例覆盖：扩容后只失效归属变化的 key，未迁移的 key 仍命中本地缓存。
