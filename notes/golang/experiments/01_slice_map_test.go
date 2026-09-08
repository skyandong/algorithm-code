// 实验 01（slice 与 map）的单元测试
//
// 纯逻辑部分：append 的副本头语义（不依赖任何输出）；
// 冒烟部分：完整跑一遍，断言关键结论输出。
package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestAppendInside 函数内 append 只改副本头的 len，调用方 len 不变，
// 但数据已写进共享底层数组（s[:1] 能偷看到）。
func TestAppendInside(t *testing.T) {
	s := make([]int, 0, 4)
	appendInside(s)

	assert.Len(t, s, 0, "调用方 len 不应变化")
	assert.Equal(t, []int{7}, s[:1], "数据其实写进了共享数组")
}

// TestAppendAndReturn 正确姿势：返回新 header。
func TestAppendAndReturn(t *testing.T) {
	s := make([]int, 0, 4)
	got := appendAndReturn(s)

	assert.Len(t, got, 1)
	assert.Equal(t, []int{7}, got)
}

// TestSliceMapSmoke 全量冒烟：跑完 slicemap 实验，断言各节关键结论。
func TestSliceMapSmoke(t *testing.T) {
	requireDemoRun(t)

	out := captureStdout(t, RunSliceMapExperiments)
	assert.Contains(t, out, "共享底层数组")
	assert.Contains(t, out, "部分删除", "map 部分删除不缩容的实测结论必须出现")
	assert.Contains(t, out, "删空", "Swiss 实现删空重置释放的版本分界必须出现")
	assert.Contains(t, out, "读多写少", "sync.Map 选型结论必须出现")
	assert.Contains(t, out, "panic: runtime error: index out of range", "a[len(a)] 越界反面教材必须演示")
	assert.Contains(t, out, "null", "nil 切片 JSON 序列化差异必须出现")
}
