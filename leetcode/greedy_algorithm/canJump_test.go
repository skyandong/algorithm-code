package greedyalgorithm

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCanJump(t *testing.T) {
	assert.True(t, canJump([]int{3, 3, 1, 1, 0}))

	assert.False(t, canJump([]int{3, 2, 1, 0, 4}))

	assert.False(t, canJump([]int{0, 2, 1, 0, 4}))
}
