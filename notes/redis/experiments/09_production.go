// # 生产排查与运维
//
// # bigkey
//
// 定义:元素数量多 或 占用内存大 的 key。
// 没有绝对标准,通常:
//
//	String  > 10KB
//	Hash/Set/ZSet/List  元素数 > 1万 或 总大小 > 1MB
//
// 危害:
//
//   - 阻塞主线程:大 key 的 O(N) 操作(HGETALL/SMEMBERS/LRANGE)拖慢所有命令
//   - 删除阻塞:DEL 大 key 同步释放内存,可能卡几百 ms
//   - 网络拥塞:返回几 MB 数据占满带宽
//   - 集群迁移卡顿:迁移大 key 期间该槽位不可用
//
// 排查:
//
//	redis-cli --bigkeys    按元素数量采样找大 key,底层用 SCAN,不阻塞,生产可用
//	                       集群模式只扫当前节点,全集群需对每个 master 单独跑
//	MEMORY USAGE key       精确查单个 key 内存,但对聚合类型是 O(N)
//	                       别对未知大小的 key 直接用,先用 --bigkeys 找到嫌疑 key 再查
//	redis-cli --memkeys    按内存大小找大 key(4.0+),底层对每个 key 调 MEMORY USAGE
//	                       生产慎用:key 多时会大量 O(N) 操作,非高峰期才跑
//
// 处理:拆分(Hash 拆成多个小 Hash)、UNLINK 异步删、lazyfree 配置。
//
// # hotkey
//
// 某个 key 访问频率极高,单节点扛不住。
//
// 排查:
//
//	redis-cli --hotkeys          需要 maxmemory-policy 设为 LFU 策略
//	OBJECT FREQ key              查单个 key 的访问频率(LFU 模式)
//	monitor                      实时观察所有命令(生产慎用,影响性能)
//
// 处理:
//
//   - 本地缓存:在应用层缓存热点数据,减少 Redis 请求
//   - 拆分热点:把一个 key 复制成多个(如 hotkey:1~hotkey:10),读时随机选一个
//   - 读写分离:热点读打到 slave
//
// # KEYS vs SCAN
//
// KEYS pattern:O(N) 扫全库,持有全局锁,期间阻塞所有命令。生产严禁。
//
// SCAN cursor [MATCH pattern] [COUNT count]:
//
//	渐进式遍历,每次返回少量 key + 下一个 cursor
//	cursor=0 开始,cursor=0 结束(完成一轮)
//	不阻塞,但可能返回重复(rehash 期间)或遗漏(遍历期间删除的 key)
//	COUNT 是建议值,不是精确值
//
// # slowlog
//
// 记录执行时间超过阈值的命令。
//
//	CONFIG SET slowlog-log-slower-than 10000   阈值(微秒),默认 10ms
//	CONFIG SET slowlog-max-len 128             最多保留条数
//	SLOWLOG GET [n]                            查最近 n 条
//	SLOWLOG LEN                                队列长度
//	SLOWLOG RESET                              清空
//
// 常见慢命令:KEYS *、HGETALL 大 Hash、LRANGE 0 -1、SMEMBERS 大 Set、复杂 Lua。
//
// # Latency Monitor(2.8.13+)
//
// SLOWLOG 只记慢命令,fork/expire-cycle/aof-write 这类内部操作它抓不到。
// Latency Monitor 专门抓这类非命令延迟事件。
//
// 默认关闭,需要:CONFIG SET latency-monitor-threshold 100
//
//	LATENCY DOCTOR              分析所有事件,给文字诊断建议
//	LATENCY HISTORY event       某类事件的历史时序
//	LATENCY LATEST              每类最近一次
//	LATENCY RESET               清空
//
// 事件类型:fork / expire-cycle / aof-write / unlink / eviction-cycle / command
//
// # CPU 毛刺排查顺序
//
//	SLOWLOG GET           → 有没有慢命令
//	LATENCY HISTORY fork  → 是不是 bgsave fork 卡顿
//	LATENCY HISTORY expire-cycle → 是不是过期 key 集中淘汰
//	redis-cli --bigkeys   → 有没有大 key 操作
//	CLIENT LIST           → 有没有 MONITOR 没关
//	INFO memory           → 内存是否接近 maxmemory(触发淘汰)
//	查 OS                 → THP / swap / CPU 未隔离

