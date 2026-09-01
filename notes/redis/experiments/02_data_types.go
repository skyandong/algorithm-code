// # 数据类型与底层结构
//
// 各类型底层编码(Redis 7.0+):
//
//	String     →  SDS(简单动态字符串)
//	Hash       →  listpack / hashtable
//	List       →  quicklist(节点为 listpack)
//	Set        →  intset / listpack / hashtable
//	Sorted Set →  listpack / skiplist + dict
//
// listpack 替代 ziplist 的原因:
// ziplist 的 prevlen 字段记录前一个 entry 的长度,改一个 entry 大小可能触发连锁更新,最坏 O(N²)。
//
// ziplist entry 结构:
//
//	| prevlen | encoding | data |
//
// listpack 把 prevlen 删掉,改成在 entry 末尾存自己的长度(backlen):
//
//	| encoding | data | backlen |
//
// 改一个 entry 完全不影响其他 entry,彻底消除连锁更新。
// 代价:不能从任意位置往前找前驱。Redis 使用场景不需要这个 ——
// 典型工程取舍:为了消除最坏情况,放弃一个用不到的能力。
//
// 编码转换阈值(可配置,触发后不可逆):
//
//	Hash       listpack → hashtable   元素数>512 或 value>64B   hash-max-listpack-entries/value
//	List       始终是 quicklist,无整体转换;单节点是 listpack,
//	           节点大小超限自动分裂新节点                  list-max-listpack-size(默认 8KB)
//	Set        intset   → hashtable   元素数>512               set-max-intset-entries
//	Set        intset   → listpack    插入非整数(仅 7.2+;<7.2 直接 hashtable)
//	Set        listpack → hashtable   元素数>128 或 value>64B   set-max-listpack-entries
//	Sorted Set listpack → skiplist    元素数>128 或 member>64B  zset-max-listpack-entries/value
//
// 生产注意:阈值调太大有风险。listpack 是 O(N) 查找,元素多了慢命令拖垮主线程。
// 曾有业务把 hash-max-listpack-entries 调到几千,单个 Hash HGETALL 直接打出慢查询。
//
// SDS(简单动态字符串):
//
//   - O(1) 取长度:头部直接存 len,C 字符串要遍历到 \0
//   - 二进制安全:不依赖 \0 判断结束,可存图片/序列化数据
//   - 空间预分配:扩容时多分配,减少内存重分配次数
//   - 惰性释放:缩短字符串时不立即释放内存,留着复用
//
// Sorted Set 为什么用跳表不用红黑树:
//
//   - 范围查询更简单:跳表找到起点后顺序遍历,红黑树要中序遍历
//   - 实现复杂度更低:红黑树插入/删除要旋转,跳表只改指针
//   - 层高随机决定(概率 0.25,最高 32 层),期望 O(log N)
//
// 跳表 vs B+ 树(追问):
//
//	跳表                          B+ 树
//	内存友好,指针少               内存占用大(每个节点存多个 key)
//	范围查询顺序遍历链表           范围查询顺序遍历叶子节点链表
//	实现简单                      实现复杂
//	随机写性能好(只改指针)        写操作可能触发页分裂
//	不适合磁盘(随机访问多)        适合磁盘(矮胖树,IO 次数少)
//
// B+ 树矮胖:树高通常只有 3~4 层,一次查询最多 3~4 次磁盘 IO。
// 跳表瘦高:查询路径长,每一跳都可能是随机内存访问,磁盘 IO 多。
// 所以 MySQL 用 B+ 树,Redis 用跳表 —— 各自适配存储介质。
//
// intset 升级机制:
// Set 全是整数时用 intset(有序数组,二分查找)。
// 插入超出当前编码范围的整数触发升级(int16→int32→int64),升级不可逆,O(N) 操作。
//
// 渐进式 rehash:
//
// 触发条件:
//
//   - 扩容:负载因子(used/size) > 1 时触发;BGSAVE/BGREWRITEAOF 期间阈值提高到 5
//   - 缩容:负载因子 < 0.1 时触发;BGSAVE/BGREWRITEAOF 期间同样抑制
//
// 为什么 BGSAVE 期间抑制 rehash:
// fork 子进程后父子共享内存页(COW)。rehash 大量写新表 → 大量页被修改 → COW 复制暴增 → 内存炸。
//
// 迁移过程:
//
//	ht[0]       旧表(待迁移)
//	ht[1]       新表(2 倍大小)
//	rehashidx   当前迁移到第几个 bucket,初始 -1,完成后置回 -1
//
// 期间读写行为:
//
//   - 写:只写 ht[1],保证 ht[0] 只减不增,迁移终会完成
//   - 读:先查 ht[0],没有再查 ht[1](key 可能还没迁过去)
//
// 工程影响:大量写入时 rehash 持续进行,每次写操作多一个 bucket 迁移开销,压测会看到轻微延迟抖动。

