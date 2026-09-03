package arrays

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNextPermutation(t *testing.T) {
	tests := []struct {
		name string
		in   []int
		want []int
	}{
		{
			name: "ascending",
			in:   []int{1, 2, 3},
			want: []int{1, 3, 2},
		},
		{
			name: "middle",
			in:   []int{1, 3, 2},
			want: []int{2, 1, 3},
		},
		{
			name: "descending wraps",
			in:   []int{3, 2, 1},
			want: []int{1, 2, 3},
		},
		{
			name: "duplicates",
			in:   []int{1, 1, 5},
			want: []int{1, 5, 1},
		},
		{
			name: "single element",
			in:   []int{1},
			want: []int{1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nums := append([]int(nil), tt.in...)
			nextPermutation(nums)
			assert.Equal(t, tt.want, nums)
		})
	}
}
