// # Interface 与反射实验
//
// 对应笔记：notes/golang/03-interface与反射.md
//
// 运行：
//
//	go run ./experiments/ interface
//
// 实验项：
//
//	Exp1：接口是 (动态类型, 动态值) 二元组；装箱即值拷贝
//	Exp2：值/指针接收者方法集断言差异（T vs *T）
//	Exp3：nil 接口 != nil 的经典事故与正确判 nil 写法
//	Exp4：类型断言 — 单值失败 panic / comma-ok 不 panic / type switch
//	Exp5：reflect 遍历结构体字段，非导出字段 CanSet=false
//	Exp6：JSON 序列化 nil vs 空 切片（null vs []）
//	Exp7：unsafe.String 零拷贝与工程红线（共享内存被改）
package main

import (
	"encoding/json"
	"fmt"
	"reflect"
	"unsafe"
)

// RunInterfaceReflectionExperiments 演示笔记 03 的核心语义。
func RunInterfaceReflectionExperiments() {
	fmt.Println("========== 第1节: 接口 = (动态类型, 动态值)；装箱即拷贝 ==========")
	demoInterfacePair()

	fmt.Println("\n========== 第2节: 值/指针接收者的方法集 ==========")
	demoMethodSet()

	fmt.Println("\n========== 第3节: nil 接口 != nil（经典事故）==========")
	demoNilInterface()

	fmt.Println("\n========== 第4节: 类型断言 — panic / comma-ok / type switch ==========")
	demoTypeAssertion()

	fmt.Println("\n========== 第5节: reflect 遍历字段，非导出字段 CanSet=false ==========")
	demoReflectFields()

	fmt.Println("\n========== 第6节: JSON — nil 切片 null vs 空切片 [] ==========")
	demoJSONNilEmpty()

	fmt.Println("\n========== 第7节: unsafe 零拷贝与红线 ==========")
	demoUnsafeString()
}

// point 用于观察装箱拷贝。
type point struct{ X, Y int }

// demoInterfacePair 笔记 03 §1：接口变量持有 (类型, 值副本)。
func demoInterfacePair() {
	var i any = point{1, 2}
	fmt.Printf("动态类型=%T 动态值=%v\n", i, i)

	// 装箱是值拷贝：接口里存的是装箱那一刻的副本
	p := point{X: 10, Y: 20}
	var boxed any = p
	p.X = 999
	fmt.Printf("修改原值后接口内=%v（大结构体装箱 = 整体拷贝一次）\n", boxed)
}

// receiver 同时具备值接收者与指针接收者方法。
type receiver struct{}

// ByValue 值接收者方法。
func (receiver) ByValue() {}

// ByPointer 指针接收者方法。
func (*receiver) ByPointer() {}

// demoMethodSet 笔记 03 §2：T 的方法集不含指针接收者方法，*T 含全部。
func demoMethodSet() {
	v := any(receiver{})
	p := any(&receiver{})

	_, ok1 := v.(interface{ ByValue() })
	_, ok2 := v.(interface{ ByPointer() })
	_, ok3 := p.(interface{ ByValue() })
	_, ok4 := p.(interface{ ByPointer() })
	fmt.Printf("T  实现值接收者接口:   %v\n", ok1) // true
	fmt.Printf("T  实现指针接收者接口: %v\n", ok2)  // false ← T 方法集不含指针方法
	fmt.Printf("*T 实现值接收者接口:   %v\n", ok3) // true
	fmt.Printf("*T 实现指针接收者接口: %v\n", ok4)  // true
}

// myError 模拟线上事故：返回 *myError(nil) 时接口非 nil。
type myError struct{ code int }

// Error 实现 error 接口（指针接收者）。
func (e *myError) Error() string { return fmt.Sprintf("code %d", e.code) }

// doBad 反面教材：返回一个「有类型无值」的接口。
func doBad() error {
	var p *myError = nil
	return p // iface: tab != nil, data == nil
}

