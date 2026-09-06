// 案例10：分区倾斜观测
// 演示：key 均匀分布 → 各分区均衡；热点 key → 消息挤进同一个分区，造成倾斜。
//
// 对应笔记：notes/akafka/07-分区与容量设计.md
// 核心结论：
//   - 分区数决定并行度上限，消息按 hash(key) % 分区数 路由
//   - 一旦业务有热点 key（如大 V、爆款商品），hash 会把它们全塞进同一分区
//   - 倾斜后果：单分区成为吞吐/积压瓶颈，其余分区空转，扩容分区数也救不了
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/twmb/franz-go/pkg/kgo"
)

const (
	topicSkewUniform = "skew-uniform"
	topicSkewHot     = "skew-hot"
)

// RunPartitionSkew 运行案例10：分区倾斜观测。
func RunPartitionSkew() {
	fmt.Println("=== 案例10：分区倾斜观测 ===")
	ctx := context.Background()

	produceUniform(ctx)
	produceHotKey(ctx)

	uniform := countPerPartition(ctx, topicSkewUniform)
	hot := countPerPartition(ctx, topicSkewHot)

	fmt.Println("\n--- 均匀 key 的分布（600 个不同 key）---")
	printDistribution(uniform)
	fmt.Println("\n--- 热点 key 的分布（600 条同 key）---")
	printDistribution(hot)

	fmt.Println("\n结论：热点 key 让所有消息挤进同一个分区，")
	fmt.Println("      该分区成为瓶颈，其余分区空转 —— 加分区数解决不了，要改造 key 或做二次分发。")
}

// produceUniform 发送 600 个不同 key 的消息，hash 后应均匀铺开。
func produceUniform(ctx context.Context) {
	client, err := kgo.NewClient(kgo.SeedBrokers(brokers), kgo.RequiredAcks(kgo.AllISRAcks()))
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	for i := 0; i < 600; i++ {
		client.ProduceSync(ctx, &kgo.Record{
			Topic: topicSkewUniform,
			Key:   []byte(fmt.Sprintf("user-%04d", i)),
			Value: []byte(fmt.Sprintf(`{"user":%d}`, i)),
		})
	}
	fmt.Println("\n已写入 600 条均匀 key 消息到", topicSkewUniform)
}

// produceHotKey 发送 600 条相同 key 的消息，全部路由到同一分区。
func produceHotKey(ctx context.Context) {
	client, err := kgo.NewClient(kgo.SeedBrokers(brokers), kgo.RequiredAcks(kgo.AllISRAcks()))
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	hotKey := "hot-user-1"
	for i := 0; i < 600; i++ {
		client.ProduceSync(ctx, &kgo.Record{
			Topic: topicSkewHot,
			Key:   []byte(hotKey), // 固定 key → 全进同一分区
			Value: []byte(fmt.Sprintf(`{"user":"%s","seq":%d}`, hotKey, i)),
		})
	}
	fmt.Println("已写入 600 条热点 key 消息到", topicSkewHot)
}

// countPerPartition 统计 topic 每个分区的消息量（end offset - start offset）。
func countPerPartition(ctx context.Context, topic string) map[int32]int64 {
	adm := newAdm()
	defer adm.Close()

	startOffsets, err := adm.ListStartOffsets(ctx, topic)
	if err != nil {
		log.Printf("获取 start offsets 失败: %v", err)
		return nil
	}
	endOffsets, err := adm.ListEndOffsets(ctx, topic)
	if err != nil {
		log.Printf("获取 end offsets 失败: %v", err)
		return nil
	}

	counts := make(map[int32]int64)
	for _, p := range sortedPartitions(startOffsets, topic) {
		s, ok1 := startOffsets.Lookup(topic, p)
		e, ok2 := endOffsets.Lookup(topic, p)
		if !ok1 || !ok2 || s.Err != nil || e.Err != nil {
			continue
		}
		counts[p] = e.Offset - s.Offset
	}
	return counts
}

// printDistribution 打印各分区消息量，并高亮倾斜程度。
func printDistribution(counts map[int32]int64) {
	if len(counts) == 0 {
		fmt.Println("  (无数据)")
		return
	}
	var total, max, min int64
	var maxP int32
	min = -1
	for p, c := range counts {
		total += c
		if c > max {
			max, maxP = c, p
		}
		if min == -1 || c < min {
			min = c
		}
	}
	fmt.Println("  PARTITION  消息数  占比")
	for p := int32(0); p < int32(len(counts)+20); p++ {
		c, ok := counts[p]
		if !ok {
			continue
		}
		pct := float64(c) / float64(total) * 100
		bar := ""
		for i := 0; i < int(pct/5); i++ {
			bar += "█"
		}
		flag := ""
		if c == max && max > min && max-min > min {
			flag = "  ⚠ 热点分区"
		}
		fmt.Printf("  %-10d %-6d %5.1f%% %s%s\n", p, c, pct, bar, flag)
	}
	fmt.Printf("  总消息数=%d, 最热分区=%d(%d 条), 最冷分区=%d 条\n", total, maxP, max, min)
}
