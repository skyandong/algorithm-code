// 案例8：存储与读路径（Segment 文件 / offset 定位）
// 演示：start/end offset、按时间戳定位 offset（对应稀疏索引 + 二分查找）、日志段大小观测
//
// 对应笔记：notes/akafka/05-存储与读路径.md
// 核心结论：
//   - 一个分区在磁盘上是一组顺序写的 Segment 文件（.log/.index/.timeindex）
//   - 按 offset 定位 = 先通过 .index 稀疏索引二分找到最近的相对位置，再顺序扫描
//   - 按时间戳定位 = 通过 .timeindex 找到时间戳对应的 offset，再走 offset 定位路径
package main

import (
	"context"
	"fmt"
	"log"
	"sort"
	"time"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
)

const topicStorage = "storage-demo"

// RunStorage 运行案例8：存储与读路径。
func RunStorage() {
	fmt.Println("=== 案例8：存储与读路径 ===")
	ctx := context.Background()

	produceTimestamped(ctx)
	time.Sleep(500 * time.Millisecond) // 等消息落盘
	showStartEndOffsets(ctx)
	locateByTimestamp(ctx)
	showLogSegmentSize(ctx)

	fmt.Println("\n结论：分区顺序写 = 一组 Segment 文件；")
	fmt.Println("      按 offset 读靠稀疏索引二分定位，按时间戳读靠 .timeindex 转成 offset 再定位。")
}

// produceTimestamped 发送一批带业务时间戳的消息，便于演示"按时间找 offset"。
func produceTimestamped(ctx context.Context) {
	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers),
		kgo.RequiredAcks(kgo.AllISRAcks()),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	fmt.Println("\n--- 1. 生产带时间戳的消息 ---")
	for i := 0; i < 12; i++ {
		// 人为让消息时间戳分布在过去 60 秒内，方便后面按时间戳定位
		ts := time.Now().Add(-time.Duration(60-i*5) * time.Second)
		client.ProduceSync(ctx, &kgo.Record{
			Topic:     topicStorage,
			Key:       []byte(fmt.Sprintf("key-%02d", i)),
			Value:     []byte(fmt.Sprintf(`{"seq":%d,"note":"storage demo"}`, i)),
			Timestamp: ts,
		})
	}
	fmt.Println("  已写入 12 条消息，时间戳从 60 秒前均匀分布到当前")
}

// showStartEndOffsets 展示每个分区的 start offset 和 end offset（HW）。
// start offset 由 log.retention/compact 决定，end offset 是下一条消息的写入位置。
func showStartEndOffsets(ctx context.Context) {
	adm := newAdm()
	defer adm.Close()

	startOffsets, err := adm.ListStartOffsets(ctx, topicStorage)
	if err != nil {
		log.Printf("获取 start offsets 失败: %v", err)
		return
	}
	endOffsets, err := adm.ListEndOffsets(ctx, topicStorage)
	if err != nil {
		log.Printf("获取 end offsets 失败: %v", err)
		return
	}

	fmt.Println("\n--- 2. 每个分区的 start / end offset ---")
	fmt.Println("  PARTITION  START  END   消息数")
	for p := int32(0); p < 6; p++ {
		s, ok1 := startOffsets.Lookup(topicStorage, p)
		e, ok2 := endOffsets.Lookup(topicStorage, p)
		if !ok1 || !ok2 || s.Err != nil || e.Err != nil {
			continue
		}
		fmt.Printf("  %-9d %-5d %-5d %d\n", p, s.Offset, e.Offset, e.Offset-s.Offset)
	}
	fmt.Println("  (start offset 由 retention/compact 决定；end offset 即 HW)")
}

// locateByTimestamp 按时间戳定位 offset：ListOffsetsAfterMilli。
// 底层走的就是 .timeindex（时间戳 → 偏移）+ .index（稀疏索引二分）这条路。
func locateByTimestamp(ctx context.Context) {
	adm := newAdm()
	defer adm.Close()

	// 定位"30 秒前"那条消息在每分区的 offset
	target := time.Now().Add(-30 * time.Second).UnixMilli()
	offsets, err := adm.ListOffsetsAfterMilli(ctx, target, topicStorage)
	if err != nil {
		log.Printf("按时间戳定位失败: %v", err)
		return
	}

	fmt.Println("\n--- 3. 按时间戳定位 offset ---")
	fmt.Printf("  目标时间戳 = 30 秒前 (%d)\n", target)
	for p := int32(0); p < 6; p++ {
		o, ok := offsets.Lookup(topicStorage, p)
		if !ok || o.Err != nil {
			continue
		}
		ts := time.UnixMilli(o.Timestamp).Format("15:04:05")
		fmt.Printf("  partition=%d → offset=%d (该位置消息时间戳 %s)\n", p, o.Offset, ts)
	}
	fmt.Println("  这是 read_committed 消费者做「时间点快照」「回溯消费」的底层能力")
}

// showLogSegmentSize 观察分区在 broker 磁盘上的日志段大小。
// 注意：DescribeLogDirs 只暴露分区级 Size；.log/.index/.timeindex 的文件明细
//
//	需要到 broker 机器看磁盘目录或走 JMX 指标，代码侧只能看到总量。
func showLogSegmentSize(ctx context.Context) {
	adm := newAdm()
	defer adm.Close()

	var set kadm.TopicsSet
	set.Add(topicStorage) // 只加 topic 不加分区 = 查该 topic 所有分区

	allDirs, err := adm.DescribeAllLogDirs(ctx, set)
	if err != nil {
		log.Printf("查询日志目录失败: %v", err)
		return
	}

	fmt.Println("\n--- 4. 各 broker 上该 topic 的日志段大小 ---")
	fmt.Println("  BROKER  PARTITION  日志段大小(bytes)")
	// DescribedAllLogDirs: map[brokerID]DescribedLogDirs（每 broker 一个 log dir）
	for brokerID, dirs := range allDirs {
		for _, p := range dirs.SortedPartitions() {
			if p.Topic != topicStorage {
				continue
			}
			fmt.Printf("  %-7d %-10d %d\n", brokerID, p.Partition, p.Size)
		}
	}
	fmt.Println("  这些字节即 .log/.index/.timeindex 等 segment 文件的总和")
}

// newAdm 创建一个绑定 kadm 的临时客户端，供只读巡检复用。
func newAdm() *kadm.Client {
	client, err := kgo.NewClient(kgo.SeedBrokers(brokers))
	if err != nil {
		log.Fatalf("创建客户端失败: %v", err)
	}
	return kadm.NewClient(client)
}

// sortedPartitions 返回升序的 partition 列表（工具函数，供巡检类实验复用）。
func sortedPartitions(offsets kadm.ListedOffsets, topic string) []int32 {
	var ps []int32
	if m, ok := offsets[topic]; ok {
		for p := range m {
			ps = append(ps, p)
		}
	}
	sort.Slice(ps, func(i, j int) bool { return ps[i] < ps[j] })
	return ps
}
