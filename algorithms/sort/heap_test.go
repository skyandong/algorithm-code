package sort

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHeapSort(t *testing.T) {
	assert := assert.New(t)
	array := []int{53, 17, 78, 9, 45, 65, 87, 23, 31}
	HeapSort(array)
	assert.Equal([]int{9, 17, 23, 31, 45, 53, 65, 78, 87}, array)
}
