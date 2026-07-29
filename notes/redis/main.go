package main

import (
	"context"
	"fmt"
	"log"

	"github.com/redis/go-redis/v9"

	"aredis/experiments"
)

func main() {
	rdb := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "123456",
		DB:       0,
	})
	defer rdb.Close()

	ctx := context.Background()
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("连不上 Redis: %v", err)
	}

	experiments.Rdb = rdb

	fmt.Println("========== 第一节:为什么这么快 ==========\n")
	experiments.Exp1Pipelining(ctx)
	experiments.Exp2SingleThreadBlock(ctx)
	experiments.Exp3ConcurrentThroughput(ctx)

	fmt.Println("========== 第三节:持久化 ==========\n")
	experiments.ExpBgsave(ctx)
	experiments.ExpAofFsync(ctx)
	experiments.ExpCowMemory(ctx)
	experiments.ExpAofRewrite(ctx)

	fmt.Println("========== 第九节:生产排查与运维 ==========\n")
	experiments.ExpKeysVsScan(ctx)
	experiments.ExpSlowlog(ctx)
	experiments.ExpLatencyMonitor(ctx)
	experiments.ExpBigkey(ctx)
	experiments.ExpProgressiveDelete(ctx)

	fmt.Println("========== 第八节:分布式锁 ==========\n")
	experiments.ExpBasicLock(ctx)
	experiments.ExpLockMutex(ctx)
	experiments.ExpLockExpiry(ctx)

	fmt.Println("========== 第七节:事务/Lua/Pipeline ==========\n")
	experiments.ExpMultiExec(ctx)
	experiments.ExpWatch(ctx)
	experiments.ExpLua(ctx)
	experiments.ExpPipelineVsSeq(ctx)

	fmt.Println("========== 第六节:内存管理与淘汰 ==========\n")
	experiments.ExpLazyDelete(ctx)
	experiments.ExpDelVsUnlink(ctx)
	experiments.ExpEvictionPolicy(ctx)

	fmt.Println("========== 第五节:缓存常见问题 ==========\n")
	experiments.ExpCachePenetration(ctx)
	experiments.ExpCacheBreakdown(ctx)
	experiments.ExpCacheAvalanche(ctx)
	experiments.ExpDelayedDoubleDelete(ctx)
	experiments.ExpBloomFilter(ctx)

	fmt.Println("========== 第二节:数据类型与底层结构 ==========\n")
	experiments.ExpHash(ctx)
	experiments.ExpSet(ctx)
	experiments.ExpZSet(ctx)
	experiments.ExpLargeValue(ctx)
	experiments.ExpRehash(ctx)
	experiments.ExpIntsetMemory(ctx)
	experiments.ExpListpackVsHashtable(ctx)
}
