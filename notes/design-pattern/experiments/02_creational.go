// # 创建型模式实验（笔记 02）
//
// 对应笔记：notes/design-pattern/02-创建型：functional-options.md
//
// 运行：go run ./experiments/ creational
//
// 实验项：
//
//	第1节：functional options —— 默认值集中 + WithXxx 自文档 + 指针字段区分"没填"
//	第2节：带校验的 Option（返回 error，构造循环短路）
//	第3节：sync.Once 单例 —— 并发下只初始化一次
package main

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// RunCreationalExperiments 演示笔记 02 的创建型模式。
func RunCreationalExperiments() {
	fmt.Println("========== 第1节: functional options ==========")
	c1Options()

	fmt.Println("\n========== 第2节: 带校验的 Option ==========")
	c2Validated()

	fmt.Println("\n========== 第3节: sync.Once 单例 ==========")
	c3Once()
}

// ---------- 第1节：functional options ----------

type serverOptions struct {
	addr    string
	timeout time.Duration
	maxConn int
}

func defaultServerOptions() serverOptions {
	return serverOptions{addr: ":8080", timeout: 30 * time.Second, maxConn: 1000}
}

// ServerOption 可选参数（指针字段版能区分"没填"和"填了零"）。
type ServerOption func(*serverOptions)

func WithTimeout(d time.Duration) ServerOption {
	return func(o *serverOptions) { o.timeout = d }
}

func WithMaxConn(n int) ServerOption {
	return func(o *serverOptions) { o.maxConn = n }
}

func NewServer(opts ...ServerOption) *serverOptions {
	o := defaultServerOptions() // 1. 默认值集中
	for _, opt := range opts { // 2. 逐个覆写
		opt(&o)
	}
	return &o
}

func c1Options() {
	def := NewServer()
	fmt.Printf("全默认: addr=%s timeout=%v maxConn=%d\n", def.addr, def.timeout, def.maxConn)

	custom := NewServer(WithTimeout(3*time.Second), WithMaxConn(10))
	fmt.Printf("覆写两个: timeout=%v maxConn=%d（调用点自文档, 任意顺序, 可零个）\n",
		custom.timeout, custom.maxConn)
}

// ---------- 第2节：带校验的 Option ----------

type validatedOptions struct {
	tls      bool
	certPath string
}

// ValidatedOption 带 error：校验失败短路构造。
type ValidatedOption func(*validatedOptions) error

func WithTLSCert(path string) ValidatedOption {
	return func(o *validatedOptions) error {
		if path == "" {
			return errors.New("WithTLSCert: empty path") // IO 类校验当场能做
		}
		o.tls, o.certPath = true, path
		return nil
	}
}

func NewValidated(opts ...ValidatedOption) (*validatedOptions, error) {
	o := &validatedOptions{}
	for _, opt := range opts {
		if err := opt(o); err != nil {
			return nil, fmt.Errorf("construct: %w", err) // 短路 + %w 保链
		}
	}
	return o, nil
}

func c2Validated() {
	if _, err := NewValidated(WithTLSCert("")); err != nil {
		fmt.Printf("空路径: err=%v（Option 当场报错, 构造短路）\n", err)
	}
	ok, _ := NewValidated(WithTLSCert("/etc/cert.pem"))
	fmt.Printf("正常路径: tls=%v cert=%s\n", ok.tls, ok.certPath)
}

// ---------- 第3节：sync.Once 单例 ----------

type config struct {
	loadedAt time.Time
}

var (
	cfgOnce sync.Once
	cfg     *config
	loadCnt atomic.Int32 // 计数：验证真的只加载一次
)

// GetConfig 包级单例：包即容器 + Once 惰性初始化。
func GetConfig() *config {
	cfgOnce.Do(func() {
		loadCnt.Add(1)
		cfg = &config{loadedAt: time.Now()} // 昂贵初始化只发生一次
	})
	return cfg
}

func c3Once() {
	var wg sync.WaitGroup
	const concurrent = 100
	firsts := make(chan *config, concurrent)

	wg.Add(concurrent)
	for i := 0; i < concurrent; i++ { // 100 个 goroutine 并发抢首初始化
		go func() {
			defer wg.Done()
			firsts <- GetConfig()
		}()
	}
	wg.Wait()
	close(firsts)

	same := true
	var first *config
	for c := range firsts {
		if first == nil {
			first = c
		} else if c != first {
			same = false
		}
	}
	fmt.Printf("100 个并发 GetConfig: 指针全同=%v, 实际加载次数=%d（Once.Do 并发安全）\n",
		same, loadCnt.Load())
	fmt.Println("对比 init(): Once 可传参可失败可延迟——main 之前跑的 init 三样都做不了")
}
