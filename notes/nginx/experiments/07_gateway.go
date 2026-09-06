package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ============================================================
// 第七节：用 nginx 手搓一个最小 API 网关
// ============================================================
// 对应配置：conf/gateway.conf（本机）或 conf/gateway-docker.conf（容器）
//
// 这一节想说明白的事：网关不是什么神秘组件，它就是
//   「把每个服务都要写一遍的横切逻辑，用 nginx 指令拼出来」
//   - auth_request      → 统一鉴权（服务里不用再验签）
//   - limit_req         → 限流
//   - map + proxy_pass  → 灰度路由
//   - proxy_set_header  → 身份透传
//
// 端口：网关 8090 / v1 后端 8071 / v2 后端 8072 / 鉴权服务 9090

const (
	gwAddr    = "127.0.0.1:8090" // 网关入口
	gwV1Addr  = "127.0.0.1:8071" // v1 后端（直连，用于演示绕过）
	authAddr  = "127.0.0.1:9090" // 鉴权服务（auth_request 子请求目标）
	gwBaseURL = "http://" + gwAddr
)

// gwTokens 模拟 token → 用户的映射。真实场景这里查 Redis 或调鉴权中心。
var gwTokens = map[string][2]string{
	"alice-token": {"1001", "gold"},
	"bob-token":   {"1002", "normal"},
}

// ============================================================
// 鉴权服务：nginx 的 auth_request 会为每笔业务请求发一个子请求打到 /auth
// 它只回状态码：200 放行，401 拒绝。用户信息放在响应头里，
// 由 nginx 的 auth_request_set 抓成变量再透传给后端。
// ============================================================
func startAuthService(addr string) {
	mux := http.NewServeMux()

	mux.HandleFunc("/auth", func(w http.ResponseWriter, r *http.Request) {
		uid, tier, ok := parseToken(r.Header.Get("Authorization"))
		if !ok {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		// 这两个头是整节的题眼：网关验一次，后端直接用，不再验第二次
		w.Header().Set("X-User-Id", uid)
		w.Header().Set("X-User-Tier", tier)
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "auth-service path=%s\n", r.URL.Path)
	})

	go func() {
		if err := http.ListenAndServe(addr, mux); err != nil {
			fmt.Printf("鉴权服务启动失败: %v\n", err)
		}
	}()
}

func parseToken(authz string) (uid, tier string, ok bool) {
	parts := strings.SplitN(authz, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return "", "", false
	}
	v, found := gwTokens[strings.TrimSpace(parts[1])]
	if !found {
		return "", "", false
	}
	return v[0], v[1], true
}

// ============================================================
// v1 / v2 后端：零 TLS、零鉴权，只信网关透传过来的头
// 这就是"后端只跑业务逻辑"的样子
// ============================================================
func startGatewayBackend(addr, version string) {
	mux := http.NewServeMux()

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"version": version, // 灰度实验用它区分走了 v1 还是 v2
			"path":    r.URL.Path,
			// 后端完全不验签，直接用网关给的身份
			"gw_user":  r.Header.Get("X-Gateway-User"),
			"gw_tier":  r.Header.Get("X-Gateway-Tier"),
			"gw_route": r.Header.Get("X-Gateway-Route"),
			"proto":    r.Header.Get("X-Forwarded-Proto"),
		})
	}
	mux.HandleFunc("/api/", handler)
	mux.HandleFunc("/", handler)

	go func() {
		if err := http.ListenAndServe(addr, mux); err != nil {
			fmt.Printf("后端 %s 启动失败: %v\n", addr, err)
		}
	}()
}

// ============================================================
// 实验主体
// ============================================================

