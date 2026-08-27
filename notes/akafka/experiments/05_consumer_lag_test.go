// 案例5 消费积压监控的单元测试
package main

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
)

// TestPrintLagReport 验证有积压时 lag 报表正常打印且 lag-demo-group 有已提交 offset。
func TestPrintLagReport(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 生产者 10 条/s，消费者每 2s 处理一批；组加入 + 首次提交约需 4s
	go produceMessages(ctx)
	go slowConsumer(ctx)
	time.Sleep(6 * time.Second)
	cancel()
	time.Sleep(500 * time.Millisecond)

	printLagReport(context.Background())

	client, err := kgo.NewClient(kgo.SeedBrokers(brokers))
	assert.NoError(t, err)
	if err != nil {
		return
	}
	defer client.Close()

	offsets, err := kadm.NewClient(client).FetchOffsets(context.Background(), "lag-demo-group")
	assert.NoError(t, err)
	assert.NotEmpty(t, len(offsets), "lag-demo-group 应有已提交的 offset")
}
