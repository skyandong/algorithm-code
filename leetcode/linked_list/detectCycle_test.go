package linkedlist

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDetectCycle(t *testing.T) {
	cycleIn := &ListNode{Val: 2}
	head := &ListNode{Val: 3, Next: cycleIn}
	cycleIn.Next = &ListNode{
		Val: 0,
		Next: &ListNode{
			Val:  -4,
			Next: cycleIn,
		},
	}
	assert.Equal(t, cycleIn, detectCycle(head))
}
