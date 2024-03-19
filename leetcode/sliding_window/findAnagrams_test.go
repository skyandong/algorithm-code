package slidingwindow

import (
	"fmt"
	"testing"
)

func TestFindAnagrams(t *testing.T) {
	res := findAnagrams("abaacbabc", "abc")
	fmt.Println(res)
}

func findAnagram(s string, p string) []int {
	lens, lenp := len(s), len(p)
	if lens < lenp {
		return nil
	}

	var freq [256]int
	for i := 0; i < lenp; i++ {
		freq[p[i]]++
	}

	left, right, count := 0, 0, lenp
	var result []int
	for right < lens {
		// 减小计数器，如果字符存在于 p 中
		if freq[s[right]] >= 1 {
			count--
		}
		freq[s[right]]--
		right++

		// 当计数器为 0 时，意味着我们找到了一个正确的排列
		if count == 0 {
			result = append(result, left)
		}

		// 如果窗口大小等于 p 的长度，则将左边界向右移动
		if right-left == lenp {
			if freq[s[left]] >= 0 {
				count++
			}
			freq[s[left]]++
			left++
		}
	}

	return result
}
