// 实验 04（泛型）的单元测试
//
// 纯逻辑部分：泛型工具函数与 Stack[T]；
// 冒烟部分：完整跑一遍断言关键输出。
package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestMax 有序类型取大。
func TestMax(t *testing.T) {
	assert.Equal(t, 7, Max(3, 7))
	assert.Equal(t, 2.5, Max(2.5, 1.5))
	assert.Equal(t, "b", Max("a", "b"))
}

// TestMap 双类型参数映射。
func TestMap(t *testing.T) {
	assert.Equal(t, []string{"1", "2", "3"}, Map([]int{1, 2, 3}, func(x int) string {
		return map[int]string{1: "1", 2: "2", 3: "3"}[x]
	}))
	assert.Empty(t, Map([]int{}, func(x int) int { return x }))
}

// TestFirstOrZero 空切片返回零值。
func TestFirstOrZero(t *testing.T) {
	assert.Zero(t, FirstOrZero([]int{}))
	assert.Equal(t, "a", FirstOrZero([]string{"a"}))
}

// TestSumTilde ~int 近似约束：UserID（底层 int）也能进。
func TestSumTilde(t *testing.T) {
	type UserID int
	assert.Equal(t, UserID(6), SumTilde([]UserID{1, 2, 3}))
	assert.Equal(t, 6, SumTilde([]int{1, 2, 3}))
}

// TestKeys comparable 约束提取 key。
func TestKeys(t *testing.T) {
	got := Keys(map[string]int{"a": 1, "b": 2})
	assert.Len(t, got, 2)
	assert.ElementsMatch(t, []string{"a", "b"}, got)
}

// TestStack 泛型栈：Push/Pop/Len 与空栈语义。
func TestStack(t *testing.T) {
	s := &Stack[int]{}

	_, ok := s.Pop()
	assert.False(t, ok, "空栈 Pop 应返回 false")

	s.Push(1)
	s.Push(2)
	v, ok := s.Pop()
	assert.True(t, ok)
	assert.Equal(t, 2, v)
	assert.Equal(t, 1, s.Len())

	doubled := StackMap(s, func(x int) int { return x * 2 })
	assert.Equal(t, []int{2}, doubled.items, "StackMap 不应修改原栈")
	assert.Equal(t, []int{1}, s.items)
}

// TestGenericsSmoke 全量冒烟。
func TestGenericsSmoke(t *testing.T) {
	requireDemoRun(t)

	out := captureStdout(t, RunGenericsExperiments)
	assert.Contains(t, out, "~int 匹配底层类型")
	assert.Contains(t, out, "方法不能有类型参数")
	assert.Contains(t, out, "GC shape", "实现机制说明必须出现")
	assert.Contains(t, out, "781 KB", "装箱分配对照的实测值必须出现")
}
