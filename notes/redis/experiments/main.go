package main

import (
	"context"
	"fmt"
	"log"

	"github.com/redis/go-redis/v9"
)

var rdb *redis.Client

func main() {
	rdb = redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "123456",
		DB:       0,
	})
	defer rdb.Close()

	ctx := context.Background()
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("连不上 Redis: %v", err)
	}

	fmt.Println("========== 第一节:为什么这么快 ==========\n")
	Exp1Pipelining(ctx)
	Exp2SingleThreadBlock(ctx)
	Exp3ConcurrentThroughput(ctx)

	fmt.Println("========== 第三节:持久化 ==========\n")
	ExpBgsave(ctx)
	ExpAofFsync(ctx)
	ExpCowMemory(ctx)
	ExpAofRewrite(ctx)

	fmt.Println("========== 第九节:生产排查与运维 ==========\n")
	ExpKeysVsScan(ctx)
	ExpSlowlog(ctx)
	ExpLatencyMonitor(ctx)
	ExpBigkey(ctx)
	ExpProgressiveDelete(ctx)

	fmt.Println("========== 第八节:分布式锁 ==========\n")
	ExpBasicLock(ctx)
	ExpLockMutex(ctx)
	ExpLockExpiry(ctx)

	fmt.Println("========== 第七节:事务/Lua/Pipeline ==========\n")
	ExpMultiExec(ctx)
	ExpWatch(ctx)
	ExpLua(ctx)
	ExpPipelineVsSeq(ctx)

	fmt.Println("========== 第六节:内存管理与淘汰 ==========\n")
	ExpLazyDelete(ctx)
	ExpDelVsUnlink(ctx)
	ExpEvictionPolicy(ctx)

	fmt.Println("========== 第五节:缓存常见问题 ==========\n")
	ExpCachePenetration(ctx)
	ExpCacheBreakdown(ctx)
	ExpCacheAvalanche(ctx)
	ExpDelayedDoubleDelete(ctx)
	ExpBloomFilter(ctx)

	fmt.Println("========== 第二节:数据类型与底层结构 ==========\n")
	ExpHash(ctx)
	ExpSet(ctx)
	ExpZSet(ctx)
	ExpLargeValue(ctx)
	ExpRehash(ctx)
	ExpIntsetMemory(ctx)
	ExpListpackVsHashtable(ctx)
}
