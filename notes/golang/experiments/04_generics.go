// # 泛型实验
//
// 对应笔记：notes/golang/03-泛型.md
//
// 本实验为断言式单元测试（无打印），断言见 04_generics_test.go：
//
//	go test -run 'TestGenerics|TestMax|TestMap|TestFirstOrZero|TestSumTilde|TestKeys|TestStack' ./experiments/
//
// —— runtime 源码对照 ——
//
// 泛型没有专属运行时结构：编译器按 GC shape 分组生成机器码，
// 同 shape 类型共享一份实现，差异由字典（dict，编译期生成的类型描述符
// 指针参数）在运行时查表补齐——这就是"GC shape + 字典"实现。
//
// src/internal/abi/type.go（字典指向的类型元数据，简化示意）
//
//	type Type struct {
//		Size_    uintptr // 类型大小
//		PtrBytes uintptr // 含指针的前缀字节数（GC 扫描用）
//		Hash     uint32
//		Kind_    uint8
//		// ...Equal、GCData、Str 等字段
//	}
package main

import "cmp"

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

// FirstOrZero 空切片返回 T 的零值（var zero T：泛型里不能写 nil，T 可能不可空）。
func FirstOrZero[T any](xs []T) T {
	var zero T
	if len(xs) == 0 {
		return zero
	}
	return xs[0]
}

// SumTilde 演示 ~int 近似约束：UserID 这类自定义底层类型也能进。
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

// Stack 泛型栈（对比 algorithms/stack 的具体类型版）：值语义，无装箱。
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

// StackMap 包级泛型函数：方法不能有类型参数，映射只能这样写（slices 包同款风格）。
func StackMap[T, U any](s *Stack[T], f func(T) U) *Stack[U] {
	r := &Stack[U]{}
	for _, x := range s.items {
		r.Push(f(x))
	}
	return r
}

// SumGeneric 泛型求和（cmp.Ordered）。
func SumGeneric[T cmp.Ordered](xs []T) T {
	var sum T
	for _, x := range xs {
		sum += x
	}
	return sum
}

// Person 示例结构。
type Person struct {
	Name string
	Age  int
}
