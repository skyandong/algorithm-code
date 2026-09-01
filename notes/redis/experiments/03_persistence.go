// # 持久化
//
// # RDB
//
// RDB 是持久化方式的名字,bgsave 是触发它的命令,dump.rdb 是产物文件。
// 三者关系:RDB 是"存档格式",bgsave 是"按下存档键",dump.rdb 是"存档文件"。
//
// bgsave 触发条件:
//
//   - 手动:BGSAVE 命令
//   - 自动:save 配置达到阈值(默认 3600/1、300/100、60/10000)
//   - 主从全量同步:slave 发起时主节点自动触发
//   - SHUTDOWN 且未开 AOF 时自动触发
//
// bgsave 过程:
//
//	主进程 fork 子进程(主进程立即返回继续处理请求)
//	      ↓
//	子进程把内存数据序列化成二进制写到临时文件
//	      ↓
//	写完后 rename 替换旧的 dump.rdb(原子操作)
//	      ↓
//	子进程退出
//
// fork + COW 原理:
//
//	fork 后父子进程共享同一份内存页(只读)
//	父进程收到写请求修改某页时,内核 COW 复制该页给父进程
//	子进程继续读原始页,完整写出快照
//
// 两个工程代价:
//
//  1. fork 阻塞主线程:fork 需要复制页表,实例内存越大越慢,几十 GB 实例可达几百 ms
//     → 用 LATENCY HISTORY fork 或 INFO persistence 的 latest_fork_usec 监控
//
//  2. COW 导致内存暴涨:写量越大,被修改的页越多,COW 复制越多
//     → 极端情况内存使用翻倍,大实例必须预留内存
//
// 子进程写期间父进程崩了怎么办:
// RDB 写到临时文件,写完后原子 rename。父进程崩了临时文件直接丢,旧 RDB 完好无损。
//
// # AOF
//
// AOF 记录每条写命令的日志,追加到 appendonly.aof 文件。
//
// fsync 策略(appendfsync):
//
//	always   每条命令刷盘  最安全,最慢,吞吐约 几百~几千 ops/s
//	everysec 每秒刷一次    推荐,宕机最多丢 1 秒(实际可能更多,见下)
//	no       交由 OS 决定  最快,宕机可能丢几十秒
//
// 为什么 everysec 实际丢的可能不止 1 秒:
// fsync 线程和主线程是异步的。如果上一次 fsync 还没完成,主线程会等待最多 2 秒。
// 加上系统负载,极端情况丢 2 秒以上。
//
// AOF 重写(bgrewriteaof):
//
//	fork 子进程把当前内存状态转成最精简的命令集写到新文件
//	重写期间父进程的新写命令同时写到:
//	  1. 旧 AOF 文件(保证旧文件完整)
//	  2. AOF 重写缓冲区(rewrite buffer)
//	子进程写完后把 rewrite buffer 追加到新文件,rename 替换旧文件
//
// 面试追问:AOF 重写缓冲区是干什么的?
// 重写期间父进程不能停,新写命令得有地方存。rewrite buffer 就是这个暂存区,
// 保证子进程写完的新文件和当前内存状态一致。
//
// # 混合持久化(Redis 4.0+)
//
// aof-use-rdb-preamble yes(5.0+ 默认开启):
//
//	AOF 文件 = 前半段 RDB(全量快照) + 后半段 AOF 增量命令
//	重启时先加载 RDB(快) → 再重放 AOF 增量(少)
//	兼具 RDB 恢复快 + AOF 数据完整的优点
//
// # 怎么选
//
//	只要 RDB          容忍丢几分钟数据,对恢复速度要求高
//	只要 AOF          数据尽量不丢,everysec 是最佳平衡
//	混合持久化(推荐)  生产默认选项,两者优点都要

