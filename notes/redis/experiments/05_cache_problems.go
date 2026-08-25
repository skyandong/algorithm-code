// # 缓存常见问题
//
// # 穿透(查不存在的数据)
//
// 现象:查询根本不存在的数据,绕过缓存直击 DB(常被恶意攻击利用)。
//
// 解决方案:
//
//   - 布隆过滤器(BF):系统启动时预加载合法 key,请求前先过滤
//   - 缓存空对象:查不到也缓存 null,设短 TTL
//
// 何时用 BF(不是每个缓存都加):
//
//   - 默认缓存空对象,覆盖大多数场景
//   - BF 只在高风险场景值得用:对外可枚举查询(用户名/手机号/ID 探测)、key 空间稀疏、高 QPS + DB 敏感
//   - BF 代价:预加载全量 key、新增 key 同步写入、假阳性、占内存
//   - BF 不支持删除:删一个 key 的 bit 会影响其他 key(bit 是共享的)
//     → Counting Bloom Filter(CBF):把 bit 换成计数器,支持删除
//       加入时计数器+1,删除时计数器-1,计数器=0 时表示不存在
//       代价:内存是普通 BF 的几倍(每个 bit 变成 4bit 或更多计数器)
//
// BF 防"穿透到 DB",不防"服务被瘫痪":
//
//	云厂商 DDoS/WAF       ← 挡洪水,越靠前越好
//	  ↓
//	网关限流              ← 挡低频异常
//	  ↓
//	布隆过滤器            ← 挡穿透到 DB
//	  ↓
//	空值缓存              ← 挡 BF 假阳性漏过的
//	  ↓
//	熔断降级              ← 保命
//
// # 击穿(热点 Key 过期)
//
// 现象:热点 key 过期瞬间,高并发全部打到 DB。
//
// 解决:
//
//   - 互斥锁:只让一个请求查 DB 并回填缓存,其他等待或返回旧值
//   - 逻辑过期:热点数据永不过期(不设 TTL),value 里存逻辑过期时间,后台异步更新
//
// 互斥锁 vs 逻辑过期的取舍:
//
//	互斥锁    强一致,但等待期间请求会阻塞,有超时风险
//	逻辑过期  高可用,但更新期间读到的是旧数据(最终一致)
//
// # 雪崩(大量 Key 同时过期)
//
// 现象:大量 key 在同一时刻过期,DB 瞬间压力暴增。
//
// 解决:
//
//   - 过期时间加随机值,打散过期时间
//   - 多级缓存(本地缓存 + Redis)
//   - 熔断降级保护 DB
//
// # 缓存与数据库一致性
//
// 首选旁路缓存(Cache Aside):更新数据库 → 删除缓存
//
// 为什么删除而不是更新缓存:
// 并发场景下"更新缓存"容易出现写覆盖问题:
//
//	线程A 更新 DB(新值) → 线程B 更新 DB(旧值) → 线程B 更新缓存(旧值) → 线程A 更新缓存(新值)
//	结果:DB 是旧值,缓存是新值 → 不一致
//
// 删除缓存后下次读时回填,避免这个问题。
//
// 延迟双删:先删缓存 → 更新 DB → sleep 几毫秒 → 再删一次
// 解决场景:更新 DB 期间有读线程把旧值回填了缓存。
//
//	T1: 写线程删缓存
//	T2: 读线程发现缓存空,查 DB(旧值),准备回填
//	T3: 写线程更新 DB(新值)
//	T4: 读线程把旧值回填缓存  ← 脏数据
//	T5: 写线程 sleep 后再删一次缓存 → 下次读到新值
//
// 其他方案:
//
//   - MQ 异步重试:删除失败时发消息重试,保证最终一致
//   - Canal 订阅 binlog:监听 DB 变更异步刷缓存,业务代码无侵入

package main

import (
	"context"
	"fmt"
	"hash/fnv"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/singleflight"
)

