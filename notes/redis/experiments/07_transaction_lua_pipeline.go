// # 事务 / Lua / Pipeline
//
// 三者经常被混淆,核心区别一句话:
//
//	Pipeline   省网络,不保证原子
//	MULTI/EXEC 保顺序,不保回滚
//	Lua        真原子,不省网络
//
// # 事务(MULTI / EXEC / DISCARD / WATCH)
//
// 执行流程:
//
//	MULTI       开启事务,后续命令入队(不执行)
//	命令1..N    返回 QUEUED,不真正执行
//	EXEC        一次性顺序执行所有入队命令
//	DISCARD     清空队列,放弃事务
//
// 两种错误的处理方式不同:
//
//  1. 入队时报错(语法错误):整个事务被拒绝,EXEC 返回错误
//  2. 执行时报错(类型错误,如对 String 执行 LPUSH):只有出错的命令失败,
//     其他命令继续执行 ← 这就是"不支持回滚"
//
// Redis 事务满足"隔离性"(执行期间不被其他客户端插入),不满足"原子性"(出错不回滚)。
//
// WATCH 乐观锁:
//
//	WATCH key      监视 key
//	MULTI ... EXEC 若 key 在 WATCH 后被修改,EXEC 返回 nil(事务放弃)
//
// 本质是 CAS:先快照,执行前检查是否被改过,被改了就放弃重试。
//
// # Lua 脚本
//
// Redis 单线程执行 Lua 脚本,执行期间不插入任何其他命令,是真原子操作。
//
// EVAL vs EVALSHA:
//
//	EVAL script numkeys key... arg...  每次发完整脚本,带宽浪费
//	SCRIPT LOAD → SHA1
//	EVALSHA sha1 numkeys ...           只发 SHA1,脚本已缓存,省带宽
//
// 经典用途:分布式锁释放(判断+删除必须原子):
//
//	if redis.call('get', KEYS[1]) == ARGV[1] then
//	    return redis.call('del', KEYS[1])
//	else
//	    return 0
//	end
//
// Lua 的代价:脚本执行时间过长阻塞主线程。
// lua-time-limit(默认 5000ms)超时后 Redis 拒绝其他命令(返回 BUSY)。
// 生产规则:Lua 脚本必须短小,不能有循环或复杂计算。
//
// # Pipeline
//
// 客户端把多条命令攒起来一次发送,减少网络 RTT。
// 服务端按顺序执行,按顺序返回。
//
// 三者对比:
//
//	          原子性           网络优化   回滚    适用场景
//	Pipeline  否(可被穿插)    是         无      批量读写,对原子性无要求
//	MULTI     顺序不被穿插    否         无      需要隔离但能接受部分失败
//	Lua       是(真原子)      否         无      判断+操作必须原子的场景

package main

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// ExpMultiExec 实验23: MULTI/EXEC 事务 —— 不支持回滚
//
// 演示事务中部分命令报错时其他命令仍然执行。
//
// 预期输出:
//
//	对 String 执行 LPUSH(类型错误) → 报错
//	同一事务中的 SET 命令 → 仍然成功执行
//	结论: Redis 事务出错不回滚
func ExpMultiExec(ctx context.Context) {
	fmt.Println("=== 实验23: MULTI/EXEC 事务不支持回滚 ===")

	rdb.Del(ctx, "tx:str", "tx:result")
	rdb.Set(ctx, "tx:str", "hello", 0)

	// TxPipelined = MULTI + 命令入队 + EXEC
	// 命令1: LPUSH 对 String 操作,类型错误
	// 命令2: SET 正常命令
	cmds, _ := rdb.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.LPush(ctx, "tx:str", "item")      // 类型错误
		pipe.Set(ctx, "tx:result", "ok", 0)    // 正常命令
		return nil
	})

	fmt.Printf("  LPUSH 结果: %v\n", cmds[0].Err())
	fmt.Printf("  SET   结果: %v\n", cmds[1].Err())

	val, _ := rdb.Get(ctx, "tx:result").Result()
	fmt.Printf("  tx:result = %q  ← SET 成功,说明事务没有因 LPUSH 报错而回滚\n\n", val)

	rdb.Del(ctx, "tx:str", "tx:result")
}

