package main

import (
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
)

// 实验 08：缓存三问小重现
//   击穿: 热点 key 失效瞬间, 无锁并发重建（100 并发 → DB 查 100 次）
//         vs 单飞重建（100 并发 → DB 查 1 次, 其余共享结果）
//   穿透: 空值缓存挡住"不存在的 key"的重复回源
//   雪崩: TTL 加随机抖动后的到期分布
// 锚点: 无锁 100 次 vs 有锁 1 次; 空值缓存后 DB 查询 100→1; TTL 抖动摊开到期时间。

// ---- mockDB: 统计查询次数（并发安全） ----
type mockDB struct {
	queries atomic.Int64
}

func (d *mockDB) query(key string) (string, bool) {
	d.queries.Add(1)
	return "value-of-" + key, true
}

func (d *mockDB) queryCount() int64 { return d.queries.Load() }

// ---- Part 1: 击穿——无锁 vs 单飞 ----

// stampedeLoad: 模拟热点 key 失效, 100 个并发请求同时回源
func stampedeLoad(db *mockDB, singleFlight bool) {
	const concurrency = 100

	var mu sync.Mutex
	var result string
	done := false

	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if singleFlight {
				// 单飞（简化版 singleflight）: 互斥 + 双检,
				// 第一个进门者查库重建, 其余拿锁后发现 done 直接共享结果
				mu.Lock()
				if !done {
					result, _ = db.query("hot-key")
					done = true
				}
				r := result
				mu.Unlock()
				_ = r
			} else {
				// 无锁: 每个请求都自己查库（击穿）
				db.query("hot-key")
			}
		}()
	}
	wg.Wait()
}

// ---- Part 2: 穿透——空值缓存 ----

type nullValueCache struct {
	mu    sync.Mutex
	store map[string]string // key → value 或 "__NULL__"
	db    *mockDB
}

func newNullValueCache(db *mockDB) *nullValueCache {
	return &nullValueCache{store: map[string]string{}, db: db}
}

func (c *nullValueCache) get(key string) (string, bool) {
	c.mu.Lock()
	v, ok := c.store[key]
	c.mu.Unlock()
	if ok {
		if v == "__NULL__" {
			return "", false // 空值命中: 不再回源
		}
		return v, true
	}
	// miss → 回源（真实场景是查 DB 不存在, 这里模拟为不存在的 key）
	c.db.query(key)
	c.mu.Lock()
	c.store[key] = "__NULL__" // 空值标记, 短 TTL
	c.mu.Unlock()
	return "", false
}

// ---- Part 3: 雪崩——TTL 抖动 ----

func ttlJitterSpread() (min, max int, sorted []int) {
	rng := rand.New(rand.NewSource(7))
	ttls := make([]int, 0, 1000)
	for i := 0; i < 1000; i++ {
		base := 300 // 基础 TTL 300s
		jitter := rng.Intn(121) - 60 // ±20% (±60s)
		ttls = append(ttls, base+jitter)
	}
	// 统计同一秒到期的最大堆积
	bucket := map[int]int{}
	for _, t := range ttls {
		bucket[t]++
	}
	maxPile := 0
	for _, c := range bucket {
		if c > maxPile {
			maxPile = c
		}
	}
	sorted = append(sorted, ttls...)
	// 插入排序找 min/max（避免引入 sort 也行, 但直接扫一遍）
	mn, mx := ttls[0], ttls[0]
	for _, t := range ttls {
		if t < mn {
			mn = t
		}
		if t > mx {
			mx = t
		}
	}
	_ = maxPile
	return mn, mx, sorted
}

func RunCacheExperiments() {
	fmt.Println("== 实验 08: 缓存三问——击穿/穿透/雪崩小重现 ==")

	// ---- Part 1: 击穿 ----
	fmt.Println("--- Part 1: 击穿——热 key 失效瞬间的并发重建 ---")
	dbNoLock := &mockDB{}
	stampedeLoad(dbNoLock, false)
	dbSingle := &mockDB{}
	stampedeLoad(dbSingle, true)

	fmt.Printf("  无锁重建   : 100 并发 → DB 查询 %d 次（全部透传, 击穿）\n", dbNoLock.queryCount())
	fmt.Printf("  单飞重建   : 100 并发 → DB 查询 %d 次（1 次重建, 99 次共享结果）\n", dbSingle.queryCount())
	fmt.Printf("  锚点: DB 压力 100 → 1, 降 %d%% → %s\n",
		(100-dbSingle.queryCount())*100/100, mark(dbSingle.queryCount() == 1))

	// ---- Part 2: 穿透 ----
	fmt.Println("\n--- Part 2: 穿透——空值缓存挡住不存在的 key ---")
	db2 := &mockDB{}
	cache := newNullValueCache(db2)
	// 恶意流量: 100 个请求查同一个不存在的 key
	for i := 0; i < 100; i++ {
		cache.get("random-attack-key")
	}
	fmt.Printf("  首次 miss 后回源: DB 查询 %d 次（若 100 次则 DB 被打穿）\n", db2.queryCount())
	fmt.Printf("  锚点: 100 个恶意请求 → DB 只承受 1 次 → %s\n", mark(db2.queryCount() == 1))
	fmt.Println("  （生产再加布隆过滤器: 不存在的 key 连空值缓存都不用碰）")

	// ---- Part 3: 雪崩 ----
	fmt.Println("\n--- Part 3: 雪崩——TTL 随机抖动摊开到期时间 ---")
	mn, mx, ttls := ttlJitterSpread()
	// 无抖动对照: 1000 个 key 全部 300s 同时到期
	fmt.Printf("  无抖动: 1000 个 key 全部在 t=300s 同时到期 → 回源洪峰 1000\n")
	// 有抖动: 统计同一秒的最大堆积
	bucket := map[int]int{}
	for _, t := range ttls {
		bucket[t]++
	}
	maxPile := 0
	for _, c := range bucket {
		if c > maxPile {
			maxPile = c
		}
	}
	fmt.Printf("  有抖动: TTL ∈ [%d, %d]s, 同一秒最大堆积 %d 个\n", mn, mx, maxPile)
	fmt.Printf("  锚点: 洪峰 1000 → %d, 降 %.1f%% → %s\n",
		maxPile, (1-float64(maxPile)/1000)*100, mark(maxPile < 1000/10))

	fmt.Println("\n→ 结论: 三问的解法分别把『重建次数/回源次数/到期堆积』从 N 压到 1/常数——")
	fmt.Println("  本质都是控制回源速率, 保护容量最贵的 DB 层（笔记 08 §2）")
}