package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

func encoding(ctx context.Context, key string) string {
	e, err := rdb.ObjectEncoding(ctx, key).Result()
	if err != nil {
		return fmt.Sprintf("err: %v", err)
	}
	return e
}

// ExpHash 实验4: Hash 编码转换 listpack → hashtable
//
// 触发条件(默认阈值):
//
//	元素数 > 512  →  hash-max-listpack-entries
//	单个 value > 64B  →  hash-max-listpack-value
//
// 预期输出:
//
//	1 个字段:      listpack
//	512 个字段:    listpack
//	513 个字段:    hashtable  ← 转换
//	删回 1 个字段: hashtable  ← 不可逆
func ExpHash(ctx context.Context) {
	fmt.Println("=== Hash: listpack → hashtable ===")
	fmt.Println("阈值: hash-max-listpack-entries=512, hash-max-listpack-value=64")

	rdb.Del(ctx, "exp:hash")

	rdb.HSet(ctx, "exp:hash", "f1", "v1")
	fmt.Printf("1 个字段:      %s\n", encoding(ctx, "exp:hash"))

	for i := 2; i <= 512; i++ {
		rdb.HSet(ctx, "exp:hash", fmt.Sprintf("f%d", i), "v")
	}
	fmt.Printf("512 个字段:    %s\n", encoding(ctx, "exp:hash"))

	rdb.HSet(ctx, "exp:hash", "f513", "v")
	fmt.Printf("513 个字段:    %s  ← 转换\n", encoding(ctx, "exp:hash"))

	for i := 2; i <= 513; i++ {
		rdb.HDel(ctx, "exp:hash", fmt.Sprintf("f%d", i))
	}
	fmt.Printf("删回 1 个字段: %s  ← 不可逆\n\n", encoding(ctx, "exp:hash"))
}

// ExpSet 实验5: Set 编码转换 intset → listpack/hashtable
//
// 纯整数时用 intset(有序数组,二分查找,内存紧凑)。
//
// 插入任意一个非整数字符串,立即升级,不可逆。
//
//	3 个整数:      intset
//	插入字符串后:  listpack  ← 触发升级
func ExpSet(ctx context.Context) {
	fmt.Println("=== Set: intset → listpack/hashtable ===")

	rdb.Del(ctx, "exp:set")

	rdb.SAdd(ctx, "exp:set", 1, 2, 3)
	fmt.Printf("3 个整数:      %s\n", encoding(ctx, "exp:set"))

	rdb.SAdd(ctx, "exp:set", "hello")
	fmt.Printf("插入字符串后:  %s  ← 触发升级\n\n", encoding(ctx, "exp:set"))
}

// ExpZSet 实验6: Sorted Set 编码转换 listpack → skiplist+dict
//
// 触发条件(默认阈值):
//
//	元素数 > 128  →  zset-max-listpack-entries
//	单个 member > 64B  →  zset-max-listpack-value
//
// 转换后同时持有 skiplist(范围查询 O(log N))和 dict(ZSCORE O(1))。
//
// 预期输出:
//
//	1 个成员:    listpack
//	128 个成员:  listpack
//	129 个成员:  skiplist  ← 转换
func ExpZSet(ctx context.Context) {
	fmt.Println("=== Sorted Set: listpack → skiplist ===")

	rdb.Del(ctx, "exp:zset")

	rdb.ZAdd(ctx, "exp:zset", redis.Z{Score: 1, Member: "a"})
	fmt.Printf("1 个成员:    %s\n", encoding(ctx, "exp:zset"))

	for i := 2; i <= 128; i++ {
		rdb.ZAdd(ctx, "exp:zset", redis.Z{Score: float64(i), Member: fmt.Sprintf("m%d", i)})
	}
	fmt.Printf("128 个成员:  %s\n", encoding(ctx, "exp:zset"))

	rdb.ZAdd(ctx, "exp:zset", redis.Z{Score: 129, Member: "m129"})
	fmt.Printf("129 个成员:  %s  ← 转换\n\n", encoding(ctx, "exp:zset"))
}

