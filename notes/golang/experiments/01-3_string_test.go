// 实验 01（string 部分），断言验证
// 对应笔记：notes/golang/01-slice、string与map底层.md 第 5~10 节
// 源码对照：src/runtime/string.go
package main

import (
	"runtime"
	"strings"
	"testing"
	"unicode/utf8"
	"unsafe"

	"github.com/stretchr/testify/assert"
)

func strData(s string) uintptr {
	return uintptr(unsafe.Pointer(unsafe.StringData(s)))
}

func TestStringHeader(t *testing.T) {
	s := "hello, world"
	sub := s[:5]

	assert.True(t, unsafe.StringData(s) == unsafe.StringData(sub), "子串只造新头，共享底层内存")

	cloned := strings.Clone(sub)
	assert.False(t, unsafe.StringData(s) == unsafe.StringData(cloned), "Clone 拷出独立小串，大字符串可回收")
}

func TestStringImmutableZeroCopy(t *testing.T) {
	b := []byte("hello")
	s := unsafe.String(unsafe.SliceData(b), len(b))
	assert.Equal(t, "hello", s)

	b[0] = 'H'
	assert.Equal(t, "Hello", s, "零拷贝产物跟着源变——绝不修改零拷贝 string 的底层")
}

// 逃逸汇：强迫 []byte(s) 拷贝
var strSink [][]byte

func TestStringConvertAlloc(t *testing.T) {
	const n = 100000
	s := strings.Repeat("x", 64)
	var before, after runtime.MemStats

	runtime.GC()
	runtime.ReadMemStats(&before)
	total := 0
	for i := 0; i < n; i++ {
		total += len([]byte(s))
	}
	runtime.ReadMemStats(&after)
	noEscape := after.TotalAlloc - before.TotalAlloc

	strSink = make([][]byte, 0, n)
	runtime.GC()
	runtime.ReadMemStats(&before)
	for i := 0; i < n; i++ {
		strSink = append(strSink, []byte(s))
	}
	runtime.ReadMemStats(&after)
	escaped := after.TotalAlloc - before.TotalAlloc
	strSink = nil

	assert.Equal(t, n*64, total)
	assert.Less(t, noEscape, uint64(1024), "不逃逸且不被修改：编译器免拷贝")
	assert.GreaterOrEqual(t, escaped, uint64(n*64), "逃逸：真拷贝 ≈ n×64B")

	m := map[string]int{"ping": 1}
	runtime.GC()
	runtime.ReadMemStats(&before)
	for i := 0; i < n; i++ {
		key := []byte("ping")
		if _, ok := m[string(key)]; ok {
		}
	}
	runtime.ReadMemStats(&after)
	assert.Equal(t, uint64(0), after.TotalAlloc-before.TotalAlloc, "m[string(b)] 编译器特例：索引免分配")
}

func TestStringConcat(t *testing.T) {
	parts := make([]string, 2000)
	for i := range parts {
		parts[i] = "abcdefgh"
	}

	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	plus := ""
	for _, p := range parts {
		plus += p
	}
	runtime.ReadMemStats(&after)
	plusAlloc := after.TotalAlloc - before.TotalAlloc

	runtime.ReadMemStats(&before)
	var bg strings.Builder
	bg.Grow(len(parts) * len(parts[0]))
	for _, p := range parts {
		bg.WriteString(p)
	}
	grow := bg.String()
	runtime.ReadMemStats(&after)
	growAlloc := after.TotalAlloc - before.TotalAlloc

	assert.Equal(t, strings.Repeat("abcdefgh", 2000), plus)
	assert.Equal(t, plus, grow)
	assert.Greater(t, plusAlloc, growAlloc*100, "+ 循环 O(n²) vs Builder+Grow 一次分配")
}

func TestStringRune(t *testing.T) {
	s := "你好Go"
	assert.Equal(t, 8, len(s), "len 是字节数")
	assert.Equal(t, 4, utf8.RuneCountInString(s), "码点数")
	assert.Equal(t, byte(0xE4), s[0], "按下标取的是字节不是字符")

	var idx []int
	for i := range s {
		idx = append(idx, i)
	}
	assert.Equal(t, []int{0, 3, 6, 7}, idx, "range 按码点迭代，索引是字节偏移")

	assert.False(t, utf8.ValidString(s[:4]), "按字节截断切碎多字节字符")
	assert.True(t, utf8.ValidString(s[:3]), "按字符边界截断合法")
}

func TestStringMapKey(t *testing.T) {
	m := map[string]int{"a": 1}
	m[string([]byte("b"))] = 2
	assert.Equal(t, 1, m["a"])
	assert.Equal(t, 2, m["b"])

	arr := map[[8]byte]int{}
	arr[[8]byte{1, 2}] = 3
	assert.Equal(t, 3, arr[[8]byte{1, 2}], "[N]byte 按值可比，零分配 key 惯用法")
}
