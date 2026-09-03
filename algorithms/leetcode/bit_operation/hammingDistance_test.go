package bitoperation

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHammingDistance(t *testing.T) {
	assert.Equal(t, 2, hammingDistance(1, 4))

	assert.Equal(t, 1, hammingDistance(3, 1))
}
