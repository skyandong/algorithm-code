// # 分布式锁
//
// # 基础实现
//
// 正确写法(单条原子命令):
//
//	SET lock_key <token> NX PX 30000
//
//   - NX:不存在才设置(互斥)
//   - PX 30000:30秒过期,防止宕机死锁
//   - value 用唯一 token(UUID),释放时验证归属
//
// 错误写法(两条命令不原子):
//
//	SETNX lock_key token   ← 成功
//	EXPIRE lock_key 30     ← 如果这里宕机,锁永不过期 → 死锁
//
// SET NX PX 是单条命令,Redis 单线程执行,天然原子。
// 这是分布式锁最经典的演进:从两条命令到一条命令。
//
// # 释放锁(Lua 保证原子)
//
//	if redis.call('get', KEYS[1]) == ARGV[1] then
//	    return redis.call('del', KEYS[1])
//	else
//	    return 0
//	end
//
// 为什么必须用 Lua,不能用 GET + DEL:
//
//	T1: 客户端A GET → 是自己的 token,准备 DEL
//	T2: 锁过期,客户端B SET NX 成功,拿到锁
//	T3: 客户端A DEL → 删掉了客户端B的锁  ← 灾难
//
// # 锁超时的本质矛盾
//
// TTL 设多长是悖论:
//
//   - 太短:业务没跑完锁过期,其他客户端进来,并发破坏
//   - 太长:持锁方宕机后,其他客户端等很久才能拿锁
//
// Redisson watchdog 的解法:
//
//	TTL 设短(默认 30s)
//	后台线程每隔 TTL/3(10s)续期一次
//	业务正常完成 → 主动释放锁,停止续期
//	持锁方宕机 → 续期线程也死了 → 锁 TTL 到期自动释放
//
// # 可重入锁
//
// 基础实现不可重入:同一线程加锁两次会死锁。
// Redisson 用 Hash 实现可重入计数:
//
//	HSET lock_key token 1      第一次加锁
//	HINCRBY lock_key token 1   重入,计数+1
//	HINCRBY lock_key token -1  释放一次,计数-1
//	计数=0 时真正删除锁
//
// # Redlock
//
// antirez 提出的多节点分布式锁:
//
//	在 N(通常5)个独立 Redis 实例上依次 SET NX
//	超过半数(N/2+1)成功 且 总耗时 < 锁有效期 → 加锁成功
//	释放:对所有节点执行 Lua 释放脚本
//
// Martin Kleppmann 的反驳(核心):
//
//	客户端持锁期间发生 GC pause 或时钟漂移
//	锁已在 Redis 里过期,其他客户端拿到了锁
//	但原客户端不知道,仍然操作共享资源 → 并发破坏
//	Redlock 没有 fencing token 机制,无法根本解决
//
// antirez 的回应:Redlock 已经足够好,极端情况是系统性问题。
//
// 实际生产选择:
//
//   - 大多数场景:单节点 Redis 锁足够,简单可靠
//   - 强一致要求:ZooKeeper / etcd(基于 Paxos/Raft,有 fencing token)
//   - 不建议 Redlock:实现复杂,边界情况多,收益有限

package experiments

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

const (
	lockScript = `
if redis.call('get', KEYS[1]) == ARGV[1] then
    return redis.call('del', KEYS[1])
else
    return 0
end`
)

// tryLock 尝试获取锁,返回 token 和是否成功
func tryLock(ctx context.Context, key string, ttl time.Duration) (string, bool) {
	token := uuid.New().String()
	ok, err := Rdb.SetNX(ctx, key, token, ttl).Result()
	if err != nil || !ok {
		return "", false
	}
	return token, true
}

// releaseLock 用 Lua 原子释放锁
func releaseLock(ctx context.Context, key, token string) bool {
	result, _ := Rdb.Eval(ctx, lockScript, []string{key}, token).Int()
	return result == 1
}

// ExpBasicLock 实验27: 分布式锁基础实现
//
// 演示 SET NX PX 加锁、Lua 释放锁的完整流程。
// 同时对比老写法(SETNX + EXPIRE)的死锁风险。
//
// 预期输出:
//
//	正确写法: 加锁成功,释放成功
//	错误 token 释放: 失败,不会删别人的锁
//	老写法风险: SETNX 和 EXPIRE 之间宕机会死锁
func ExpBasicLock(ctx context.Context) {
	fmt.Println("=== 实验27: 分布式锁基础实现 ===")

	const lockKey = "dlock:basic"
	Rdb.Del(ctx, lockKey)

	// 加锁
	token, ok := tryLock(ctx, lockKey, 30*time.Second)
	fmt.Printf("  加锁: %v  token: %s...\n", ok, token[:8])

	// 重复加锁失败(互斥)
	_, ok2 := tryLock(ctx, lockKey, 30*time.Second)
	fmt.Printf("  重复加锁: %v  ← 互斥,已被持有\n", ok2)

	// 用错误 token 释放 → 失败
	released := releaseLock(ctx, lockKey, "wrong-token")
	fmt.Printf("  错误 token 释放: %v  ← 不会删别人的锁\n", released)

	// 用正确 token 释放 → 成功
	released = releaseLock(ctx, lockKey, token)
	fmt.Printf("  正确 token 释放: %v\n", released)

	// 释放后可以重新加锁
	_, ok3 := tryLock(ctx, lockKey, 30*time.Second)
	fmt.Printf("  释放后重新加锁: %v\n\n", ok3)

	Rdb.Del(ctx, lockKey)
}

