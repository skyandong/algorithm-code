package main

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// 实验 07：Registry + Collector——并发安全的指标注册表（笔记 07 §6、12 §3）
// 实现: Collector 接口(Describe/Collect) + Registry 快照式 Gather + 无锁累加
// 演示: ① 100 goroutine 并发累加是否丢失 ② 快照式 Gather 与"边读边序列化"的一致性差异
// 锚点: ① 并发累加总数精确（atomic CAS, 热路径不加 mutex）
//       ② Gather 先把值全部读出再做序列化, 保证 _sum 与 _count 自洽（12 篇 Q8）
//       ③ 输出按 (指标名, label) 排序稳定, 多次抓取的 diff 无噪音

// labelPair / labelSet: 排序后序列化, 保证同一 label 集得到同一个 key
type labelPair struct{ k, v string }

type labelSet []labelPair

func (ls labelSet) key() string {
	sorted := make(labelSet, len(ls))
	copy(sorted, ls)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].k < sorted[j].k })
	var b strings.Builder
	for _, p := range sorted {
		b.WriteString(p.k + "=" + p.v + "\x00")
	}
	return b.String()
}

func (ls labelSet) toMap() map[string]string {
	m := make(map[string]string, len(ls))
	for _, p := range ls {
		m[p.k] = p.v
	}
	return m
}

// floatCell: 用 float64 位模式做原子累加（12 §3: 热路径必须无锁）
type floatCell struct {
	bits uint64
	ls   labelSet
}

func atomicAddFloat(c *floatCell, delta float64) {
	for {
		old := atomic.LoadUint64(&c.bits)
		next := math.Float64frombits(old) + delta
		if atomic.CompareAndSwapUint64(&c.bits, old, math.Float64bits(next)) {
			return
		}
	}
}

func atomicLoadFloat(c *floatCell) float64 {
	return math.Float64frombits(atomic.LoadUint64(&c.bits))
}

// ---- Collector 接口 ----

type collector07 interface {
	Describe() desc
	Collect() []sample     // 一次读完 = 快照
	CollectLive() []sample // 边序列化边读（反面教材）
}

// counterVec: Counter——只增
type counterVec struct {
	d     desc
	mu    sync.RWMutex // 读路径走 RLock（12 §3: 热路径无锁竞争）
	cells map[string]*floatCell
	order []string
}

func newCounterVec(d desc) *counterVec {
	return &counterVec{d: d, cells: map[string]*floatCell{}}
}

func (c *counterVec) Describe() desc { return c.d }

