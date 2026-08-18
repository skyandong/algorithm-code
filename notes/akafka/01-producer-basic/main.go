// 案例1：生产者基础
// 演示：同步发送、异步发送、批量发送、acks 配置、发送失败回调
package main

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

const (
	brokers = "localhost:9092"
	topic   = "demo-basic"
)

func main() {
	fmt.Println("=== 案例1：生产者基础 ===")
	syncProduce()
	asyncProduce()
	batchProduce()
}

// syncProduce 同步发送：每条消息等待 broker 确认后再继续
// 适合：对可靠性要求极高、不在意吞吐的场景（如审计日志）
func syncProduce() {
	fmt.Println("\n--- 同步发送 ---")

	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers),
		// acks=all：Leader + 所有 ISR Follower 落盘才返回
		kgo.RequiredAcks(kgo.AllISRAcks()),
		kgo.RecordDeliveryTimeout(10*time.Second),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	ctx := context.Background()
	for i := 0; i < 5; i++ {
		record := &kgo.Record{
			Topic: topic,
			Key:   []byte("order-" + strconv.Itoa(i)),
			Value: []byte(fmt.Sprintf(`{"id":%d,"event":"created","ts":%d}`, i, time.Now().UnixMilli())),
		}
		// ProduceSync 阻塞直到收到 broker 的 ack
		results := client.ProduceSync(ctx, record)
		if err := results.FirstErr(); err != nil {
			log.Printf("发送失败 key=%s: %v", record.Key, err)
			continue
		}
		r := results[0].Record
		fmt.Printf("  已确认 key=%-12s partition=%d offset=%d\n", r.Key, r.Partition, r.Offset)
	}
}

// asyncProduce 异步发送：发出后立即返回，通过回调处理结果
// 适合：高吞吐场景，业务逻辑不依赖发送结果的即时确认
func asyncProduce() {
	fmt.Println("\n--- 异步发送（回调处理结果）---")

	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers),
		kgo.RequiredAcks(kgo.AllISRAcks()),
		// 批量等待时间：最多等 10ms 凑一批，减少网络请求次数
		kgo.ProducerLinger(10*time.Millisecond),
		// 单批最大字节数
		kgo.ProducerBatchMaxBytes(1<<20), // 1MB
	)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	var (
		wg      sync.WaitGroup
		success atomic.Int64
		failure atomic.Int64
	)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		record := &kgo.Record{
			Topic: topic,
			Key:   []byte("event-" + strconv.Itoa(i)),
			Value: []byte(fmt.Sprintf(`{"seq":%d,"data":"async-demo"}`, i)),
		}
		// Produce 立即返回，结果通过 callback 通知
		client.Produce(context.Background(), record, func(r *kgo.Record, err error) {
			defer wg.Done()
			if err != nil {
				failure.Add(1)
				log.Printf("  [失败] key=%s err=%v", r.Key, err)
				// 工程实践：失败写入死信队列或告警
				return
			}
			success.Add(1)
		})
	}

	wg.Wait()
	fmt.Printf("  异步发送完成：成功=%d 失败=%d\n", success.Load(), failure.Load())
}

// batchProduce 批量发送：一次提交多条消息，franz-go 自动合批
// 工程实践：攒够一批或超过时间阈值就发送，大幅提升吞吐
func batchProduce() {
	fmt.Println("\n--- 批量发送 ---")

	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers),
		kgo.RequiredAcks(kgo.AllISRAcks()),
		kgo.ProducerLinger(50*time.Millisecond),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	ctx := context.Background()
	records := make([]*kgo.Record, 20)
	for i := range records {
		records[i] = &kgo.Record{
			Topic: topic,
			Key:   []byte(fmt.Sprintf("batch-key-%02d", i)),
			Value: []byte(fmt.Sprintf(`{"batch_seq":%d}`, i)),
		}
	}

	start := time.Now()
	results := client.ProduceSync(ctx, records...)
	elapsed := time.Since(start)

	var errCount int
	for _, res := range results {
		if res.Err != nil {
			errCount++
		}
	}
	fmt.Printf("  批量发送 %d 条，耗时 %v，失败 %d 条\n", len(records), elapsed, errCount)
}
