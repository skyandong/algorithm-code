// 案例8 存储与读路径的单元测试
//
// 纯逻辑部分：sortedPartitions（分区号升序）
// 集成部分：写入带时间戳的消息 → end offset 增长 → 按时间戳能定位到 [start, end] 之间的 offset
package main

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/twmb/franz-go/pkg/kadm"
)

// TestSortedPartitions 分区号升序，topic 不存在时返回空。
func TestSortedPartitions(t *testing.T) {
	offsets := kadm.ListedOffsets{
		topicStorage: {2: {}, 0: {}, 1: {}},
	}
	assert.Equal(t, []int32{0, 1, 2}, sortedPartitions(offsets, topicStorage))
	assert.Empty(t, sortedPartitions(offsets, "topic-not-exist"))
	assert.Empty(t, sortedPartitions(kadm.ListedOffsets{}, topicStorage))
}

// TestProduceTimestamped 写入 12 条带时间戳的消息后，分区总消息数应增加。
func TestProduceTimestamped(t *testing.T) {
	requireKafka(t)

	ctx := context.Background()
	before := sumCounts(countPerPartition(ctx, topicStorage))

	produceTimestamped(ctx)
	time.Sleep(500 * time.Millisecond) // 等消息可见

	after := sumCounts(countPerPartition(ctx, topicStorage))
	assert.Greater(t, after, before, "%s 的总消息数应增加", topicStorage)
}

// TestLocateByTimestamp 按时间戳定位的 offset 必须落在该分区的 [start, end] 区间内。
func TestLocateByTimestamp(t *testing.T) {
	requireKafka(t)

	ctx := context.Background()
	produceTimestamped(ctx)
	time.Sleep(500 * time.Millisecond)

	adm := newAdm()
	defer adm.Close()

	startOffsets, err := adm.ListStartOffsets(ctx, topicStorage)
	assert.NoError(t, err)
	endOffsets, err := adm.ListEndOffsets(ctx, topicStorage)
	assert.NoError(t, err)

	// 案例里写入的时间戳覆盖过去 60 秒，取 30 秒前一定能命中
	target := time.Now().Add(-30 * time.Second).UnixMilli()
	located, err := adm.ListOffsetsAfterMilli(ctx, target, topicStorage)
	assert.NoError(t, err)

	var checked int
	for _, p := range sortedPartitions(startOffsets, topicStorage) {
		s, ok1 := startOffsets.Lookup(topicStorage, p)
		e, ok2 := endOffsets.Lookup(topicStorage, p)
		if !ok1 || !ok2 || s.Err != nil || e.Err != nil {
			continue
		}
		o, ok := located.Lookup(topicStorage, p)
		if !ok || o.Err != nil {
			continue
		}
		assert.GreaterOrEqual(t, o.Offset, s.Offset, "partition=%d 定位结果不该早于 start offset", p)
		assert.LessOrEqual(t, o.Offset, e.Offset, "partition=%d 定位结果不该超过 end offset", p)
		checked++
	}
	assert.Positive(t, checked, "至少应校验一个分区")
}

// TestShowStartEndOffsets 输出 start / end offset 表头。
func TestShowStartEndOffsets(t *testing.T) {
	requireKafka(t)

	out := captureStdout(t, func() { showStartEndOffsets(context.Background()) })
	assert.Contains(t, out, "PARTITION")
	assert.Contains(t, out, "START")
}

// TestShowLogSegmentSize 输出各 broker 上分区的日志段大小表头。
func TestShowLogSegmentSize(t *testing.T) {
	requireKafka(t)

	out := captureStdout(t, func() { showLogSegmentSize(context.Background()) })
	assert.Contains(t, out, "日志段大小")
}
