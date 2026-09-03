package main

import (
	"context"
	"fmt"
	"sync"
)

// 实验 05：进程内 trace 上下文透传与 span 组装
// 实现: 极简 OTel 语义——trace_id 全链路透传 + span(parent 指针) 树组装（笔记 05 §3）
// 演示: gateway → order-svc → db/redis, gateway → user-svc 的调用树
// 锚点: ① 同一次请求的所有 span 共享同一 trace_id
//       ② 树形结构正确（parent-child 关系与调用栈一致）
//       ③ 总耗时 = 根 span 跨度; 各段耗时可定位慢段。

// span: 最小要素（笔记 05 §3）
type span struct {
	traceID  string
	spanID   string
	parentID string
	name     string
	startMs  int
	endMs    int
	attrs    map[string]string
}

// tracer: 收集 span, 按 traceID 组装树
type tracer struct {
	mu    sync.Mutex
	spans []*span
	clock int // 虚拟时钟(ms)——确定性输出
}

func newTracer() *tracer { return &tracer{} }

func (t *tracer) now() int { return t.clock }

func (t *tracer) advance(ms int) { t.clock += ms }

// startSpan: 开启新 span（context 携带父子关系, 跨"服务边界"传递）
func (t *tracer) startSpan(ctx context.Context, name string, attrs map[string]string) (context.Context, *span) {
	s := &span{
		traceID:  traceIDFrom(ctx),
		spanID:   fmt.Sprintf("S%d", len(t.spans)+1),
		parentID: spanIDFrom(ctx),
		name:     name,
		startMs:  t.now(),
		attrs:    attrs,
	}
	t.mu.Lock()
	t.spans = append(t.spans, s)
	t.mu.Unlock()
	return withSpan(ctx, s.traceID, s.spanID), s
}

func (t *tracer) endSpan(s *span) {
	s.endMs = t.now() // 真实实现: 异步上报, 不阻塞业务; 这里直接落值
}

// ---- 极简 context 携带（真实用 otel 的 trace.ContextWithSpan）----

type ctxKey int

const (
	keyTraceID ctxKey = iota
	keySpanID
)

func withSpan(ctx context.Context, traceID, spanID string) context.Context {
	ctx = context.WithValue(ctx, keyTraceID, traceID)
	return context.WithValue(ctx, keySpanID, spanID)
}

func traceIDFrom(ctx context.Context) string {
	if v, ok := ctx.Value(keyTraceID).(string); ok {
		return v
	}
	return "" // 根 span: 由调用方注入新 traceID
}

func spanIDFrom(ctx context.Context) string {
	if v, ok := ctx.Value(keySpanID).(string); ok {
		return v
	}
	return "" // 根
}

// ---- 模拟一次请求: 网关 → 两个下游服务 ----

func (t *tracer) simulateRequest() string {
	rootCtx := withSpan(context.Background(), "T-abc123", "") // 网关开新 trace

	// 根 span: gateway
	gwCtx, gw := t.startSpan(rootCtx, "gateway", map[string]string{"http.path": "/checkout"})
	// 模拟网关自身处理 5ms
	t.advance(5)

	// 调 order-svc（透传 trace_id, 开子 span）
	orderCtx, order := t.startSpan(gwCtx, "order-svc:Checkout", nil)
	t.advance(3)
	// order-svc 内部调 db（再下一层）
	dbCtx, db := t.startSpan(orderCtx, "db:SELECT orders", map[string]string{"db.statement": "SELECT * FROM orders WHERE uid=?"})
	t.advance(40) // db 慢!
	t.endSpan(db)
	_ = dbCtx
	// order-svc 调 redis
	_, cache := t.startSpan(orderCtx, "redis:GET stock", map[string]string{"db.operation": "GET"})
	t.advance(8)
	t.endSpan(cache)
	t.endSpan(order) // order 总计 3+40+8=51ms

	// 网关再调 user-svc
	userCtx, user := t.startSpan(gwCtx, "user-svc:GetProfile", nil)
	t.advance(6)
	t.endSpan(user)
	_ = userCtx

	t.advance(2)
	t.endSpan(gw) // gateway 总计 5+51+6+2=64ms
	return gw.traceID
}

