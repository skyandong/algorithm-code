// 案例4 死信队列的单元测试
package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/twmb/franz-go/pkg/kgo"
)

// TestProduceOrders 验证 produceOrders 的 5 条订单消息全部到达 orders。
func TestProduceOrders(t *testing.T) {
	requireKafka(t)

	produceOrders()

	want := []string{"order-001", "order-002", "order-003", "order-004", "order-005"}
	missing := consumeKeys(t, topicOrders, want)
	assert.Empty(t, missing, "10 秒内未消费到全部订单")
}

// TestProcessOrder 验证非法 JSON 一定处理失败。
func TestProcessOrder(t *testing.T) {
	assert.Error(t, processOrder([]byte(`{"amount":invalid}`)), "非法 JSON 必须被拒绝并返回错误")
}

// TestMessageEnvelopeRoundTrip 死信信封序列化后要能原样还原（重试次数、原始 topic、原始载荷）。
// 重新投递全靠这些元数据，少一个字段死信就回不到原 topic。
func TestMessageEnvelopeRoundTrip(t *testing.T) {
	env := MessageEnvelope{
		OriginalTopic: topicOrders,
		RetryCount:    maxRetries,
		LastError:     "JSON 解析失败",
		FirstFailedAt: time.Now().UTC().Truncate(time.Second),
		Payload:       []byte(`{"order_id":"O001"}`),
	}

	data, err := json.Marshal(env)
	assert.NoError(t, err, "信封序列化不应出错")

	var got MessageEnvelope
	assert.NoError(t, json.Unmarshal(data, &got), "信封反序列化不应出错")
	assert.Equal(t, env.OriginalTopic, got.OriginalTopic, "死信回投依赖原始 topic 原样还原")
	assert.Equal(t, env.RetryCount, got.RetryCount, "重试计数需还原以决定再投策略")
	assert.Equal(t, env.LastError, got.LastError, "最后错误需还原便于排查死信原因")
	assert.Equal(t, env.Payload, got.Payload, "业务原始载荷需完整还原")
	assert.True(t, env.FirstFailedAt.Equal(got.FirstFailedAt), "首次失败时间需精确还原以便排障")
}

// TestProcessWithRetry 验证坏消息重试 3 次后转入 DLQ。
func TestProcessWithRetry(t *testing.T) {
	requireKafka(t)

	dlqProducer, err := kgo.NewClient(kgo.SeedBrokers(brokers), kgo.RequiredAcks(kgo.AllISRAcks()))
	assert.NoError(t, err, "创建 DLQ 生产者不应出错")
	if err != nil {
		return
	}
	defer dlqProducer.Close()

	r := &kgo.Record{Topic: topicOrders, Key: []byte("order-bad-001"), Value: []byte(`{"amount":invalid}`)}
	assert.NoError(t, processWithRetry(context.Background(), r, dlqProducer), "坏消息重试后应成功转入 DLQ")

	missing := consumeKeys(t, topicDLQ, []string{"order-bad-001"})
	assert.Empty(t, missing, "10 秒内未消费到 DLQ 消息")
}
