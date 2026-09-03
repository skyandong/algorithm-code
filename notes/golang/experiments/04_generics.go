// # 泛型实验
//
// 对应笔记：notes/golang/04-泛型.md
//
// 运行（接入 main.go 后）：
//
//	go run ./experiments/ generics
//
// 实验项：
//
//	第1节：类型参数 + 推断 + 显式实例化 + var zero T
//	第2节：约束：any / comparable / cmp.Ordered / ~近似（UserID 案例）
//	第3节：泛型数据结构（Stack[T]）替代 any——值语义零装箱
//	第4节：标准库 slices/maps/cmp（含 Go 1.23 迭代器）
//	第5节：方法不能有类型参数——限制与包级函数绕法
//	第6节：泛型 vs 接口：装箱分配对照（MemStats）
package main

import (
	"cmp"
	"fmt"
	"maps"
	"runtime"
	"slices"
	"strconv"
)

// RunGenericsExperiments 演示笔记 4 的泛型行为。
func RunGenericsExperiments() {
	fmt.Println("========== 第1节: 类型参数与推断 ==========")
	genBasic()

	fmt.Println("\n========== 第2节: 约束与类型集 ==========")
	genConstraints()

	fmt.Println("\n========== 第3节: 泛型数据结构 ==========")
	genStack()

	fmt.Println("\n========== 第4节: 标准库 slices/maps/cmp ==========")
	genStdlib()

	fmt.Println("\n========== 第5节: 方法不能有类型参数 ==========")
	genMethodLimit()

	fmt.Println("\n========== 第6节: 泛型 vs 接口装箱对照 ==========")
	genVsInterface()
}

// genBasic 第1节：泛型函数、推断与显式实例化。
func genBasic() {
	// 泛型函数：同一算法，多种类型
	fmt.Printf("Max(3, 7)=%v Max(2.5, 1.5)=%v（T 分别推断为 int/float64）\n",
		Max(3, 7), Max(2.5, 1.5))

	// Map：双类型参数，从实参推断
	words := Map([]int{1, 2, 3}, strconv.Itoa) // T=int, U=string
	fmt.Printf("Map([]int, Itoa) = %v（双参数推断 T=int U=string）\n", words)

	// 显式实例化：歧义或可读性场景
	fmt.Printf("Map[int,string] 显式 = %v\n", Map[int, string]([]int{4}, strconv.Itoa))

	// var zero T：泛型里拿零值的惯用法（nil 不合法——T 可能不可空）
	fmt.Printf("FirstOrZero([]int{}) = %v（var zero T，int 零值 0）\n", FirstOrZero([]int{}))
	fmt.Printf("FirstOrZero([]string{\"a\"}) = %q（string 零值 \"\"）\n", FirstOrZero([]string{"a"}))
}

// Max 有序类型取大。
func Max[T cmp.Ordered](a, b T) T {
	if a > b {
		return a
	}
	return b
}

// Map 把 []T 变 []U。
func Map[T, U any](xs []T, f func(T) U) []U {
	r := make([]U, 0, len(xs))
	for _, x := range xs {
		r = append(r, f(x))
	}
	return r
}

// FirstOrZero 空切片返回 T 的零值。
func FirstOrZero[T any](xs []T) T {
	var zero T // 惯用法：不能用 nil
	if len(xs) == 0 {
		return zero
	}
	return xs[0]
}

// genConstraints 第2节：类型集约束、~ 近似、comparable。
func genConstraints() {
	// UserID 的底层类型是 int，但它是新类型
	type UserID int
	uid := UserID(42)

	// 严格匹配 int：UserID 不满足；~int：满足
	sum := SumTilde([]UserID{uid, 2, 3}) // ~int 放行：UserID 编译通过
	fmt.Printf("SumTilde([]UserID{%d,2,3}) = %v（~int 匹配底层类型，UserID 通过）\n", uid, sum)
	fmt.Println("对照: 约束写 int（不带 ~）时 UserID 编译报错（只匹配 int 本身）")

	// comparable：== 可用的类型集合
	keys := Keys(map[string]int{"a": 1, "b": 2})
	fmt.Printf("Keys(map[string]int) = %v（K comparable，map key 约束的标准写法）\n", keys)

	k2 := Keys(map[int]string{1: "x"})
	fmt.Printf("Keys(map[int]string) = %v（同一份代码，K 换成 int）\n", k2)
}

// SumTilde 演示 ~int 近似约束。
func SumTilde[T ~int](xs []T) T {
	var sum T
	for _, x := range xs {
		sum += x
	}
	return sum
}

// Keys 提取 map 的所有 key（comparable 约束）。
func Keys[K comparable, V any](m map[K]V) []K {
	r := make([]K, 0, len(m))
	for k := range m {
		r = append(r, k)
	}
	return r
}

// genStack 第3节：泛型容器替代 any——值语义，编译期类型安全。
func genStack() {
	s := &Stack[int]{}
	s.Push(1)
	s.Push(2)
	s.Push(3)
	v, _ := s.Pop()
	fmt.Printf("Stack[int]: Push 1/2/3 后 Pop = %v，Len = %d\n", v, s.Len())

	ss := &Stack[string]{}
	ss.Push("a")
	sv, _ := ss.Pop()
	fmt.Printf("Stack[string]: Pop = %q（同一份源码，两个实例化）\n", sv)
	fmt.Println("对比 any 版: 取出要断言 x.(int)，断错运行时才炸；泛型版编译期保证")
}

