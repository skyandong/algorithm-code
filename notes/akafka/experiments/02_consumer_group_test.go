// 案例2 消费者组的单元测试
package main

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/twmb/franz-go/pkg/kgo"
)

// TestProcessRecord 验证空消息体报错、正常消息体通过。
func TestProcessRecord(t *testing.T) {
	assert.Error(t, processRecord(&kgo.Record{}))
	assert.NoError(t, processRecord(&kgo.Record{Value: []byte(`{"id":1}`)}))
}

// TestConsume 验证消费者能消费 demo-basic 且 ctx 取消后正常退出。
func TestConsume(t *testing.T) {
	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers),
		kgo.ConsumerGroup(groupIDDemo),
		kgo.ConsumeTopics(topicBasic2),
		kgo.DisableAutoCommit(),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
	)
	assert.NoError(t, err)
	if err != nil {
		return
	}
	defer client.Close()

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(2*time.Second, cancel)

	done := make(chan struct{})
	go func() {
		consume(ctx, client)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Error("consume 未在 ctx 取消后退出")
	}
}
