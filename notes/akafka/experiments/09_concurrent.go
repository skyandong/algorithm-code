// 案例9：消费并发模型 —— 位移提交竞态 vs 滑动窗口
// 演示：多线程并发消费时，offset 提交超前会导致崩溃恢复丢消息；
//
//	正确做法是滑动窗口提交（只提交到连续处理完的位置）。
//
// 对应笔记：notes/akafka/08-消费并发模型.md
// 核心结论：
//   - offset 提交必须"单调且不超前"：只能提交到「连续处理完」的边界
//   - 乱序提交（谁先处理完谁先提交）会让 offset 跳到中间，前面的消息若进程挂了就丢
//   - 滑动窗口：维护已完成的 offset 集合，只推进到最长连续前缀
package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

const topicConcurrent = "concurrent-demo"

// RunConcurrent 运行案例9：位移提交竞态 vs 滑动窗口。
func RunConcurrent() {
	fmt.Println("=== 案例9：位移提交竞态 vs 滑动窗口 ===")
	produceSequential()
	slidingWindowCommit()
	fmt.Println("\n结论：offset 提交必须落在「连续处理完」的边界，滑动窗口才是安全的。")
}

// produceSequential 生产 12 条 seq 递增的消息（单分区，offset 连续）。
func produceSequential() {
	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers),
		kgo.RequiredAcks(kgo.AllISRAcks()),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	ctx := context.Background()
	fmt.Println("\n--- 1. 生产 12 条顺序消息 ---")
	for i := 0; i < 12; i++ {
		client.ProduceSync(ctx, &kgo.Record{
			Topic: topicConcurrent,
			Key:   []byte("single-key"), // 固定 key → 保证进同一分区 → offset 连续
			Value: []byte(fmt.Sprintf(`{"seq":%d}`, i)),
		})
	}
	fmt.Println("  已写入 seq=0..11，单分区 offset 连续")
}

// slidingWindowCommit 消费并模拟并发处理，用滑动窗口提交 offset。
func slidingWindowCommit() {
	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers),
		kgo.ConsumerGroup("concurrent-demo-group"),
		kgo.ConsumeTopics(topicConcurrent),
		kgo.DisableAutoCommit(),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 一次性拉取全部 12 条
	var records []*kgo.Record
	for len(records) < 12 {
		fetches := client.PollFetches(ctx)
		if fetches.Err() != nil {
			return
		}
		fetches.EachRecord(func(r *kgo.Record) {
			records = append(records, r)
		})
	}
	fmt.Println("\n--- 2. 模拟并发处理（处理耗时随机，完成顺序必然乱序）---")

	// 关键状态：滑动窗口提交所需的三件套
	var (
		mu           sync.Mutex
		completed    = make(map[int64]bool) // 已处理完的 offset 集合
		nextToCommit int64                  // 期望提交的下一个 offset（= 连续前缀边界）
		naive        = int64(-1)            // naive 提交会提交到的"最大已完成 offset"（错误示范）
	)

	var wg sync.WaitGroup
	for _, r := range records {
		wg.Add(1)
		go func(r *kgo.Record) {
			defer wg.Done()
			// 随机耗时，故意制造乱序完成
			time.Sleep(time.Duration(rand.Intn(200)) * time.Millisecond)

			mu.Lock()
			completed[r.Offset] = true

			// 错误示范：naive 提交 = 直接提交当前最大的已完成 offset
			if r.Offset > naive {
				naive = r.Offset
			}

			// 正确做法：滑动窗口 —— 只有当 nextToCommit 连续完成才推进
			for completed[nextToCommit] {
				delete(completed, nextToCommit)
				nextToCommit++
			}

			mu.Unlock()
		}(r)
	}
	wg.Wait()

	fmt.Printf("  naive 提交会提交到 offset=%d（最大已完成），但中间可能有空洞\n", naive)
	fmt.Printf("  滑动窗口提交只推进到 offset=%d（最长连续前缀）\n", nextToCommit)
	fmt.Println("  差异：若在乱序处理中进程崩溃，naive 提交会丢中间未完成的消息；")
	fmt.Println("        滑动窗口提交的 offset 之前全部处理完，恢复后从该处继续，不丢不重。")
}
