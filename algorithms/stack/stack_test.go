package stack

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStack(t *testing.T) {
	s := NewStack()

	// 空栈 Top/Pop 返回 nil，Empty 为 true
	assert.True(t, s.Empty())
	assert.Nil(t, s.Top())
	assert.Nil(t, s.Pop())

	// Push 1,2,3 后 Top 为 3
	s.Push(1)
	s.Push("two")
	s.Push(3)
	assert.False(t, s.Empty())
	assert.Equal(t, 3, s.Len())
	assert.Equal(t, 3, s.Top())

	// Pop 顺序为 LIFO
	v := s.Pop()
	assert.Equal(t, 3, v)
	assert.Equal(t, "two", s.Pop())
	assert.Equal(t, 1, s.Pop())
	assert.True(t, s.Empty())
}
