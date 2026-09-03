package main

import (
	"fmt"
	"sync"
	"time"
)

// 实验 01：内存注册中心
// 实现: 注册/心跳/TTL 摘除/订阅推送（笔记 01 §2）
// 演示: ① 实例宕机（停止心跳）→ 调用方名单自动收敛
//       ② 优雅退出（主动反注册, 毫秒级摘除）vs 非优雅退出（等 TTL, 秒级摘除）
//       ③ 心跳正常但健康检查失败 → "活着的僵尸"被探针摘除
// 锚点: 宕机实例在 TTL 内被摘除; 优雅摘除耗时 << TTL; 僵尸实例被健康检查摘除。

const (
	registryTTL       = 300 * time.Millisecond // TTL ≥ 3×心跳间隔
	registryHeartbeat = 100 * time.Millisecond
)

// instance: 注册的实例
type instance struct {
	addr        string
	lastBeat    time.Time
	healthy     bool // 健康检查结果（探针）——与心跳是两层
	gracefulOut bool // 主动反注册标记
}

// registry: 内存注册中心（mutex 保护, 变更推送订阅者）
type registry struct {
	mu        sync.Mutex
	services  map[string]map[string]*instance // 服务名 → addr → 实例
	subscribe []func(svc string, addrs []string)
}

func newRegistry() *registry {
	return &registry{services: map[string]map[string]*instance{}}
}

func (r *registry) register(svc, addr string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.services[svc] == nil {
		r.services[svc] = map[string]*instance{}
	}
	r.services[svc][addr] = &instance{addr: addr, lastBeat: time.Now(), healthy: true}
	r.notifyLocked(svc)
}

// deregister: 优雅退出的第一步——主动摘除, 不等 TTL
func (r *registry) deregister(svc, addr string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.services[svc], addr)
	r.notifyLocked(svc)
}

func (r *registry) heartbeat(svc, addr string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if in, ok := r.services[svc][addr]; ok {
		in.lastBeat = time.Now()
	}
}

// setHealth: 模拟健康检查探针结果（进程活着但依赖故障 → unhealthy）
func (r *registry) setHealth(svc, addr string, healthy bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if in, ok := r.services[svc][addr]; ok {
		in.healthy = healthy
	}
	r.notifyLocked(svc)
}

func (r *registry) notifyLocked(svc string) {
	addrs := make([]string, 0, len(r.services[svc]))
	for a, in := range r.services[svc] {
		if in.healthy { // 订阅者只收到健康实例名单
			addrs = append(addrs, a)
		}
	}
	for _, cb := range r.subscribe {
		cb(svc, append([]string(nil), addrs...))
	}
}

// lookup: 调用方查名单（只返回健康实例）
func (r *registry) lookup(svc string) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	addrs := make([]string, 0)
	for a, in := range r.services[svc] {
		if in.healthy {
			addrs = append(addrs, a)
		}
	}
	return addrs
}

// sweep: 剔除超 TTL 未心跳的实例（消极发现）
func (r *registry) sweep(svc string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for a, in := range r.services[svc] {
		if time.Since(in.lastBeat) > registryTTL {
			delete(r.services[svc], a)
		}
	}
	r.notifyLocked(svc)
}