// gwGet 发一个请求到网关，返回状态码和响应体
func gwGet(path string, headers map[string]string) (int, string) {
	req, err := http.NewRequest("GET", gwBaseURL+path, nil)
	if err != nil {
		return 0, err.Error()
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err.Error()
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, strings.TrimSpace(string(body))
}

// gwAlive 探测网关是否起来。没起来就给指引，不要让实验崩掉。
func gwAlive() bool {
	c := &http.Client{Timeout: 800 * time.Millisecond}
	resp, err := c.Get(gwBaseURL + "/healthz")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == 200
}

func Exp7Gateway() {
	fmt.Println("目标：用 nginx 拼出网关四件套 —— 鉴权 / 限流 / 灰度 / 透传")

	if !gwAlive() {
		fmt.Println()
		fmt.Println("⚠️  网关没起来（8090 连不上），跳过本节的链路验证。")
		fmt.Println("    请先按下面任意一种方式启动 nginx：")
		fmt.Println()
		fmt.Println("    方式 A（本机，需 brew install nginx）：")
		fmt.Println("      cd ../conf && mkdir -p logs run")
		fmt.Println("      nginx -c \"$(pwd)/gateway.conf\"")
		fmt.Println()
		fmt.Println("    方式 B（Docker，不用装 nginx）：")
		fmt.Println("      cd ../conf && mkdir -p logs")
		fmt.Println("      docker run --rm --name gw-lab -p 8090:8090 \\")
		fmt.Println("        -v \"$(pwd)/gateway-docker.conf\":/etc/nginx/nginx.conf:ro \\")
		fmt.Println("        -v \"$(pwd)/logs\":/var/log/nginx \\")
		fmt.Println("        nginx:1.27-alpine")
		fmt.Println()
		fmt.Println("    （后端和鉴权服务已经在本进程里起好了，只差 nginx 这一层）")
		return
	}

	// ---------- 7.1 统一鉴权 ----------
	fmt.Println()
	fmt.Println("---------- 7.1 统一鉴权（auth_request）----------")
	fmt.Println("后端代码里没有任何验签逻辑，全靠网关这一层挡住。")

	code, body := gwGet("/api/hello", nil)
	fmt.Printf("  无 token          → %d  %s\n", code, body)

	code, body = gwGet("/api/hello", map[string]string{"Authorization": "Bearer wrong-token"})
	fmt.Printf("  错误 token        → %d  %s\n", code, body)

	code, body = gwGet("/api/hello", map[string]string{"Authorization": "Bearer alice-token"})
	fmt.Printf("  正确 token(alice) → %d  %s\n", code, body)
	fmt.Println("  注意 gw_user=1001 / gw_tier=gold：身份是网关注入的，后端没验签。")

	// ---------- 7.2 内部端点不可直连 ----------
	fmt.Println()
	fmt.Println("---------- 7.2 鉴权端点为什么必须加 internal ----------")
	code, body = gwGet("/_gw_auth", map[string]string{"Authorization": "Bearer alice-token"})
	fmt.Printf("  外部直连 /_gw_auth → %d\n", code)
	fmt.Println("  404 才是对的。少了 internal，任何人都能绕过业务路由直接问鉴权服务。")

	// ---------- 7.3 灰度分流 ----------
	fmt.Println()
	fmt.Println("---------- 7.3 灰度分流（map + 变量 proxy_pass）----------")
	time.Sleep(1200 * time.Millisecond) // 让限流桶空一点，避免干扰

	for _, canary := range []string{"", "1"} {
		h := map[string]string{"Authorization": "Bearer bob-token"}
		if canary != "" {
			h["X-Canary"] = canary
		}
		_, body = gwGet("/api/hello", h)
		label := "默认流量"
		if canary != "" {
			label = "X-Canary:1"
		}
		fmt.Printf("  %-12s → %s\n", label, body)
	}

	// ---------- 7.4 限流 ----------
	fmt.Println()
	fmt.Println("---------- 7.4 限流（rate=3r/s burst=5）----------")
	time.Sleep(1500 * time.Millisecond) // 等漏桶排空，否则拿到的是上一节的残留

	ok, limited := 0, 0
	for i := 0; i < 15; i++ {
		c, _ := gwGet("/api/hello", map[string]string{"Authorization": "Bearer alice-token"})
		switch c {
		case 200:
			ok++
		case 429:
			limited++
		}
	}
	fmt.Printf("  连打 15 次：200 × %d，429 × %d\n", ok, limited)
	fmt.Println("  前几个吃掉 burst 额度，后面被漏桶挡掉 —— 后端一次都没被压到。")

	// ---------- 7.5 网关不是安全边界 ----------
	fmt.Println()
	fmt.Println("---------- 7.5 直连后端：网关拦不住绕过 ----------")
	resp, err := http.Get("http://" + gwV1Addr + "/api/hello")
	if err == nil {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		fmt.Printf("  绕过网关直连 8071 → %d  %s\n", resp.StatusCode, strings.TrimSpace(string(b)))
		fmt.Println("  gw_user 是空的：请求压根没过网关，鉴权自然没生效。")
		fmt.Println("  → 网关是流量入口，不是安全边界。真正的隔离靠网络策略")
		fmt.Println("    （后端只监听内网 / 服务网格 mTLS），别指望应用层网关兜底。")
	}

	fmt.Println()
	fmt.Printf("本节用到：鉴权服务 %s，v1 后端 8071，v2 后端 8072，网关 %s\n", authAddr, gwAddr)
}
