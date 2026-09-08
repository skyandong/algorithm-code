package main

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"time"
)

// 实验 13：手写 Collector 模式 Exporter（笔记 13 第 5 节）
// 实现: Describe/Collect 分离 + 多实例 target 参数 + 目标不可用时只让该实例 up=0 + Collect 超时
// 演示: 起一个真实 HTTP /metrics 端点, 用 http.Get 抓一次（等价于 Prometheus 的一次 scrape）
// 锚点: ① 每次抓取返回的指标名与 label 集完全稳定（Describe 的元信息不变）
//       ② 单个目标不可用时, 只有它自己的 up=0, 其他实例照常输出
//       ③ Collect 有超时——慢目标不会拖垮整轮抓取（13 第 5 节 第一条纪律）
//       ④ 输出是合法 exposition: +Inf 桶存在且与 count 相等

// backend: 被监控的内部系统（这里模拟一个队列服务）
type backend struct {
	addr   string
	depth  int
	slow   bool // 模拟响应很慢
	broken bool // 模拟不可用
}

// query: 问目标系统要状态; 带 1 秒超时（13 第 5 节: 超时返回空, 绝不卡住）
func (b *backend) query() (depth int, err error) {
	if b.slow {
		time.Sleep(3 * time.Second) // 超过 collectTimeout → 这次 collect 放弃该目标
	}
	if b.broken {
		return 0, fmt.Errorf("connection refused")
	}
	return b.depth, nil
}

const collectTimeout = 500 * time.Millisecond

// queueExporter: 一个 Exporter 代理多个后端实例（13 第 1 节: 集中式部署）
type queueExporter struct {
	mu       sync.Mutex
	backends []*backend
}

// Describe: 元信息——必须在多次抓取之间保持稳定（13 第 5 节 第四条纪律）
func (e *queueExporter) Describe() []desc {
	return []desc{
		{name: "queue_depth", help: "队列积压深度", typ: typeGauge},
		{name: "queue_processed_total", help: "已处理消息总数", typ: typeCounter},
		{name: "queue_process_duration_seconds", help: "单条处理耗时分布", typ: typeHistogram},
	}
}

// Collect: 每次抓取现问现答——遍历所有目标, 逐个查询并输出
func (e *queueExporter) Collect() []sample {
	e.mu.Lock()
	targets := append([]*backend(nil), e.backends...)
	e.mu.Unlock()

	out := make([]sample, 0)
	for _, b := range targets {
		labels := map[string]string{"instance": b.addr}

		// up 指标: 目标不可用 → 只让这个实例 up=0（13 第 5 节 第二条纪律）
		depth, err := e.queryWithTimeout(b)
		if err != nil {
			out = append(out, sample{name: "up", labels: labels, value: 0})
			continue // 其他实例不受影响
		}
		out = append(out, sample{name: "up", labels: labels, value: 1})
		out = append(out, sample{name: "queue_depth", labels: labels, value: float64(depth)})
		out = append(out, sample{name: "queue_processed_total", labels: labels, value: float64(depth * 7)})
		// histogram 三件套
		buckets := []float64{0.01, 0.05, 0.1, plusInf}
		counts := []float64{120, 240, 980, 1000}
		for i, ub := range buckets {
			lb := map[string]string{"instance": b.addr, "le": formatValue(ub)}
			out = append(out, sample{name: "queue_process_duration_seconds_bucket", labels: lb, value: counts[i]})
		}
		out = append(out, sample{name: "queue_process_duration_seconds_sum", labels: labels, value: 183.4})
		out = append(out, sample{name: "queue_process_duration_seconds_count", labels: labels, value: 1000})
	}
	return out
}

// queryWithTimeout: collect 必须有超时（慢目标不能拖垮整轮抓取）
func (e *queueExporter) queryWithTimeout(b *backend) (int, error) {
	type result struct {
		depth int
		err   error
	}
	ch := make(chan result, 1)
	go func() {
		d, err := b.query()
		ch <- result{d, err}
	}()
	select {
	case r := <-ch:
		return r.depth, r.err
	case <-time.After(collectTimeout):
		return 0, fmt.Errorf("collect timeout")
	}
}

