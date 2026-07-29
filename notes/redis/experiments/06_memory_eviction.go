// # 内存管理与淘汰
//
// # 过期 Key 删除机制
//
// Redis 用两种机制组合删除过期 key,平衡 CPU 和内存:
//
// 惰性删除(被动):
//
//	访问某个 key 时才检查是否过期,过期则删除并返回 nil
//	优点:不耗费额外 CPU
//	缺点:过期 key 如果一直不被访问,内存永远不释放
//
// 定期删除(主动):
//
//	每隔 100ms(由 hz 配置,默认 10)随机抽样检查一批 key
//	抽 20 个,删掉过期的,若过期比例 > 25% 继续抽下一批
//	避免长期积压大量过期 key 占内存
//
// 两者的盲区:定期删除是随机抽样,小概率 key 可能长期漏掉。
// 生产里 INFO keyspace 的 expires 字段可以观察待过期 key 数量。
//
// # 大 Key 删除阻塞
//
// DEL 是同步操作:删一个有 100 万元素的 Hash,主线程要把所有内存释放完才返回,
// 期间阻塞所有命令,可能卡几百 ms。
//
// Redis 4.0+ 的解法:
//
//	UNLINK       异步删除,主线程只做"解除引用",实际内存释放交给后台线程
//	lazyfree-lazy-eviction yes   淘汰时异步释放
//	lazyfree-lazy-expire yes     过期时异步释放
//	lazyfree-lazy-server-del yes DEL 内部也走异步
//
// 生产规则:大 key(元素数 > 1万 或 内存 > 1MB)一律用 UNLINK。
//
// # 淘汰策略(maxmemory-policy)
//
// 内存达到 maxmemory 时触发淘汰,8 种策略:
//
//	noeviction        不淘汰,写操作报错(默认)
//	allkeys-lru       所有 key 按 LRU 淘汰
//	allkeys-lfu       所有 key 按 LFU 淘汰(4.0+)
//	allkeys-random    所有 key 随机淘汰
//	volatile-lru      只淘汰设了过期时间的 key,按 LRU
//	volatile-lfu      只淘汰设了过期时间的 key,按 LFU
//	volatile-random   只淘汰设了过期时间的 key,随机
//	volatile-ttl      只淘汰设了过期时间的 key,优先删快过期的
//
// 近似 LRU vs 真 LRU:
// 真 LRU 需要维护全局双向链表,每次访问更新链表,内存和 CPU 开销大。
// Redis 用近似 LRU:随机采样 N 个 key(maxmemory-samples,默认 5),
// 选最久未访问的淘汰。N 越大越接近真 LRU,但 CPU 开销越高。
//
// LFU(Least Frequently Used):
// 用 Morris 计数器记录访问频率(8 bit,最大 255),随时间衰减。
// 比 LRU 更适合热点数据场景:LRU 会把历史热点当冷数据淘汰,LFU 不会。
//
// 怎么选:
//
//	缓存场景(key 可重建)   allkeys-lru 或 allkeys-lfu
//	有冷热数据区分         allkeys-lfu 更精准
//	部分 key 不能淘汰      volatile-lru(不设 TTL 的 key 永不淘汰)
//	兜底保证服务可用       noeviction + 监控告警

package experiments

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// ExpLazyDelete 实验20: 惰性删除 vs 定期删除 —— 内存释放时机
//
// 设置大量短 TTL 的 key,过期后对比:
//   - 不访问:定期删除负责清理,有延迟
//   - 访问过期 key:惰性删除立即触发
//
// 预期输出:
//
//	过期后立即查 INFO keyspace,expires 数量仍 > 0(定期删除还没扫到)
//	访问过期 key 返回 nil(惰性删除触发)
func ExpLazyDelete(ctx context.Context) {
	fmt.Println("=== 实验20: 惰性删除 vs 定期删除 ===")

	// 写入 1000 个 100ms 过期的 key
	const n = 1000
	for i := 0; i < n; i++ {
		Rdb.Set(ctx, fmt.Sprintf("lazy:%d", i), i, 100*time.Millisecond)
	}

	info, _ := Rdb.Info(ctx, "keyspace").Result()
	fmt.Printf("  写入后 keyspace:\n")
	for _, line := range strings.Split(info, "\r\n") {
		if strings.HasPrefix(line, "db") {
			fmt.Printf("    %s\n", line)
		}
	}

	// 等 key 全部过期
	time.Sleep(200 * time.Millisecond)

	// 过期后立即查:定期删除可能还没扫到,内存未必释放
	info, _ = Rdb.Info(ctx, "keyspace").Result()
	fmt.Printf("  过期 200ms 后 keyspace(定期删除可能未扫到):\n")
	hasDB := false
	for _, line := range strings.Split(info, "\r\n") {
		if strings.HasPrefix(line, "db") {
			fmt.Printf("    %s\n", line)
			hasDB = true
		}
	}
	if !hasDB {
		fmt.Println("    (已清空)")
	}

	// 惰性删除:访问过期 key 立即触发删除
	val, err := Rdb.Get(ctx, "lazy:1").Result()
	if err != nil {
		fmt.Println("  访问过期 key:nil ← 惰性删除触发,该 key 已被删除")
	} else {
		fmt.Printf("  访问过期 key:%s\n", val)
	}
	fmt.Println("  结论: 惰性删除访问时立即触发;定期删除有延迟,不保证过期立刻释放内存\n")
}