// ExpWatch 实验24: WATCH 乐观锁 —— CAS 模式
//
// 用 WATCH 保护"读 → 改 → 写"操作:
// 若 key 在 WATCH 后被其他客户端修改,EXEC 返回 nil,事务放弃。
//
// 预期输出:
//
//	key 未被修改: 事务成功执行
//	key 被修改后: 事务放弃,返回 nil
func ExpWatch(ctx context.Context) {
	fmt.Println("=== 实验24: WATCH 乐观锁 ===")

	rdb.Set(ctx, "watch:counter", 0, 0)

	// 场景1: 没有并发修改,事务成功
	err := rdb.Watch(ctx, func(tx *redis.Tx) error {
		val, err := tx.Get(ctx, "watch:counter").Int()
		if err != nil {
			return err
		}
		_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
			pipe.Set(ctx, "watch:counter", val+1, 0)
			return nil
		})
		return err
	}, "watch:counter")
	fmt.Printf("  无并发修改,事务结果: %v\n", err)
	val, _ := rdb.Get(ctx, "watch:counter").Result()
	fmt.Printf("  watch:counter = %s\n", val)

	// 场景2: 模拟并发修改,事务被放弃
	rdb.Set(ctx, "watch:counter", 0, 0)
	err = rdb.Watch(ctx, func(tx *redis.Tx) error {
		// 在 WATCH 之后、EXEC 之前,模拟另一个客户端修改了 key
		rdb.Set(ctx, "watch:counter", 999, 0)

		_, err := tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
			pipe.Set(ctx, "watch:counter", 100, 0)
			return nil
		})
		return err
	}, "watch:counter")

	fmt.Printf("\n  有并发修改,事务结果: %v  (TxFailedErr = 事务放弃)\n", err)
	val, _ = rdb.Get(ctx, "watch:counter").Result()
	fmt.Printf("  watch:counter = %s  ← 仍是 999,事务的 SET 100 未执行\n\n", val)

	rdb.Del(ctx, "watch:counter")
}

// ExpLua 实验25: Lua 脚本 —— 真原子操作 + EVALSHA
//
// 对比两种分布式锁释放方式:
//
//	非原子(GET + DEL): 判断和删除之间可能被插入,删掉别人的锁
//	原子(Lua):        判断和删除在同一脚本,不可分割
//
// 同时演示 EVALSHA 省带宽。
func ExpLua(ctx context.Context) {
	fmt.Println("=== 实验25: Lua 原子性 + EVALSHA ===")

	const lockKey = "lua:lock"
	const myToken = "my-token"
	const otherToken = "other-token"

	luaScript := `
if redis.call('get', KEYS[1]) == ARGV[1] then
    return redis.call('del', KEYS[1])
else
    return 0
end`

	// 用正确 token 释放 → 成功
	rdb.Set(ctx, lockKey, myToken, 10*time.Second)
	result, _ := rdb.Eval(ctx, luaScript, []string{lockKey}, myToken).Int()
	fmt.Printf("  正确 token 释放: %d  (1=成功删除)\n", result)

	exists, _ := rdb.Exists(ctx, lockKey).Result()
	fmt.Printf("  锁是否还在: %d  (0=已释放)\n", exists)

	// 用错误 token 释放 → 失败(保护别人的锁)
	rdb.Set(ctx, lockKey, otherToken, 10*time.Second)
	result, _ = rdb.Eval(ctx, luaScript, []string{lockKey}, myToken).Int()
	fmt.Printf("\n  错误 token 释放: %d  (0=拒绝,不会删别人的锁)\n", result)

	exists, _ = rdb.Exists(ctx, lockKey).Result()
	fmt.Printf("  锁是否还在: %d  (1=未被删)\n", exists)

	// EVALSHA:缓存脚本
	sha, _ := rdb.ScriptLoad(ctx, luaScript).Result()
	fmt.Printf("\n  SCRIPT LOAD SHA1: %s\n", sha[:8]+"...")
	result, _ = rdb.EvalSha(ctx, sha, []string{lockKey}, otherToken).Int()
	fmt.Printf("  EVALSHA 结果: %d\n", result)
	fmt.Println("  结论: EVALSHA 只传 40 字节 SHA1,脚本越长越省带宽\n")

	rdb.Del(ctx, lockKey)
}

// ExpPipelineVsSeq 实验26: Pipeline vs 逐条 —— RTT 差异
//
// 1000 次 RPUSH,对比逐条 vs Pipeline 的耗时。
// 同时说明 Pipeline 和 MULTI 的本质区别。
func ExpPipelineVsSeq(ctx context.Context) {
	fmt.Println("=== 实验26: Pipeline vs 逐条 ===")

	rdb.Del(ctx, "pipe:list")

	// 逐条
	start := time.Now()
	for i := 0; i < 1000; i++ {
		rdb.RPush(ctx, "pipe:list", i)
	}
	seqDur := time.Since(start)
	rdb.Del(ctx, "pipe:list")

	// Pipeline
	start = time.Now()
	rdb.Pipelined(ctx, func(pipe redis.Pipeliner) error {
		for i := 0; i < 1000; i++ {
			pipe.RPush(ctx, "pipe:list", i)
		}
		return nil
	})
	pipeDur := time.Since(start)

	fmt.Printf("  逐条 1000 次 RPUSH:  %v\n", seqDur)
	fmt.Printf("  Pipeline 1000 次:    %v\n", pipeDur)
	fmt.Printf("  提速: %.1fx\n\n", float64(seqDur)/float64(pipeDur))

	fmt.Println("  Pipeline vs MULTI 的本质区别:")
	fmt.Println("    Pipeline: 客户端打包,命令可能被其他客户端穿插,只省 RTT")
	fmt.Println("    MULTI:    服务端保证命令不被穿插,但不省 RTT,也不回滚")
	fmt.Println("    Lua:      真原子,服务端执行,脚本过长会阻塞主线程\n")

	rdb.Del(ctx, "pipe:list")
}