// ExpRehash 实验8: 渐进式 rehash
//
// 写入大量 key 触发 hashtable 扩容,观察 rehash 期间的延迟抖动。
//
// INFO stats 里的 total_commands_processed 和写入延迟可以看出 rehash 的分摊开销。
// DEBUG RELOAD 强制触发一次完整 rehash(生产严禁,实验专用)。
//
// 预期现象:
//
//	rehash 期间每次写操作多迁移一个 bucket,延迟略高于稳定态
//	DEBUG RELOAD 后延迟恢复正常
func ExpRehash(ctx context.Context) {
	fmt.Println("=== 实验8: 渐进式 rehash ===")

	// 清理旧数据
	var keys []string
	for i := 0; i < 200; i++ {
		keys = append(keys, fmt.Sprintf("rehash:key:%d", i))
	}
	rdb.Del(ctx, keys...)

	// 写入 200 个 key,触发从 listpack 升级到 hashtable 再到扩容
	for i := 0; i < 200; i++ {
		rdb.HSet(ctx, "rehash:hash", fmt.Sprintf("f%d", i), i)
	}
	fmt.Printf("写入 200 个字段后编码: %s\n", encoding(ctx, "rehash:hash"))

	// 测稳定态下 HGET 延迟基线
	start := time.Now()
	for i := 0; i < 1000; i++ {
		rdb.HGet(ctx, "rehash:hash", fmt.Sprintf("f%d", i%200))
	}
	baseline := time.Since(start)
	fmt.Printf("稳定态 1000 次 HGET:  %v  (%.2fμs/op)\n",
		baseline, float64(baseline.Microseconds())/1000)

	// DEBUG RELOAD 强制触发完整 rehash
	rdb.Do(ctx, "DEBUG", "RELOAD")

	// rehash 完成后再测
	start = time.Now()
	for i := 0; i < 1000; i++ {
		rdb.HGet(ctx, "rehash:hash", fmt.Sprintf("f%d", i%200))
	}
	after := time.Since(start)
	fmt.Printf("RELOAD 后 1000 次 HGET: %v  (%.2fμs/op)\n",
		after, float64(after.Microseconds())/1000)
	fmt.Println("结论: rehash 分摊到每次操作,单次开销极小,用户几乎无感知\n")

	rdb.Del(ctx, "rehash:hash")
}

// ExpIntsetMemory 实验9: intset 升级不可逆 + 内存对比
//
// 升级后即使删掉触发升级的大整数,编码和内存也不会降回来。
// 生产里有人以为删掉大整数内存会释放,结果内存一直居高不下。
//
// 预期输出:
//
//	插入小整数:   intset    内存小
//	插入大整数:   intset    内存增加(编码升级 int16→int64)
//	删掉大整数后: intset    内存不降  ← 不可逆
func ExpIntsetMemory(ctx context.Context) {
	fmt.Println("=== 实验9: intset 升级不可逆 + 内存对比 ===")

	rdb.Del(ctx, "exp:intset")

	// 插入 10 个小整数(int16 范围内)
	for i := 1; i <= 10; i++ {
		rdb.SAdd(ctx, "exp:intset", i)
	}
	mem1, _ := rdb.MemoryUsage(ctx, "exp:intset").Result()
	fmt.Printf("10 个小整数:  encoding=%s  memory=%d bytes\n",
		encoding(ctx, "exp:intset"), mem1)

	// 插入一个超出 int16 范围的整数,触发升级到 int64
	rdb.SAdd(ctx, "exp:intset", 99999999999)
	mem2, _ := rdb.MemoryUsage(ctx, "exp:intset").Result()
	fmt.Printf("插入大整数后: encoding=%s  memory=%d bytes  ← 升级到 int64\n",
		encoding(ctx, "exp:intset"), mem2)

	// 删掉大整数
	rdb.SRem(ctx, "exp:intset", 99999999999)
	mem3, _ := rdb.MemoryUsage(ctx, "exp:intset").Result()
	fmt.Printf("删掉大整数后: encoding=%s  memory=%d bytes  ← 编码仍 int64,回不到升级前\n\n",
		encoding(ctx, "exp:intset"), mem3)
}

