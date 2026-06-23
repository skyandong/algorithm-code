package linkedlist

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCopyRandomList(t *testing.T) {
	// 输入：head = [[7,null],[13,0],[11,4],[10,2],[1,0]]
	n0 := &Node{Val: 7}
	n1 := &Node{Val: 13}
	n2 := &Node{Val: 11}
	n3 := &Node{Val: 10}
	n4 := &Node{Val: 1}
	n0.Next = n1
	n1.Next = n2
	n2.Next = n3
	n3.Next = n4
	n0.Random = nil
	n1.Random = n0
	n2.Random = n4
	n3.Random = n2
	n4.Random = n0
	copied := copyRandomList(n0)
	assert.NotNil(t, copied)

	assert.Equal(t, 7, copied.Val)
	assert.Nil(t, copied.Random)

	assert.Equal(t, 13, copied.Next.Val)
	assert.Equal(t, 7, copied.Next.Random.Val)

	assert.Equal(t, 11, copied.Next.Next.Val)
	assert.Equal(t, 1, copied.Next.Next.Random.Val)

	assert.Equal(t, 10, copied.Next.Next.Next.Val)
	assert.Equal(t, 11, copied.Next.Next.Next.Random.Val)
	
	assert.Equal(t, 1, copied.Next.Next.Next.Next.Val)
	assert.Equal(t, 7, copied.Next.Next.Next.Next.Random.Val)

	// 输入：head = [[1,1],[2,1]]
	n0 = &Node{Val: 1}
	n1 = &Node{Val: 2}
	n0.Next = n1
	n0.Random = n1
	n1.Random = n1
	copied = copyRandomList(n0)
	assert.NotNil(t, copied)
	assert.Equal(t, 1, copied.Val)
	assert.Equal(t, 2, copied.Random.Val)
	assert.Equal(t, 2, copied.Next.Val)
	assert.Equal(t, 2, copied.Next.Random.Val)

	// 输入：head = [[3,null],[3,0],[3,null]]
	n0 = &Node{Val: 3}
	n1 = &Node{Val: 3}
	n2 = &Node{Val: 3}
	n0.Next = n1
	n1.Next = n2
	n0.Random = nil
	n1.Random = n0
	n2.Random = nil
	copied = copyRandomList(n0)
	assert.NotNil(t, copied)
	assert.Equal(t, 3, copied.Val)
	assert.Nil(t, copied.Random)
	assert.Equal(t, 3, copied.Next.Val)
	assert.Equal(t, 3, copied.Next.Random.Val)
	assert.Equal(t, 3, copied.Next.Next.Val)
	assert.Nil(t, copied.Next.Next.Random)

	// 空链表
	assert.Nil(t, copyRandomList(nil))
}