// ExpDelVsUnlink 实验21: DEL vs UNLINK 阻塞对比
//
// 构造一个大 Hash(10万字段),分别用 DEL 和 UNLINK 删除,对比耗时。
//
// 预期输出:
//
//	DEL   耗时明显(主线程同步释放内存)
//	UNLINK 耗时极短(主线程只解除引用,后台线程异步释放)
func ExpDelVsUnlink(ctx context.Context) {
	fmt.Println("=== 实验21: DEL vs UNLINK 阻塞对比 ===")

	const fields = 100000

	// 构造大 Hash
	build := func(key string) {
		pipe := Rdb.Pipeline()
		for i := 0; i < fields; i += 1000 {
			for j := i; j < i+1000 && j < fields; j++ {
				pipe.HSet(ctx, key, fmt.Sprintf("f%d", j), j)
			}
			pipe.Exec(ctx)
		}
	}

	build("big:del")
	build("big:unlink")

	mem, _ := Rdb.MemoryUsage(ctx, "big:del").Result()
	fmt.Printf("  大 Hash(%d 字段)内存: %d KB\n\n", fields, mem/1024)

	// DEL
	start := time.Now()
	Rdb.Del(ctx, "big:del")
	delDur := time.Since(start)
	fmt.Printf("  DEL    耗时: %v  (主线程同步释放,期间阻塞所有命令)\n", delDur)

	// UNLINK
	start = time.Now()
	Rdb.Unlink(ctx, "big:unlink")
	unlinkDur := time.Since(start)
	fmt.Printf("  UNLINK 耗时: %v  (主线程只解除引用,后台异步释放)\n", unlinkDur)

	fmt.Printf("  提速: %.1fx\n", float64(delDur)/float64(unlinkDur))
	fmt.Println("  结论: 大 key 务必用 UNLINK,DEL 会阻塞主线程\n")
}

// ExpEvictionPolicy 实验22: 淘汰策略 —— LRU vs LFU
//
// 写入一批 key,其中部分高频访问(热点),部分从不访问(冷数据)。
// 触发淘汰后观察哪些 key 被淘汰:
//
//	LRU 淘汰最久未访问的 → 热点 key 如果最近没访问也会被淘汰
//	LFU 淘汰访问频率最低的 → 历史热点 key 频率高,不会被淘汰
func ExpEvictionPolicy(ctx context.Context) {
	fmt.Println("=== 实验22: 淘汰策略 LRU vs LFU ===")

	// 检查 maxmemory 是否配置
	cfg, _ := Rdb.ConfigGet(ctx, "maxmemory").Result()
	maxmem := cfg["maxmemory"]
	if maxmem == "0" {
		fmt.Println("  maxmemory=0(未限制),跳过淘汰实验")
		fmt.Println("  设置方式: CONFIG SET maxmemory 10mb")
		fmt.Println()
		return
	}

	// 查当前策略
	policyCfg, _ := Rdb.ConfigGet(ctx, "maxmemory-policy").Result()
	fmt.Printf("  当前策略: %s  maxmemory: %s\n\n", policyCfg["maxmemory-policy"], maxmem)

	// 写入 100 个 key
	for i := 0; i < 100; i++ {
		Rdb.Set(ctx, fmt.Sprintf("evict:%d", i), i, 0)
	}

	// 模拟热点访问:key 0~9 高频访问
	for round := 0; round < 50; round++ {
		for i := 0; i < 10; i++ {
			Rdb.Get(ctx, fmt.Sprintf("evict:%d", i))
		}
	}

	// 查 LFU 频率(需要 LFU 策略才有意义)
	fmt.Println("  热点 key(evict:0~4) 的 OBJECT FREQ:")
	for i := 0; i < 5; i++ {
		freq, err := Rdb.Do(ctx, "OBJECT", "FREQ", fmt.Sprintf("evict:%d", i)).Int64()
		if err != nil {
			fmt.Printf("    evict:%d  (需要 LFU 策略才能查频率)\n", i)
			break
		}
		fmt.Printf("    evict:%d  freq=%d\n", i, freq)
	}

	fmt.Println()
	fmt.Println("  LRU vs LFU 的核心区别:")
	fmt.Println("    LRU: 淘汰最久未访问的 key")
	fmt.Println("         热点 key 如果有一段时间没访问,照样被淘汰")
	fmt.Println("    LFU: 淘汰访问频率最低的 key")
	fmt.Println("         历史热点 key 频率高,即使最近没访问也不会被淘汰")
	fmt.Println("    推荐: 有明显冷热数据的场景用 allkeys-lfu\n")

	// 清理
	for i := 0; i < 100; i++ {
		Rdb.Del(ctx, fmt.Sprintf("evict:%d", i))
	}
}
