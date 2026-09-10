// # Interface 与反射实验
//
// 对应笔记：notes/golang/02-interface与反射.md
//
// 本实验为断言式单元测试（无打印），断言见 03_interface_reflection_test.go：
//
//	go test -run 'TestInterface|TestBoxing|TestMethodSet|TestNilInterface|TestTypeAssertion|TestReflect|TestJSON|TestUnsafeString' ./experiments/
//	go test -bench BenchmarkReflectSetInt ./experiments/   // 反射三档开销对照
//
// —— runtime 源码对照 ——
//
// src/runtime/runtime2.go
// type eface struct { // 空 interface{}
//
//		_type *_type         // 动态类型元数据
//		data  unsafe.Pointer // 数据指针（值大时指向堆副本）
//	}
//
// type iface struct { // 带方法集的 interface
//
//		tab  *itab          // itab：接口类型 + 动态类型 + 方法表 + hash
//		data unsafe.Pointer
//	}
package main

import "fmt"

// point 用于观察装箱拷贝。
type point struct{ X, Y int }

// boxingSink 强制装箱对象逃逸（不写全局的话编译器可能优化掉装箱）。
var boxingSink any

// receiver 同时具备值接收者与指针接收者方法。
type receiver struct{}

// ByValue 值接收者方法。
func (receiver) ByValue() {}

// ByPointer 指针接收者方法。
func (*receiver) ByPointer() {}

// myError 模拟线上事故：返回 *myError(nil) 时接口非 nil。
type myError struct{ code int }

// Error 实现 error 接口（指针接收者）。
func (e *myError) Error() string { return fmt.Sprintf("code %d", e.code) }

// doBad 反面教材：返回一个「有类型无值」的接口。
func doBad() error {
	var p *myError = nil
	return p // iface: tab != nil, data == nil
}

// user 含导出与非导出字段，用于 reflect 观察。
type user struct {
	Name string // 导出字段
	age  int    // 非导出字段
}

// reflectPerfUser 反射开销对照用：字段必须导出，否则 SetInt 会 panic。
type reflectPerfUser struct {
	Name string
	Age  int
}
