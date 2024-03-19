package substring

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSubarraySum(t *testing.T) {
	assert.Equal(t, 2, subarraySum([]int{1, 1, 1}, 2))

	assert.Equal(t, 2, subarraySum([]int{1, 2, 3}, 3))

	assert.Equal(t, 1, subarraySum([]int{-1, -1, 1}, 1))

	assert.Equal(t, 2, subarraySum([]int{-9, 1, 2, 3}, 3))

	assert.Equal(t, 4, subarraySum([]int{1, 2, 1, 2, 1}, 3))

	assert.Equal(t, 3, subarraySum([]int{1, -1, 0}, 0))
}
