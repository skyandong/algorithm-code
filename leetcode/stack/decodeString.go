package leetcode

import (
	"strconv"
	"strings"
)

// 字符串解码
func decodeString(s string) string {
	i := 0
	return decode(s, &i)
}

func decode(s string, i *int) string {
	var builder strings.Builder
	for *i < len(s) {
		ch := s[*i]
		if ch == ']' {
			*i++
			return builder.String()
		} else if ch >= '0' && ch <= '9' {
			var numBuilder strings.Builder
			for *i < len(s) && s[*i] >= '0' && s[*i] <= '9' {
				numBuilder.WriteByte(s[*i])
				*i++
			}
			*i++ // skip '['
			num, _ := strconv.Atoi(numBuilder.String())
			sub := decode(s, i)
			builder.WriteString(strings.Repeat(sub, num))
		} else {
			builder.WriteByte(ch)
			*i++
		}
	}
	return builder.String()
}
