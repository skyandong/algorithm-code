// pprof-lab: 一个故意带病的 Go 服务，供 pprof 排障练习（配套 notes/golang/11）。
//
// 四种"病灶"，对应面试与线上的高频根因：
//
//	① 正则回溯   GET /cpu    —— (a+)+$ 对长串是指数级回溯，CPU 杀手（11 篇第 8 节 排查路径的靶子）
//	② 锁竞争     GET /lock   —— 全局 mutex 内做耗时操作，block profile 能看到等待
//	③ 协程泄漏   GET /leak   —— 无缓冲 channel 发送后无人接收，goroutine 数只涨不跌
//	④ 内存泄漏   GET /mem    —— slice 截取持有整个底层数组（11 篇/01 篇讲过的经典坑）
//
// 另有 /hash 作为"正常热点"对照：烧 CPU 但确实在做该做的事。
//
// 用法（完整工作流见 README.md）：
//
//	go run .                # 起服务：业务 :8081，pprof :6060（仅 localhost，见 11 篇第 4.1 节）
//	go tool pprof -http=:8083 http://localhost:6060/debug/pprof/profile?seconds=30
//	                        # 工程标准姿势：抓 30s CPU 并打开 Web UI，VIEW → Flame Graph 看火焰图
//
// 这里的火焰图/调用图/top/list 全部用 go tool pprof 自带的 Web UI（工程流行做法），
// 不引入任何第三方工具；离线分析（先把 .pb.gz 保存下来）的姿势也见 README。

package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof" // 把 pprof 路由注册到 DefaultServeMux
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"
)

func main() {
	mode := flag.String("mode", "serve", "serve（目前只有这一个；火焰图用 go tool pprof -http，见 README）")
	flag.Parse()
	if *mode != "serve" {
		log.Fatalf("未知 mode %q —— 火焰图不需要本程序做任何事, 直接用:", *mode)
	}

	startSickServer()
	log.Println("pprof-lab 已启动: 业务 :8081, pprof :6060（仅 localhost）")
	log.Println("下一步: go tool pprof -http=:8083 'http://localhost:6060/debug/pprof/profile?seconds=30'")
	select {} // 常驻
}

// ---- 病灶 ①: 请求路径里重复编译正则（Go 里真实存在的 CPU 杀手） ----
//
// 关键知识点: Go 的 regexp 是 RE2 引擎, 线性时间, **天然免疫**"灾难性回溯"
// （(a+)+$ 指数爆炸是 Java/Python/JS 等回溯型引擎的病, 见下方对照函数）。
// 所以 Go 服务 CPU 打满时, 正则的真实问题不是"回溯", 而是更朴素的: 在每次请求里
// 重新 Compile——编译的代价远高于匹配, pprof 里 regexp.Compile/syntax.Parse 会高居榜首。

func regexOnHotPath(n int) bool {
	// 反面教材: 包级编译一次就好, 这里每次调用都 Compile
	re := regexp.MustCompile(`(\w+)@(\w+)\.(com|cn|org)`)
	return re.MatchString(strings.Repeat("user@example.com ", n))
}

// 对照组: RE2 没有回溯——同样"危险"的正则, 在 Go 里就是线性时间。
// 在 Java/Python 里下面这行对 30 个 a 是天文数字, 在 Go 里只要几微秒。
var evilRe = regexp.MustCompile(`^(a+)+$`)

func regexBacktrack(n int) bool {
	input := make([]byte, n)
	for i := range input {
		input[i] = 'a'
	}
	input = append(input, 'X') // 结尾放非 a, 让 (a+)+ 永远匹配失败, 跑满整条自动机
	return evilRe.Match(input)
}

// ---- 对照组: 正常热点（真的在干活） ----

func hashLoop(rounds int) [32]byte {
	var h [32]byte
	for i := 0; i < rounds; i++ {
		h[0] = byte(i)
		// 简单的混合运算, 制造稳定 CPU 热点但不泄漏
		for j := 0; j < 1000; j++ {
			h[j%32] = h[j%32]*31 + byte(j)
			h[(j*7)%32] ^= h[(j*13)%32] >> 3
		}
	}
	return h
}

// ---- 病灶 ②: 锁竞争（临界区里干慢活） ----

var (
	globalMu   sync.Mutex
	globalData = make(map[int]int)
)

func lockContend() {
	globalMu.Lock()
	defer globalMu.Unlock()
	// 临界区里做 2ms 的"数据库查询"——持有锁的时间远超必要, 排队的人全被拖住
	time.Sleep(2 * time.Millisecond)
	globalData[len(globalData)%1024] = len(globalData)
}

// ---- 病灶 ③: goroutine 泄漏（发送到无人接收的 channel） ----

var (
	leakOnce sync.Once
	leakedCh chan struct{}
	leakQuit = make(chan struct{})
	leakMu   sync.Mutex
	leakedN  int
)

func startLeakChannel() {
	leakOnce.Do(func() {
		leakedCh = make(chan struct{}) // 无缓冲, 且永远没有接收方
	})
}

