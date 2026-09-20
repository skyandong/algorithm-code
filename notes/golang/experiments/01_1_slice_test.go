// 实验 01_1（slice 部分），断言验证；string 部分见 01_2_string_test.go，map 部分见 01_3_map_test.go
// 对应笔记：notes/golang/01-slice、string与map底层.md 第 1~4、15 节
// 源码对照：src/runtime/slice.go
package main

import (
	"encoding/json"
	"sync"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/assert"
)

// dataPtr 取切片底层数组的首地址（用于断言共享/独立）
func dataPtr[T any](s []T) uintptr {
	return uintptr(unsafe.Pointer(unsafe.SliceData(s)))
}

// TestSliceHeader 第 1 节：slice 头就是 ptr/len/cap 三个字段
func TestSliceHeader(t *testing.T) {
	s := []int{10, 20, 30}

	// 切片表达式：新头指向同一数组中间，cap 跟着下界一起减
	sub := s[1:]
	assert.Equal(t, dataPtr(s)+unsafe.Sizeof(s[0]), dataPtr(sub),
		"s[1:] 的 ptr = 原 array + 1×元素宽度")
	assert.Equal(t, 2, cap(sub), "cap 从 3 减到 2（不只 ptr 前移）")
	sub[0] = 99
	assert.Equal(t, 99, s[1], "两个头共享同一数组")

	// 赋值：两份头各自独立，动一份不影响另一份
	s2 := s
	s2 = s2[:2]
	assert.Equal(t, 3, len(s), "改 s2 的头不影响 s")
	assert.Equal(t, dataPtr(s), dataPtr(s2), "缩头不改 ptr，仍是同一数组")
}

// TestAppendSharedAndSeparated 第 2 节：追加何时共享底层数组
func TestAppendSharedAndSeparated(t *testing.T) {
	a := make([]int, 0, 1)

	b := append(a, 99)
	assert.Equal(t, dataPtr(a), dataPtr(b), "未超 cap：b 与 a 共享底层数组")

	c := append(a, 100)
	assert.Equal(t, 100, b[0], "两次 append 写同一槽位，c 顶掉 b 的 99")

	// 一次追加 4 个：len 1→5 越过 cap=1，必须新分配并把旧元素拷过去
	d := append(c, 1, 2, 3, 4)
	assert.NotEqual(t, dataPtr(c), dataPtr(d), "len=5 > cap=1：越界，扩容分配新数组")
	assert.Equal(t, []int{100, 1, 2, 3, 4}, d, "旧元素被拷进新数组，追加值接在后面")
	assert.Greater(t, cap(d), cap(c), "新数组容量比原来的 cap=1 大")
}

// TestAppendAcrossCall 第 2 节：函数内 append 不改调用方的 len，接住返回值才看得见追加的元素
func TestAppendAcrossCall(t *testing.T) {
	// 就地定义：闭包也是函数，同样跨了一次调用边界（多头传参、结果没人接）
	appendInside := func(s []int) {
		s = append(s, 7)
	}
	appendAndReturn := func(s []int) []int {
		return append(s, 7)
	}

	s := make([]int, 0, 4)
	appendInside(s)

	assert.Len(t, s, 0, "调用方 len 不变")
	assert.Equal(t, []int{7}, s[:1], "数据其实写进了共享数组")

	assert.Equal(t, []int{7}, appendAndReturn(make([]int, 0, 4)), "对外返回的新头带着追加的元素")
}

// TestGrowCopiesElementValuesNotPointees 第 3 节：扩容搬的是元素值（浅拷贝）
// 元素是指针时只拷指针本身，指向的对象仍是同一份——扩容不会深拷贝
func TestGrowCopiesElementValuesNotPointees(t *testing.T) {
	type payload struct{ n int }
	p := &payload{n: 1}

	s := make([]*payload, 0, 1)
	s = append(s, p) // 还在 cap 内，不换数组
	oldArr := dataPtr(s)

	s = append(s, &payload{n: 2}) // len 越过 cap=1 → growslice：新数组 + memmove 元素值
	assert.NotEqual(t, oldArr, dataPtr(s), "越过 cap：底层数组换成新分配的一块")

	assert.Same(t, p, s[0], "元素值是指针，扩容搬的就是这个指针：指向的 payload 还是同一份")
	s[0].n = 99
	assert.Equal(t, 99, p.n, "透过新 slice 改元素指向的对象，原指针看得到变化——证明没被深拷贝")
}

// TestSubsliceLeak 第 4 节：子切片扣住整块底层数组
func TestSubsliceLeak(t *testing.T) {
	big := make([]byte, 1<<20) // 1 MiB
	big[len(big)-2], big[len(big)-1] = 'h', 'i'
	tail := big[len(big)-2:]

	assert.Equal(t, dataPtr(big)+uintptr(len(big))-2, dataPtr(tail),
		"tail 起始地址 = big 起始 + len-2：落在同一分配块内部，整块 1 MiB 都被它钉住")

	fixed := make([]byte, len(tail))
	copy(fixed, tail)

	assert.Equal(t, []byte("hi"), fixed, "copy 把内容搬了出来")
	assert.NotEqual(t, dataPtr(big), dataPtr(fixed), "fixed 是独立分配，与 big 无关")
}

// TestNilEmptySlice 第 14 节：nil 切片与空切片的分工
func TestNilEmptySlice(t *testing.T) {
	var nilSlice []int
	emptySlice := []int{}

	// 用法上等价：读长度、直接 append 都不需要先判 nil
	assert.Len(t, nilSlice, 0, "nil 切片 len 是 0")
	assert.Len(t, emptySlice, 0, "空切片 len 是 0（判空一律 len()==0）")
	assert.Equal(t, []int{1}, append(nilSlice, 1), "nil 切片可直接 append")
	assert.Equal(t, []int{1}, append(emptySlice, 1), "空切片同样可直接 append")

	// 可区分：与 nil 比较、底层指针、JSON 序列化
	assert.Nil(t, nilSlice, "nil 切片 == nil")
	assert.NotNil(t, emptySlice, "空切片的底层数组是 zerobase，指针非 nil")
	assert.Nil(t, unsafe.SliceData(nilSlice), "nil 切片没有底层数组")
	assert.NotNil(t, unsafe.SliceData(emptySlice), "空切片的 SliceData 指向 zerobase")

	nb, err := json.Marshal(nilSlice)
	assert.NoError(t, err, "nil 切片可序列化")
	assert.Equal(t, "null", string(nb), "nil 切片序列化成 null")

	eb, err := json.Marshal(emptySlice)
	assert.NoError(t, err, "空切片可序列化")
	assert.Equal(t, "[]", string(eb), "空切片序列化成 []")
}

// TestRangeClosureValueCopy 第 12 节：range 值副本自增不影响原 slice。
func TestRangeClosureValueCopy(t *testing.T) {
	type rngT struct{ V int }
	incr := func(t *rngT, wg *sync.WaitGroup) {
		defer wg.Done()
		t.V++
	}

	var wg sync.WaitGroup
	wg.Add(10)
	ts := make([]rngT, 10)
	for i := 0; i < 10; i++ {
		ts[i] = rngT{i}
	}
	for _, tr := range ts {
		go incr(&tr, &wg) // 值副本自增，不影响 ts
	}
	wg.Wait()

	for i := range ts {
		assert.Equal(t, i, ts[i].V, "range 值副本自增不改原 slice 元素")
	}
}
