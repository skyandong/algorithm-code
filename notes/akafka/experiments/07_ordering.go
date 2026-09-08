// 案例7：顺序性保证
// 演示：同 key 路由落到同一分区、消费端并发破坏顺序、按 key 分发恢复顺序
//
// 对应笔记：notes/akafka/04-顺序性.md
// 核心结论：Kafka 只保证分区内有序；端到端有序 = 同 key 同分区 + 消费端对这个 key 串行处理
package main

import (
	"context"
	"errors"
	"fmt"
	"hash/crc32"
	"log"
	"sync"
	"time"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
)

const (
	topicOrdering  = "order-events"
	orderStatusNum = 5 // 每个订单的状态事件数
)

// orderStatuses 订单状态机：必须按此顺序处理，否则状态机会错乱
// 例如"已签收"先于"已支付"被处理，业务上就是错的
var orderStatuses = []string{"CREATED", "PAID", "SHIPPED", "DELIVERED", "COMPLETED"}

// processCost 模拟不同状态的处理耗时（毫秒）
// 故意让靠后的状态更耗时，这样并发处理时顺序必然被打乱，便于观察
var processCost = map[string]time.Duration{
	"CREATED":   10 * time.Millisecond,
	"PAID":      20 * time.Millisecond,
	"SHIPPED":   40 * time.Millisecond,
	"DELIVERED": 80 * time.Millisecond,
	"COMPLETED": 5 * time.Millisecond,
}

// RunOrdering 运行案例7：顺序性保证。
func RunOrdering() {
	fmt.Println("=== 案例7：顺序性保证 ===")
	keyRoutingDemo()
	noKeyRoutingDemo()
	concurrentOutOfOrderDemo()
	keyDispatchInOrderDemo()
	fmt.Println("\n结论：分区内有序只是写入侧的顺序；消费端一旦并发，")
	fmt.Println("      同一 key 的处理顺序就没了 —— 必须按 key 分发到固定 worker。")
}

// keyRoutingDemo 验证：相同 key 的消息全部落到同一分区。
func keyRoutingDemo() {
	fmt.Println("\n--- 1. 指定 key：同 key 必进同一分区 ---")
	fmt.Println("发送 3 个订单 × 5 个状态事件，key = 订单号")

	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers),
		kgo.RequiredAcks(kgo.AllISRAcks()),
		kgo.RecordDeliveryTimeout(10*time.Second),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	ctx := context.Background()
	for _, orderID := range []string{"ORD-1001", "ORD-1002", "ORD-1003"} {
		for _, status := range orderStatuses {
			client.ProduceSync(ctx, &kgo.Record{
				Topic: topicOrdering,
				Key:   []byte(orderID),
				Value: []byte(fmt.Sprintf(`{"order_id":"%s","status":"%s"}`, orderID, status)),
			})
		}
	}

	// 从最早的 offset 读回，观察每个 key 落在哪些分区
	records := fetchAll(ctx, topicOrdering, 15)
	partitionOf := make(map[string]map[int32]bool)
	for _, r := range records {
		orderID := string(r.Key)
		if partitionOf[orderID] == nil {
			partitionOf[orderID] = make(map[int32]bool)
		}
		partitionOf[orderID][r.Partition] = true
	}

	for _, orderID := range []string{"ORD-1001", "ORD-1002", "ORD-1003"} {
		parts := partitionOf[orderID]
		fmt.Printf("  key=%s → 落在 %d 个分区 %v", orderID, len(parts), sortedKeys(parts))
		if len(parts) == 1 {
			fmt.Println("  ✓ 单一分区，写入顺序有保障")
		} else {
			fmt.Println("  ✗ 跨分区，顺序无法保证")
		}
	}
}