package main

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// ExpKeysVsScan 实验30: KEYS vs SCAN 阻塞对比
//
// 写入 10000 个 key,对比 KEYS * 和 SCAN 的耗时与行为差异。
//
// 预期输出:
//
//	KEYS:  一次返回所有 key,耗时随 key 数量线性增长
//	SCAN:  多次迭代,每次返回少量,总耗时相近但不阻塞主线程
func ExpKeysVsScan(ctx context.Context) {
	fmt.Println("=== 实验30: KEYS vs SCAN ===")

	// 写入测试数据
	const n = 10000
	pipe := rdb.Pipeline()
	for i := 0; i < n; i++ {
		pipe.Set(ctx, fmt.Sprintf("scan:key:%d", i), i, time.Minute)
	}
	pipe.Exec(ctx)

	// KEYS:一次扫全库
	start := time.Now()
	keys, _ := rdb.Keys(ctx, "scan:key:*").Result()
	keysDur := time.Since(start)
	fmt.Printf("  KEYS scan:key:*  返回 %d 个,耗时 %v  ← 阻塞主线程\n", len(keys), keysDur)

	// SCAN:渐进式遍历
	start = time.Now()
	var cursor uint64
	var scanCount int
	var iterations int
	for {
		var batch []string
		var err error
		batch, cursor, err = rdb.Scan(ctx, cursor, "scan:key:*", 100).Result()
		if err != nil {
			break
		}
		scanCount += len(batch)
		iterations++
		if cursor == 0 {
			break
		}
	}
	scanDur := time.Since(start)
	fmt.Printf("  SCAN scan:key:*  返回 %d 个,迭代 %d 次,耗时 %v  ← 渐进式不阻塞\n",
		scanCount, iterations, scanDur)
	fmt.Println("  结论: KEYS 阻塞主线程,生产严禁;SCAN 渐进式,每次只处理少量 key\n")

	// 清理
	pipe = rdb.Pipeline()
	for i := 0; i < n; i++ {
		pipe.Del(ctx, fmt.Sprintf("scan:key:%d", i))
	}
	pipe.Exec(ctx)
}

// ExpSlowlog 实验31: slowlog 慢查询记录
//
// 用 DEBUG SLEEP 制造慢命令,观察 slowlog 是否记录。
//
// 预期输出:
//
//	DEBUG SLEEP 0.1 执行后,SLOWLOG GET 能看到这条记录
func ExpSlowlog(ctx context.Context) {
	fmt.Println("=== 实验31: slowlog 慢查询 ===")

	// 设置阈值为 50ms
	rdb.ConfigSet(ctx, "slowlog-log-slower-than", "50000")
	rdb.Do(ctx, "SLOWLOG", "RESET")

	// 制造一个慢命令:sleep 100ms
	rdb.Do(ctx, "DEBUG", "SLEEP", "0.1")

	// 查 slowlog
	logs, _ := rdb.SlowLogGet(ctx, 5).Result()
	fmt.Printf("  slowlog 记录数: %d\n", len(logs))
	for _, log := range logs {
		fmt.Printf("  命令: %v  耗时: %dμs\n", log.Args, log.Duration.Microseconds())
	}
	if len(logs) == 0 {
		fmt.Println("  (DEBUG SLEEP 可能被禁用,尝试用大 KEYS 触发)")
	}
	fmt.Println("  结论: slowlog 记录超过阈值的命令,是排查慢命令的第一工具\n")

	rdb.ConfigSet(ctx, "slowlog-log-slower-than", "10000")
}