// ExpLockMutex 实验28: 分布式锁保证互斥 —— 并发场景验证
//
// 100 个并发 goroutine 抢同一把锁,验证同一时刻只有一个持锁。
//
// 预期输出:
//
//	并发计数结果 = 100(无锁可能小于100,有锁一定等于100)
func ExpLockMutex(ctx context.Context) {
	fmt.Println("=== 实验28: 分布式锁并发互斥验证 ===")

	const lockKey = "dlock:mutex"
	const n = 100
	Rdb.Del(ctx, lockKey)
	Rdb.Set(ctx, "dlock:counter", 0, 0)

	// 无锁版本:并发读改写,结果不确定
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Go(func() {
			val, _ := Rdb.Get(ctx, "dlock:counter").Int()
			time.Sleep(time.Microsecond) // 模拟业务耗时,放大竞态
			Rdb.Set(ctx, "dlock:counter", val+1, 0)
		})
	}
	wg.Wait()
	noLockResult, _ := Rdb.Get(ctx, "dlock:counter").Int()
	fmt.Printf("  无锁并发 %d 次 +1,结果: %d  (期望100,丢失更新导致偏小)\n", n, noLockResult)

	// 有锁版本:每次操作前加锁
	Rdb.Set(ctx, "dlock:counter", 0, 0)
	var failCount int64
	for i := 0; i < n; i++ {
		wg.Go(func() {
			for {
				token, ok := tryLock(ctx, lockKey, 5*time.Second)
				if !ok {
					atomic.AddInt64(&failCount, 1)
					time.Sleep(time.Millisecond)
					continue
				}
				val, _ := Rdb.Get(ctx, "dlock:counter").Int()
				time.Sleep(time.Microsecond)
				Rdb.Set(ctx, "dlock:counter", val+1, 0)
				releaseLock(ctx, lockKey, token)
				return
			}
		})
	}
	wg.Wait()
	lockResult, _ := Rdb.Get(ctx, "dlock:counter").Int()
	fmt.Printf("  有锁并发 %d 次 +1,结果: %d  (期望100,锁保证串行)\n", n, lockResult)
	fmt.Printf("  抢锁失败重试次数: %d\n\n", failCount)

	Rdb.Del(ctx, lockKey, "dlock:counter")
}

// ExpLockExpiry 实验29: 锁超时与 watchdog 续期
//
// 演示锁 TTL 过短导致业务未完成锁就过期的问题,
// 以及 watchdog 续期的解法。
//
// 预期输出:
//
//	TTL 过短: 业务执行期间锁被其他客户端抢走
//	watchdog: 定期续期,业务完成前锁不过期
func ExpLockExpiry(ctx context.Context) {
	fmt.Println("=== 实验29: 锁超时 + watchdog 续期 ===")

	const lockKey = "dlock:expiry"
	Rdb.Del(ctx, lockKey)

	// TTL 过短的问题:TTL=100ms,但业务需要 300ms
	token, _ := tryLock(ctx, lockKey, 100*time.Millisecond)
	fmt.Printf("  加锁成功,TTL=100ms,token: %s...\n", token[:8])

	// 模拟业务执行 200ms(超过 TTL)
	time.Sleep(200 * time.Millisecond)

	// 此时锁已过期,其他客户端可以拿到锁
	token2, ok2 := tryLock(ctx, lockKey, 5*time.Second)
	fmt.Printf("  业务执行中(200ms后),其他客户端抢锁: %v  ← 锁已过期被抢走\n", ok2)

	// 原客户端尝试释放 → 失败(锁已不是自己的)
	released := releaseLock(ctx, lockKey, token)
	fmt.Printf("  原客户端释放锁: %v  ← 失败,锁已被其他人持有\n\n", released)

	// watchdog 续期方案
	Rdb.Del(ctx, lockKey)
	token3, _ := tryLock(ctx, lockKey, 300*time.Millisecond)
	fmt.Printf("  watchdog 方案: 加锁 TTL=300ms\n")

	// 启动 watchdog:每 100ms 续期一次
	done := make(chan struct{})
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				// 续期:重置 TTL
				Rdb.PExpire(ctx, lockKey, 300*time.Millisecond)
				fmt.Println("  watchdog: 续期一次")
			case <-done:
				return
			}
		}
	}()

	// 业务执行 350ms(超过原始 TTL)
	time.Sleep(350 * time.Millisecond)
	close(done)
	wg.Wait()

	// 验证锁还在
	val, _ := Rdb.Get(ctx, lockKey).Result()
	fmt.Printf("  业务完成(350ms后)锁还在: %v\n", val == token3)

	// 业务完成,主动释放
	released = releaseLock(ctx, lockKey, token3)
	fmt.Printf("  主动释放锁: %v\n\n", released)

	releaseLock(ctx, lockKey, token2)
	Rdb.Del(ctx, lockKey)
}
