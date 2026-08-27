// 案例5：消费积压监控（Consumer Lag）
// 演示：用 kadm 获取每个 Partition 的 HW 和消费者已提交 Offset，计算积压量
// 工程实践：接入 Prometheus/Grafana 监控告警，积压超阈值触发扩容
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sort"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
)

const (
	pollInterval      = 5 * time.Second
	lagAlertThreshold = 100
)

type LagInfo struct {
	Topic        string
	Partition    int32
	GroupID      string
	CommitOffset int64
	HighWater    int64
	Lag          int64
}

// RunConsumerLag 运行案例5：消费积压监控（Ctrl+C 退出）。
func RunConsumerLag() {
	fmt.Println("=== 案例5：消费积压监控 ===")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go produceMessages(ctx)
	go slowConsumer(ctx)

	time.Sleep(2 * time.Second)

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	fmt.Println("\n开始监控（每5秒刷新），按 Ctrl+C 退出...")
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			printLagReport(ctx)
		}
	}
}

func printLagReport(ctx context.Context) {
	client, err := kgo.NewClient(kgo.SeedBrokers(brokers))
	if err != nil {
		log.Printf("创建客户端失败: %v", err)
		return
	}
	defer client.Close()

	adm := kadm.NewClient(client)

	// 获取所有消费者组
	groups, err := adm.ListGroups(ctx)
	if err != nil {
		log.Printf("获取消费者组失败: %v", err)
		return
	}

	var groupIDs []string
	for _, g := range groups.Sorted() {
		groupIDs = append(groupIDs, g.Group)
	}

	if len(groupIDs) == 0 {
		fmt.Println("暂无消费者组")
		return
	}

	// 用 FetchManyOffsets 批量获取所有组的已提交 offset
	allOffsets := adm.FetchManyOffsets(ctx, groupIDs...)

	// 收集所有 topic 用于查询 HW
	topicSet := make(map[string]struct{})
	for _, r := range allOffsets {
		if r.Err != nil {
			continue
		}
		r.Fetched.Each(func(o kadm.OffsetResponse) {
			if o.Err == nil {
				topicSet[o.Topic] = struct{}{}
			}
		})
	}

	if len(topicSet) == 0 {
		fmt.Println("暂无已提交的 offset")
		return
	}

	topics := make([]string, 0, len(topicSet))
	for t := range topicSet {
		topics = append(topics, t)
	}

	// 获取各 partition 的 HW（最新可消费位置）
	endOffsets, err := adm.ListEndOffsets(ctx, topics...)
	if err != nil {
		log.Printf("获取 end offsets 失败: %v", err)
		return
	}

	// 计算积压
	var lags []LagInfo
	var totalLag int64

	for groupID, r := range allOffsets {
		if r.Err != nil {
			continue
		}
		r.Fetched.Each(func(o kadm.OffsetResponse) {
			if o.Err != nil {
				return
			}
			hw, ok := endOffsets.Lookup(o.Topic, o.Partition)
			if !ok || hw.Err != nil {
				return
			}
			lag := hw.Offset - o.At
			if lag < 0 {
				lag = 0
			}
			totalLag += lag
			lags = append(lags, LagInfo{
				Topic:        o.Topic,
				Partition:    o.Partition,
				GroupID:      groupID,
				CommitOffset: o.At,
				HighWater:    hw.Offset,
				Lag:          lag,
			})
		})
	}

	sort.Slice(lags, func(i, j int) bool {
		return lags[i].Lag > lags[j].Lag
	})

	fmt.Printf("\n[%s] Lag 报告\n", time.Now().Format("15:04:05"))
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "GROUP\tTOPIC\tPARTITION\tCOMMITTED\tHW\tLAG\tSTATUS")
	fmt.Fprintln(w, "-----\t-----\t---------\t---------\t--\t---\t------")
	for _, l := range lags {
		status := "✓ 正常"
		if l.Lag > lagAlertThreshold {
			status = "⚠ 告警"
		}
		fmt.Fprintf(w, "%s\t%s\t%d\t%d\t%d\t%d\t%s\n",
			l.GroupID, l.Topic, l.Partition, l.CommitOffset, l.HighWater, l.Lag, status)
	}
	w.Flush()
	fmt.Printf("总积压：%d 条\n", totalLag)

	// 工程实践：上报 Prometheus
	// gauge.WithLabelValues(group, topic, partition).Set(float64(lag))
}

func produceMessages(ctx context.Context) {
	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers),
		kgo.RequiredAcks(kgo.LeaderAck()),
		// 幂等生产要求 acks=all，这里为吞吐用 acks=1，需显式关闭幂等
		kgo.DisableIdempotentWrite(),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	seq := 0
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			seq++
			client.Produce(ctx, &kgo.Record{
				Topic: topicBasic,
				Key:   []byte(fmt.Sprintf("msg-%05d", seq)),
				Value: []byte(fmt.Sprintf(`{"seq":%d}`, seq)),
			}, nil)
		}
	}
}

func slowConsumer(ctx context.Context) {
	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers),
		kgo.ConsumerGroup("lag-demo-group"),
		kgo.ConsumeTopics(topicBasic),
		kgo.DisableAutoCommit(),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	for {
		fetches := client.PollFetches(ctx)
		if fetches.Err() != nil {
			return
		}
		// 故意处理很慢，制造积压
		time.Sleep(2 * time.Second)
		client.CommitUncommittedOffsets(ctx)
	}
}
