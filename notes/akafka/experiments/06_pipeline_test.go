// 案例6 多 Topic 流水线的单元测试
package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestPipelineServices 验证流水线运行后 orders 中出现订单事件。
func TestPipelineServices(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go orderService(ctx)
	go paymentService(ctx)
	go notificationService(ctx)
	go auditService(ctx)

	time.Sleep(4 * time.Second)
	cancel()
	time.Sleep(500 * time.Millisecond)

	seen := seenKeys(t, "orders", 3*time.Second)
	found := false
	for k := range seen {
		if strings.HasPrefix(k, "ORD-") {
			found = true
			break
		}
	}
	assert.True(t, found, "orders 中应存在流水线产生的订单事件")
}