// noKeyRoutingDemo 对比：不指定 key 时消息被分散，无顺序可言。
func noKeyRoutingDemo() {
	fmt.Println("\n--- 2. 不指定 key：粘性分区，消息被分散 ---")

	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers),
		kgo.RequiredAcks(kgo.AllISRAcks()),
		kgo.RecordDeliveryTimeout(10*time.Second),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	ctx := context.Background()
	for i := 0; i < 15; i++ {
		client.ProduceSync(ctx, &kgo.Record{
			Topic: topicOrdering,
			// Key 为 nil：franz-go 默认 StickyKeyPartitioner 会粘住一个分区攒批，
			// 批满后再换下一个 —— 目的是提高吞吐，代价是完全没有 key 维度的顺序保证
			Value: []byte(fmt.Sprintf(`{"seq":%d,"note":"no-key"}`, i)),
		})
	}

	records := fetchAll(ctx, topicOrdering, 15)
	dist := make(map[int32]int)
	for _, r := range records {
		dist[r.Partition]++
	}
	fmt.Printf("  15 条无 key 消息在各分区的分布：%v\n", dist)
	fmt.Println("  ✗ 消息分散在多个分区，跨分区无任何顺序保证")
}

// concurrentOutOfOrderDemo 复现：多线程并发处理同一 key，顺序被打乱。
func concurrentOutOfOrderDemo() {
	fmt.Println("\n--- 3. 消费端并发处理：同一 key 顺序被打乱 ---")
	fmt.Println("模拟：每条消息丢进 goroutine 池并发处理（处理耗时按状态递增）")

	ctx := context.Background()
	records := fetchAll(ctx, topicOrdering, 15)

	var (
		mu       sync.Mutex
		wg       sync.WaitGroup
		finished []string // 实际完成顺序
	)

	for _, r := range records {
		if string(r.Key) == "" {
			continue // 跳过无 key 的消息
		}
		wg.Add(1)
		go func(r *kgo.Record) {
			defer wg.Done()
			status := parseStatus(r.Value)
			time.Sleep(processCost[status]) // 模拟业务处理
			mu.Lock()
			finished = append(finished, fmt.Sprintf("%s/%s", r.Key, status))
			mu.Unlock()
		}(r)
	}
	wg.Wait()

	fmt.Println("  实际完成顺序：")
	for i, f := range finished {
		fmt.Printf("    %2d. %s\n", i+1, f)
	}
	fmt.Println("  ✗ 可以看到 COMPLETED/DELIVERED 跑到 CREATED 前面去了")

	broken := countOrderViolations(finished)
	fmt.Printf("  顺序错乱的状态数：%d\n", broken)
}

// dispatchWorkerNum 进程内 worker 数：与分区数无关，只是消费端并行度。
const dispatchWorkerNum = 4

// keyDispatchInOrderDemo 正确做法：按 key 哈希分发到固定 worker，同 key 串行。
func keyDispatchInOrderDemo() {
	fmt.Println("\n--- 4. 按 key 分发到固定 worker：同 key 串行，顺序恢复 ---")

	records := fetchAll(context.Background(), topicOrdering, 15)
	finished := dispatchByKey(records)

	fmt.Println("  实际完成顺序：")
	for i, f := range finished {
		fmt.Printf("    %2d. %s\n", i+1, f)
	}

	broken := countOrderViolations(finished)
	if broken == 0 {
		fmt.Println("  ✓ 每个订单的 5 个状态严格按 CREATED → PAID → SHIPPED → DELIVERED → COMPLETED 推进")
	} else {
		fmt.Printf("  ✗ 仍有 %d 处错乱\n", broken)
	}
	fmt.Printf("\n  注意：worker 数 %d 与分区数无关 —— 这是进程内并行，不影响分区内顺序\n", dispatchWorkerNum)
}

// dispatchByKey 按 hash(key) % workerNum 分发到固定 worker，每个 worker 串行处理自己的队列，
// 返回实际完成顺序（形如 "ORD-1001/PAID"）。
//
// 抽成纯函数是为了能脱离 Kafka 单测：顺序保证来自分发策略本身，与消息从哪来无关。
func dispatchByKey(records []*kgo.Record) []string {
	type job struct {
		key    string
		status string
	}
	// 每个 worker 一个带缓冲的队列，同一 key 只进一个队列
	queues := make([]chan job, dispatchWorkerNum)
	for i := range queues {
		queues[i] = make(chan job, 64)
	}

	var (
		mu       sync.Mutex
		wg       sync.WaitGroup
		finished []string
	)

	// 启动 worker：每个 worker 串行消费自己的队列
	for i := 0; i < dispatchWorkerNum; i++ {
		wg.Add(1)
		go func(q chan job) {
			defer wg.Done()
			for j := range q {
				time.Sleep(processCost[j.status])
				mu.Lock()
				finished = append(finished, fmt.Sprintf("%s/%s", j.key, j.status))
				mu.Unlock()
			}
		}(queues[i])
	}

	// 分发：hash(key) % workerNum，保证同 key 一定进同一个 worker
	for _, r := range records {
		if string(r.Key) == "" {
			continue
		}
		key := string(r.Key)
		idx := int(crc32.ChecksumIEEE([]byte(key))) % dispatchWorkerNum
		queues[idx] <- job{key: key, status: parseStatus(r.Value)}
	}
	for i := range queues {
		close(queues[i])
	}
	wg.Wait()

	return finished
}

