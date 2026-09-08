package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"time"
)

// 实验 10：HTTP 中间件埋 RED 指标（笔记 12 第 1 节 第 2 节 第 6 节）
// 实现: 复用 07 的并发安全 registry, 在中间件里埋 Rate/Errors/Duration, 业务代码零感知
// 演示: 真实起一个 HTTP 服务, 并发打流量, 验证计数精确、路径归一化、埋点开销、panic 兜底
// 锚点: ① 并发 1000 请求后计数精确无丢失（atomic 累加）
//       ② 路径归一化: /user/{1..100} 只产生 1 条 series（不归一 = 100 条, 高基数灾难）
//       ③ 埋点 O(1) 无分配: 每次 Observe 在百纳秒量级
//       ④ handler panic 被中间件兜住, 指标照记, 服务不挂（12 篇: 监控绝不能打挂业务）

// routePattern: 路径归一化——把 /user/123 归成 /user/{id}（12 第 5 节: 高基数第一禁忌）
// 真实框架里直接用 gin/hertz 的 ctx.FullPath(), 这里手写演示原理
func routePattern(path string) string {
	segments := strings.Split(strings.Trim(path, "/"), "/")
	for i, s := range segments {
		if s == "" {
			continue
		}
		// 纯数字或看起来像 ID 的段 → 归一为占位符
		if isIDLike(s) {
			segments[i] = "{id}"
		}
	}
	return "/" + strings.Join(segments, "/")
}

func isIDLike(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// statusRecorder: 包装 ResponseWriter 以捕获状态码
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) { r.status = code }

// metricsMiddleware: 唯一正确的埋点位置——中间件（12 第 1 节）
func metricsMiddleware(reqs *counterVec, dur *histVec, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		route := routePattern(r.URL.Path)
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		panicked := false

		defer func() {
			if p := recover(); p != nil {
				panicked = true
				rec.status = http.StatusInternalServerError
			}
			// 埋点放在 defer 里: 业务 panic 时请求也必须被记录——
			// 恰恰是失败请求最有监控价值（12 第 6 节: 埋点失败/业务失败都不该丢失可观测性）
			reqs.Inc(labelSet{{"route", route}, {"method", r.Method}, {"status", fmt.Sprintf("%d", rec.status)}})
			dur.Observe(labelSet{{"route", route}}, time.Since(start).Seconds())
			if panicked {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte("internal error"))
			}
		}()

		next.ServeHTTP(rec, r)
	})
}

func RunMiddlewareExperiments() {
	fmt.Println("=== 实验 10: 中间件埋 RED 指标 ===")

	reqs := newCounterVec(desc{name: "http_requests_total", help: "请求总数", typ: typeCounter})
	dur := newHistVec(desc{name: "http_request_duration_seconds", help: "延迟", typ: typeHistogram},
		[]float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, plusInf})

	mux := http.NewServeMux()
	mux.HandleFunc("/pay", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Microsecond)
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/boom", func(w http.ResponseWriter, r *http.Request) {
		panic("业务 panic") // 锚点 ④
	})
	srv := httptest.NewServer(metricsMiddleware(reqs, dur, mux))
	defer srv.Close()

	// 锚点 ①: 并发打流量, 计数必须精确
	const clients, perClient = 20, 50
	var wg sync.WaitGroup
	for c := 0; c < clients; c++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perClient; i++ {
				resp, err := http.Get(srv.URL + "/pay")
				if err == nil {
					_ = resp.Body.Close()
				}
			}
		}()
	}
	wg.Wait()

	total := 0.0
	for _, s := range reqs.Collect() {
		if s.labels["status"] == "200" {
			total += s.value
		}
	}
	want := float64(clients * perClient)
	fmt.Printf("%s 锚点① 并发计数: got=%.0f want=%.0f（%d 客户端 × %d 次）\n",
		mark(total == want), total, want, clients, perClient)

	// 锚点 ②: 路径归一化
	before := countSeries(reqs)
	for i := 1; i <= 100; i++ {
		resp, err := http.Get(fmt.Sprintf("%s/user/%d", srv.URL, i))
		if err == nil {
			_ = resp.Body.Close()
		}
	}
	after := countSeries(reqs)
	// 归一化后只新增 1 条 series（/user/{id} 的 404），不归一会新增 100 条
	added := after - before
	fmt.Printf("  打 100 个 /user/{1..100} 请求后新增 series 数 = %d\n", added)
	fmt.Printf("%s 锚点② 路径归一化: 新增 %d 条（未归一将是 100 条, 高基数灾难）\n",
		mark(added == 1), added)

	// 锚点 ③: 埋点开销（O(1) 无分配）
	const loops = 100000
	ls := labelSet{{"route", "/pay"}}
	dur.Observe(ls, 0.01) // 预热, 让 series 先建立
	start := time.Now()
	for i := 0; i < loops; i++ {
		dur.Observe(ls, 0.01)
	}
	perOp := float64(time.Since(start).Nanoseconds()) / float64(loops)
	fmt.Printf("  埋点耗时: %.0f ns/op（%d 次 Observe, 含 7 个桶的 atomic 累加; -race 下会放大数倍）\n", perOp, loops)
	fmt.Printf("  占比: 一次 200µs 的业务请求中, 埋点占 %.3f%%\n", perOp/1000/200*100)
	fmt.Printf("%s 锚点③ 埋点开销可控: %.0f ns/op < 20µs, 占业务耗时 < 10%%\n", mark(perOp < 20000), perOp)

	// 锚点 ④: panic 兜底
	resp, err := http.Get(srv.URL + "/boom")
	statusOK := err == nil && resp.StatusCode == http.StatusInternalServerError
	if err == nil {
		_ = resp.Body.Close()
	}
	stillAlive := true
	if r2, e2 := http.Get(srv.URL + "/pay"); e2 != nil || r2.StatusCode != http.StatusOK {
		stillAlive = false
	} else {
		_ = r2.Body.Close()
	}
	fmt.Printf("  /boom 返回 500=%v, 之后 /pay 仍正常=%v\n", statusOK, stillAlive)
	fmt.Printf("%s 锚点④ panic 兜底: 业务 panic 被中间件 recover, 指标照记且服务不挂\n",
		mark(statusOK && stillAlive))

	// 输出一份真实 /metrics 文本
	reg := &metricsRegistry{}
	reg.MustRegister(reqs)
	reg.MustRegister(dur)
	lines := renderSamples(reg.gather(false))
	sort.Strings(lines)
	fmt.Println("  /metrics 抽样（前 6 行）:")
	for i, l := range lines {
		if i >= 6 {
			break
		}
		fmt.Println("    " + l)
	}
}

func countSeries(c *counterVec) int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.cells)
}