// ExpCachePenetration 实验15: 缓存穿透 —— 空值缓存拦截
//
// 模拟查询不存在的 key,对比:
//
//	无保护:每次都穿透到 DB(用计数器模拟)
//	空值缓存:第一次穿透后缓存 null,后续请求直接命中缓存
func ExpCachePenetration(ctx context.Context) {
	fmt.Println("=== 实验15: 缓存穿透 —— 空值缓存 ===")

	rdb.Del(ctx, "user:99999")

	var dbHits int64

	// 模拟查询函数:先查缓存,缓存没有查 DB,DB 也没有则缓存空值
	query := func() string {
		val, err := rdb.Get(ctx, "user:99999").Result()
		if err == nil {
			return val // 缓存命中
		}
		// 缓存未命中,查 DB
		atomic.AddInt64(&dbHits, 1)
		dbResult := "" // DB 里也不存在
		if dbResult == "" {
			// 缓存空值,TTL 30s 防止内存浪费
			rdb.Set(ctx, "user:99999", "NULL", 30*time.Second)
			return "NULL"
		}
		return dbResult
	}

	// 100 次并发查询同一个不存在的 key
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Go(func() { query() })
	}
	wg.Wait()

	fmt.Printf("  100 次并发查询不存在的 key\n")
	fmt.Printf("  实际打到 DB 的次数: %d\n", atomic.LoadInt64(&dbHits))
	fmt.Println("  结论: 空值缓存后只有第一次穿透,后续全部命中缓存\n")

	rdb.Del(ctx, "user:99999")
}

// ExpCacheBreakdown 实验16: 缓存击穿 —— 互斥锁保护
//
// 模拟热点 key 过期瞬间的并发击穿,对比有无互斥锁的 DB 访问次数。
//
// 预期输出:
//
//	无锁:  100 次并发全部打到 DB
//	有锁:  只有 1 次打到 DB,其余等待后命中缓存
func ExpCacheBreakdown(ctx context.Context) {
	fmt.Println("=== 实验16: 缓存击穿 —— 互斥锁保护 ===")

	const key = "hotkey"
	const lockKey = "hotkey:lock"
	var dbHitsNoLock, dbHitsWithLock int64

	// 无锁版本:key 过期后 100 个并发全打到 DB
	rdb.Del(ctx, key)
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Go(func() {
			if _, err := rdb.Get(ctx, key).Result(); err != nil {
				// 缓存未命中,直接查 DB
				atomic.AddInt64(&dbHitsNoLock, 1)
				time.Sleep(5 * time.Millisecond) // 模拟 DB 查询
				rdb.Set(ctx, key, "value", 10*time.Second)
			}
		})
	}
	wg.Wait()
	fmt.Printf("  无互斥锁: %d 次请求打到 DB\n", atomic.LoadInt64(&dbHitsNoLock))

	// 有锁版本:用 SET NX 做分布式互斥锁
	rdb.Del(ctx, key, lockKey)
	for i := 0; i < 100; i++ {
		wg.Go(func() {
			if _, err := rdb.Get(ctx, key).Result(); err != nil {
				// 抢锁
				ok, _ := rdb.SetNX(ctx, lockKey, "1", 3*time.Second).Result()
				if ok {
					// 抢到锁,查 DB 并回填
					atomic.AddInt64(&dbHitsWithLock, 1)
					time.Sleep(5 * time.Millisecond)
					rdb.Set(ctx, key, "value", 10*time.Second)
					rdb.Del(ctx, lockKey)
				} else {
					// 未抢到锁,等待后重试
					time.Sleep(10 * time.Millisecond)
					rdb.Get(ctx, key)
				}
			}
		})
	}
	wg.Wait()
	fmt.Printf("  有互斥锁: %d 次请求打到 DB\n", atomic.LoadInt64(&dbHitsWithLock))
	fmt.Println("  结论: 互斥锁把击穿从 N 次 DB 查询降到 1 次\n")

	rdb.Del(ctx, key, lockKey)

	// singleflight 版本
	var dbHitsSF int64
	var sfg singleflight.Group
	rdb.Del(ctx, key)

	for i := 0; i < 100; i++ {
		wg.Go(func() {
			// Do 保证同一个 key 并发时只执行一次函数,其余等待并共享结果
			sfg.Do(key, func() (any, error) {
				atomic.AddInt64(&dbHitsSF, 1)
				time.Sleep(5 * time.Millisecond) // 模拟 DB 查询
				val := "value"
				rdb.Set(ctx, key, val, 10*time.Second)
				return val, nil
			})
		})
	}
	wg.Wait()
	fmt.Printf("  singleflight: %d 次请求打到 DB\n", atomic.LoadInt64(&dbHitsSF))
	fmt.Println("  对比:")
	fmt.Println("    互斥锁    抢不到锁需要重试,逻辑自己控制")
	fmt.Println("    singleflight 等待者直接拿到结果,无需重试,代码更干净\n")

	rdb.Del(ctx, key)
}

