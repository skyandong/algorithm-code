// 案例1 生产者的单元测试（syncProduce / asyncProduce / batchProduce）
// 共享的消费验证辅助函数（consumeKeys / seenKeys）也定义在此文件。
//
// 依赖本地 Kafka（notes/akafka 下执行 make up 启动）。
//
// 运行：
//
//	go test ./experiments/ -v -timeout 120s
package main

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/twmb/franz-go/pkg/kgo"
)

// consumeKeys 消费 topic，10s 内集齐 want 中的 key，返回缺失的。
func consumeKeys(t *testing.T, topic string, want []string) []string {
	t.Helper()

	cl, err := kgo.NewClient(
		kgo.SeedBrokers(brokers),
		kgo.ConsumeTopics(topic),
	)
	assert.NoError(t, err)
	if err != nil {
		return want
	}
	defer cl.Close()

	got := make(map[string]bool)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for ctx.Err() == nil {
		for _, r := range cl.PollRecords(ctx, 100).Records() {
			got[string(r.Key)] = true
		}
		allFound := true
		for _, k := range want {
			if !got[k] {
				allFound = false
				break
			}
		}
		if allFound {
			break
		}
	}

	var missing []string
	for _, k := range want {
		if !got[k] {
			missing = append(missing, k)
		}
	}
	return missing
}

// seenKeys 消费 topic 直到 d 超时，返回期间见过的所有 key。
func seenKeys(t *testing.T, topic string, d time.Duration, opts ...kgo.Opt) map[string]bool {
	t.Helper()

	cl, err := kgo.NewClient(append([]kgo.Opt{
		kgo.SeedBrokers(brokers),
		kgo.ConsumeTopics(topic),
	}, opts...)...)
	assert.NoError(t, err)
	if err != nil {
		return nil
	}
	defer cl.Close()

	keys := make(map[string]bool)
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()

	for ctx.Err() == nil {
		for _, r := range cl.PollRecords(ctx, 100).Records() {
			keys[string(r.Key)] = true
		}
	}
	return keys
}

// TestSyncProduce 验证同步发送的 5 条 order-* 消息全部到达 broker。
func TestSyncProduce(t *testing.T) {
	syncProduce()

	want := []string{"order-0", "order-1", "order-2", "order-3", "order-4"}
	missing := consumeKeys(t, topicBasic, want)
	assert.Empty(t, missing, "10 秒内未消费到全部消息")
}

// TestAsyncProduce 验证异步发送的 10 条 event-* 消息全部到达 broker。
func TestAsyncProduce(t *testing.T) {
	asyncProduce()

	want := make([]string, 10)
	for i := range want {
		want[i] = fmt.Sprintf("event-%d", i)
	}
	missing := consumeKeys(t, topicBasic, want)
	assert.Empty(t, missing, "10 秒内未消费到全部消息")
}

// TestBatchProduce 验证批量发送的 20 条 batch-key-* 消息全部到达 broker。
func TestBatchProduce(t *testing.T) {
	batchProduce()

	want := make([]string, 20)
	for i := range want {
		want[i] = fmt.Sprintf("batch-key-%02d", i)
	}
	missing := consumeKeys(t, topicBasic, want)
	assert.Empty(t, missing, "10 秒内未消费到全部消息")
}
