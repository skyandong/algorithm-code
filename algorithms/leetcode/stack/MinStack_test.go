package leetcode

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMinStack(t *testing.T) {
	// LeetCode 示例
	obj := Constructor()
	obj.Push(-2)
	obj.Push(0)
	obj.Push(-3)
	assert.Equal(t, -3, obj.GetMin())
	obj.Pop()
	assert.Equal(t, 0, obj.Top())
	assert.Equal(t, -2, obj.GetMin())

	// 重复最小值:弹出后 min 仍是同一个值
	s := Constructor()
	s.Push(2)
	s.Push(2)
	s.Push(2)
	assert.Equal(t, 2, s.GetMin())
	s.Pop()
	assert.Equal(t, 2, s.GetMin())
	s.Pop()
	assert.Equal(t, 2, s.GetMin())

	// 递增: min 始终是第一个
	inc := Constructor()
	inc.Push(1)
	inc.Push(2)
	inc.Push(3)
	assert.Equal(t, 1, inc.GetMin())

	// 递减: 每次入栈都更新 min
	dec := Constructor()
	dec.Push(3)
	dec.Push(2)
	dec.Push(1)
	assert.Equal(t, 1, dec.GetMin())
	dec.Pop()
	assert.Equal(t, 2, dec.GetMin())
	dec.Pop()
	assert.Equal(t, 3, dec.GetMin())

	// 弹出非最小值不影响 min
	m := Constructor()
	m.Push(5)
	m.Push(3)
	m.Push(4)
	m.Push(2)
	assert.Equal(t, 2, m.GetMin())
	m.Pop() // 移除 2
	assert.Equal(t, 3, m.GetMin())
	m.Pop() // 移除 4 (非最小,min 不变)
	assert.Equal(t, 3, m.GetMin())
	m.Pop() // 移除 3
	assert.Equal(t, 5, m.GetMin())

	// Top/GetMin 不修改栈
	assert.Equal(t, 5, m.Top())
	assert.Equal(t, 5, m.GetMin())
	assert.Equal(t, 5, m.Top())
}
