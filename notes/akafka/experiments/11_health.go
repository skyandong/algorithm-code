// 案例11：集群健康巡检（kadm 一次性只读检查）
// 演示：集群概览（broker/controller）、topic 分区与副本健康、消费者组状态与 lag。
//
// 对应笔记：notes/akafka/09-监控与运维.md
// 核心结论：
//   - 健康巡检三看：broker 是否齐、副本 ISR 是否掉队、消费 lag 是否积压
//   - ISR < Replicas 说明有副本落后或宕机，写入仍可用但容灾能力下降
//   - lag 持续增长 = 消费跟不上生产，需要扩容消费者或排查慢处理
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"text/tabwriter"

	"github.com/twmb/franz-go/pkg/kadm"
)

// RunHealth 运行案例11：集群健康巡检。
func RunHealth() {
	fmt.Println("=== 案例11：集群健康巡检 ===")
	ctx := context.Background()
	adm := newAdm()
	defer adm.Close()

	showClusterOverview(ctx, adm)
	showTopicHealth(ctx, adm)
	showGroupHealth(ctx, adm)

	fmt.Println("\n结论：巡检关注 broker 存活、ISR 掉队、lag 积压三类指标，")
	fmt.Println("      生产上把这些接入 Prometheus/Grafana 自动告警即可。")
}

// showClusterOverview 集群概览：cluster id、controller、broker 列表。
func showClusterOverview(ctx context.Context, adm *kadm.Client) {
	meta, err := adm.Metadata(ctx)
	if err != nil {
		log.Printf("获取元数据失败: %v", err)
		return
	}

	fmt.Println("\n--- 1. 集群概览 ---")
	fmt.Printf("  ClusterID: %s\n", meta.Cluster)
	fmt.Printf("  Controller: broker %d\n", meta.Controller)

	fmt.Println("  Brokers:")
	for _, b := range meta.Brokers {
		rack := ""
		if b.Rack != nil {
			rack = *b.Rack
		}
		fmt.Printf("    node %-3d %s:%d  rack=%s\n", b.NodeID, b.Host, b.Port, rack)
	}
}

// showTopicHealth topic 分区/副本健康：分区数、副本数、ISR 是否掉队。
func showTopicHealth(ctx context.Context, adm *kadm.Client) {
	meta, err := adm.Metadata(ctx)
	if err != nil {
		log.Printf("获取元数据失败: %v", err)
		return
	}

	fmt.Println("\n--- 2. Topic 分区与副本健康 ---")
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "  TOPIC\tPARTITIONS\tREPLICAS\tISR不足分区\t状态")
	fmt.Fprintln(w, "  -----\t----------\t--------\t----------\t----")
	for _, t := range meta.Topics.Sorted() {
		if t.IsInternal {
			continue // 跳过 __consumer_offsets 等内部 topic
		}
		parts := t.Partitions.Sorted()
		if len(parts) == 0 {
			continue
		}
		replicas := t.Partitions.NumReplicas()
		// 统计 ISR < Replicas 的分区（副本掉队）
		underISR := underISRPartitions(parts)
		status := topicStatus(underISR)
		underStr := "-"
		if len(underISR) > 0 {
			underStr = fmt.Sprint(underISR)
		}
		fmt.Fprintf(w, "  %s\t%d\t%d\t%s\t%s\n", t.Topic, len(parts), replicas, underStr, status)
	}
	w.Flush()
}

// underISRPartitions 返回 ISR 副本数少于 Replicas 的分区号（副本掉队或宕机）。
// 单副本 topic 永远不会掉队，因为 Replicas == ISR == [leader]。
func underISRPartitions(parts []kadm.PartitionDetail) []int32 {
	var under []int32
	for _, p := range parts {
		if len(p.ISR) < len(p.Replicas) {
			under = append(under, p.Partition)
		}
	}
	return under
}

// topicStatus 按 ISR 掉队情况给出 topic 健康状态。
func topicStatus(underISR []int32) string {
	if len(underISR) > 0 {
		return "⚠ 副本掉队"
	}
	return "✓ 健康"
}

// showGroupHealth 消费者组状态与 lag。
func showGroupHealth(ctx context.Context, adm *kadm.Client) {
	groups, err := adm.ListGroups(ctx)
	if err != nil {
		log.Printf("获取消费者组失败: %v", err)
		return
	}

	groupList := groups.Sorted()
	if len(groupList) == 0 {
		fmt.Println("\n--- 3. 消费者组 ---")
		fmt.Println("  (当前无活跃消费者组，运行 group/lag/pipeline 等实验后可见)")
		return
	}

	fmt.Println("\n--- 3. 消费者组状态 ---")
	for _, g := range groupList {
		fmt.Printf("  group=%-24s state=%s coordinator=broker %d\n", g.Group, g.State, g.Coordinator)
	}

	// 用 Lag 一次性拉取所有组的积压
	groupIDs := groups.Groups()
	lags, err := adm.Lag(ctx, groupIDs...)
	if err != nil {
		log.Printf("获取 lag 失败: %v", err)
		return
	}

	fmt.Println("\n--- 4. 消费积压（Lag）---")
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "  GROUP\tTOPIC\tPARTITION\tLAG")
	fmt.Fprintln(w, "  -----\t-----\t---------\t---")
	var totalLag int64
	for _, l := range lags.Sorted() {
		for _, m := range l.Lag.Sorted() {
			if m.Lag < 0 {
				continue
			}
			totalLag += m.Lag
			fmt.Fprintf(w, "  %s\t%s\t%d\t%d\n", l.Group, m.Topic, m.Partition, m.Lag)
		}
	}
	w.Flush()
	fmt.Printf("  总积压：%d 条\n", totalLag)
}