// With: 热路径——先无锁命中已有 cell, 未命中才加锁创建（12 §3 两级缓存）
func (c *counterVec) With(ls labelSet) *floatCell {
	key := ls.key()
	if cell := c.loadFast(key); cell != nil {
		return cell
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if cell, ok := c.cells[key]; ok {
		return cell
	}
	cell := &floatCell{ls: ls}
	c.cells[key] = cell
	c.order = append(c.order, key)
	return cell
}

// loadFast: 读锁命中已有 cell（真实实现用 atomic.Pointer 指向只读快照, 完全无锁）
func (c *counterVec) loadFast(key string) *floatCell {
	c.mu.RLock()
	cell := c.cells[key]
	c.mu.RUnlock()
	return cell
}

func (c *counterVec) Inc(ls labelSet) { atomicAddFloat(c.With(ls), 1) }

func (c *counterVec) Collect() []sample {
	c.mu.Lock()
	cells := make([]*floatCell, 0, len(c.order))
	for _, k := range c.order {
		cells = append(cells, c.cells[k])
	}
	c.mu.Unlock()
	out := make([]sample, 0, len(cells))
	for _, cell := range cells {
		out = append(out, sample{name: c.d.name, labels: cell.ls.toMap(), value: atomicLoadFloat(cell)})
	}
	return out
}

// CollectLive: 反面教材——遍历过程中值仍在变（真实序列化有 I/O 延迟, 会被放大）
func (c *counterVec) CollectLive() []sample { return c.Collect() }

// histVec: Histogram——分桶 + sum + count
type histVec struct {
	d       desc
	buckets []float64
	mu      sync.Mutex
	series  map[string]*histCell
	order   []string
}

type histCell struct {
	counts []*floatCell
	sum    *floatCell
	count  *floatCell
	ls     labelSet
}

func newHistVec(d desc, buckets []float64) *histVec {
	return &histVec{d: d, buckets: buckets, series: map[string]*histCell{}}
}

func (h *histVec) Describe() desc { return h.d }

func (h *histVec) With(ls labelSet) *histCell {
	key := ls.key()
	h.mu.Lock()
	defer h.mu.Unlock()
	if c, ok := h.series[key]; ok {
		return c
	}
	counts := make([]*floatCell, len(h.buckets))
	for i := range counts {
		counts[i] = &floatCell{}
	}
	c := &histCell{counts: counts, sum: &floatCell{}, count: &floatCell{}, ls: ls}
	h.series[key] = c
	h.order = append(h.order, key)
	return c
}

// Observe: 桶是累计的——所有上界 >= v 的桶都要 +1（07 §2）
func (h *histVec) Observe(ls labelSet, v float64) {
	c := h.With(ls)
	for i, ub := range h.buckets {
		if v <= ub {
			atomicAddFloat(c.counts[i], 1)
		}
	}
	atomicAddFloat(c.sum, v)
	atomicAddFloat(c.count, 1)
}

func (h *histVec) Collect() []sample {
	h.mu.Lock()
	cells := make([]*histCell, 0, len(h.order))
	for _, k := range h.order {
		cells = append(cells, h.series[k])
	}
	h.mu.Unlock()

	out := make([]sample, 0)
	for _, c := range cells {
		// 关键点: 先把 sum/count/桶全部读进内存, 再组装样本——这就是快照
		buckets := make([]float64, len(h.buckets))
		for i := range h.buckets {
			buckets[i] = atomicLoadFloat(c.counts[i])
		}
		sum := atomicLoadFloat(c.sum)
		count := atomicLoadFloat(c.count)
		for i, ub := range h.buckets {
			labels := c.ls.toMap()
			labels["le"] = formatValue(ub)
			out = append(out, sample{name: h.d.name + "_bucket", labels: labels, value: buckets[i]})
		}
		out = append(out, sample{name: h.d.name + "_sum", labels: c.ls.toMap(), value: sum})
		out = append(out, sample{name: h.d.name + "_count", labels: c.ls.toMap(), value: count})
	}
	return out
}

// CollectLive: 反面教材——组装样本的过程中重新读值（中间有"序列化耗时"）
func (h *histVec) CollectLive() []sample {
	h.mu.Lock()
	cells := make([]*histCell, 0, len(h.order))
	for _, k := range h.order {
		cells = append(cells, h.series[k])
	}
	h.mu.Unlock()
	out := make([]sample, 0)
	for _, c := range cells {
		for i, ub := range h.buckets {
			labels := c.ls.toMap()
			labels["le"] = formatValue(ub)
			out = append(out, sample{name: h.d.name + "_bucket", labels: labels, value: atomicLoadFloat(c.counts[i])})
		}
		time.Sleep(10 * time.Millisecond) // 模拟序列化 I/O 延迟
		out = append(out, sample{name: h.d.name + "_sum", labels: c.ls.toMap(), value: atomicLoadFloat(c.sum)})
		time.Sleep(10 * time.Millisecond) // 关键: sum 与 count 之间也有延迟 → 二者读到不同时刻
		out = append(out, sample{name: h.d.name + "_count", labels: c.ls.toMap(), value: atomicLoadFloat(c.count)})
	}
	return out
}

// metricsRegistry: 指标注册表——Gather 并发收集所有 collector 并排序输出
type metricsRegistry struct {
	mu   sync.Mutex
	cols []collector07
}

func (r *metricsRegistry) MustRegister(c collector07) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cols = append(r.cols, c)
}

func (r *metricsRegistry) gather(live bool) []sample {
	r.mu.Lock()
	cols := append([]collector07(nil), r.cols...)
	r.mu.Unlock()

	var mu sync.Mutex
	var out []sample
	var wg sync.WaitGroup
	for i := range cols {
		wg.Add(1)
		go func(c collector07) {
			defer wg.Done()
			var s []sample
			if live {
				s = c.CollectLive()
			} else {
				s = c.Collect()
			}
			mu.Lock()
			out = append(out, s...)
			mu.Unlock()
		}(cols[i])
	}
	wg.Wait()

	// 锚点 ③: 稳定排序——(指标名, label 序列化)
	sort.Slice(out, func(i, j int) bool {
		if out[i].name != out[j].name {
			return out[i].name < out[j].name
		}
		return sortedLabelKey(out[i].labels) < sortedLabelKey(out[j].labels)
	})
	return out
}

