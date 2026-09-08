package main

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// 实验 06：手写 Prometheus 文本暴露格式（笔记 07 §6）
// 实现: 三类指标的 # HELP / # TYPE 与样本行生成, 外加三条硬约束的校验
// 演示: 一个支付服务 /metrics 的完整文本输出
// 锚点: ① 每个指标名的 # TYPE 只出现一次（重复会让 Prometheus 抓取失败）
//       ② Histogram 的 +Inf 桶必须存在且等于 count
//       ③ label 值里的引号/反斜杠/换行被正确转义

type metricType string

const (
	typeCounter   metricType = "counter"
	typeGauge     metricType = "gauge"
	typeHistogram metricType = "histogram"
)

// desc: 指标元信息（对应 client_golang 的 Desc）
type desc struct {
	name string
	help string
	typ  metricType
}

// sample: 一条样本（指标名 + 标签 + 值）
type sample struct {
	name   string
	labels map[string]string
	value  float64
}

// ---- 三类指标的内存表示（本实验只关心序列化, 并发安全见 07）----

type counter struct {
	desc     desc
	values   map[string]float64 // key: 序列化的 label 集
	labelSet map[string]map[string]string
}

type gauge struct {
	desc     desc
	values   map[string]float64
	labelSet map[string]map[string]string
}

type histogram struct {
	desc     desc
	buckets  []float64           // 上界, 升序, 最后一个必须是 +Inf
	values   map[string][]uint64 // key: label 集 → 各桶累计计数
	sums     map[string]float64
	counts   map[string]uint64
	labelSet map[string]map[string]string
}

// escapeLabelValue: 只转义三个字符——双引号、反斜杠、换行（exposition 规范）
func escapeLabelValue(v string) string {
	var b strings.Builder
	for _, r := range v {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// formatLabels: 标签按 key 字典序输出——顺序不影响 identity, 但输出必须稳定（否则 diff 噪音）
func formatLabels(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+`="`+escapeLabelValue(labels[k])+`"`)
	}
	return "{" + strings.Join(parts, ",") + "}"
}

// formatValue: Prometheus 用 'g' 格式; +Inf 有特殊字面量
func formatValue(v float64) string {
	if v == plusInf {
		return "+Inf"
	}
	return strconv.FormatFloat(v, 'g', -1, 64)
}

// plusInf: 桶序列的最后一个上界必须是 +Inf（07 §2）
var plusInf = math.Inf(1)

func sampleLine(name string, labels map[string]string, v float64) string {
	return name + formatLabels(labels) + " " + formatValue(v)
}

// render: 把一组指标渲染成 /metrics 文本
func render(metrics []any) string {
	var b strings.Builder
	seen := map[string]bool{}
	for _, m := range metrics {
		switch x := m.(type) {
		case *counter:
			writeHeader(&b, x.desc, &seen)
			for _, key := range sortedKeys(x.values) {
				b.WriteString(sampleLine(x.desc.name, x.labelSet[key], x.values[key]) + "\n")
			}
		case *gauge:
			writeHeader(&b, x.desc, &seen)
			for _, key := range sortedKeys(x.values) {
				b.WriteString(sampleLine(x.desc.name, x.labelSet[key], x.values[key]) + "\n")
			}
		case *histogram:
			writeHeader(&b, x.desc, &seen)
			for _, key := range sortedKeys(x.counts) {
				labels := x.labelSet[key]
				for i, ub := range x.buckets {
					bl := mergeLabel(labels, "le", formatValue(ub))
					b.WriteString(sampleLine(x.desc.name+"_bucket", bl, float64(x.values[key][i])) + "\n")
				}
				b.WriteString(sampleLine(x.desc.name+"_sum", labels, x.sums[key]) + "\n")
				b.WriteString(sampleLine(x.desc.name+"_count", labels, float64(x.counts[key])) + "\n")
			}
		}
	}
	return b.String()
}