func leakGoroutine() {
	startLeakChannel()
	go func() {
		// 经典死法: 发送阻塞, 这个 goroutine 永远出不去
		leakedCh <- struct{}{}
	}()
	leakMu.Lock()
	leakedN++
	leakMu.Unlock()
}

// ---- 病灶 ④: 内存泄漏（slice 截取持有大底层数组） ----

var memorySink = make(map[int][]byte)

func leakMemory(id int) {
	big := make([]byte, 1<<20) // 1MB
	for i := range big {
		big[i] = byte(id)
	}
	// 只要前 16 字节, 但 small 持有整个 1MB 底层数组 → map 越攒越大
	small := big[:16]
	memorySink[id] = small
}

// ---- 业务路由与自动负载 ----

func startSickServer() {
	// block / mutex profile 默认关闭, 必须显式开采样（11 篇第 4.2 节）:
	// block: 每次阻塞事件等待超过 10µs 就记录; mutex: 按 1/5 比例记录争用
	runtime.SetBlockProfileRate(10_000)
	runtime.SetMutexProfileFraction(5)

	mux := http.NewServeMux()
	mux.HandleFunc("/cpu", func(w http.ResponseWriter, _ *http.Request) {
		start := time.Now()
		ok := regexOnHotPath(200)
		fmt.Fprintf(w, "regexOnHotPath(200)=%v 耗时 %v（大头在 Compile 不在 Match）\n", ok, time.Since(start))
	})
	mux.HandleFunc("/hash", func(w http.ResponseWriter, _ *http.Request) {
		start := time.Now()
		hashLoop(50)
		fmt.Fprintf(w, "hashLoop(50) 耗时 %v\n", time.Since(start))
	})
	mux.HandleFunc("/lock", func(w http.ResponseWriter, _ *http.Request) {
		start := time.Now()
		lockContend()
		fmt.Fprintf(w, "lockContend 耗时 %v（含 2ms 临界区等待）\n", time.Since(start))
	})
	mux.HandleFunc("/leak", func(w http.ResponseWriter, _ *http.Request) {
		before := runtime.NumGoroutine()
		leakGoroutine()
		fmt.Fprintf(w, "goroutine: %d → %d\n", before, runtime.NumGoroutine())
	})
	mux.HandleFunc("/mem", func(w http.ResponseWriter, r *http.Request) {
		id := time.Now().UnixNano()
		leakMemory(int(id % 1e6))
		fmt.Fprintf(w, "memorySink 条目数: %d（每条名义 16B, 实际持有 1MB）\n", len(memorySink))
		_ = r
	})
	mux.HandleFunc("/status", func(w http.ResponseWriter, _ *http.Request) {
		leakMu.Lock()
		n := leakedN
		leakMu.Unlock()
		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)
		fmt.Fprintf(w, "goroutines=%d 泄漏计数=%d heapAlloc=%.1fMB sink条目=%d\n",
			runtime.NumGoroutine(), n, float64(ms.HeapAlloc)/(1<<20), len(memorySink))
	})

	go autoLoad()

	go func() {
		// pprof 只绑 localhost（11 篇第 4.1 节: 无鉴权, 不能直接暴露公网）
		if err := http.ListenAndServe("localhost:6060", nil); err != nil {
			log.Printf("pprof 端口退出: %v", err)
		}
	}()

	if err := http.ListenAndServe(":8081", mux); err != nil {
		log.Fatalf("业务端口退出: %v", err)
	}
}

// autoLoad: 并发 worker 连续制造稳定负载, 保证 CPU profile 有足够的采样密度。
// （真实线上服务的 CPU 也常常是被持续流量烧起来的——profile 的本质是"抓正在发生的忙"。）
func autoLoad() {
	leakTick := time.NewTicker(800 * time.Millisecond)
	defer leakTick.Stop()

	for i := 0; i < 3; i++ {
		go func(id int) {
			for {
				hashLoop(20)       // 正常热点, 稳定占一部分 CPU
				lockContend()      // 锁竞争持续存在
				regexOnHotPath(80) // 热路径重复编译正则（Go 里的真实 CPU 杀手）
				regexBacktrack(22) // 对照: RE2 线性, 微秒级
				_ = id
			}
		}(i)
	}

	// 纯 CPU worker: 不碰锁只烧计算, 保证 CPU profile 有真实热度
	// （真实服务里这对应"序列化/加密/图像处理"这类纯计算型流量）
	// 配比刻意调过: 让 hashLoop（正当热点）与 regexp.Compile（病灶①）都能在火焰图上形成主柱
	for i := 0; i < 4; i++ {
		go func() {
			for {
				hashLoop(10)
				regexOnHotPath(400)
			}
		}()
	}

	for {
		select {
		case <-leakTick.C:
			leakGoroutine()                          // goroutine 缓慢泄漏
			leakMemory(int(time.Now().Unix() % 1e6)) // 内存缓慢泄漏
		case <-leakQuit:
			return
		}
	}
}
