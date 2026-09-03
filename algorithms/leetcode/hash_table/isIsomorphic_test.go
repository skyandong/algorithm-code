package hashtable

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsIsomorphic(t *testing.T) {
	assert.True(t, isIsomorphic("add", "egg"))

	assert.False(t, isIsomorphic("foo", "bar"))

	assert.True(t, isIsomorphic("paper", "title"))
}