// assembleTree: 按 traceID 聚拢, parent 指针建树, 打印
func (t *tracer) assembleTree(traceID string) string {
	byID := map[string]*span{}
	var roots []*span
	for _, s := range t.spans {
		if s.traceID == traceID {
			byID[s.spanID] = s
		}
	}
	for _, s := range t.spans {
		if s.traceID == traceID && s.parentID == "" {
			roots = append(roots, s)
		}
	}
	var sb []string
	var walk func(s *span, depth int)
	walk = func(s *span, depth int) {
		dur := s.endMs - s.startMs
		line := fmt.Sprintf("%s└─ %s [%s→%s] %dms", strings_Repeat("  ", depth), s.name, s.spanID, s.parentID, dur)
		if len(s.attrs) > 0 {
			line += fmt.Sprintf(" %v", s.attrs)
		}
		sb = append(sb, line)
		// 找孩子（顺序即记录顺序）
		for _, c := range t.spans {
			if c.traceID == traceID && c.parentID == s.spanID {
				walk(c, depth+1)
			}
		}
	}
	for _, r := range roots {
		walk(r, 0)
	}
	out := ""
	for _, l := range sb {
		out += l + "\n"
	}
	return out
}

// strings_Repeat: 避免直接 import strings 的别名小函数
func strings_Repeat(s string, n int) string {
	r := ""
	for i := 0; i < n; i++ {
		r += s
	}
	return r
}

func RunTraceExperiments() {
	fmt.Println("== 实验 05: trace 上下文透传与 span 树组装 ==")

	t := newTracer()
	traceID := t.simulateRequest()

	fmt.Println("--- 调用树（虚拟时钟, 确定性输出） ---")
	fmt.Print(t.assembleTree(traceID))

	fmt.Println("--- 验收 ---")
	// 1. 所有 span 同一 trace_id
	sameTrace := true
	spanCount := 0
	for _, s := range t.spans {
		if s.traceID != traceID {
			sameTrace = false
		}
		spanCount++
	}
	fmt.Printf("  全链路同一 trace_id: %d 个 span 全部 T-abc123 → %s\n", spanCount, mark(sameTrace))

	// 2. 树形关系正确: 根是 gateway, 其下 order/user, order 下 db/redis
	byName := map[string]*span{}
	for _, s := range t.spans {
		byName[s.name] = s
	}
	gw, order, db := byName["gateway"], byName["order-svc:Checkout"], byName["db:SELECT orders"]
	fmt.Printf("  树形: gateway(根, parent='') → %s\n", mark(gw != nil && gw.parentID == ""))
	fmt.Printf("  树形: order 是 gateway 的孩子 → %s\n", mark(order != nil && order.parentID == gw.spanID))
	fmt.Printf("  树形: db 是 order 的孩子 → %s\n", mark(db != nil && db.parentID == order.spanID))

	// 3. 耗时账本: 慢段可定位
	gwDur := gw.endMs - gw.startMs
	dbDur := db.endMs - db.startMs
	orderDur := order.endMs - order.startMs
	fmt.Printf("  耗时: gateway %dms = order %dms + user 6ms + 自身 10ms 左右\n", gwDur, orderDur)
	fmt.Printf("  定位: db span %dms 占 order 的 %.0f%%——慢点一目了然 → %s\n",
		dbDur, float64(dbDur)/float64(orderDur)*100, mark(dbDur == 40 && orderDur == 51))

	fmt.Println("\n→ 结论: trace_id 聚拢 + parent 指针建树, 一次请求的因果与耗时全貌即可视化")
	fmt.Println("  真实栈: OTel SDK 埋点(context透传) → OTel Collector(采样/批量) → Jaeger/Tempo(存储/查询)")
	fmt.Println("  现成 demo: demos/tracing（OpenTelemetry 端到端）")
}