func sortedLabelKey(m map[string]string) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k + "=" + m[k] + "\x00")
	}
	return b.String()
}

func RunCollectorExperiments() {
	fmt.Println("=== 实验 07: Registry + Collector 并发安全与快照 ===")

	// 锚点 ①: 并发累加不丢失
	reqs := newCounterVec(desc{name: "http_requests_total", help: "请求总数", typ: typeCounter})
	const goroutines, perG = 100, 1000
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ls := labelSet{{"route", "/pay"}, {"status", "200"}}
			for i := 0; i < perG; i++ {
				reqs.Inc(ls)
			}
		}()
	}
	wg.Wait()

	got := reqs.Collect()[0].value
	want := float64(goroutines * perG)
	fmt.Printf("%s 锚点① 并发累加: got=%.0f want=%.0f（%d goroutine × %d 次, atomic CAS 无丢失）\n",
		mark(got == want), got, want, goroutines, perG)

	// 锚点 ②: 快照 vs 边读边写
	dur := newHistVec(desc{name: "http_request_duration_seconds", help: "延迟", typ: typeHistogram},
		[]float64{0.05, 0.1, 0.5, plusInf})
	stop := make(chan struct{})
	var bg sync.WaitGroup
	bg.Add(1)
	go func() {
		defer bg.Done()
		ls := labelSet{{"route", "/pay"}}
		for {
			select {
			case <-stop:
				return
			default:
				dur.Observe(ls, 1.0) // 每次 1 秒, 真实平均值必然是 1.0
			}
		}
	}()

	time.Sleep(150 * time.Millisecond) // 先让后台积累样本, 否则窗口内只有 0 个点

	snap := dur.gatherOne(false)
	live := dur.gatherOne(true)
	close(stop)
	bg.Wait()

	avgSnap := snap.sum / snap.count
	avgLive := live.sum / live.count
	devSnap := math.Abs(avgSnap - 1.0)
	devLive := math.Abs(avgLive - 1.0)
	fmt.Printf("  快照模式: sum=%.0f count=%.0f 平均=%.6f（偏差 %.2e）\n", snap.sum, snap.count, avgSnap, devSnap)
	fmt.Printf("  边读边写: sum=%.0f count=%.0f 平均=%.6f（偏差 %.2e）\n", live.sum, live.count, avgLive, devLive)
	fmt.Printf("%s 锚点② 快照更自洽: 不一致窗口从 20ms 压到纳秒级, 偏差小 %.0f 倍\n",
		mark(!math.IsNaN(devSnap) && devSnap < devLive/10), devLive/max(devSnap, 1e-12))

	// 锚点 ③: 输出稳定
	reg := &metricsRegistry{}
	reg.MustRegister(reqs)
	reg.MustRegister(dur)
	a := renderSamples(reg.gather(false))
	b := renderSamples(reg.gather(false))
	stable := true
	for i := range a {
		// 计数类指标会变化, 只比较"行的形状"（名字与标签）
		if i < len(b) {
			na, _, _ := strings.Cut(a[i], " ")
			nb, _, _ := strings.Cut(b[i], " ")
			if na != nb {
				stable = false
			}
		}
	}
	fmt.Printf("%s 锚点③ 输出顺序稳定: 两次 gather 的 %d 行样本名序列一致\n", mark(stable), len(a))
}

type histSnapshot struct{ sum, count float64 }

func (h *histVec) gatherOne(live bool) histSnapshot {
	var s []sample
	if live {
		s = h.CollectLive()
	} else {
		s = h.Collect()
	}
	var sum, count float64
	for _, x := range s {
		switch {
		case strings.HasSuffix(x.name, "_sum"):
			sum = x.value
		case strings.HasSuffix(x.name, "_count"):
			count = x.value
		}
	}
	return histSnapshot{sum: sum, count: count}
}

func renderSamples(samples []sample) []string {
	out := make([]string, 0, len(samples))
	for _, s := range samples {
		out = append(out, sampleLine(s.name, s.labels, s.value))
	}
	return out
}
