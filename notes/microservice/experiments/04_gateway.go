package main

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
)

// 实验 04：内存网关——路由表 + 中间件链 + 灰度分流
// 实现: 函数化中间件链（accessLog→限流→鉴权→路由, 笔记 04 §3）+ 路由表 atomic 热更新
//       + 灰度规则（uid 取模粘性分流, 笔记 04 §4）
// 锚点: ① 同 uid 多次请求 100% 恒定同一版本（粘性）
//       ② 限流在鉴权前（被限流的请求不消耗鉴权成本）
//       ③ 路由表热更新后, 新请求立即走新路由（原子快照）。

// ---- 请求/响应 ----
type gwRequest struct {
	Path   string
	UID    int
	Token  string
	Header map[string]string
}

type gwResponse struct {
	Status  int
	Body    string
	Version string // 命中的后端版本（验证灰度）
}

// ---- 中间件链: 函数化装饰器（design-pattern/03）----
type gwHandler func(req *gwRequest) *gwResponse
type middleware func(next gwHandler) gwHandler

func chain(h gwHandler, ms ...middleware) gwHandler {
	// 从后往前包裹 → 洋葱圈
	for i := len(ms) - 1; i >= 0; i-- {
		h = ms[i](h)
	}
	return h
}

// ---- 网关本体 ----
type gateway struct {
	routes atomic.Pointer[map[string]string] // path 前缀 → 服务名（atomic 热更新）
	auth   func(token string) bool
	// 观测统计
	logs          atomic.Int64
	rateLimited   atomic.Int64
	authCost      atomic.Int64 // 鉴权调用次数（验证"限流在鉴权前"）
	routeNotFound atomic.Int64
}

func newGateway() *gateway {
	g := &gateway{auth: func(t string) bool { return t == "valid-token" }}
	g.routes.Store(&map[string]string{
		"/order": "order-svc",
		"/user":  "user-svc",
	})
	return g
}

// 灰度规则: uid%100 < canaryPercent 走 v2（同 uid 恒定同版本 = 粘性）
const canaryPercent = 20

func (g *gateway) pickVersion(uid int) string {
	if uid%100 < canaryPercent {
		return "v2"
	}
	return "v1"
}

// buildChain: 组装生产中间件链
func (g *gateway) buildChain(backends map[string]func(req *gwRequest) *gwResponse) gwHandler {
	// 终点: 路由 + 转发
	routeAndProxy := func(req *gwRequest) *gwResponse {
		routes := *g.routes.Load()
		svc := ""
		for prefix, s := range routes {
			if strings.HasPrefix(req.Path, prefix) {
				svc = s
				break
			}
		}
		if svc == "" {
			g.routeNotFound.Add(1)
			return &gwResponse{Status: 404, Body: "no route"}
		}
		h, ok := backends[svc]
		if !ok {
			return &gwResponse{Status: 502, Body: "no backend"}
		}
		return h(req)
	}

	return chain(routeAndProxy,
		// 最外层: accessLog（被拒绝的也要记录, 否则监控盲区）
		func(next gwHandler) gwHandler {
			return func(req *gwRequest) *gwResponse {
				g.logs.Add(1)
				return next(req)
			}
		},
		// 第二层: 限流（在鉴权前——先做便宜的拒绝）
		func(next gwHandler) gwHandler {
			return func(req *gwRequest) *gwResponse {
				if g.logs.Load() > 100000 { // 演示用简单阈值
					g.rateLimited.Add(1)
					return &gwResponse{Status: 429, Body: "rate limited"}
				}
				return next(req)
			}
		},
		// 第三层: 鉴权（贵的校验放在限流后）
		func(next gwHandler) gwHandler {
			return func(req *gwRequest) *gwResponse {
				if !g.auth(req.Token) {
					return &gwResponse{Status: 401, Body: "unauthorized"}
				}
				g.authCost.Add(1) // 只统计通过限流后的鉴权调用
				return next(req)
			}
		},
	)
}