// ExpCacheAvalanche 实验17: 缓存雪崩 —— 统一过期 vs 随机过期
//
// 模拟 1000 个 key 统一过期 vs 随机过期,观察过期后一段时间内的缓存命中率分布。
//
// 预期输出:
//
//	统一过期: 过期瞬间命中率骤降到 0,DB 压力集中
//	随机过期: 命中率平滑下降,DB 压力分散
func ExpCacheAvalanche(ctx context.Context) {
	fmt.Println("=== 实验17: 缓存雪崩 —— 统一过期 vs 随机过期 ===")

	const n = 200

	// 统一过期:所有 key 同一个 TTL
	for i := 0; i < n; i++ {
		rdb.Set(ctx, fmt.Sprintf("avalanche:uniform:%d", i), i, 2*time.Second)
	}

	// 随机过期:TTL 在 2~4 秒之间随机
	for i := 0; i < n; i++ {
		jitter := time.Duration(i%2000) * time.Millisecond
		rdb.Set(ctx, fmt.Sprintf("avalanche:random:%d", i), i, 2*time.Second+jitter)
	}

	// 等统一过期的 key 全部过期
	time.Sleep(2200 * time.Millisecond)

	// 查询命中率
	hitUniform, hitRandom := 0, 0
	for i := 0; i < n; i++ {
		if _, err := rdb.Get(ctx, fmt.Sprintf("avalanche:uniform:%d", i)).Result(); err == nil {
			hitUniform++
		}
		if _, err := rdb.Get(ctx, fmt.Sprintf("avalanche:random:%d", i)).Result(); err == nil {
			hitRandom++
		}
	}

	fmt.Printf("  2秒后命中率:\n")
	fmt.Printf("  统一过期: %d/%d (%.0f%%) ← 全部过期,DB 瞬间承压\n",
		hitUniform, n, float64(hitUniform)/n*100)
	fmt.Printf("  随机过期: %d/%d (%.0f%%) ← 部分存活,DB 压力分散\n",
		hitRandom, n, float64(hitRandom)/n*100)
	fmt.Println("  结论: 过期时间加随机值是防雪崩最简单有效的手段\n")

	// 清理
	for i := 0; i < n; i++ {
		rdb.Del(ctx, fmt.Sprintf("avalanche:uniform:%d", i))
		rdb.Del(ctx, fmt.Sprintf("avalanche:random:%d", i))
	}
}