func RunRegistryExperiments() {
	fmt.Println("== 实验 01: 注册中心——摘除/收敛/优雅退出 ==")
	fmt.Printf("TTL=%v, 心跳间隔=%v（TTL ≥ 3×心跳, 覆盖抖动与 GC）\n", registryTTL, registryHeartbeat)

	r := newRegistry()

	// 订阅者（调用方）: 收名单推送
	var mu sync.Mutex
	latest := []string{}
	changes := 0
	r.subscribe = append(r.subscribe, func(svc string, addrs []string) {
		mu.Lock()
		latest = addrs
		changes++
		mu.Unlock()
	})

	// ---- 场景一: 3 实例注册, 1 个宕机（停心跳）----
	fmt.Println("\n--- 场景一: 实例宕机 → 名单收敛 ---")
	for _, a := range []string{"10.0.0.1:8080", "10.0.0.2:8080", "10.0.0.3:8080"} {
		r.register("order-svc", a)
	}
	fmt.Printf("注册 3 实例, 名单: %v\n", r.lookup("order-svc"))

	// 存活实例继续心跳; 10.0.0.3 "宕机"（无心跳）
	deadline := time.Now().Add(registryTTL + 100*time.Millisecond)
	for time.Now().Before(deadline) {
		r.heartbeat("order-svc", "10.0.0.1:8080")
		r.heartbeat("order-svc", "10.0.0.2:8080")
		time.Sleep(50 * time.Millisecond)
	}
	r.sweep("order-svc") // 对账扫描（生产是定时任务持续跑）

	mu.Lock()
	afterCrash := append([]string(nil), latest...)
	mu.Unlock()
	fmt.Printf("10.0.0.3 停止心跳 → TTL 超时被摘除, 名单: %v\n", afterCrash)
	fmt.Printf("锚点: 宕机实例从名单消失 → %s\n", mark(len(afterCrash) == 2 && !contains(afterCrash, "10.0.0.3:8080")))

	// ---- 场景二: 优雅退出 vs 非优雅退出 ----
	fmt.Println("\n--- 场景二: 优雅退出 vs 非优雅退出 ---")

	// 优雅: 主动反注册
	r.register("order-svc", "10.0.0.3:8080")
	t0 := time.Now()
	r.deregister("order-svc", "10.0.0.3:8080")
	gracefulDur := time.Since(t0)
	gracefulGone := !contains(r.lookup("order-svc"), "10.0.0.3:8080")
	fmt.Printf("优雅退出（主动反注册）: 耗时 %v, 立即从名单消失 → %s\n", gracefulDur, mark(gracefulGone))

	// 非优雅: 直接失联, 等 TTL
	r.register("order-svc", "10.0.0.3:8080")
	t1 := time.Now()
	// 存活者继续心跳, 死者不管
	d := time.Now().Add(registryTTL + 60*time.Millisecond)
	for time.Now().Before(d) {
		r.heartbeat("order-svc", "10.0.0.1:8080")
		r.heartbeat("order-svc", "10.0.0.2:8080")
		time.Sleep(40 * time.Millisecond)
	}
	r.sweep("order-svc")
	crashDur := time.Since(t1)
	fmt.Printf("非优雅退出（等 TTL 超时）: 耗时 %v（≈TTL）\n", crashDur)
	fmt.Printf("锚点: 优雅摘除 %v << 非优雅 %v → %s\n", gracefulDur, crashDur, mark(gracefulDur < crashDur/3))

	// ---- 场景三: 心跳正常但健康检查失败（"活着的僵尸"）----
	fmt.Println("\n--- 场景三: 健康探针摘除僵尸实例 ---")
	fmt.Printf("摘除前名单: %v\n", r.lookup("order-svc"))
	r.heartbeat("order-svc", "10.0.0.2:8080") // 心跳照常（进程活着）
	r.setHealth("order-svc", "10.0.0.2:8080", false) // 但探针失败（DB 依赖挂了）
	afterZombie := r.lookup("order-svc")
	fmt.Printf("心跳正常 + 探针失败 → 立即摘除, 名单: %v\n", afterZombie)
	fmt.Printf("锚点: 僵尸实例被探针摘除 → %s\n", mark(!contains(afterZombie, "10.0.0.2:8080")))
	fmt.Println("  （只靠心跳会漏掉『进程活着但不能服务』——两层缺一不可）")

	mu.Lock()
	totalChanges := changes
	mu.Unlock()
	fmt.Printf("\n统计: 名单推送 %d 次（注册 4 + 摘除 3 + 健康变更 1）\n", totalChanges)
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