package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ExpBgsave 实验11: 触发 bgsave,观察 fork 耗时和 RDB 状态
//
// INFO 关键字段:
//
//	rdb_last_bgsave_status   上次 bgsave 结果(ok/err)
//	rdb_last_bgsave_time_sec 上次 bgsave 耗时(秒)
//	rdb_last_save_time       上次成功保存的 unix 时间戳
//	latest_fork_usec         上次 fork 耗时(微秒) ← 监控 fork 阻塞的核心指标
//	                         注意版本差异:6.x 在 INFO stats 里,7.x 在 INFO persistence 里,
//	                         所以这里直接查全量 INFO 兜底
func ExpBgsave(ctx context.Context) {
	fmt.Println("=== 实验11: bgsave + fork 耗时 ===")

	// 触发 bgsave
	rdb.BgSave(ctx)

	// 等 bgsave 完成
	for {
		info, _ := rdb.Info(ctx, "persistence").Result()
		if strings.Contains(info, "rdb_bgsave_in_progress:0") {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	// 全量 INFO:latest_fork_usec 的所在 section 随版本变化
	info, _ := rdb.Info(ctx).Result()
	for _, line := range strings.Split(info, "\r\n") {
		switch {
		case strings.HasPrefix(line, "rdb_last_bgsave_status"),
			strings.HasPrefix(line, "rdb_last_bgsave_time_sec"),
			strings.HasPrefix(line, "rdb_last_save_time"),
			strings.HasPrefix(line, "latest_fork_usec"):
			fmt.Println(" ", line)
		}
	}
	fmt.Println("结论: latest_fork_usec 是监控 fork 阻塞的核心指标,生产建议接入告警\n")
}

// ExpAofFsync 实验12: 三种 fsync 策略写入吞吐对比
//
// 通过 CONFIG SET 动态切换 fsync 策略,对比相同写入量下的耗时。
// 生产不要随意切换 always,这里仅用小数据量演示差距。
//
// 预期输出(真实机械盘/高写入量下):
//
//	always   最慢(每条命令都等刷盘)
//	everysec 中间
//	no       最快(OS 决定刷盘时机)
//
// 注意:本地 docker(macOS 文件系统 fsync 语义弱)+ 逐条命令 RTT 主导的环境下,
// 三档差距可能淹没在网络噪声里 —— 这本身就是个实验结论:fsync 的代价要在
// 真实磁盘和批量写入下才显著。
func ExpAofFsync(ctx context.Context) {
	fmt.Println("=== 实验12: fsync 策略写入吞吐对比 ===")

	// 先查当前 AOF 是否开启
	cfg, _ := rdb.ConfigGet(ctx, "appendonly").Result()
	aofEnabled := false
	if len(cfg) >= 1 {
		aofEnabled = cfg["appendonly"] == "yes"
	}
	if !aofEnabled {
		fmt.Println("  AOF 未开启(appendonly=no),跳过实验")
		fmt.Println("  开启方式: CONFIG SET appendonly yes\n")
		return
	}

	// 保存原始策略
	origCfg, _ := rdb.ConfigGet(ctx, "appendfsync").Result()
	orig := origCfg["appendfsync"]
	defer rdb.ConfigSet(ctx, "appendfsync", orig)

	const n = 1000
	strategies := []string{"always", "everysec", "no"}

	for _, s := range strategies {
		rdb.ConfigSet(ctx, "appendfsync", s)
		time.Sleep(100 * time.Millisecond)

		start := time.Now()
		for i := 0; i < n; i++ {
			rdb.Set(ctx, fmt.Sprintf("fsync:%s:%d", s, i), i, 0)
		}
		dur := time.Since(start)
		fmt.Printf("  %-10s %d 次 SET: %v  (%.2fμs/op)\n",
			s, n, dur, float64(dur.Microseconds())/n)

		// 清理
		for i := 0; i < n; i++ {
			rdb.Del(ctx, fmt.Sprintf("fsync:%s:%d", s, i))
		}
	}
	fmt.Println("结论: 理论上 always 每条刷盘最安全但最慢,everysec 是性能与安全的最佳平衡")
	fmt.Println("      本地 docker 环境下差距可能不明显(fsync 语义弱 + RTT 主导),真实磁盘上才显著\n")
}

// ExpCowMemory 实验13: bgsave 期间 COW 内存变化
//
// 关键认知:COW 复制的页是内核层面的开销,体现在进程 RSS(used_memory_rss),
// Redis 自身视角的 used_memory 根本看不到 COW —— 它只统计自己申请的内存。
// 所以本实验同时观察两个指标:
//
//	used_memory      Redis 自身申请的内存(新增 key 会涨,与 COW 无关)
//	used_memory_rss  OS 视角的进程常驻内存(COW 复制会让它超出 used_memory 的涨幅)
//
// 预期输出:
//
//	bgsave 期间持续写入 → used_memory_rss 的涨幅超过 used_memory 的涨幅(差值就是 COW)
//	bgsave 结束后 RSS 回落(子进程退出,复制的页被释放)
func ExpCowMemory(ctx context.Context) {
	fmt.Println("=== 实验13: bgsave 期间 COW 内存变化 ===")

	memBefore, rssBefore := usedMemory(ctx), rssMemory(ctx)
	fmt.Printf("  bgsave 前:   used_memory=%d KB  rss=%d KB\n", memBefore/1024, rssBefore/1024)

	// 触发 bgsave
	rdb.BgSave(ctx)
	time.Sleep(50 * time.Millisecond)

	// bgsave 期间持续写入,触发 COW
	val := strings.Repeat("x", 1024) // 1KB value
	for i := 0; i < 500; i++ {
		rdb.Set(ctx, fmt.Sprintf("cow:%d", i), val, 0)
	}

	memDuring, rssDuring := usedMemory(ctx), rssMemory(ctx)
	fmt.Printf("  写入 500KB:  used_memory=%d KB (涨 %d KB)  rss=%d KB (涨 %d KB)\n",
		memDuring/1024, (memDuring-memBefore)/1024,
		rssDuring/1024, (rssDuring-rssBefore)/1024)
	fmt.Println("  rss 涨幅中超出 used_memory 涨幅的部分 ≈ COW 复制的页")

	// 等 bgsave 完成
	for {
		info, _ := rdb.Info(ctx, "persistence").Result()
		if strings.Contains(info, "rdb_bgsave_in_progress:0") {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	_, rssAfter := usedMemory(ctx), rssMemory(ctx)
	fmt.Printf("  bgsave 后:   rss=%d KB\n", rssAfter/1024)
	fmt.Println("结论: COW 的内存开销要看 RSS 而不是 used_memory;写量越大 COW 复制越多,")
	fmt.Println("      大实例 bgsave 期间 RSS 可能接近翻倍,必须预留内存余量\n")

	// 清理
	for i := 0; i < 500; i++ {
		rdb.Del(ctx, fmt.Sprintf("cow:%d", i))
	}
}

func rssMemory(ctx context.Context) int64 {
	info, _ := rdb.Info(ctx, "memory").Result()
	for _, line := range strings.Split(info, "\r\n") {
		if strings.HasPrefix(line, "used_memory_rss:") {
			v, _ := strconv.ParseInt(strings.TrimPrefix(line, "used_memory_rss:"), 10, 64)
			return v
		}
	}
	return 0
}

// ExpAofRewrite 实验14: AOF 重写(bgrewriteaof)+ 重写缓冲区
//
// AOF 重写过程:
//
//	fork 子进程把当前内存转成最精简命令集写到新文件
//	重写期间父进程新写命令同时写到:
//	  1. 旧 AOF 文件(保证旧文件完整)
//	  2. AOF 重写缓冲区(rewrite buffer)← 这是关键
//	子进程写完后把 rewrite buffer 追加到新文件,rename 替换旧文件
//
// rewrite buffer 的作用:
// 保证子进程写完的新文件和当前内存状态一致。
// 没有它,重写期间的新写命令就丢了。
//
// 观察指标(INFO persistence):
//
//	aof_enabled              AOF 是否开启
//	aof_rewrite_in_progress  是否正在重写
//	aof_current_size         当前 AOF 文件大小(字节)
//	aof_base_size            上次重写后的基准大小
func ExpAofRewrite(ctx context.Context) {
	fmt.Println("=== 实验14: AOF 重写(bgrewriteaof) ===")

	// 检查 AOF 是否开启
	cfg, _ := rdb.ConfigGet(ctx, "appendonly").Result()
	if cfg["appendonly"] != "yes" {
		fmt.Println("  AOF 未开启,跳过实验")
		fmt.Println("  开启方式: CONFIG SET appendonly yes\n")
		return
	}

	// 重写前状态
	printAofInfo := func(label string) {
		info, _ := rdb.Info(ctx, "persistence").Result()
		fmt.Printf("  [%s]\n", label)
		for _, line := range strings.Split(info, "\r\n") {
			switch {
			case strings.HasPrefix(line, "aof_enabled"),
				strings.HasPrefix(line, "aof_rewrite_in_progress"),
				strings.HasPrefix(line, "aof_current_size"),
				strings.HasPrefix(line, "aof_base_size"):
				fmt.Printf("    %s\n", line)
			}
		}
	}

	// 先写一批数据制造 AOF 体积
	for i := 0; i < 500; i++ {
		rdb.Set(ctx, fmt.Sprintf("aof:key:%d", i), i, 0)
	}
	// 再删掉一半,制造冗余命令(重写后这些 DEL 和原 SET 都会被消除)
	for i := 0; i < 250; i++ {
		rdb.Del(ctx, fmt.Sprintf("aof:key:%d", i))
	}

	printAofInfo("重写前")

	// 触发 AOF 重写
	rdb.BgRewriteAOF(ctx)

	// 重写期间继续写入,这些命令会进 rewrite buffer
	for i := 500; i < 600; i++ {
		rdb.Set(ctx, fmt.Sprintf("aof:key:%d", i), i, 0)
	}

	printAofInfo("重写期间")

	// 等重写完成
	for {
		info, _ := rdb.Info(ctx, "persistence").Result()
		if strings.Contains(info, "aof_rewrite_in_progress:0") {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	printAofInfo("重写完成后")
	fmt.Println("  结论: aof_current_size 应小于重写前(冗余命令被压缩)")
	fmt.Println("        重写期间写入的命令通过 rewrite buffer 追加到新文件,一条不丢\n")

	// 清理
	for i := 0; i < 600; i++ {
		rdb.Del(ctx, fmt.Sprintf("aof:key:%d", i))
	}
}

func usedMemory(ctx context.Context) int64 {
	info, _ := rdb.Info(ctx, "memory").Result()
	for _, line := range strings.Split(info, "\r\n") {
		if strings.HasPrefix(line, "used_memory:") {
			v, _ := strconv.ParseInt(strings.TrimPrefix(line, "used_memory:"), 10, 64)
			return v
		}
	}
	return 0
}