// countOrderViolations 统计顺序错乱：对每个 key，检查状态推进是否违反了状态机下标顺序。
func countOrderViolations(finished []string) int {
	lastIdx := make(map[string]int)
	var broken int
	for _, f := range finished {
		// f 形如 "ORD-1001/PAID"
		for i := 0; i < len(f); i++ {
			if f[i] == '/' {
				key := f[:i]
				status := f[i+1:]
				idx := statusIndex(status)
				if prev, ok := lastIdx[key]; ok && idx < prev {
					broken++
				}
				lastIdx[key] = idx
				break
			}
		}
	}
	return broken
}

func statusIndex(status string) int {
	for i, s := range orderStatuses {
		if s == status {
			return i
		}
	}
	return -1
}

// parseStatus 从消息体 {"order_id":"x","status":"PAID"} 中取出 status 字段。
func parseStatus(value []byte) string {
	const key = `"status":"`
	s := string(value)
	for i := 0; i+len(key) <= len(s); i++ {
		if s[i:i+len(key)] == key {
			rest := s[i+len(key):]
			for j := 0; j < len(rest); j++ {
				if rest[j] == '"' {
					return rest[:j]
				}
			}
		}
	}
	return ""
}

// fetchAll 从 topic 的最早 offset 开始读取，最多取 limit 条后返回。
// 用 ConsumePartitions 直连分区消费（不加入消费者组），适合这类"回放+观察"的实验。
func fetchAll(ctx context.Context, topic string, limit int) []*kgo.Record {
	// 先查该 topic 有哪些分区
	admClient, err := kgo.NewClient(kgo.SeedBrokers(brokers))
	if err != nil {
		log.Printf("创建客户端失败: %v", err)
		return nil
	}
	defer admClient.Close()

	adm := kadm.NewClient(admClient)
	meta, err := adm.Metadata(ctx, topic)
	if err != nil {
		log.Printf("获取元数据失败: %v", err)
		return nil
	}
	detail, ok := meta.Topics[topic]
	if !ok {
		log.Printf("topic %s 不存在，请先执行 make topics", topic)
		return nil
	}

	offsets := make(map[string]map[int32]kgo.Offset)
	offsets[topic] = make(map[int32]kgo.Offset)
	for _, p := range detail.Partitions {
		offsets[topic][p.Partition] = kgo.NewOffset().AtStart()
	}

	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers),
		kgo.ConsumePartitions(offsets),
		kgo.FetchMaxWait(2*time.Second),
		kgo.FetchMinBytes(1),
	)
	if err != nil {
		log.Printf("创建消费者失败: %v", err)
		return nil
	}
	defer client.Close()

	// 最多等 6 秒，够把所有历史消息拉完
	readCtx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()

	var records []*kgo.Record
	total := len(detail.Partitions) * orderStatusNum
	want := limit
	if want > total*3 {
		want = total * 3
	}

	for len(records) < want {
		fetches := client.PollFetches(readCtx)
		if errors.Is(fetches.Err(), context.DeadlineExceeded) || errors.Is(fetches.Err(), context.Canceled) {
			break
		}
		if fetches.Err() != nil {
			log.Printf("拉取失败: %v", fetches.Err())
			break
		}
		fetches.EachRecord(func(r *kgo.Record) {
			records = append(records, r)
		})
		if readCtx.Err() != nil {
			break
		}
	}
	return records
}

// sortedKeys 把 map 的 key 排序后输出，保证打印结果稳定。
func sortedKeys(m map[int32]bool) []int32 {
	var keys []int32
	for k := range m {
		keys = append(keys, k)
	}
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}