// ExpLatencyMonitor 实验32: Latency Monitor —— 抓非命令延迟事件
//
// 开启 Latency Monitor,用 DEBUG SLEEP 触发 command 延迟事件,
// 观察 LATENCY HISTORY 和 LATENCY DOCTOR 的输出。
//
// 预期输出:
//
//	LATENCY LATEST 能看到 command 事件
//	LATENCY DOCTOR 给出诊断建议
func ExpLatencyMonitor(ctx context.Context) {
	fmt.Println("=== 实验32: Latency Monitor ===")

	// 开启监控,阈值 50ms
	rdb.ConfigSet(ctx, "latency-monitor-threshold", "50")
	rdb.Do(ctx, "LATENCY", "RESET")

	// 触发延迟事件
	rdb.Do(ctx, "DEBUG", "SLEEP", "0.1")

	time.Sleep(100 * time.Millisecond)

	// LATENCY LATEST
	result, _ := rdb.Do(ctx, "LATENCY", "LATEST").Result()
	fmt.Printf("  LATENCY LATEST: %v\n", result)

	// LATENCY DOCTOR
	doctor, _ := rdb.Do(ctx, "LATENCY", "DOCTOR").Result()
	fmt.Println("  LATENCY DOCTOR:")
	for _, line := range strings.Split(fmt.Sprintf("%v", doctor), ".") {
		line = strings.TrimSpace(line)
		if line != "" {
			fmt.Printf("    %s\n", line)
		}
	}
	fmt.Println()
	fmt.Println("  SLOWLOG vs Latency Monitor:")
	fmt.Println("    SLOWLOG:          只记慢命令,fork/expire/aof-write 抓不到")
	fmt.Println("    Latency Monitor:  记所有延迟事件,包括内部非命令操作")
	fmt.Println("    生产建议:两个都开,互补\n")

	// 恢复默认
	rdb.ConfigSet(ctx, "latency-monitor-threshold", "0")
	rdb.Do(ctx, "LATENCY", "RESET")
}

// ExpBigkey 实验33: bigkey 检测与处理
//
// 构造不同大小的 key,用 MEMORY USAGE 观察内存占用。
// 演示大 key 的 HGETALL 耗时 vs 小 key。
func ExpBigkey(ctx context.Context) {
	fmt.Println("=== 实验33: bigkey 检测 ===")

	rdb.Del(ctx, "big:hash", "small:hash")

	// 小 Hash:10 个字段
	for i := 0; i < 10; i++ {
		rdb.HSet(ctx, "small:hash", fmt.Sprintf("f%d", i), i)
	}

	// 大 Hash:10000 个字段
	p := rdb.Pipeline()
	for i := 0; i < 10000; i++ {
		p.HSet(ctx, "big:hash", fmt.Sprintf("f%d", i), i)
	}
	p.Exec(ctx)

	smallMem, _ := rdb.MemoryUsage(ctx, "small:hash").Result()
	bigMem, _ := rdb.MemoryUsage(ctx, "big:hash").Result()
	fmt.Printf("  small:hash(10字段):    %d bytes\n", smallMem)
	fmt.Printf("  big:hash(10000字段):   %d bytes = %d KB\n", bigMem, bigMem/1024)

	// HGETALL 耗时对比
	start := time.Now()
	rdb.HGetAll(ctx, "small:hash")
	smallDur := time.Since(start)

	start = time.Now()
	rdb.HGetAll(ctx, "big:hash")
	bigDur := time.Since(start)

	fmt.Printf("\n  HGETALL small:hash: %v\n", smallDur)
	fmt.Printf("  HGETALL big:hash:   %v  ← 阻塞主线程\n", bigDur)
	fmt.Println("\n  处理方案:")
	fmt.Println("    拆分: 按业务维度拆成多个小 Hash")
	fmt.Println("    删除: UNLINK 异步删,避免 DEL 阻塞")
	fmt.Println("    遍历: 用 HSCAN 代替 HGETALL,渐进式读取\n")

	rdb.Unlink(ctx, "big:hash", "small:hash")
}

