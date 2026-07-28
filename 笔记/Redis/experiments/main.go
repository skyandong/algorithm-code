package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

var rdb *redis.Client

func main() {
	rdb = redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "123456",
		DB:       0,
	})
	defer rdb.Close()

	ctx := context.Background()
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("连不上 Redis: %v (确认 localhost:6379 在跑、密码 123456)", err)
	}

	exp1_pipelining(ctx)
	exp2_single_thread_block(ctx)
	exp3_concurrent_throughput(ctx)
}

// 实验1: pipelining vs 逐条
// 证明: 瓶颈是网络 RTT, 不是 CPU. pipeline 把 N 个 RTT 压成 1 个.
func exp1_pipelining(ctx context.Context) {
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
}

// 实验2: 单线程阻塞
// 一个慢命令(Lua 忙等)在跑时, 另一个普通 GET 被完全卡住.
// 证明: 命令单线程执行, 一条慢命令会阻塞所有后续命令.
func exp2_single_thread_block(ctx context.Context) {
	rdb.Set(ctx, "probe", "hello", 0)

	t0 := time.Now()
	rdb.Get(ctx, "probe")
	base := time.Since(t0)

	var wg sync.WaitGroup
	wg.Add(2)
	var blocked time.Duration

	// A: 跑一个耗时的 Lua 忙等, 纯烧 CPU, 会独占主线程.
	//    50M 次循环约 0.5~2s, 远低于 lua-time-limit(默认 5s), 所以是阻塞而非 BUSY 报错.
	go func() {
		defer wg.Done()
		rdb.Eval(ctx, `local x=0 for i=1,50000000 do x=x+i end return x`, nil)
	}()

	// B: 等 Lua 跑起来后测一个普通 GET, 应被卡住.
	go func() {
		defer wg.Done()
		time.Sleep(100 * time.Millisecond)
		s := time.Now()
		rdb.Get(ctx, "probe")
		blocked = time.Since(s)
	}()

	wg.Wait()

	fmt.Println("=== 实验2: 单线程阻塞 ===")
	fmt.Printf("正常 GET 延迟:         %v\n", base)
	fmt.Printf("慢命令期间的 GET 延迟:  %v\n", blocked)
	fmt.Println("结论: 普通命令被慢命令卡住 —— 单线程执行, 一条慢命令阻塞所有后续命令")
	fmt.Println("      (这正是官方强调'避免大 key、避免 KEYS *'的原因)")
	fmt.Println("      如看不到阻塞, 调大 Lua 循环次数或减小 B 的 sleep\n")
}

// 实验3: 并发客户端吞吐 (I/O 多路复用)
// 100 个并发连接同时写, 单线程 Redis 靠 epoll 多路复用全接住.
func exp3_concurrent_throughput(ctx context.Context) {
	const clients = 100
	const perClient = 1000

	var wg sync.WaitGroup
	start := time.Now()
	for c := 0; c < clients; c++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < perClient; i++ {
				rdb.Set(ctx, fmt.Sprintf("conc:%d:%d", id, i), i, 0)
			}
		}(c)
	}
	wg.Wait()
	dur := time.Since(start)
	total := clients * perClient

	fmt.Println("=== 实验3: 并发吞吐 (I/O 多路复用) ===")
	fmt.Printf("%d 并发客户端 × %d 次 SET = %d 次, 耗时 %v\n", clients, perClient, total, dur)
	fmt.Printf("吞吐: %.0f ops/sec\n", float64(total)/dur.Seconds())
	fmt.Println("结论: 单线程 Redis 靠 epoll 多路复用, 同时服务大量并发连接\n")
}
