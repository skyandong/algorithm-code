// 案例3 Exactly-Once 的单元测试
package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/twmb/franz-go/pkg/kgo"
)

// TestIdempotentProduce 验证幂等发送的 txn-001 消息到达 account-debit。
func TestIdempotentProduce(t *testing.T) {
	idempotentProduce()

	missing := consumeKeys(t, topicDebit, []string{"txn-001"})
	assert.Empty(t, missing, "10 秒内未消费到 txn-001")
}

// TestTransactionalProduce 验证事务提交的 txn-100 消息到达全部 3 个 topic。
func TestTransactionalProduce(t *testing.T) {
	transactionalProduce()

	for _, topic := range []string{topicDebit, topicCredit, topicAudit} {
		missing := consumeKeys(t, topic, []string{"txn-100"})
		assert.Empty(t, missing, "topic %s 10 秒内未消费到 txn-100", topic)
	}
}

// TestTransactionalWithAbort 验证回滚的 txn-abort 消息对 read_committed 消费者不可见。
func TestTransactionalWithAbort(t *testing.T) {
	transactionalWithAbort()

	seen := seenKeys(t, topicDebit, 3*time.Second, kgo.FetchIsolationLevel(kgo.ReadCommitted()))
	assert.False(t, seen["txn-abort"], "回滚的消息不应被 read_committed 消费者看到")
}
