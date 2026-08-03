// 案例3：Exactly-Once 语义
// 演示：幂等生产者（单分区去重）、事务生产者（跨分区原子写）
// 场景：转账——从账户A扣款消息和向账户B入账消息必须同时成功或同时失败
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

const (
	brokers     = "localhost:9092"
	topicDebit  = "account-debit"
	topicCredit = "account-credit"
	topicAudit  = "account-audit"
	transactID  = "transfer-service-1"
)

func main() {
	fmt.Println("=== 案例3：Exactly-Once 语义 ===")
	idempotentProduce()
	transactionalProduce()
	transactionalWithAbort()
}

// idempotentProduce 幂等生产者
// 原理：Producer 携带 PID + SequenceNumber，Broker 端去重
// 保证：单分区、单会话内消息不重复（网络重试安全）
func idempotentProduce() {
	fmt.Println("\n--- 幂等生产者 ---")

	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers),
		kgo.RequiredAcks(kgo.AllISRAcks()),
		kgo.ProducerBatchCompression(kgo.SnappyCompression()),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	ctx := context.Background()
	record := &kgo.Record{
		Topic: topicDebit,
		Key:   []byte("txn-001"),
		Value: []byte(`{"txn_id":"txn-001","account":"A","amount":-100,"ts":1234567890}`),
	}

	results := client.ProduceSync(ctx, record)
	if err := results.FirstErr(); err != nil {
		log.Printf("幂等发送失败: %v", err)
		return
	}
	r := results[0].Record
	fmt.Printf("  幂等发送成功 partition=%d offset=%d\n", r.Partition, r.Offset)
}

// transactionalProduce 事务生产者
// 两条消息（扣款+入账）在同一事务中，要么全部提交，要么全部回滚
// Consumer 需要设置 isolation.level=read_committed 才能看到已提交的事务消息
func transactionalProduce() {
	fmt.Println("\n--- 事务生产者（跨 Topic 原子写）---")

	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers),
		kgo.RequiredAcks(kgo.AllISRAcks()),
		// TransactionalID 必须唯一且固定，重启后复用用于 epoch 防止僵尸写
		kgo.TransactionalID(transactID),
		kgo.RecordDeliveryTimeout(15*time.Second),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	ctx := context.Background()

	type Transfer struct {
		TxnID   string
		FromAcc string
		ToAcc   string
		Amount  int
	}
	transfer := Transfer{"txn-100", "account-A", "account-B", 100}

	if err := client.BeginTransaction(); err != nil {
		log.Fatalf("开始事务失败: %v", err)
	}
	fmt.Printf("  开始事务 txn_id=%s\n", transfer.TxnID)

	debitMsg := &kgo.Record{
		Topic: topicDebit,
		Key:   []byte(transfer.TxnID),
		Value: []byte(fmt.Sprintf(`{"txn_id":"%s","account":"%s","delta":-%d}`, transfer.TxnID, transfer.FromAcc, transfer.Amount)),
	}
	creditMsg := &kgo.Record{
		Topic: topicCredit,
		Key:   []byte(transfer.TxnID),
		Value: []byte(fmt.Sprintf(`{"txn_id":"%s","account":"%s","delta":+%d}`, transfer.TxnID, transfer.ToAcc, transfer.Amount)),
	}
	auditMsg := &kgo.Record{
		Topic: topicAudit,
		Key:   []byte(transfer.TxnID),
		Value: []byte(fmt.Sprintf(`{"txn_id":"%s","from":"%s","to":"%s","amount":%d,"status":"committed"}`,
			transfer.TxnID, transfer.FromAcc, transfer.ToAcc, transfer.Amount)),
	}

	results := client.ProduceSync(ctx, debitMsg, creditMsg, auditMsg)
	if err := results.FirstErr(); err != nil {
		// 发送失败，回滚事务
		client.EndTransaction(ctx, kgo.TryAbort)
		log.Fatalf("事务内发送失败，已回滚: %v", err)
	}

	if err := client.EndTransaction(ctx, kgo.TryCommit); err != nil {
		log.Fatalf("提交事务失败: %v", err)
	}

	for _, res := range results {
		fmt.Printf("  事务消息已提交 topic=%-20s partition=%d offset=%d\n",
			res.Record.Topic, res.Record.Partition, res.Record.Offset)
	}
}

// transactionalWithAbort 演示事务回滚
func transactionalWithAbort() {
	fmt.Println("\n--- 事务回滚演示 ---")

	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers),
		kgo.RequiredAcks(kgo.AllISRAcks()),
		kgo.TransactionalID(transactID+"-abort-demo"),
		kgo.RecordDeliveryTimeout(15*time.Second),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	ctx := context.Background()

	if err := client.BeginTransaction(); err != nil {
		log.Fatalf("开始事务失败: %v", err)
	}

	client.ProduceSync(ctx, &kgo.Record{
		Topic: topicDebit,
		Key:   []byte("txn-abort"),
		Value: []byte(`{"txn_id":"txn-abort","will_be_rolled_back":true}`),
	})

	fmt.Println("  业务校验失败，回滚事务...")
	if err := client.EndTransaction(ctx, kgo.TryAbort); err != nil {
		log.Printf("回滚失败: %v", err)
		return
	}
	fmt.Println("  事务已回滚，消息对消费者不可见")
}
