package main

import (
	"fmt"
	"sync"
	"sync/atomic"
)

// 实验 02：秒杀系统不超卖闭环（内存模拟）
// 模拟笔记 02 的三层防线:
//   L4 Redis 预扣:   atomic Add(-1) 挡量（模拟单命令原子性）
//   L6 MySQL 兜底:   互斥锁模拟行锁 + 条件更新 (stock>0 才扣)
//   对账:            请求结束后三方比对（Redis 余量 / DB 库存 / 订单数）
// 锚点: 10 万并发请求, 库存 1000 → 成功订单恰好 1000, 0 超卖, 0 少卖。

const (
	seckillStock   = 1000   // 初始库存
	seckillReqs    = 100000 // 并发请求数
)

// mockRedisStock: 模拟 Redis DECR/INCR 的原子预扣
type mockRedisStock struct {
	n atomic.Int32
}

func (r *mockRedisStock) decr() int32  { return r.n.Add(-1) }
func (r *mockRedisStock) incr() int32  { return r.n.Add(1) }
func (r *mockRedisStock) value() int32 { return r.n.Load() }

// mockDBStock: 模拟 MySQL 行锁 + 条件更新
type mockDBStock struct {
	mu    sync.Mutex
	stock int
	// 统计: 条件更新成功 / 被拒绝的次数
	updates int
	rejects int
}

// conditionalUpdate: UPDATE stock SET stock=stock-1 WHERE sku=? AND stock>0
// 返回 affected rows (0=拒绝)
func (d *mockDBStock) conditionalUpdate() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.stock > 0 {
		d.stock--
		d.updates++
		return 1
	}
	d.rejects++
	return 0
}

func RunSeckillExperiments() {
	fmt.Println("== 实验 02: 秒杀闭环——10 万并发请求 0 超卖 ==")

	redis := &mockRedisStock{}
	redis.n.Store(seckillStock)
	db := &mockDBStock{stock: seckillStock}

	orders := make([]int, 0, seckillStock)
	var ordersMu sync.Mutex
	var rejectedByRedis, rejectedByDB atomic.Int64
	orderSeq := atomic.Int64{}

	var wg sync.WaitGroup
	for i := 0; i < seckillReqs; i++ {
		wg.Add(1)
		go func(uid int) {
			defer wg.Done()

			// ---- L4: Redis 预扣（原子 DECR）----
			n := redis.decr()
			if n < 0 {
				// 失败必须回滚: 多个失败者各自看到 -1/-2/-3..., 都要还原自己的那次减法
				redis.incr()
				rejectedByRedis.Add(1)
				return
			}

			// ---- L5→L6: MQ 异步后 DB 条件更新兜底 ----
			// (本实验同步模拟; affected=0 说明 Redis 与 DB 状态漂移, 拒绝)
			if db.conditionalUpdate() == 0 {
				redis.incr() // 回滚 Redis, 保持与 DB 一致
				rejectedByDB.Add(1)
				return
			}

			// ---- 抢购成功, 生成订单（订单号 = DB 成功次数）----
			id := orderSeq.Add(1)
			ordersMu.Lock()
			orders = append(orders, int(id))
			ordersMu.Unlock()
		}(i)
	}
	wg.Wait()

	// ---- 对账任务: 三方比对 ----
	fmt.Printf("初始库存        : %d\n", seckillStock)
	fmt.Printf("并发请求数      : %d\n", seckillReqs)
	fmt.Printf("Redis 拒绝数    : %d (预扣后 <0, 已各自回滚)\n", rejectedByRedis.Load())
	fmt.Printf("DB 拒绝数       : %d (条件更新 affected=0)\n", rejectedByDB.Load())
	fmt.Printf("DB 条件更新执行 : %d 次, 成功 %d 次\n", db.updates+db.rejects, db.updates)
	fmt.Printf("成功订单数      : %d\n", len(orders))

	redisLeft := redis.value()
	dbLeft := db.stock
	fmt.Printf("对账: Redis 余量=%d, DB 库存=%d, 订单数=%d\n", redisLeft, dbLeft, len(orders))

	// ---- 验证红线: 不超卖 + 不少卖 ----
	sold := seckillStock - dbLeft
	fmt.Println("\n--- 验收 ---")
	fmt.Printf("  不超卖: 订单数 %d ≤ 库存 %d → %s\n", len(orders), seckillStock, mark(len(orders) <= seckillStock))
	fmt.Printf("  不少卖: 订单数 %d == 实际售出 %d → %s\n", len(orders), sold, mark(len(orders) == sold))
	fmt.Printf("  恰好售罄: DB 余量 == 0 → %s\n", mark(dbLeft == 0))
	fmt.Printf("  Redis/DB 一致: %d == %d → %s\n", redisLeft, dbLeft, mark(int(redisLeft) == dbLeft))

	// 订单号唯一性（模拟 MQ 重复消费的幂等校验视角）
	seen := make(map[int]bool, len(orders))
	dup := 0
	for _, id := range orders {
		if seen[id] {
			dup++
		}
		seen[id] = true
	}
	fmt.Printf("  订单号无重复  : 重复 %d 个 → %s\n", dup, mark(dup == 0))

	if len(orders) == seckillStock && dbLeft == 0 && redisLeft == 0 && dup == 0 {
		fmt.Println("→ 结论: 10 万并发, 1000 库存, 0 超卖 0 少卖, 三方对账一致 ✓")
	} else {
		fmt.Println("→ 结论: 存在不一致! 违反验收红线 ✗")
	}
}
