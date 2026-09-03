package backtracking

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGenerateParenthesis(t *testing.T) {
	assert.Equal(t, []string{"()"}, generateParenthesis(1))

	assert.ElementsMatch(t,
		[]string{"((()))", "(()())", "(())()", "()(())", "()()()"},
		generateParenthesis(3))

	got := generateParenthesis(4)
	assert.Len(t, got, 14)
	for _, s := range got {
		assert.True(t, isValidParen(s), "invalid: %q", s)
	}
}

func isValidParen(s string) bool {
	b := 0
	for _, c := range s {
		if c == '(' {
			b++
		} else {
			b--
		}
		if b < 0 {
			return false
		}
	}
	return b == 0
}