// demoNilInterface 笔记 03 §3：nil 指针装箱进接口后 != nil。
func demoNilInterface() {
	err := doBad()
	fmt.Printf("err != nil: %v（动态类型=%T）—— 预期 nil 却不是 nil，经典事故\n", err != nil, err)

	v := reflect.ValueOf(err)
	fmt.Printf("reflect 判断：Kind=%v IsNil=%v（先判 Kind 再 IsNil，非指针类会 panic）\n", v.Kind(), v.IsNil())

	// 正确写法一：想返回 nil 就显式 return nil（错误路径直接返回 nil）
	// 正确写法二：调用方拿到 error 后用 errors.Is/As，而不是与 nil 玩花样
}

// demoTypeAssertion 笔记 03 §5：断言失败的三种姿态。
func demoTypeAssertion() {
	var i any = 42

	v := i.(int)
	fmt.Printf("断言成功：%v\n", v)

	if s, ok := i.(string); ok {
		_ = s
	} else {
		fmt.Println("comma-ok 失败：不 panic，ok=false（推荐写法）")
	}

	func() {
		defer func() {
			if r := recover(); r != nil {
				fmt.Println("单值断言失败会 panic（可 recover，但别依赖它做控制流）：", r)
			}
		}()
		_ = i.(string) // panic: interface conversion
	}()

	switch x := i.(type) {
	case string:
		_ = x
	case int:
		fmt.Println("type switch 命中 int 分支：", x)
	}
}

// user 含导出与非导出字段，用于 reflect 观察。
type user struct {
	Name string // 导出字段
	age  int    // 非导出字段
}

// demoReflectFields 笔记 03 §6：CanSet / CanInterface 由可寻址性与导出性决定。
func demoReflectFields() {
	u := user{Name: "tal", age: 30}
	v := reflect.ValueOf(&u).Elem() // 取 Elem() 才可寻址（CanSet 的前提）
	t := v.Type()

	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		fv := v.Field(i)
		fmt.Printf("%-5s 导出=%-6v CanSet=%-6v CanInterface=%-6v Kind=%v\n",
			f.Name, f.IsExported(), fv.CanSet(), fv.CanInterface(), fv.Kind())
	}

	v.Field(0).SetString("review") // 导出 + 可寻址：Set 成功
	fmt.Printf("SetString 后 u.Name=%q（Set 直接写回了原变量）\n", u.Name)
	fmt.Printf("非导出字段读没问题：v.Field(1).Int()=%d（但 Set/Interface 不行）\n", v.Field(1).Int())

	func() {
		defer func() {
			if r := recover(); r != nil {
				fmt.Println("对非导出字段 SetInt：", r)
			}
		}()
		v.Field(1).SetInt(31) // panic: using value obtained using unexported field
	}()
}

// demoJSONNilEmpty 笔记 01 §9 / 03：nil 切片序列化为 null，空切片为 []。
func demoJSONNilEmpty() {
	var nilSlice []int
	emptySlice := []int{}

	b1, _ := json.Marshal(nilSlice)
	b2, _ := json.Marshal(emptySlice)
	fmt.Printf("nil 切片 -> %s\n", b1) // null
	fmt.Printf("空 切片 -> %s\n", b2)   // []
	fmt.Println("API 返回数组字段时 nil 会变成 null —— 需要空数组就显式初始化 []int{}")
}

// demoUnsafeString 笔记 03 §8：unsafe.String/unsafe.SliceData 零拷贝及红线。
func demoUnsafeString() {
	b := []byte("hello")
	s := unsafe.String(unsafe.SliceData(b), len(b)) // 零拷贝：直接把 b 的内存当 string
	fmt.Printf("零拷贝 string=%q（与 b 共享同一块内存）\n", s)

	// 红线：string 语义上不可变 —— 修改 b 会连带改掉 s，破坏只读假设
	b[0] = 'H'
	fmt.Printf("修改 b 后 s=%q（同一块内存被改了 —— 这就是不能乱用的原因）\n", s)
}
