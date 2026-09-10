// 实验 04（泛型），断言验证
// 对应笔记：notes/golang/03-泛型.md
package main

import (
	"cmp"
	"maps"
	"runtime"
	"slices"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMax(t *testing.T) {
	assert.Equal(t, 7, Max(3, 7))
	assert.Equal(t, 2.5, Max(2.5, 1.5))
	assert.Equal(t, "b", Max("a", "b"))
}

func TestMapInferenceAndExplicit(t *testing.T) {
	assert.Equal(t, []string{"1", "2", "3"}, Map([]int{1, 2, 3}, strconv.Itoa), "T/U 从实参推断")
	assert.Equal(t, []string{"4"}, Map[int, string]([]int{4}, strconv.Itoa), "显式实例化")
	assert.Empty(t, Map([]int{}, func(x int) int { return x }))
}

func TestFirstOrZero(t *testing.T) {
	assert.Zero(t, FirstOrZero([]int{}), "var zero T：T 是 int 时即 0")
	assert.Empty(t, FirstOrZero([]string{}), "var zero T：T 是 string 时即 \"\"")
	assert.Equal(t, "a", FirstOrZero([]string{"a"}))
}

func TestSumTilde(t *testing.T) {
	type UserID int
	assert.Equal(t, UserID(6), SumTilde([]UserID{1, 2, 3}), "~int 匹配底层类型，UserID 也能进")
	assert.Equal(t, 6, SumTilde([]int{1, 2, 3}))
}

func TestKeys(t *testing.T) {
	got := Keys(map[string]int{"a": 1, "b": 2})
	assert.Len(t, got, 2)
	assert.ElementsMatch(t, []string{"a", "b"}, got, "K comparable")

	assert.ElementsMatch(t, []int{1}, Keys(map[int]string{1: "x"}), "同一份代码换 K")
}

func TestStack(t *testing.T) {
	s := &Stack[int]{}

	_, ok := s.Pop()
	assert.False(t, ok, "空栈 Pop 返回 false 与零值")

	s.Push(1)
	s.Push(2)
	v, ok := s.Pop()
	assert.True(t, ok)
	assert.Equal(t, 2, v)
	assert.Equal(t, 1, s.Len())

	doubled := StackMap(s, func(x int) int { return x * 2 })
	assert.Equal(t, []int{2}, doubled.items, "包级泛型函数绕开「方法不能有类型参数」的限制")
	assert.Equal(t, []int{1}, s.items, "StackMap 不改原栈")

	ss := &Stack[string]{}
	ss.Push("a")
	sv, _ := ss.Pop()
	assert.Equal(t, "a", sv, "同一份源码两个实例化")
}

func TestGenericsStdlib(t *testing.T) {
	xs := []int{5, 2, 8, 1}
	slices.Sort(xs)
	assert.Equal(t, []int{1, 2, 5, 8}, xs)
	assert.True(t, slices.Contains(xs, 8))
	assert.Equal(t, 1, slices.Index(xs, 2))

	people := []Person{{"ann", 30}, {"bob", 25}, {"cat", 35}}
	slices.SortFunc(people, func(a, b Person) int { return cmp.Compare(a.Age, b.Age) })
	assert.Equal(t, []Person{{"bob", 25}, {"ann", 30}, {"cat", 35}}, people, "cmp.Compare 是 -1/0/1 积木")

	m := map[string]int{"a": 1, "b": 2}
	got := []string{}
	for k := range maps.Keys(m) {
		got = append(got, k)
	}
	assert.ElementsMatch(t, []string{"a", "b"}, got, "maps.Keys 是迭代器")

	maps.DeleteFunc(m, func(k string, v int) bool { return v < 2 })
	assert.Equal(t, map[string]int{"b": 2}, m)
}

func sumAny(xs []any) int {
	sum := 0
	for _, v := range xs {
		sum += v.(int)
	}
	return sum
}

func TestGenericsNoBoxing(t *testing.T) {
	const n = 100000

	ints := make([]int, 0, n)
	boxed := make([]any, 0, n)

	var before, after runtime.MemStats

	runtime.GC()
	runtime.ReadMemStats(&before)
	for i := 0; i < n; i++ {
		ints = append(ints, i+1000)
	}
	runtime.ReadMemStats(&after)
	genericAlloc := after.TotalAlloc - before.TotalAlloc

	runtime.GC()
	runtime.ReadMemStats(&before)
	for i := 0; i < n; i++ {
		boxed = append(boxed, i+1000) // 装箱：取值 >255，避开小整数静态缓存
	}
	runtime.ReadMemStats(&after)
	ifaceAlloc := after.TotalAlloc - before.TotalAlloc

	assert.Equal(t, SumGeneric(ints), sumAny(boxed), "两条路径结果一致")
	assert.Less(t, genericAlloc, uint64(1024), "[]int 值语义：背板预分配后零额外分配")
	assert.GreaterOrEqual(t, ifaceAlloc, uint64(n*8), "[]any 每个值装箱一次：≥ n×8B（理论上约 781KB）")
}
