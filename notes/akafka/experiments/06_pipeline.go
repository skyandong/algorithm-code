// 案例6：多 Topic 流水线（Event-Driven Pipeline）
// 场景：电商下单流程 orders → payment-requests → notifications
//   1. 下单服务生产订单事件到 orders topic
//   2. 支付服务消费 orders，处理支付，产出支付结果到 payment-requests
//   3. 通知服务消费 payment-requests，发送用户通知到 notifications
//   4. 审计服务消费 notifications，记录全链路日志
// 演示：消费-转换-生产的流水线模式，每个服务是独立的消费者组
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

// --- 事件定义 ---

type OrderEvent struct {
	OrderID  string  `json:"order_id"`
	UserID   int     `json:"user_id"`
	Amount   float64 `json:"amount"`
	Item     string  `json:"item"`
	CreateAt int64   `json:"created_at"`
}

type PaymentEvent struct {
	OrderID   string  `json:"order_id"`
	UserID    int     `json:"user_id"`
	Amount    float64 `json:"amount"`
	Status    string  `json:"status"` // success / failed
	PaymentID string  `json:"payment_id"`
	PaidAt    int64   `json:"paid_at"`
}

type NotificationEvent struct {
	UserID    int    `json:"user_id"`
	OrderID   string `json:"order_id"`
	Channel   string `json:"channel"` // sms / push / email
	Message   string `json:"message"`
	CreatedAt int64  `json:"created_at"`
}

// RunPipeline 运行案例6：多 Topic 流水线（Ctrl+C 退出）。
func RunPipeline() {
	fmt.Println("=== 案例6：多 Topic 流水线 ===")
	fmt.Println("流程：下单 → 支付 → 通知 → 审计")
	fmt.Println()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 启动各个服务
	go orderService(ctx)
	go paymentService(ctx)
	go notificationService(ctx)
	go auditService(ctx)

	<-ctx.Done()
	time.Sleep(500 * time.Millisecond)
	fmt.Println("\n所有服务已停止")
}

// orderService 下单服务：模拟用户下单，生产订单事件
func orderService(ctx context.Context) {
	client, err := kgo.NewClient(kgo.SeedBrokers(brokers), kgo.RequiredAcks(kgo.AllISRAcks()))
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	items := []string{"iPhone 16", "MacBook Pro", "AirPods", "iPad", "Apple Watch"}
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	seq := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			seq++
			order := OrderEvent{
				OrderID:  fmt.Sprintf("ORD-%06d", seq),
				UserID:   rand.Intn(1000) + 1,
				Amount:   float64(rand.Intn(10000)+100) / 100.0,
				Item:     items[rand.Intn(len(items))],
				CreateAt: time.Now().UnixMilli(),
			}
			data, _ := json.Marshal(order)
			results := client.ProduceSync(ctx, &kgo.Record{
				Topic: "orders",
				Key:   []byte(order.OrderID),
				Value: data,
			})
			if results.FirstErr() == nil {
				fmt.Printf("[下单服务] 新订单 order_id=%-12s user=%d item=%s amount=%.2f\n",
					order.OrderID, order.UserID, order.Item, order.Amount)
			}
		}
	}
}

