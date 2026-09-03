// # 结构型模式实验（笔记 03）
//
// 对应笔记：notes/design-pattern/03-结构型：装饰器与代理.md
//
// 运行：go run ./experiments/ structural
//
// 实验项：
//
//	第1节：装饰器叠罗汉 —— logging → retry → real，顺序即语义
//	第2节：HTTP 中间件链 —— Chain 倒序包裹，参数顺序=执行顺序，可拦截
//	第3节：适配器 + 编译期断言 var _ Interface
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"
)

// RunStructuralExperiments 演示笔记 03 的结构型模式。
func RunStructuralExperiments() {
	fmt.Println("========== 第1节: 装饰器叠罗汉 ==========")
	s1Decorator()

	fmt.Println("\n========== 第2节: HTTP 中间件链 ==========")
	s2Middleware()

	fmt.Println("\n========== 第3节: 适配器与编译断言 ==========")
	s3Adapter()
}

// ---------- 第1节：对象装饰器 ----------

type KV interface {
	Get(ctx context.Context, k string) (string, error)
}

type memKV struct{ m map[string]string }

func (s *memKV) Get(_ context.Context, k string) (string, error) {
	v, ok := s.m[k]
	if !ok {
		return "", errors.New("key not found")
	}
	return v, nil
}

// retryDeco 重试装饰器（持有式：精细控制 Get 的重试语义）。
type retryDeco struct {
	inner KV
	n     int
}

func (r *retryDeco) Get(ctx context.Context, k string) (string, error) {
	var err error
	for i := 0; i < r.n; i++ {
		if v, e := r.inner.Get(ctx, k); e == nil {
			return v, nil
		} else {
			err = e
		}
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
	}
	return "", fmt.Errorf("after %d retries: %w", r.n, err) // %w 保链
}

// loggingDeco 日志装饰器（嵌入式：未覆写的方法自动转发）。
type loggingDeco struct {
	KV // 嵌入接口：借全部方法
}

func (l loggingDeco) Get(ctx context.Context, k string) (string, error) {
	start := time.Now()
	v, err := l.KV.Get(ctx, k) // 显式转发被装饰的那个
	fmt.Printf("  [log] Get(%q) -> %v, err=%v, cost=%v\n", k, v, err, time.Since(start).Round(time.Microsecond))
	return v, err
}

func s1Decorator() {
	var svc KV = &memKV{m: map[string]string{"hit": "value"}}

	// 叠罗汉：logging 包 retry 包 real（*T 方法集——golang 03 篇的方法集陷阱在此真实上演）
	svc = &retryDeco{svc, 3}
	svc = loggingDeco{svc}

	fmt.Println("-- 查不存在的 key：日志显示 retry 内的 3 次尝试都被记录（retry 在内层）--")
	_, err := svc.Get(context.Background(), "miss")
	fmt.Printf("最终错误: %v（retry 内部用包装保留错误链）\n", err)

	fmt.Println("-- 查命中的 key：一次成功 --")
	v, _ := svc.Get(context.Background(), "hit")
	fmt.Printf("成功: %q\n", v)
}

// ---------- 第2节：中间件链 ----------

// Middleware 函数级装饰：Handler 的函数。
type Middleware func(http.Handler) http.Handler

func loggingMW(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Printf("  [logging] %s %s 进\n", r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
	})
}

func authMW(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Token") == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return // 拦截：不转发（对象装饰器做不到的流控）
		}
		next.ServeHTTP(w, r)
	})
}

// Chain 倒序包裹：让参数顺序 = 执行顺序（logging 最外层）。
func Chain(h http.Handler, mws ...Middleware) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}

func s2Middleware() {
	final := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "handler done")
	})

	h := Chain(final, loggingMW, authMW)

	fmt.Println("-- 无 Token：auth 拦截，handler 不执行 --")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, mustReq("GET /api"))
	fmt.Printf("  状态码: %d（handler 未执行, 被中间件短路）\n", rec.Code)

	fmt.Println("-- 有 Token：穿到底 --")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, withToken(mustReq("GET /api"), "secret"))
	fmt.Printf("  状态码: %d, 响应: %s", rec.Code, rec.Body.String())
}

func mustReq(line string) *http.Request {
	parts := strings.Fields(line)
	req, _ := http.NewRequest(parts[0], "http://x"+parts[1], nil)
	return req
}

func withToken(r *http.Request, t string) *http.Request {
	r.Header.Set("Token", t)
	return r
}

// ---------- 第3节：适配器 ----------

// Authorizer 新世界要的接口。
type Authorizer interface {
	Authorize(ctx context.Context, token string) (bool, error)
}

var ErrDenied = errors.New("denied")

// LegacyAuth 旧代码（不能动）。
type LegacyAuth struct{}

func (l *LegacyAuth) CheckToken(token string) bool { return token == "old-key" }

// legacyAdapter 适配器：只做翻译（签名+错误语义），不夹业务。
type legacyAdapter struct{ *LegacyAuth }

// 编译期断言：实现缺失当场编译错，不用等到使用处。
var _ Authorizer = legacyAdapter{&LegacyAuth{}}

func (a legacyAdapter) Authorize(_ context.Context, token string) (bool, error) {
	if !a.CheckToken(token) {
		return false, ErrDenied // 旧 bool 翻译成新世界哨兵
	}
	return true, nil
}

func s3Adapter() {
	var auth Authorizer = legacyAdapter{&LegacyAuth{}}

	ok, err := auth.Authorize(context.Background(), "old-key")
	fmt.Printf("旧凭据: ok=%v err=%v\n", ok, err)

	ok, err = auth.Authorize(context.Background(), "bad")
	fmt.Printf("坏凭据: ok=%v err=%v（bool 被翻译成哨兵 ErrDenied）\n", ok, err)
	fmt.Println("var _ Authorizer = legacyAdapter{...} —— 编译期断言, 漏实现当场报错")
}