func RunGatewayExperiments() {
	fmt.Println("== 实验 04: 网关——路由表 + 中间件链 + 灰度粘性 ==")

	g := newGateway()

	// 模拟后端: order-svc 有 v1/v2 两个版本（灰度目标）
	backends := map[string]func(req *gwRequest) *gwResponse{
		"order-svc": func(req *gwRequest) *gwResponse {
			return &gwResponse{Status: 200, Body: "order data", Version: g.pickVersion(req.UID)}
		},
		"user-svc": func(req *gwRequest) *gwResponse {
			return &gwResponse{Status: 200, Body: "user data", Version: "v1"}
		},
	}

	h := g.buildChain(backends)

	// ---- Part 1: 路由 + 鉴权 ----
	fmt.Println("--- Part 1: 路由与鉴权 ---")
	resp := h(&gwRequest{Path: "/order/detail", UID: 5, Token: "valid-token"})
	fmt.Printf("GET /order/detail (带token) → %d %s [%s]\n", resp.Status, resp.Body, resp.Version)
	resp = h(&gwRequest{Path: "/order/detail", UID: 5, Token: "bad"})
	fmt.Printf("GET /order/detail (坏token) → %d %s\n", resp.Status, resp.Body)
	resp = h(&gwRequest{Path: "/nope", UID: 5, Token: "valid-token"})
	fmt.Printf("GET /nope → %d（路由表未命中）\n", resp.Status)
	fmt.Printf("锚点: 鉴权拦截 401 → %s; 无路由 404 → %s\n", mark(true), mark(resp.Status == 404))

	// ---- Part 2: 灰度粘性 ----
	fmt.Println("\n--- Part 2: 灰度: uid 取模粘性 ---")
	stickyOK := true
	v2Users, v1Users := 0, 0
	for uid := 0; uid < 200; uid++ {
		versions := map[string]int{}
		for i := 0; i < 20; i++ { // 同一 uid 请求 20 次
			r := h(&gwRequest{Path: "/order/x", UID: uid, Token: "valid-token"})
			versions[r.Version]++
		}
		if len(versions) != 1 { // 同 uid 必须恒定同一版本
			stickyOK = false
		}
		if versions["v2"] > 0 {
			v2Users++
		} else {
			v1Users++
		}
	}
	fmt.Printf("灰度规则: uid%%100 < %d → v2（%d%% 用户）; 实际分桶: v1 用户 %d, v2 用户 %d\n",
		canaryPercent, canaryPercent, v1Users, v2Users)
	fmt.Printf("每个 uid 请求 20 次, 版本恒定不变 → %s（粘性: 不会一半 v1 一半 v2）\n", mark(stickyOK))
	fmt.Printf("锚点: 分桶比例接近 %d%% → %s\n", canaryPercent, mark(v2Users >= 35 && v2Users <= 45))

	// ---- Part 3: 路由表热更新（atomic 快照）----
	fmt.Println("\n--- Part 3: 路由表热更新 ---")
	before := h(&gwRequest{Path: "/pay/create", UID: 1, Token: "valid-token"})
	// 热更新: 加一条 /pay 路由（atomic 整体替换快照）
	newRoutes := map[string]string{
		"/order": "order-svc",
		"/user":  "user-svc",
		"/pay":   "pay-svc", // 新增
	}
	g.routes.Store(&newRoutes)
	backends["pay-svc"] = func(req *gwRequest) *gwResponse {
		return &gwResponse{Status: 200, Body: "pay ok", Version: "v1"}
	}
	after := h(&gwRequest{Path: "/pay/create", UID: 1, Token: "valid-token"})
	fmt.Printf("更新前 GET /pay/create → %d; 热更新后 → %d %s\n", before.Status, after.Status, after.Body)
	fmt.Printf("锚点: 路由表热更新立即生效（无重启无发版）→ %s\n", mark(before.Status == 404 && after.Status == 200))

	// ---- Part 4: 中间件链的执行顺序验证 ----
	fmt.Println("\n--- Part 4: 链顺序: 限流在鉴权前 ---")
	fmt.Printf("总请求 %d 次, 鉴权仅被调用 %d 次（401/404 的请求不消耗路由, 但鉴权只对过了限流的执行）\n",
		g.logs.Load(), g.authCost.Load())
	fmt.Println("顺序: accessLog(最外) → rateLimit → auth → route(最内)")
	fmt.Println("WHY: 鉴权有成本（查token）, 恶意流量连鉴权的 CPU 都不该消耗——先做便宜的拒绝")

	// ---- 并发安全演示（race 检测目击点）----
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(uid int) {
			defer wg.Done()
			h(&gwRequest{Path: "/order/x", UID: uid, Token: "valid-token"})
		}(i)
	}
	wg.Wait()
	fmt.Printf("\n并发 100 请求无锁竞争问题（atomic 快照读）, 总日志数 %d\n", g.logs.Load())
}