// paymentService 支付服务：消费订单事件，处理支付，生产支付结果
func paymentService(ctx context.Context) {
	consumer, err := kgo.NewClient(
		kgo.SeedBrokers(brokers),
		kgo.ConsumerGroup("payment-service"),
		kgo.ConsumeTopics("orders"),
		kgo.DisableAutoCommit(),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer consumer.Close()

	producer, err := kgo.NewClient(kgo.SeedBrokers(brokers), kgo.RequiredAcks(kgo.AllISRAcks()))
	if err != nil {
		log.Fatal(err)
	}
	defer producer.Close()

	for {
		fetches := consumer.PollFetches(ctx)
		if errors.Is(fetches.Err(), context.Canceled) {
			return
		}

		fetches.EachRecord(func(r *kgo.Record) {
			var order OrderEvent
			if err := json.Unmarshal(r.Value, &order); err != nil {
				return
			}

			// 模拟支付处理（90% 成功率）
			time.Sleep(200 * time.Millisecond)
			status := "success"
			if rand.Float32() < 0.1 {
				status = "failed"
			}

			payment := PaymentEvent{
				OrderID:   order.OrderID,
				UserID:    order.UserID,
				Amount:    order.Amount,
				Status:    status,
				PaymentID: fmt.Sprintf("PAY-%s", order.OrderID),
				PaidAt:    time.Now().UnixMilli(),
			}
			data, _ := json.Marshal(payment)

			// 生产支付结果（消费-处理-生产是原子的，可以用事务保证）
			results := producer.ProduceSync(ctx, &kgo.Record{
				Topic: "payment-requests",
				Key:   []byte(payment.OrderID),
				Value: data,
			})
			if results.FirstErr() == nil {
				icon := "✓"
				if status == "failed" {
					icon = "✗"
				}
				fmt.Printf("[支付服务]  %s 支付%s order_id=%-12s payment_id=%s\n",
					icon, status, payment.OrderID, payment.PaymentID)
			}
		})

		consumer.CommitUncommittedOffsets(ctx)
	}
}

// notificationService 通知服务：消费支付结果，发送用户通知
func notificationService(ctx context.Context) {
	consumer, err := kgo.NewClient(
		kgo.SeedBrokers(brokers),
		kgo.ConsumerGroup("notification-service"),
		kgo.ConsumeTopics("payment-requests"),
		kgo.DisableAutoCommit(),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer consumer.Close()

	producer, err := kgo.NewClient(kgo.SeedBrokers(brokers), kgo.RequiredAcks(kgo.AllISRAcks()))
	if err != nil {
		log.Fatal(err)
	}
	defer producer.Close()

	channels := []string{"sms", "push", "email"}

	for {
		fetches := consumer.PollFetches(ctx)
		if errors.Is(fetches.Err(), context.Canceled) {
			return
		}

		fetches.EachRecord(func(r *kgo.Record) {
			var payment PaymentEvent
			if err := json.Unmarshal(r.Value, &payment); err != nil {
				return
			}

			var msg string
			if payment.Status == "success" {
				msg = fmt.Sprintf("您的订单 %s 支付成功，金额 %.2f 元", payment.OrderID, payment.Amount)
			} else {
				msg = fmt.Sprintf("您的订单 %s 支付失败，请重试", payment.OrderID)
			}

			notification := NotificationEvent{
				UserID:    payment.UserID,
				OrderID:   payment.OrderID,
				Channel:   channels[rand.Intn(len(channels))],
				Message:   msg,
				CreatedAt: time.Now().UnixMilli(),
			}
			data, _ := json.Marshal(notification)

			results := producer.ProduceSync(ctx, &kgo.Record{
				Topic: "notifications",
				Key:   []byte(payment.OrderID),
				Value: data,
			})
			if results.FirstErr() == nil {
				fmt.Printf("[通知服务]  📨 [%s] user=%d order_id=%-12s msg=%s\n",
					notification.Channel, notification.UserID, notification.OrderID, notification.Message)
			}
		})

		consumer.CommitUncommittedOffsets(ctx)
	}
}

// auditService 审计服务：消费通知事件，记录全链路审计日志
func auditService(ctx context.Context) {
	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers),
		kgo.ConsumerGroup("audit-service"),
		kgo.ConsumeTopics("notifications"),
		kgo.DisableAutoCommit(),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	for {
		fetches := client.PollFetches(ctx)
		if errors.Is(fetches.Err(), context.Canceled) {
			return
		}

		fetches.EachRecord(func(r *kgo.Record) {
			var n NotificationEvent
			if err := json.Unmarshal(r.Value, &n); err != nil {
				return
			}
			fmt.Printf("[审计服务]  📋 order_id=%-12s user=%d channel=%s 全链路完成\n",
				n.OrderID, n.UserID, n.Channel)
			// 工程实践：写入 ES / ClickHouse 做全链路追踪
		})

		client.CommitUncommittedOffsets(ctx)
	}
}