// ExpDelayedDoubleDelete 实验18: 延迟双删 —— 防止读线程回填脏数据
//
// 模拟不双删时读线程把旧值回填缓存的场景:
//
//	T1: 写线程删缓存
//	T2: 读线程缓存 miss,查 DB(旧值),准备回填
//	T3: 写线程更新 DB(新值)
//	T4: 读线程把旧值回填缓存 ← 脏数据
//	T5: 写线程 sleep 后再删一次 → 修复
func ExpDelayedDoubleDelete(ctx context.Context) {
	fmt.Println("=== 实验18: 延迟双删 ===")

	const key = "user:1:name"

	// 初始状态:缓存和 DB 都是旧值
	rdb.Set(ctx, key, "old_value", time.Minute)
	dbValue := "old_value"

	var wg sync.WaitGroup

	// 写线程:延迟双删
	wg.Go(func() {
		// 第一次删缓存
		rdb.Del(ctx, key)
		// 更新 DB
		time.Sleep(5 * time.Millisecond)
		dbValue = "new_value"
		// sleep 后第二次删缓存(等读线程可能的回填完成)
		time.Sleep(50 * time.Millisecond)
		rdb.Del(ctx, key)
	})

	// 读线程:在写线程删缓存后、更新 DB 前读取,把旧值回填
	//
	// 注意:这里用 time.Sleep(2ms) 人为制造了"刚好撞上"的窗口期。
	// 现实中触发条件不止"主动删除",还包括:
	//   1. 缓存自然过期(TTL 到了)恰好发生在写线程更新 DB 之前
	//   2. 高并发下读线程抢先一步查到了 DB 旧值
	// 这两种情况在生产里同样会发生,而且更难复现,延迟双删统一解决。
	//
	// 还有一个更隐蔽的问题:DB 主从同步延迟
	//
	//   写线程更新 master DB(新值)
	//         ↓
	//   读线程查 DB —— 打到了 slave,主从同步还没完成,拿到旧值
	//         ↓
	//   读线程把旧值回填缓存
	//         ↓
	//   写线程第二次删缓存
	//         ↓
	//   下次读又打到 slave,同步还没完 → 又回填旧值
	//
	// 所以延迟双删的 sleep 时间必须覆盖主从同步延迟,否则第二次删完还会被回填。
	// sleep 时间是玄学,网络抖动或 slave 落后严重时都会失效。
	//
	// 更彻底的解法:Canal 订阅 binlog
	// 监听的是 slave 同步完成后的 binlog 事件,确认同步完再删缓存,
	// 从根本上消除主从延迟导致的窗口期。
	wg.Go(func() {
		time.Sleep(2 * time.Millisecond) // 模拟"刚好"在写线程删缓存后、更新 DB 前进来
		val, err := rdb.Get(ctx, key).Result()
		if err != nil {
			// 缓存 miss(主动删除或自然过期都会走到这里)
			// 此时 DB 还是旧值,读线程拿到旧值回填
			val = dbValue
			rdb.Set(ctx, key, val, time.Minute)
		}
		fmt.Printf("  读线程回填值: %s  (此时 DB 还是旧值,已造成脏缓存)\n", val)
	})

	wg.Wait()

	// 双删后再读
	time.Sleep(10 * time.Millisecond)
	final, err := rdb.Get(ctx, key).Result()
	if err != nil {
		final = "(已删除,下次读会从 DB 取到新值)"
	}
	fmt.Printf("  双删后缓存值: %s\n", final)
	fmt.Printf("  DB 当前值:    %s\n", dbValue)
	fmt.Println("  结论: 延迟双删消除了读线程回填旧值的窗口期\n")

	rdb.Del(ctx, key)
}

// bloomKey 用于存储 BF bitmap 的 Redis key
const bloomKey = "bf:demo"

// bloomSize bitmap 大小(bit 数),越大假阳性率越低
const bloomSize = 1 << 17 // 128K bits = 16KB

// bloomHash 用 k 个不同种子的 FNV 哈希模拟 k 个哈希函数,返回 k 个 bit 位置
func bloomHash(item string, k int) []int64 {
	positions := make([]int64, k)
	for i := 0; i < k; i++ {
		h := fnv.New64a()
		// 不同种子:在 item 前拼一个 byte 区分哈希函数
		h.Write([]byte{byte(i)})
		h.Write([]byte(item))
		positions[i] = int64(h.Sum64() % bloomSize)
	}
	return positions
}

