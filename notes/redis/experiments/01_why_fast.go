// # 为什么 Redis 这么快
//
// 核心观点(antirez):Redis 快不是"因为单线程所以快",
// 而是"瓶颈不在 CPU,所以单线程就够了"。
// 单线程的好处是无锁、无上下文切换,让单核跑接近极限。
//
// 四个支柱:
//
//	1. 基于内存:数据读写直接操作内存,避免磁盘 I/O 瓶颈
//	2. 高效数据结构:SDS / 跳表 / listpack / 渐进式 rehash,时间复杂度低
//	3. 单线程命令执行:无锁、无上下文切换,单核跑满
//	4. I/O 多路复用:epoll 事件循环,单线程高效处理海量并发连接
//
// 单线程为什么"够用"反而最优:
//
//   - 瓶颈是网络和内存带宽,不是 CPU —— 单线程已能逼近硬件极限
//   - 多线程要加锁,锁竞争 + 上下文切换可能反而拖慢
//   - 代价:单条命令慢会放大阻塞所有后续命令
//     → 官方强调"避免大 key(>10KB)、避免 KEYS *"的原因
//
// Redis 6.0 多线程 I/O:补网络,不是推翻单线程:
//
//   - 6.0 把网络读写(socket 读、协议解析、回包写)分摊到 I/O 线程池
//   - 配置项:io-threads(建议 2-4),io-threads-do-reads
//   - 命令执行仍是单线程 —— 数据结构操作不加锁,线程安全
//
// 真正的瓶颈在哪(官方观点):
//
//   - 网络 / 内存带宽,而非 CPU
//   - pipelining 能把吞吐拉到数倍(官方基准实测)
//   - 大 key、慢命令会拖垮单线程 → 生产用 SCAN 代替 KEYS,UNLINK 代替 DEL
//
// 来源:antirez.com / redis.io/docs / 黄健宏《Redis设计与实现》

package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// Exp1Pipelining 实验1: Pipelining vs 逐条
//
// 瓶颈是网络 RTT,不是 CPU。pipeline 把 N 次 RTT 压成 1 次:
//
//	逐条:    write → flush → read | write → flush → read | ...   N 次 RTT
//	pipeline: write×N → flush → read×N                            1 次 RTT
//
// go-redis 的 Pipelined() 做的就是"攒命令 → 一次 flush → 一次性读回包"。
func Exp1Pipelining(ctx context.Context) {
	const n = 10000

	start := time.Now()
	for i := 0; i < n; i++ {
		rdb.Set(ctx, fmt.Sprintf("bench:seq:%d", i), i, 0)
	}
	seq := time.Since(start)

	start = time.Now()
	_, err := rdb.Pipelined(ctx, func(pipe redis.Pipeliner) error {
		for i := 0; i < n; i++ {
			pipe.Set(ctx, fmt.Sprintf("bench:pipe:%d", i), i, 0)
		}
		return nil
	})
	if err != nil {
		log.Fatalf("pipeline: %v", err)
	}
	pipe := time.Since(start)

	fmt.Println("=== 实验1: pipelining vs 逐条 ===")
	fmt.Printf("逐条 %d 次 SET:     %v\n", n, seq)
	fmt.Printf("pipeline %d 次 SET: %v\n", n, pipe)
	fmt.Printf("提速: %.1fx  <- 瓶颈是网络 RTT, pipeline 把 N 次 RTT 压成 1 次\n\n",
		float64(seq.Nanoseconds())/float64(pipe.Nanoseconds()))

	// 清理基准 key,避免污染后续实验(SCAN/KEYS 等)
	del := rdb.Pipeline()
	for i := 0; i < n; i++ {
		del.Del(ctx, fmt.Sprintf("bench:seq:%d", i))
		del.Del(ctx, fmt.Sprintf("bench:pipe:%d", i))
	}
	del.Exec(ctx)
}

// Exp2SingleThreadBlock 实验2: 单线程阻塞
//
// 单线程的代价:一条慢命令独占主线程,后续所有命令排队等待。
//
// 用 Lua 忙等模拟慢命令,同时用另一个 goroutine 测普通 GET 的延迟。
//
//	正常 GET:          < 1ms
//	慢命令期间的 GET:  几百ms  ← 被阻塞
//
// 这正是官方强调"避免大 key、避免 KEYS *"的原因。
func Exp2SingleThreadBlock(ctx context.Context) {
	rdb.Set(ctx, "probe", "hello", 0)

	t0 := time.Now()
	rdb.Get(ctx, "probe")
	base := time.Since(t0)

	var wg sync.WaitGroup
	var blocked time.Duration

	// A: Lua 忙等模拟慢命令, 独占主线程约 0.5~2s
	wg.Go(func() {
		rdb.Eval(ctx, `local x=0 for i=1,50000000 do x=x+i end return x`, nil)
	})

	// B: 等 Lua 跑起来后测普通 GET, 应被卡住
	wg.Go(func() {
		time.Sleep(100 * time.Millisecond)
		s := time.Now()
		rdb.Get(ctx, "probe")
		blocked = time.Since(s)
	})

	wg.Wait()

	fmt.Println("=== 实验2: 单线程阻塞 ===")
	fmt.Printf("正常 GET 延迟:         %v\n", base)
	fmt.Printf("慢命令期间的 GET 延迟:  %v\n", blocked)
	fmt.Println("结论: 普通命令被慢命令卡住 —— 单线程执行, 一条慢命令阻塞所有后续命令")
	fmt.Println("      如看不到阻塞, 调大 Lua 循环次数或减小 B 的 sleep\n")
}

// Exp3ConcurrentThroughput 实验3: 并发吞吐(I/O 多路复用)
//
// 单线程靠 epoll 事件循环同时监听所有连接的可读/可写事件,不需要为每个连接开一个线程。
//
// 100 个并发客户端同时写,Redis 全接住。
//
//	单线程 + epoll ≠ 串行处理连接
//	单线程 + epoll = 串行执行命令,但并发接收请求
func Exp3ConcurrentThroughput(ctx context.Context) {
	const clients = 100
	const perClient = 1000

	var wg sync.WaitGroup
	start := time.Now()
	for c := 0; c < clients; c++ {
		id := c
		wg.Go(func() {
			for i := 0; i < perClient; i++ {
				rdb.Set(ctx, fmt.Sprintf("conc:%d:%d", id, i), i, 0)
			}
		})
	}
	wg.Wait()
	dur := time.Since(start)
	total := clients * perClient

	fmt.Println("=== 实验3: 并发吞吐 (I/O 多路复用) ===")
	fmt.Printf("%d 并发客户端 × %d 次 SET = %d 次, 耗时 %v\n", clients, perClient, total, dur)
	fmt.Printf("吞吐: %.0f ops/sec\n", float64(total)/dur.Seconds())
	fmt.Println("结论: 单线程 Redis 靠 epoll 多路复用, 同时服务大量并发连接\n")

	// 清理 10 万个测试 key,避免污染实例内存和后续 SCAN/KEYS 实验
	del := rdb.Pipeline()
	for c := 0; c < clients; c++ {
		for i := 0; i < perClient; i++ {
			del.Del(ctx, fmt.Sprintf("conc:%d:%d", c, i))
		}
	}
	del.Exec(ctx)
}
