// 案例2：消费者组
// 演示：手动提交 offset、并发处理、优雅关闭、Rebalance 感知
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

const (
	topicBasic2 = topicBasic // 复用共享 Topic（demo-basic）
	groupIDDemo = "demo-group-1"
	batchSize   = 10 // 每次最多拉取并处理的消息数
)

// RunConsumerGroup 运行案例2：消费者组（Ctrl+C 退出）。
func RunConsumerGroup() {
	fmt.Println("=== 案例2：消费者组 ===")
	fmt.Printf("消费 topic=%s group=%s\n\n", topicBasic2, groupIDDemo)

	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers),
		kgo.ConsumerGroup(groupIDDemo),
		kgo.ConsumeTopics(topicBasic2),
		// 关闭自动提交，由业务代码手动控制 offset
		kgo.DisableAutoCommit(),
		// 从最早的消息开始消费（首次加入组时）
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
		// Rebalance 感知回调
		kgo.OnPartitionsAssigned(func(ctx context.Context, c *kgo.Client, assigned map[string][]int32) {
			for t, partitions := range assigned {
				fmt.Printf("[Rebalance] 分配到 topic=%s partitions=%v\n", t, partitions)
			}
		}),
		kgo.OnPartitionsRevoked(func(ctx context.Context, c *kgo.Client, revoked map[string][]int32) {
			// 撤销前必须提交当前已处理的 offset，否则下次会重复消费
			if err := c.CommitUncommittedOffsets(ctx); err != nil {
				log.Printf("[Rebalance] 提交 offset 失败: %v", err)
			}
			for t, partitions := range revoked {
				fmt.Printf("[Rebalance] 撤销 topic=%s partitions=%v\n", t, partitions)
			}
		}),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// 监听系统信号，实现优雅关闭
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fmt.Println("开始消费，按 Ctrl+C 优雅退出...")
	consume(ctx, client)
	fmt.Println("\n消费者已优雅退出")
}

func consume(ctx context.Context, client *kgo.Client) {
	var totalProcessed int

	for {
		// PollFetches 阻塞直到有消息或 ctx 取消
		fetches := client.PollFetches(ctx)

		if errors.Is(fetches.Err(), context.Canceled) {
			break
		}
		if errs := fetches.Errors(); len(errs) > 0 {
			for _, e := range errs {
				log.Printf("拉取错误 topic=%s partition=%d: %v", e.Topic, e.Partition, e.Err)
			}
			continue
		}

		// 遍历本次拉取的所有消息
		fetches.EachRecord(func(r *kgo.Record) {
			// 模拟业务处理
			if err := processRecord(r); err != nil {
				// 工程实践：处理失败不提交此 offset，下次会重新消费
				// 或写入死信队列（见案例4）
				log.Printf("处理失败 key=%s: %v", r.Key, err)
				return
			}
			totalProcessed++
			fmt.Printf("  [消费] partition=%d offset=%-6d key=%-20s val=%s\n",
				r.Partition, r.Offset, r.Key, r.Value)
		})

		// 手动提交：批量提交本次拉取中所有已处理消息的 offset
		// 比逐条提交性能高很多
		if err := client.CommitUncommittedOffsets(ctx); err != nil {
			if !errors.Is(err, context.Canceled) {
				log.Printf("提交 offset 失败: %v", err)
			}
		}

		fmt.Printf("  [进度] 累计消费 %d 条\n", totalProcessed)
		time.Sleep(200 * time.Millisecond) // 模拟处理间隔
	}
}

func processRecord(r *kgo.Record) error {
	// 模拟业务逻辑：解析 JSON、写 DB 等
	// 这里只做简单校验
	if len(r.Value) == 0 {
		return fmt.Errorf("空消息体")
	}
	return nil
}