// Stack 泛型栈（对比 algorithms/stack 的具体类型版）。
type Stack[T any] struct {
	items []T
}

// Push 入栈。
func (s *Stack[T]) Push(v T) { s.items = append(s.items, v) }

// Pop 出栈。
func (s *Stack[T]) Pop() (T, bool) {
	if len(s.items) == 0 {
		var zero T
		return zero, false
	}
	v := s.items[len(s.items)-1]
	s.items = s.items[:len(s.items)-1]
	return v, true
}

// Len 栈大小。
func (s *Stack[T]) Len() int { return len(s.items) }

// genStdlib 第4节：标准库泛型工具（Go 1.21+，迭代器 Go 1.23+）。
func genStdlib() {
	xs := []int{5, 2, 8, 1}
	slices.Sort(xs) // cmp.Ordered，值语义零装箱
	fmt.Printf("slices.Sort = %v Contains(8)=%v Index(2)=%v\n",
		xs, slices.Contains(xs, 8), slices.Index(xs, 2))

	// 自定义排序：SortFunc + cmp.Compare（不稳定排序，保序用 SortStableFunc）
	people := []Person{{"ann", 30}, {"bob", 25}, {"cat", 35}}
	slices.SortFunc(people, func(a, b Person) int { return cmp.Compare(a.Age, b.Age) })
	fmt.Printf("SortFunc by Age = %v（cmp.Compare 是 -1/0/1 积木）\n", people)

	// maps：Go 1.23+ 迭代器（func(yield)，零分配）
	m := map[string]int{"a": 1, "b": 2}
	fmt.Print("maps.Keys 迭代器: ")
	for k := range maps.Keys(m) {
		fmt.Printf("%s ", k)
	}
	fmt.Println()

	maps.DeleteFunc(m, func(k string, v int) bool { return v < 2 })
	fmt.Printf("maps.DeleteFunc(v<2) 后 = %v\n", m)
}

// Person 示例结构。
type Person struct {
	Name string
	Age  int
}

// genMethodLimit 第5节：方法不能有类型参数——包级函数绕法。
func genMethodLimit() {
	// ✗ 编译错误（注释演示）：
	//   func (s *Stack[T]) Map[U any](f func(T) U) *Stack[U]  // method cannot have type parameters
	//   WHY: 接口满足性检查会不可判定（S 要实现多少个 Map[U] 才算实现接口）
	fmt.Println("func (s *Stack[T]) Map[U any](...) → 编译错误：方法不能有类型参数")

	// ✓ 绕法：包级泛型函数（slices 包就是这个风格）
	s := &Stack[int]{}
	s.Push(1)
	s.Push(2)
	doubled := StackMap(s, func(x int) int { return x * 2 })
	fmt.Printf("StackMap(s, ×2) = %v（包级泛型函数绕开方法限制）\n", doubled.items)
}

// StackMap 包级泛型函数：栈的元素映射。
func StackMap[T, U any](s *Stack[T], f func(T) U) *Stack[U] {
	r := &Stack[U]{}
	for _, x := range s.items {
		r.Push(f(x))
	}
	return r
}

// genVsInterface 第6节：泛型零装箱 vs interface 装箱分配对照。
func genVsInterface() {
	const n = 100000

	// 泛型版：T=int 全程值语义
	ints := make([]int, 0, n)
	for i := 0; i < n; i++ {
		ints = append(ints, i+1000) // 取值 >255，避开小整数静态缓存
	}

	// interface 版：同样的数据装进 []any → 每个元素装箱
	anys := make([]any, 0, n)
	for i := 0; i < n; i++ {
		anys = append(anys, i+1000) // int → any：逃逸 + 超出小整数缓存 → 堆分配
	}

	// 分别统计：泛型求和 vs 断言求和（结果一致，成本不同）
	gs := SumGeneric(ints)
	is := 0
	for _, v := range anys {
		is += v.(int) // 拆箱断言
	}

	// 分配对比：重跑装箱过程量 TotalAlloc（背板数组在计量前预分配，只测装箱本身）
	var before, after runtime.MemStats
	boxed := make([]any, 0, n)
	runtime.GC()
	runtime.ReadMemStats(&before)
	for i := 0; i < n; i++ {
		boxed = append(boxed, i+1000)
	}
	runtime.ReadMemStats(&after)
	ifaceAlloc := after.TotalAlloc - before.TotalAlloc

	fmt.Printf("%d 个 int 装进 []any: 堆分配 %d KB（每个值装箱一次 8B，理论值≈781KB；>255 无静态缓存）\n",
		n, ifaceAlloc/1024)
	fmt.Printf("结果一致: %v（泛型 []int 全程值语义零装箱；[]any 断言取回才拿到静态类型）\n", gs == is)
	fmt.Println("注意: 字典间接 vs itab 分发性能相当或略快，但省装箱——热路径差异主要在这里")
}

// SumGeneric 泛型求和（cmp.Ordered）。
func SumGeneric[T cmp.Ordered](xs []T) T {
	var sum T
	for _, x := range xs {
		sum += x
	}
	return sum
}