// scrapeText: 把一次 Collect 的结果渲染成 /metrics 文本（本实验独立实现, 避免与 06 的 render 冲突）
func scrapeText(samples []sample) string {
	sort.SliceStable(samples, func(i, j int) bool {
		if samples[i].name != samples[j].name {
			return samples[i].name < samples[j].name
		}
		return sortedLabelKey(samples[i].labels) < sortedLabelKey(samples[j].labels)
	})
	var b strings.Builder
	seen := map[string]bool{}
	for _, s := range samples {
		base := strings.TrimSuffix(s.name, "_bucket")
		base = strings.TrimSuffix(base, "_sum")
		base = strings.TrimSuffix(base, "_count")
		if !seen[base] {
			seen[base] = true
			b.WriteString("# HELP " + base + " exported by queue_exporter\n")
			b.WriteString("# TYPE " + base + " " + guessType(base) + "\n")
		}
		b.WriteString(sampleLine(s.name, s.labels, s.value) + "\n")
	}
	return b.String()
}

func guessType(name string) string {
	if strings.HasSuffix(name, "_total") || strings.HasSuffix(name, "_seconds") {
		return "counter"
	}
	return "gauge"
}

func RunExporterExperiments() {
	fmt.Println("=== 实验 13: 手写 Collector 模式 Exporter ===")

	exp := &queueExporter{backends: []*backend{
		{addr: "queue-1:9090", depth: 120},
		{addr: "queue-2:9090", depth: 340},
		{addr: "queue-3:9090", depth: 8, broken: true}, // 不可用
	}}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/metrics" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		_, _ = io.WriteString(w, scrapeText(exp.Collect()))
	}))
	defer srv.Close()

	// 抓一次（等价于 Prometheus 的一次 scrape）
	resp, err := http.Get(srv.URL + "/metrics")
	if err != nil {
		fmt.Printf("%s 抓取失败: %v\n", mark(false), err)
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	text := string(body)

	lines := strings.Split(strings.TrimSpace(text), "\n")
	fmt.Printf("抓取 %s/metrics: %d 行, Content-Type=%s\n", srv.URL, len(lines), resp.Header.Get("Content-Type"))
	for _, l := range lines {
		if strings.Contains(l, "up{") || strings.HasPrefix(l, "# TYPE queue_depth") || strings.Contains(l, "queue_depth{") {
			fmt.Println("    " + l)
		}
	}

	// 锚点 ②: 坏实例只让自己 up=0
	up0 := strings.Contains(text, `up{instance="queue-3:9090"} 0`)
	up1 := strings.Contains(text, `up{instance="queue-1:9090"} 1`) && strings.Contains(text, `up{instance="queue-2:9090"} 1`)
	hasDepth := strings.Contains(text, `queue_depth{instance="queue-2:9090"} 340`)
	fmt.Printf("%s 锚点② 故障隔离: 坏实例 up=0 且其他实例 up=1、指标照常输出\n",
		mark(up0 && up1 && hasDepth))

	// 锚点 ①: 元信息稳定——连抓两次, 指标名与 label 集完全一致
	a := exp.Collect()
	b := exp.Collect()
	sig := func(samples []sample) []string {
		out := make([]string, 0, len(samples))
		for _, s := range samples {
			out = append(out, s.name+" "+sortedLabelKey(s.labels))
		}
		sort.Strings(out)
		return out
	}
	sa, sb := sig(a), sig(b)
	stable := len(sa) == len(sb)
	if stable {
		for i := range sa {
			if sa[i] != sb[i] {
				stable = false
			}
		}
	}
	fmt.Printf("%s 锚点① 元信息稳定: 两次 Collect 的 %d 个 (指标名+label集) 完全一致\n",
		mark(stable), len(sa))

	// 锚点 ③: 慢目标被超时截断, 不拖垮整轮抓取
	exp.mu.Lock()
	exp.backends = append(exp.backends, &backend{addr: "queue-slow:9090", depth: 1, slow: true})
	exp.mu.Unlock()
	start := time.Now()
	samples := exp.Collect()
	elapsed := time.Since(start)
	slowUp0 := false
	for _, s := range samples {
		if s.name == "up" && s.labels["instance"] == "queue-slow:9090" && s.value == 0 {
			slowUp0 = true
		}
	}
	fmt.Printf("  加入一个 3s 慢目标后, 整轮 Collect 耗时 %v（超时阈值 %v）\n", elapsed.Round(time.Millisecond), collectTimeout)
	fmt.Printf("%s 锚点③ Collect 超时: 慢目标被判 up=0, 整轮耗时被压在阈值附近而不是 3s\n",
		mark(slowUp0 && elapsed < 2*time.Second))

	// 锚点 ④: +Inf 桶 == count
	infOK := strings.Contains(text, `le="+Inf"} 1000`) && strings.Contains(text, `queue_process_duration_seconds_count{instance="queue-1:9090"} 1000`)
	fmt.Printf("%s 锚点④ +Inf 桶与 count 一致（1000）: 输出是合法 exposition\n", mark(infOK))
}
