package hashtable

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCanConstruct(t *testing.T) {
	assert.False(t, canConstruct("a", "b"))

	assert.False(t, canConstruct("aa", "ab"))

	assert.True(t, canConstruct("aa", "aab"))
}
