// 案例9 位移提交竞态 vs 滑动窗口的单元测试
//
// 核心是 advanceWindow 这个纯函数：提交点只能落在「连续处理完」的边界上，
// 遇到第一个空洞必须停住 —— 这是不丢消息的关键。
package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestAdvanceWindow 滑动窗口推进的各种情形。
func TestAdvanceWindow(t *testing.T) {
	// 0/1/2 已完成，3 是空洞 → 只能推进到 3
	completed := map[int64]bool{0: true, 1: true, 2: true, 4: true}
	assert.Equal(t, int64(3), advanceWindow(completed, 0))
	assert.False(t, completed[0], "已提交的 offset 应移出窗口")
	assert.True(t, completed[4], "空洞之后的完成项必须留在窗口里等空洞补上")

	// 起点本身就是空洞 → 原地不动
	assert.Equal(t, int64(5), advanceWindow(map[int64]bool{9: true}, 5))

	// 空窗口
	assert.Equal(t, int64(0), advanceWindow(map[int64]bool{}, 0))

	// 全部连续 → 一路推到底
	assert.Equal(t, int64(3), advanceWindow(map[int64]bool{0: true, 1: true, 2: true}, 0))

	// 乱序补齐：先到 4/5，最后补上 3 → 一次推进到 6
	completed = map[int64]bool{4: true, 5: true}
	assert.Equal(t, int64(3), advanceWindow(completed, 3), "空洞未补时不能前进")
	completed[3] = true
	assert.Equal(t, int64(6), advanceWindow(completed, 3), "空洞补上后应一次推进到 6")
}

// TestSlidingWindowCommit 端到端跑一遍消费者组 + 滑动窗口提交。
func TestSlidingWindowCommit(t *testing.T) {
	requireKafka(t)

	produceSequential()

	out := captureStdout(t, slidingWindowCommit)
	assert.Contains(t, out, "naive 提交会提交到 offset=")
	assert.Contains(t, out, "滑动窗口提交只推进到 offset=")
}
