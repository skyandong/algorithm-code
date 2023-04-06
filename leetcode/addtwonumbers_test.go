package leetcode

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAddTwoNumbers(t *testing.T) {
	head1 := &ListNode{
		Val: 2,
		Next: &ListNode{
			Val: 4,
			Next: &ListNode{
				Val: 3,
			},
		},
	}
	head2 := &ListNode{
		Val: 4,
		Next: &ListNode{
			Val: 6,
			Next: &ListNode{
				Val: 6,
			},
		},
	}
	res := AddTwoNumbers(head1, head2)
	assert.Equal(t, 6, res.Val)
	assert.Equal(t, 0, res.Next.Val)
	assert.Equal(t, 0, res.Next.Next.Val)
	assert.Equal(t, 1, res.Next.Next.Next.Val)
}