// ExpProgressiveDelete 实验34: 大 key 渐进式删除
//
// 直接 DEL/UNLINK 大 key 的问题:
//   - DEL:主线程同步释放,阻塞所有命令
//   - UNLINK:后台异步释放,但超大 key(百万元素)释放期间内存不降反升
//
// 渐进式删除:用 SCAN 系列命令每次删一批元素,批次间 sleep 分散压力。
// 主线程每次只感知一小批删除的开销,完全无感知。
//
// 各类型的渐进式删除方式:
//
//	Hash   HSCAN → HDEL  每批 100 个字段
//	Set    SSCAN → SREM  每批 100 个成员
//	ZSet   ZREMRANGEBYRANK 每次删前 100 个(不需要 SCAN)
//	List   LTRIM 每次从头砍掉 100 个
//	String 直接 UNLINK(无法拆分)
//
// 预期输出:
//
//	渐进式删除耗时比 DEL 长,但每批耗时极短,主线程不感知
func ExpProgressiveDelete(ctx context.Context) {
	fmt.Println("=== 实验34: 大 key 渐进式删除 ===")

	const fields = 50000

	// 构造大 Hash
	build := func(key string) {
		for i := 0; i < fields; i += 500 {
			p := rdb.Pipeline()
			for j := i; j < i+500 && j < fields; j++ {
				p.HSet(ctx, key, fmt.Sprintf("f%d", j), j)
			}
			p.Exec(ctx)
		}
	}

	rdb.Del(ctx, "prog:hash", "direct:hash")
	build("prog:hash")
	build("direct:hash")

	mem, _ := rdb.MemoryUsage(ctx, "prog:hash").Result()
	fmt.Printf("  大 Hash(%d 字段): %d KB\n\n", fields, mem/1024)

	// 直接 DEL:主线程同步
	start := time.Now()
	rdb.Del(ctx, "direct:hash")
	delDur := time.Since(start)
	fmt.Printf("  直接 DEL 耗时: %v  ← 主线程同步释放全部内存\n", delDur)

	// 渐进式删除 Hash
	const batchSize = 100
	var totalBatches int
	var maxBatchDur time.Duration

	start = time.Now()
	for {
		// HSCAN 取一批字段
		keys, cursor, _ := rdb.HScan(ctx, "prog:hash", 0, "*", batchSize).Result()
		if len(keys) == 0 {
			break
		}

		// keys 是 [field1, value1, field2, value2, ...] 交替的
		fields := make([]string, 0, len(keys)/2)
		for i := 0; i < len(keys); i += 2 {
			fields = append(fields, keys[i])
		}

		batchStart := time.Now()
		rdb.HDel(ctx, "prog:hash", fields...)
		batchDur := time.Since(batchStart)

		if batchDur > maxBatchDur {
			maxBatchDur = batchDur
		}
		totalBatches++

		// 批次间 sleep 1ms,把压力分散到时间轴上
		time.Sleep(time.Millisecond)

		if cursor == 0 {
			break
		}
	}
	// 最后删空 key
	rdb.Del(ctx, "prog:hash")
	totalDur := time.Since(start)

	fmt.Printf("  渐进式删除耗时: %v  批次: %d  单批最大耗时: %v\n",
		totalDur, totalBatches, maxBatchDur)
	fmt.Println("\n  结论:")
	fmt.Println("  - 渐进式删除总耗时更长(有 sleep),但每批只删 100 个字段")
	fmt.Println("  - 主线程每次感知的延迟极小,业务流量不受影响")
	fmt.Println("  - 生产建议: bigkey 不要在线上高峰期直接删,用渐进式脚本在低峰期执行\n")
}
