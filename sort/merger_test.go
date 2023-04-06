package sort

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestMergerSort(t *testing.T) {
	assert := assert.New(t)
	array := []int{53, 17, 78, 9, 45, 65, 87, 23, 31}
	MergerSort(array)
	assert.Equal([]int{9, 17, 23, 31, 45, 53, 65, 78, 87}, array)
}