// ExpListpackVsHashtable 实验10: listpack vs hashtable HGET 性能差异
//
// listpack 是顺序扫描 O(N),hashtable 是 O(1)。
// 字段数少时 listpack 更省内存,字段数多时 hashtable 更快。
//
// 预期输出:
//
//	listpack(10字段)  HGET 均值 < hashtable(200字段)
//	但 listpack 内存占用远小于 hashtable
func ExpListpackVsHashtable(ctx context.Context) {
	fmt.Println("=== 实验10: listpack vs hashtable HGET 性能 ===")

	rdb.Del(ctx, "exp:lp", "exp:ht")

	// listpack: 10 个字段,在阈值内
	for i := 0; i < 10; i++ {
		rdb.HSet(ctx, "exp:lp", fmt.Sprintf("f%d", i), i)
	}
	// hashtable: 200 个字段,超过阈值
	for i := 0; i < 200; i++ {
		rdb.HSet(ctx, "exp:ht", fmt.Sprintf("f%d", i), i)
	}

	memLp, _ := rdb.MemoryUsage(ctx, "exp:lp").Result()
	memHt, _ := rdb.MemoryUsage(ctx, "exp:ht").Result()
	fmt.Printf("listpack  10字段  encoding=%s  memory=%d bytes\n",
		encoding(ctx, "exp:lp"), memLp)
	fmt.Printf("hashtable 200字段 encoding=%s  memory=%d bytes\n",
		encoding(ctx, "exp:ht"), memHt)

	// 各跑 10000 次 HGET
	const n = 10000
	start := time.Now()
	for i := 0; i < n; i++ {
		rdb.HGet(ctx, "exp:lp", fmt.Sprintf("f%d", i%10))
	}
	lpDur := time.Since(start)

	start = time.Now()
	for i := 0; i < n; i++ {
		rdb.HGet(ctx, "exp:ht", fmt.Sprintf("f%d", i%200))
	}
	htDur := time.Since(start)

	fmt.Printf("\nHGET %d 次:\n", n)
	fmt.Printf("  listpack(10):   %v  (%.2fμs/op)\n",
		lpDur, float64(lpDur.Microseconds())/n)
	fmt.Printf("  hashtable(200): %v  (%.2fμs/op)\n",
		htDur, float64(htDur.Microseconds())/n)
	fmt.Println("结论: listpack 字段少时因网络 RTT 主导,差距不大;字段数上去后 O(N) 扫描会拖垮主线程\n")

	rdb.Del(ctx, "exp:lp", "exp:ht")
}

// ExpLargeValue 实验7: 单个 value 过大触发 Hash 编码转换
//
// value 长度超过 hash-max-listpack-value(默认 64B)时,
//
// 不管字段数多少,立即从 listpack 升级到 hashtable。
//
// 预期输出:
//
//	value=64B:  listpack
//	value=65B:  hashtable  ← 转换
func ExpLargeValue(ctx context.Context) {
	fmt.Println("=== Hash: 单个 value 过大触发转换 ===")
	fmt.Println("阈值: hash-max-listpack-value=64")

	rdb.Del(ctx, "exp:hash:val")

	rdb.HSet(ctx, "exp:hash:val", "k", strings.Repeat("x", 64))
	fmt.Printf("value=64B:   %s\n", encoding(ctx, "exp:hash:val"))

	rdb.HSet(ctx, "exp:hash:val", "k", strings.Repeat("x", 65))
	fmt.Printf("value=65B:   %s  ← 转换\n\n", encoding(ctx, "exp:hash:val"))
}
