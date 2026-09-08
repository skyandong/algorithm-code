// 案例11 集群健康巡检的单元测试
//
// 纯逻辑部分：ISR 掉队判定、topic 健康状态
// 集成部分：三个巡检函数连上集群后能正常输出（kadm 只读调用）
package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/twmb/franz-go/pkg/kadm"
)

// TestUnderISRPartitions 只有 ISR < Replicas 的分区才算掉队。
func TestUnderISRPartitions(t *testing.T) {
	parts := []kadm.PartitionDetail{
		{Partition: 0, Replicas: []int32{1, 2}, ISR: []int32{1, 2}}, // 健康
		{Partition: 1, Replicas: []int32{1, 2}, ISR: []int32{1}},    // 掉队
		{Partition: 2, Replicas: []int32{1}, ISR: []int32{1}},       // 单副本，永远健康
	}
	assert.Equal(t, []int32{1}, underISRPartitions(parts))

	assert.Empty(t, underISRPartitions(nil))
	assert.Empty(t, underISRPartitions([]kadm.PartitionDetail{
		{Partition: 0, Replicas: []int32{1, 2, 3}, ISR: []int32{1, 2, 3}},
	}))
}

// TestTopicStatus 有掉队分区才报警。
func TestTopicStatus(t *testing.T) {
	assert.Equal(t, "✓ 健康", topicStatus(nil))
	assert.Equal(t, "✓ 健康", topicStatus([]int32{}))
	assert.Equal(t, "⚠ 副本掉队", topicStatus([]int32{1, 2}))
}

// TestShowClusterOverview 集群概览输出 cluster id / controller / broker 列表。
func TestShowClusterOverview(t *testing.T) {
	requireKafka(t)

	adm := newAdm()
	defer adm.Close()

	out := captureStdout(t, func() { showClusterOverview(context.Background(), adm) })
	assert.Contains(t, out, "ClusterID")
	assert.Contains(t, out, "Controller")
	assert.Contains(t, out, "Brokers")
}

// TestShowTopicHealth topic 健康表输出表头（且跳过内部 topic）。
func TestShowTopicHealth(t *testing.T) {
	requireKafka(t)

	adm := newAdm()
	defer adm.Close()

	out := captureStdout(t, func() { showTopicHealth(context.Background(), adm) })
	assert.Contains(t, out, "TOPIC")
	assert.Contains(t, out, "PARTITIONS")
	assert.Contains(t, out, "ISR不足分区")
	assert.NotContains(t, out, "__consumer_offsets", "内部 topic 不应出现在巡检表里")
}

// TestShowGroupHealth 消费者组巡检：有组就打状态，没组也给提示，不能 panic。
func TestShowGroupHealth(t *testing.T) {
	requireKafka(t)

	adm := newAdm()
	defer adm.Close()

	out := captureStdout(t, func() { showGroupHealth(context.Background(), adm) })
	assert.Contains(t, out, "消费者组")
}
