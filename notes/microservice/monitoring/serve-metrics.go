// serve-metrics: 一个零依赖的最小 /metrics 服务，用来喂监控栈里的 go-service 目标。
//
// 为什么单独放这里：仓库里的实验（experiments/）都是跑完即退出的 httptest，
// 没有一个"常驻 + 暴露 8080"的服务可供 Prometheus 持续抓取。这个文件补上这块，
// 让 monitoring/ 的闭环能真正跑通（go-service 从 down 变 up，Grafana 看板有数据）。
//
// 跑法（在 notes/microservice 目录下）:
//
//	go run ./monitoring/serve-metrics.go
//
// 然后: curl http://localhost:8080/metrics  看到 exposition 文本
//       Prometheus 下一轮抓取后 go-service 变 up，RED 看板开始有曲线
//
// 指标名与 monitoring/grafana/dashboards/red-dashboard.json 的 expr 对齐。

package main

import (
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ---- 手写指标（笔记 12 §2，与 experiments/ 同思路，但这里是常驻服务）----

var (
	requestsTotal = make(map[string]*uint64) // key: route|status|method
	requestsMu    sync.RWMutex
)

// 模拟业务流量：每 100ms 打一批请求（约 300 QPS），喂给指标
func fakeTraffic() {
	routes := []string{"/pay", "/user/{id}", "/order", "/search"}
	statuses := []string{"200", "200", "200", "200", "200", "200", "200", "200", "200", "500"}
	for {
		for i := 0; i < 30; i++ {
			route := routes[rand.Intn(len(routes))]
			status := statuses[rand.Intn(len(statuses))]
			key := route + "|" + status + "|" + "GET"
			requestsMu.Lock()
			if requestsTotal[key] == nil {
				var u uint64
				requestsTotal[key] = &u
			}
			atomic.AddUint64(requestsTotal[key], 1)
			requestsMu.Unlock()

			// 模拟延迟分布：多数快请求，少数慢请求（双峰，方便看 P99）
			if rand.Float64() < 0.05 {
				observeLatency(0.4 + rand.Float64()*0.3)
			} else {
				observeLatency(0.02 + rand.Float64()*0.06)
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// ---- 延迟 histogram（笔记 07 §2：桶累计语义）----

var latencyBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}
var latencyCounts = make([]uint64, len(latencyBuckets)+1) // 最后一个是 +Inf 桶
var latencySum uint64
var latencyCnt uint64

func observeLatency(seconds float64) {
	for i, ub := range latencyBuckets {
		if seconds <= ub {
			atomic.AddUint64(&latencyCounts[i], 1)
		}
	}
	atomic.AddUint64(&latencyCounts[len(latencyBuckets)], 1) // +Inf 桶恒等于总数
	atomic.AddUint64(&latencySum, math.Float64bits(seconds))
	atomic.AddUint64(&latencyCnt, 1)
}

// ---- /metrics 输出 ----

func writeMetrics(w http.ResponseWriter, r *http.Request) {
	_ = r
	var b strings.Builder

	b.WriteString("# HELP http_requests_total 请求总数\n")
	b.WriteString("# TYPE http_requests_total counter\n")
	requestsMu.RLock()
	keys := make([]string, 0, len(requestsTotal))
	for k := range requestsTotal {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		parts := strings.Split(k, "|")
		b.WriteString(fmt.Sprintf("http_requests_total{route=%q,status=%q,method=%q} %d\n",
			parts[0], parts[1], parts[2], atomic.LoadUint64(requestsTotal[k])))
	}
	requestsMu.RUnlock()

	b.WriteString("# HELP http_request_duration_seconds 请求延迟\n")
	b.WriteString("# TYPE http_request_duration_seconds histogram\n")
	for i, ub := range latencyBuckets {
		b.WriteString(fmt.Sprintf("http_request_duration_seconds_bucket{le=%q} %d\n",
			strconv.FormatFloat(ub, 'g', -1, 64), atomic.LoadUint64(&latencyCounts[i])))
	}
	b.WriteString(fmt.Sprintf("http_request_duration_seconds_bucket{le=\"+Inf\"} %d\n",
		atomic.LoadUint64(&latencyCounts[len(latencyBuckets)])))
	b.WriteString(fmt.Sprintf("http_request_duration_seconds_sum %s\n",
		strconv.FormatFloat(math.Float64frombits(atomic.LoadUint64(&latencySum)), 'g', -1, 64)))
	b.WriteString(fmt.Sprintf("http_request_duration_seconds_count %d\n",
		atomic.LoadUint64(&latencyCnt)))

	b.WriteString("# HELP go_goroutines 当前 goroutine 数\n")
	b.WriteString("# TYPE go_goroutines gauge\n")
	b.WriteString(fmt.Sprintf("go_goroutines %d\n", runtime.NumGoroutine()))

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	_, _ = w.Write([]byte(b.String()))
}

func main() {
	go fakeTraffic()
	http.HandleFunc("/metrics", writeMetrics)
	const addr = ":18080" // 不用 8080：宿主 8080 常被 kafka-ui 占用
	fmt.Println("serving /metrics on " + addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		panic(err)
	}
}
