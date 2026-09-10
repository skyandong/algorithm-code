// 实验 01（slice 部分），断言验证；map 部分见 01-2_map_test.go
// 对应笔记：notes/golang/01-slice、string与map底层.md 第 1~4、15 节
// 源码对照：src/runtime/slice.go
package main

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/assert"
)

// dataPtr 取切片底层数组的首地址（用于断言共享/独立）
func dataPtr[T any](s []T) uintptr {
	return uintptr(unsafe.Pointer(unsafe.SliceData(s)))
}

// 包级变量：强制逃逸，走 runtime.growslice 真实路径（栈上小 append 观察不到扩容规律）
var growthSink []int

// TestSliceHeader 第 1 节：slice 头就是 ptr/len/cap 三个字段
func TestSliceHeader(t *testing.T) {
	s := []int{10, 20, 30}
	assert.Len(t, s, 3)
	assert.Equal(t, 3, cap(s))

	sub := s[1:]
	assert.Len(t, sub, 2)
	assert.Equal(t, 2, cap(sub))
	assert.Equal(t, dataPtr(s)+unsafe.Sizeof(s[0]), dataPtr(sub),
		"s[1:] 的数据指针 = 原 array + 1×元素宽度：新 header 指向同一数组中间")

	sub[0] = 99
	assert.Equal(t, 99, s[1], "两个 header 共享一个数组")

	s2 := s
	assert.Equal(t, dataPtr(s), dataPtr(s2), "赋值拷贝 ptr：两个头指向同一底层数组")
	assert.Len(t, s2, 3, "赋值拷贝 len")
	assert.Equal(t, 3, cap(s2), "赋值拷贝 cap")

	s2 = s2[:2]
	assert.Len(t, s, 3, "改 s2 的头不影响 s：头是各自独立的副本")
	assert.Equal(t, dataPtr(s), dataPtr(s2), "缩头不改 ptr，仍共享同一数组")
}

// TestAppendSharedAndSeparated 第 2 节：追加何时共享底层数组
func TestAppendSharedAndSeparated(t *testing.T) {
	// cap=1 是有意的：b 与 c 都往同一个预留槽位写，才能演示「后写的顶掉先写的」
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

// TestAppendInside 第 2 节：函数内 append 不改调用方的 len
func TestAppendInside(t *testing.T) {
	// 就地定义：闭包也是函数，同样跨了一次调用边界（多头传参、结果没人接）
	appendInside := func(s []int) {
		s = append(s, 7)
	}

	s := make([]int, 0, 4)
	appendInside(s)

	assert.Len(t, s, 0, "调用方 len 不变")
	assert.Equal(t, []int{7}, s[:1], "数据其实写进了共享数组")
}

// TestAppendAndReturn 第 2 节：结果传出去才会更新头
func TestAppendAndReturn(t *testing.T) {
	appendAndReturn := func(s []int) []int {
		return append(s, 7)
	}

	assert.Equal(t, []int{7}, appendAndReturn(make([]int, 0, 4)))
}

// TestGrowthCapSequence 第 3 节：growslice 的 cap 增长序列
func TestGrowthCapSequence(t *testing.T) {
	growthSink = make([]int, 0)
	growthSink = append(growthSink, 1) // 首跳 cap 0→1 是 newLen > 2*oldCap 的按需分配分支，不属翻倍段
	prev := cap(growthSink)
	for i := 2; i <= 10000; i++ {
		growthSink = append(growthSink, i)
		if cap(growthSink) == prev {
			continue
		}
		if prev < 256 {
			assert.Equal(t, 2*prev, cap(growthSink), "翻倍段")
		} else {
			ratio := float64(cap(growthSink)) / float64(prev)
			assert.GreaterOrEqual(t, ratio, 1.25, "平滑公式下限")
			assert.LessOrEqual(t, ratio, 2.0, "sizeclass 对齐只往上修")
		}
		prev = cap(growthSink)
	}
	assert.GreaterOrEqual(t, cap(growthSink), 10000)
}

// TestDeleteWays 第 4 节：删除元素的三种写法
func TestDeleteWays(t *testing.T) {
	a := []int{1, 2, 3, 4}
	a = a[:len(a)-1]
	assert.Equal(t, []int{1, 2, 3}, a)
	assert.Equal(t, 4, a[:cap(a)][len(a)], "截断不清槽位，指针元素会泄漏")

	b := []int{1, 2, 3, 4, 5}
	b = append(b[:1], b[2:]...)
	assert.Equal(t, []int{1, 3, 4, 5}, b, "copy 覆盖：保序")
	assert.Equal(t, 5, b[:cap(b)][len(b)], "尾部残留旧值")

	c := []int{1, 2, 3, 4, 5}
	c[1] = c[len(c)-1]
	c = c[:len(c)-1]
	assert.Equal(t, []int{1, 5, 3, 4}, c, "swap-delete：不保序，无尾部残留")

	v := 42
	p := []*int{&v, &v, &v}
	p[len(p)-1] = nil
	p = p[:len(p)-1]
	assert.Nil(t, p[:cap(p)][len(p)], "指针元素截断前清槽位，被删对象才可 GC")

	func() {
		defer func() {
			assert.Contains(t, fmt.Sprint(recover()), "index out of range", "a[len(a)] 越界反面教材")
		}()
		q := make([]*int, 2, 2)
		q = q[:1]
		q[len(q)] = nil
	}()
}

// TestSubsliceLeak 第 4 节：子切片扣住整块底层数组
func TestSubsliceLeak(t *testing.T) {
	big := make([]byte, 1<<20) // 1 MiB
	tail := big[len(big)-2:]

	tp, bp := dataPtr(tail), dataPtr(big)
	assert.Greater(t, tp, bp)
	assert.Less(t, tp, bp+1<<20, "tail 指向 big 内部：tail 活着，1 MiB 就回收不了")

	fixed := make([]byte, len(tail))
	copy(fixed, tail)
	fp := dataPtr(fixed)
	assert.True(t, fp < bp || fp >= bp+1<<20, "copy 修复后 fixed 独立于 big")
}

// TestNilEmptySlice 第 15 节：nil 切片与空切片的分工
func TestNilEmptySlice(t *testing.T) {
	var nilSlice []int
	emptySlice := []int{}

	assert.Equal(t, 0, len(nilSlice))
	assert.Equal(t, 0, len(emptySlice), "判空一律 len()==0")

	assert.True(t, nilSlice == nil)
	assert.False(t, emptySlice == nil)

	assert.Nil(t, unsafe.SliceData(nilSlice))
	assert.NotNil(t, unsafe.SliceData(emptySlice), "空切片数据指针指向 zerobase")

	assert.True(t, reflect.ValueOf(nilSlice).IsNil())
	assert.False(t, reflect.ValueOf(emptySlice).IsNil())

	nb, _ := json.Marshal(nilSlice)
	eb, _ := json.Marshal(emptySlice)
	assert.Equal(t, "null", string(nb), "nil 切片 JSON 是 null")
	assert.Equal(t, "[]", string(eb), "空切片 JSON 是 []")
}