// bloomAdd 把一个 item 写入 BF
func bloomAdd(ctx context.Context, item string) {
	for _, pos := range bloomHash(item, 4) {
		rdb.SetBit(ctx, bloomKey, pos, 1)
	}
}

// bloomExists 查询 item 是否在 BF 中
// 返回 true = 可能存在(有假阳性);返回 false = 一定不存在
func bloomExists(ctx context.Context, item string) bool {
	for _, pos := range bloomHash(item, 4) {
		if rdb.GetBit(ctx, bloomKey, pos).Val() == 0 {
			return false
		}
	}
	return true
}

// ExpBloomFilter 实验19: 手撸布隆过滤器(Redis bitmap 实现)
//
// 布隆过滤器底层是一个 bit 数组 + k 个哈希函数:
//
//	写入:对 item 计算 k 个哈希,把对应 bit 位置置 1
//	查询:对 item 计算 k 个哈希,所有 bit 位都是 1 → 可能存在
//	      任意一个 bit 位是 0 → 一定不存在
//
// 假阳性原因:不同 item 的哈希可能撞到同一个 bit 位,导致误判为"存在"。
// 不会假阴性:写入时置的 bit 不会清零,所以存在的一定能查到。
// 不支持删除:删一个 item 的 bit 可能影响其他 item。
//
// 内存对比:
//
//	BF(128K bits):固定 16KB,无论加多少 key
//	Redis SET:每个 key 约 50~100 字节,10000 个 key ≈ 500KB~1MB
//
// 预期输出:
//
//	合法 key 查询:假阴性率 = 0%(一定能查到)
//	非法 key 查询:假阳性率 ≈ 1~3%(bit 碰撞导致误判)
func ExpBloomFilter(ctx context.Context) {
	fmt.Println("=== 实验19: 布隆过滤器(Redis bitmap 手撸) ===")
	fmt.Printf("  bitmap 大小: %d bits = %d KB,哈希函数数量: 4\n\n", bloomSize, bloomSize/8/1024)

	rdb.Del(ctx, bloomKey)

	// 预加载 10000 个合法 key(模拟系统启动时加载 DB 存量数据)
	const total = 10000
	for i := 0; i < total; i++ {
		bloomAdd(ctx, fmt.Sprintf("user:%d", i))
	}

	// 查 BF 实际占用内存
	mem, _ := rdb.MemoryUsage(ctx, bloomKey).Result()
	fmt.Printf("  加载 %d 个合法 key 后 BF 内存: %d KB\n", total, mem/1024)

	// 验证合法 key:全部应该返回存在(无假阴性)
	falseNegative := 0
	for i := 0; i < total; i++ {
		if !bloomExists(ctx, fmt.Sprintf("user:%d", i)) {
			falseNegative++
		}
	}
	fmt.Printf("  合法 key 查询 %d 次,假阴性: %d (%.2f%%)\n",
		total, falseNegative, float64(falseNegative)/total*100)

	// 查询不存在的 key:统计假阳性率
	falsePositive := 0
	for i := total; i < total*2; i++ {
		if bloomExists(ctx, fmt.Sprintf("user:%d", i)) {
			falsePositive++
		}
	}
	fmt.Printf("  非法 key 查询 %d 次,假阳性: %d (%.2f%%)\n",
		total, falsePositive, float64(falsePositive)/total*100)

	fmt.Println("\n  结论:")
	fmt.Println("  - 假阴性率 = 0%:存在的 key 一定能查到,不会漏放")
	fmt.Println("  - 假阳性率 ≈ 1~3%:少量不存在的 key 被误判为存在(可接受)")
	fmt.Println("  - 不支持删除:删一个 key 的 bit 会影响其他 key")
	fmt.Printf("  - 内存极省:%d KB 存了 %d 个 key\n\n", mem/1024, total)

	rdb.Del(ctx, bloomKey)
}