func writeHeader(b *strings.Builder, d desc, seen *map[string]bool) {
	if (*seen)[d.name] {
		// 约束 ①: 同一指标名的 # TYPE 只能出现一次
		panic("duplicate # TYPE for " + d.name)
	}
	(*seen)[d.name] = true
	b.WriteString("# HELP " + d.name + " " + d.help + "\n")
	b.WriteString("# TYPE " + d.name + " " + string(d.typ) + "\n")
}

func mergeLabel(src map[string]string, k, v string) map[string]string {
	out := make(map[string]string, len(src)+1)
	for kk, vv := range src {
		out[kk] = vv
	}
	out[k] = v
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// ---- 实验主体 ----

func RunExpositionExperiments() {
	fmt.Println("=== 实验 06: exposition 文本格式 ===")

	requests := &counter{
		desc: desc{
			name: "http_requests_total",
			help: "HTTP 请求总数",
			typ:  typeCounter,
		},
		values: map[string]float64{
			`method="POST",route="/pay",status="200"`: 3481,
			`method="POST",route="/pay",status="500"`: 40,
		},
		labelSet: map[string]map[string]string{
			`method="POST",route="/pay",status="200"`: {"method": "POST", "route": "/pay", "status": "200"},
			`method="POST",route="/pay",status="500"`: {"method": "POST", "route": "/pay", "status": "500"},
		},
	}

	inflight := &gauge{
		desc: desc{
			name: "http_requests_inflight",
			help: "正在处理中的请求数",
			typ:  typeGauge,
		},
		values:   map[string]float64{"": 17},
		labelSet: map[string]map[string]string{"": nil},
	}

	// 延迟 histogram: 桶边界贴 SLO 设（07 §3: 桶宽决定分位数误差）
	duration := &histogram{
		desc: desc{
			name: "http_request_duration_seconds",
			help: "HTTP 请求延迟分布",
			typ:  typeHistogram,
		},
		buckets: []float64{0.05, 0.1, 0.2, 0.5, 1, plusInf},
		values: map[string][]uint64{
			`route="/pay"`: {120, 240, 980, 1000, 1000, 1000},
		},
		sums:   map[string]float64{`route="/pay"`: 183.4},
		counts: map[string]uint64{`route="/pay"`: 1000},
		labelSet: map[string]map[string]string{
			`route="/pay"`: {"route": "/pay"},
		},
	}

	out := render([]any{requests, inflight, duration})
	fmt.Println(out)

	// 锚点 ①: # TYPE 每个指标名恰好一次
	typeCount := map[string]int{}
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "# TYPE ") {
			typeCount[strings.Fields(line)[2]]++
		}
	}
	allOnce := len(typeCount) == 3
	for name, n := range typeCount {
		if n != 1 {
			allOnce = false
			fmt.Printf("  %s 出现 %d 次 TYPE\n", name, n)
		}
	}
	fmt.Printf("%s 锚点① # TYPE 唯一: 3 个指标各一次\n", mark(allOnce))

	// 锚点 ②: +Inf 桶存在且等于 count
	last := duration.buckets[len(duration.buckets)-1]
	infOK := last == plusInf && duration.values[`route="/pay"`][len(duration.buckets)-1] == duration.counts[`route="/pay"`]
	fmt.Printf("%s 锚点② +Inf 桶存在且 == count(1000)\n", mark(infOK))

	// 锚点 ③: label 转义
	raw := "he said \"hi\"\npath\\to"
	escaped := escapeLabelValue(raw)
	escOK := strings.Contains(escaped, `\"`) && strings.Contains(escaped, `\n`) && strings.Contains(escaped, `\\`)
	fmt.Printf("%s 锚点③ 转义: %s\n", mark(escOK), escaped)

	// 桶的累计语义（07 §2: le 桶是累计的, 不是独立的）
	cum := duration.values[`route="/pay"`]
	cumOK := true
	for i := 1; i < len(cum); i++ {
		if cum[i] < cum[i-1] {
			cumOK = false
		}
	}
	fmt.Printf("%s 锚点④ le 桶单调不减（累计语义）: %v\n", mark(cumOK), cum)
}
