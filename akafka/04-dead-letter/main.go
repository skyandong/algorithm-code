// 案例4：死信队列（Dead Letter Queue）
// 演示：消费失败重试 N 次后转入死信 Topic，死信消费者单独处理
// 场景：订单消息处理失败，超过重试次数后进入死信队列供人工排查
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

const (
	brokers        = "localhost:9092"
	topicOrders    = "orders"
	topicDLQ       = "orders-dlq" // Dead Letter Queue
	groupID        = "order-processor"
	maxRetries     = 3
)

// MessageEnvelope 带重试元数据的消息包装
type MessageEnvelope struct {
	OriginalTopic string    `json:"original_topic"`
	RetryCount    int       `json:"retry_count"`
	LastError     string    `json:"last_error"`
	FirstFailedAt time.Time `json:"first_failed_at"`
	Payload       []byte    `json:"payload"`
}

func main() {
	fmt.Println("=== 案例4：死信队列 ===")

	// 先往 orders topic 发几条测试消息
	produceOrders()
	time.Sleep(500 * time.Millisecond)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 启动普通消费者（会把失败消息转入 DLQ）
	go runOrderConsumer(ctx)

	// 启动 DLQ 消费者（处理死信消息）
	go runDLQConsumer(ctx)

	<-ctx.Done()
	fmt.Println("\n优雅退出")
}

func produceOrders() {
	client, err := kgo.NewClient(kgo.SeedBrokers(brokers), kgo.RequiredAcks(kgo.AllISRAcks()))
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	orders := []string{
		`{"order_id":"O001","user_id":1,"amount":99.9}`,
		`{"order_id":"O002","user_id":2,"amount":invalid}`, // 故意的坏消息
		`{"order_id":"O003","user_id":3,"amount":199.0}`,
		`{"order_id":"O004","user_id":4,"amount":bad_data}`, // 故意的坏消息
		`{"order_id":"O005","user_id":5,"amount":59.0}`,
	}

	records := make([]*kgo.Record, len(orders))
	for i, o := range orders {
		records[i] = &kgo.Record{
			Topic: topicOrders,
			Key:   []byte(fmt.Sprintf("order-%03d", i+1)),
			Value: []byte(o),
		}
	}
	results := client.ProduceSync(context.Background(), records...)
	fmt.Printf("生产了 %d 条订单消息（含 %d 条坏消息）\n\n", len(records), 2)
	_ = results
}

func runOrderConsumer(ctx context.Context) {
	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers),
		kgo.ConsumerGroup(groupID),
		kgo.ConsumeTopics(topicOrders),
		kgo.DisableAutoCommit(),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// DLQ 生产者，用于把失败消息转入死信 topic
	dlqProducer, err := kgo.NewClient(kgo.SeedBrokers(brokers), kgo.RequiredAcks(kgo.AllISRAcks()))
	if err != nil {
		log.Fatal(err)
	}
	defer dlqProducer.Close()

	fmt.Println("[OrderConsumer] 开始消费...")

	for {
		fetches := client.PollFetches(ctx)
		if errors.Is(fetches.Err(), context.Canceled) {
			break
		}

		fetches.EachRecord(func(r *kgo.Record) {
			if err := processWithRetry(ctx, r, dlqProducer); err != nil {
				log.Printf("[OrderConsumer] 消息已进入 DLQ key=%s", r.Key)
			}
		})

		client.CommitUncommittedOffsets(ctx)
	}
}

// processWithRetry 带重试逻辑的处理，超过次数转 DLQ
func processWithRetry(ctx context.Context, r *kgo.Record, dlqProducer *kgo.Client) error {
	var envelope MessageEnvelope

	// 判断是否是从 DLQ 重试回来的（重新投递场景）
	if err := json.Unmarshal(r.Value, &envelope); err != nil || envelope.OriginalTopic == "" {
		// 普通消息，封装成 envelope
		envelope = MessageEnvelope{
			OriginalTopic: r.Topic,
			RetryCount:    0,
			Payload:       r.Value,
		}
	}

	for attempt := envelope.RetryCount; attempt < maxRetries; attempt++ {
		err := processOrder(envelope.Payload)
		if err == nil {
			fmt.Printf("[OrderConsumer] ✓ 处理成功 key=%-12s attempt=%d\n", r.Key, attempt+1)
			return nil
		}

		envelope.RetryCount = attempt + 1
		envelope.LastError = err.Error()
		if attempt == 0 {
			envelope.FirstFailedAt = time.Now()
		}

		fmt.Printf("[OrderConsumer] ✗ 处理失败 key=%-12s attempt=%d err=%v\n", r.Key, attempt+1, err)

		if attempt < maxRetries-1 {
			// 指数退避重试
			backoff := time.Duration(1<<uint(attempt)) * 100 * time.Millisecond
			time.Sleep(backoff)
		}
	}

	// 超过重试次数，转入 DLQ
	return sendToDLQ(ctx, r.Key, &envelope, dlqProducer)
}

func processOrder(payload []byte) error {
	var order map[string]interface{}
	if err := json.Unmarshal(payload, &order); err != nil {
		return fmt.Errorf("JSON 解析失败: %w", err)
	}
	// 模拟随机失败（5% 概率）
	if rand.Float32() < 0.05 {
		return errors.New("下游服务超时")
	}
	return nil
}

func sendToDLQ(ctx context.Context, key []byte, envelope *MessageEnvelope, producer *kgo.Client) error {
	data, _ := json.Marshal(envelope)
	results := producer.ProduceSync(ctx, &kgo.Record{
		Topic: topicDLQ,
		Key:   key,
		Value: data,
		// 用 Header 标记来源，方便 DLQ 消费者路由
		Headers: []kgo.RecordHeader{
			{Key: "x-original-topic", Value: []byte(envelope.OriginalTopic)},
			{Key: "x-retry-count", Value: []byte(fmt.Sprintf("%d", envelope.RetryCount))},
			{Key: "x-last-error", Value: []byte(envelope.LastError)},
		},
	})
	if err := results.FirstErr(); err != nil {
		return fmt.Errorf("写入 DLQ 失败: %w", err)
	}
	fmt.Printf("[OrderConsumer] → DLQ key=%-12s retries=%d reason=%s\n", key, envelope.RetryCount, envelope.LastError)
	return nil
}

func runDLQConsumer(ctx context.Context) {
	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers),
		kgo.ConsumerGroup("dlq-handler"),
		kgo.ConsumeTopics(topicDLQ),
		kgo.DisableAutoCommit(),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	fmt.Println("[DLQConsumer] 监听死信队列...")

	for {
		fetches := client.PollFetches(ctx)
		if errors.Is(fetches.Err(), context.Canceled) {
			break
		}

		fetches.EachRecord(func(r *kgo.Record) {
			var envelope MessageEnvelope
			json.Unmarshal(r.Value, &envelope)

			// 提取 Header 信息
			headers := make(map[string]string)
			for _, h := range r.Headers {
				headers[h.Key] = string(h.Value)
			}

			fmt.Printf("[DLQConsumer] 收到死信 key=%-12s original_topic=%s retries=%s error=%s\n",
				r.Key,
				headers["x-original-topic"],
				headers["x-retry-count"],
				headers["x-last-error"],
			)
			// 工程实践：
			// 1. 告警通知（钉钉/飞书）
			// 2. 写入人工处理系统
			// 3. 条件满足时重新投递到原始 Topic
		})

		client.CommitUncommittedOffsets(ctx)
	}
}
