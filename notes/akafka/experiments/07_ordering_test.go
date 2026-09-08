// 案例7 顺序性保证的单元测试
//
// 纯逻辑部分（parseStatus / statusIndex / countOrderViolations / sortedKeys）不依赖 Kafka；
// 集成部分验证「同 key 必进同一分区」和「按 key 分发后顺序恢复」。
package main

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/twmb/franz-go/pkg/kgo"
)

// TestParseStatus 从消息体里取 status 字段。
func TestParseStatus(t *testing.T) {
	assert.Equal(t, "PAID", parseStatus([]byte(`{"order_id":"ORD-1","status":"PAID"}`)))
	assert.Equal(t, "COMPLETED", parseStatus([]byte(`{"order_id":"x","status":"COMPLETED"}`)))
	assert.Equal(t, "", parseStatus([]byte(`{"order_id":"x"}`)), "没有 status 字段应返回空串")
	assert.Equal(t, "", parseStatus(nil))
}

// TestStatusIndex 状态机下标：已知状态递增，未知状态 -1。
func TestStatusIndex(t *testing.T) {
	for i, s := range orderStatuses {
		assert.Equal(t, i, statusIndex(s))
	}
	assert.Equal(t, -1, statusIndex("UNKNOWN"))
}

// TestCountOrderViolations 统计同 key 的状态倒退次数。
func TestCountOrderViolations(t *testing.T) {
	// 严格按 CREATED → COMPLETED 推进，0 处错乱
	assert.Equal(t, 0, countOrderViolations([]string{
		"ORD-1/CREATED", "ORD-1/PAID", "ORD-1/SHIPPED", "ORD-1/DELIVERED", "ORD-1/COMPLETED",
	}))

	// COMPLETED 跑到 CREATED 前面 → 1 处错乱
	assert.Equal(t, 1, countOrderViolations([]string{"ORD-1/COMPLETED", "ORD-1/CREATED"}))

	// 不同 key 之间互不影响：并发只打乱同 key 内部
	assert.Equal(t, 0, countOrderViolations([]string{"ORD-2/PAID", "ORD-1/CREATED"}))

	// 两个 key 各自倒退一次
	assert.Equal(t, 2, countOrderViolations([]string{
		"ORD-1/DELIVERED", "ORD-1/CREATED",
		"ORD-2/COMPLETED", "ORD-2/PAID",
	}))
}

// TestSortedKeys 分区号升序输出，保证打印稳定。
func TestSortedKeys(t *testing.T) {
	assert.Equal(t, []int32{0, 2, 5}, sortedKeys(map[int32]bool{5: true, 0: true, 2: true}))
	assert.Empty(t, sortedKeys(map[int32]bool{}))
}

// TestKeyRoutingDemo 同 key 的所有状态事件必须落在同一分区。
func TestKeyRoutingDemo(t *testing.T) {
	requireKafka(t)

	keyRoutingDemo() // 发送 3 个订单 × 5 个状态

	records := fetchAll(context.Background(), topicOrdering, 15)
	assert.NotEmpty(t, records, "fetchAll 应能读到 order-events 的历史消息")

	partitionOf := make(map[string]map[int32]bool)
	for _, r := range records {
		key := string(r.Key)
		if key == "" {
			continue // 跳过无 key 的实验消息
		}
		if partitionOf[key] == nil {
			partitionOf[key] = make(map[int32]bool)
		}
		partitionOf[key][r.Partition] = true
	}

	assert.NotEmpty(t, partitionOf, "应读到带 key 的订单事件")
	for key, parts := range partitionOf {
		assert.Len(t, parts, 1, "key=%s 的消息必须全部落在同一分区，实际落在 %v", key, parts)
	}
}

// TestNoKeyRoutingDemo 无 key 时消息被分散（粘性分区），输出里有分区分布。
func TestNoKeyRoutingDemo(t *testing.T) {
	requireKafka(t)

	out := captureStdout(t, noKeyRoutingDemo)
	assert.Contains(t, out, "无 key 消息在各分区的分布")
}

// TestFetchAll 从最早 offset 回放，能读到刚写入的消息。
func TestFetchAll(t *testing.T) {
	requireKafka(t)

	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers),
		kgo.RequiredAcks(kgo.AllISRAcks()),
		kgo.RecordDeliveryTimeout(10*time.Second),
	)
	assert.NoError(t, err)
	if err != nil {
		return
	}
	defer client.Close()

	// 用独立 key 避免和其它用例混在一起
	key := fmt.Sprintf("fetch-all-%d", time.Now().UnixNano())
	res := client.ProduceSync(context.Background(), &kgo.Record{
		Topic: topicOrdering,
		Key:   []byte(key),
		Value: []byte(`{"order_id":"` + key + `","status":"CREATED"}`),
	})
	assert.NoError(t, res.FirstErr())

	var found bool
	for _, r := range fetchAll(context.Background(), topicOrdering, 30) {
		if string(r.Key) == key {
			found = true
			assert.Equal(t, topicOrdering, r.Topic)
			assert.GreaterOrEqual(t, r.Partition, int32(0))
			assert.GreaterOrEqual(t, r.Offset, int64(0))
			break
		}
	}
	assert.True(t, found, "fetchAll 应能读到刚写入的 key=%s", key)
}

// TestConcurrentOutOfOrderDemo 并发处理同 key 会打乱顺序（输出里有错乱统计）。
func TestConcurrentOutOfOrderDemo(t *testing.T) {
	requireKafka(t)

	out := captureStdout(t, concurrentOutOfOrderDemo)
	assert.Contains(t, out, "顺序错乱的状态数")
}

// TestDispatchByKey 按 key 分发到固定 worker 后，同一 key 严格按状态机推进。
// 用构造的消息而不是 topic 里的历史数据：topic 会跨轮次累积，同 key 会重复出现，
// 那时"倒退"是数据造成的，不是分发策略的问题。
func TestDispatchByKey(t *testing.T) {
	var records []*kgo.Record
	for _, orderID := range []string{"ORD-1001", "ORD-1002", "ORD-1003"} {
		for _, status := range orderStatuses {
			records = append(records, &kgo.Record{
				Key:   []byte(orderID),
				Value: []byte(fmt.Sprintf(`{"order_id":"%s","status":"%s"}`, orderID, status)),
			})
		}
	}

	finished := dispatchByKey(records)
	assert.Len(t, finished, 15)
	assert.Equal(t, 0, countOrderViolations(finished),
		"同 key 必须严格保序，实际完成顺序：%v", finished)
}

// TestKeyDispatchInOrderDemo 端到端跑一遍（含从 topic 回放），只做冒烟断言。
func TestKeyDispatchInOrderDemo(t *testing.T) {
	requireKafka(t)

	out := captureStdout(t, keyDispatchInOrderDemo)
	assert.Contains(t, out, "按 key 分发到固定 worker")
}
