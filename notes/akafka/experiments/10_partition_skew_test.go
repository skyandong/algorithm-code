// 案例10 分区倾斜观测的单元测试
//
// 纯逻辑部分：printDistribution 的倾斜判定与输出
// 集成部分：600 个均匀 key 铺满 6 个分区；600 条热点 key 全挤进 1 个分区
package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestPrintDistributionEmpty 空分布要给出友好提示而不是 panic。
func TestPrintDistributionEmpty(t *testing.T) {
	out := captureStdout(t, func() { printDistribution(nil) })
	assert.Contains(t, out, "(无数据)")
}

// TestPrintDistributionHotKey 某分区独占全部消息时应打出热点标记。
func TestPrintDistributionHotKey(t *testing.T) {
	counts := map[int32]int64{0: 600, 1: 0, 2: 0}
	out := captureStdout(t, func() { printDistribution(counts) })
	assert.Contains(t, out, "总消息数=600")
	assert.Contains(t, out, "热点分区")
}

// TestPrintDistributionUniform 均匀分布不该出现热点标记。
func TestPrintDistributionUniform(t *testing.T) {
	counts := map[int32]int64{0: 100, 1: 100, 2: 100, 3: 100, 4: 100, 5: 100}
	out := captureStdout(t, func() { printDistribution(counts) })
	assert.Contains(t, out, "总消息数=600")
	assert.NotContains(t, out, "热点分区")
}

// TestCountPerPartitionUniform 均匀 key 应铺满 6 个分区且不严重倾斜。
func TestCountPerPartitionUniform(t *testing.T) {
	requireKafka(t)

	ctx := context.Background()
	produceUniform(ctx)

	counts := countPerPartition(ctx, topicSkewUniform)
	assert.Len(t, counts, 6, "%s 应有 6 个分区", topicSkewUniform)
	for p, c := range counts {
		assert.Positive(t, c, "分区 %d 不应为空（均匀 key 应铺满所有分区）", p)
	}
	assert.LessOrEqual(t, maxCount(counts), minCount(counts)*2+10,
		"均匀 key 不该严重倾斜: %v", counts)
}

// TestCountPerPartitionHotKey 热点 key 的全部消息都挤在同一个分区。
func TestCountPerPartitionHotKey(t *testing.T) {
	requireKafka(t)

	ctx := context.Background()
	produceHotKey(ctx)

	counts := countPerPartition(ctx, topicSkewHot)
	assert.Len(t, counts, 6)

	total := sumCounts(counts)
	assert.Positive(t, total)
	assert.Equal(t, total, maxCount(counts),
		"热点 key 的消息应全在同一个分区，加分区数也救不了: %v", counts)
}
