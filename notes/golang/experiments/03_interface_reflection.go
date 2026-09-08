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
//	Exp4：接口装箱 —— 值拷贝语义与逃逸堆分配成本
//	Exp5：类型断言 — 单值失败 panic / comma-ok 不 panic / type switch
//	Exp6：reflect 遍历结构体字段，非导出字段 CanSet=false
//	Exp7：reflect 性能开销 —— 直接赋值 vs 缓存 Value vs 全路径
//	Exp8：unsafe.String 零拷贝与工程红线（共享内存被改）
//	附加：JSON 序列化 nil vs 空 切片（null vs []）
package main

import (
	"encoding/json"
	"fmt"
	"reflect"
	"runtime"
	"time"
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

	fmt.Println("\n========== 第4节: 接口装箱 —— 值拷贝与逃逸成本 ==========")
	demoBoxingEscape()

	fmt.Println("\n========== 第5节: 类型断言 — panic / comma-ok / type switch ==========")
	demoTypeAssertion()

	fmt.Println("\n========== 第6节: reflect 遍历字段，非导出字段 CanSet=false ==========")
	demoReflectFields()

	fmt.Println("\n========== 第7节: reflect 的性能开销 —— 缓存 Value 能省大半 ==========")
	demoReflectCost()

	fmt.Println("\n========== 第8节: unsafe 零拷贝与红线 ==========")
	demoUnsafeString()

	fmt.Println("\n========== 附加: JSON — nil 切片 null vs 空切片 [] ==========")
	demoJSONNilEmpty()
}

// point 用于观察装箱拷贝。
type point struct{ X, Y int }

// boxingSink 强制装箱对象逃逸（不写全局的话编译器可能优化掉装箱）。
var boxingSink any

// demoBoxingEscape 笔记 03 第 4 节：装箱 = 值拷贝；逃逸时每次装箱都堆分配。
func demoBoxingEscape() {
	// 值拷贝：装箱后改原值，接口里仍是装箱那一刻的副本
	p := point{X: 1, Y: 2}
	var i any = p // 拷贝一份 point 进接口
	p.X = 100
	fmt.Printf("装箱后改原值: i=%v 原值=%v（接口持有副本，互不影响）\n", i, p)

	// 逃逸时的装箱成本：64B 结构体装箱 → 拷贝 64B + 堆分配
	type big struct{ buf [64]byte }
	var before, after runtime.MemStats
	const n = 100000
	runtime.GC()
	runtime.ReadMemStats(&before)
	for k := 0; k < n; k++ {
		var b big
		b.buf[0] = byte(k)
		boxingSink = b // 装箱并逃逸 → 每次分配一个装箱对象
	}
	runtime.ReadMemStats(&after)
	boxingSink = nil
	fmt.Printf("64B 结构体装箱×10万: 堆分配 %d KB（值拷贝进装箱对象 + 堆分配；小整数 0~255 有静态缓存例外）\n",
		(after.TotalAlloc-before.TotalAlloc)/1024)
	fmt.Println("工程推论: 热路径上 interface 参数 = 隐性拷贝 + 分配；泛型或具体类型可省（见 04 实验第 6 节对照）")
}

// demoReflectCost 笔记 03 第 7 节：reflect 慢在每次装箱/查找/分发，缓存 Value 省大半。
func demoReflectCost() {
	type user struct {
		Name string
		Age  int
	}
	u := &user{}
	const n = 1000000

	start := time.Now()
	for i := 0; i < n; i++ {
		u.Age = i // 直接赋值：一条 MOV
	}
	direct := time.Since(start)

	// 缓存 reflect.Value：只查一次字段，之后循环里只剩 SetInt
	fv := reflect.ValueOf(u).Elem().FieldByName("Age")
	start = time.Now()
	for i := 0; i < n; i++ {
		fv.SetInt(int64(i))
	}
	cached := time.Since(start)

	// 全路径：每次循环都 ValueOf + Elem + FieldByName（反射的典型反面写法）
	start = time.Now()
	for i := 0; i < n; i++ {
		reflect.ValueOf(u).Elem().FieldByName("Age").SetInt(int64(i))
	}
	full := time.Since(start)

	fmt.Printf("赋值×100万: 直接=%v  缓存 reflect.Value=%v  全路径(ValueOf+FieldByName)=%v\n",
		direct, cached, full)
	fmt.Println("WHY: 全路径每次都走类型解析+字段查找+装箱；缓存后只剩 SetInt 的类型检查")
	fmt.Println("工程姿势: 缓存 Field/Type 元数据；序列化等重反射场景用代码生成（easyjson）或 unsafe 偏移")
}

// demoInterfacePair 笔记 03 第 1 节：接口变量持有 (类型, 值副本)。
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

// demoMethodSet 笔记 03 第 2 节：T 的方法集不含指针接收者方法，*T 含全部。
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

// demoNilInterface 笔记 03 第 3 节：nil 指针装箱进接口后 != nil。
func demoNilInterface() {
	err := doBad()
	fmt.Printf("err != nil: %v（动态类型=%T）—— 预期 nil 却不是 nil，经典事故\n", err != nil, err)

	v := reflect.ValueOf(err)
	fmt.Printf("reflect 判断：Kind=%v IsNil=%v（先判 Kind 再 IsNil，非指针类会 panic）\n", v.Kind(), v.IsNil())

	// 正确写法一：想返回 nil 就显式 return nil（错误路径直接返回 nil）
	// 正确写法二：调用方拿到 error 后用 errors.Is/As，而不是与 nil 玩花样
}

// demoTypeAssertion 笔记 03 第 5 节：断言失败的三种姿态。
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

// demoReflectFields 笔记 03 第 6 节：CanSet / CanInterface 由可寻址性与导出性决定。
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

// demoJSONNilEmpty 笔记 01 第 9 节 / 03：nil 切片序列化为 null，空切片为 []。
func demoJSONNilEmpty() {
	var nilSlice []int
	emptySlice := []int{}

	b1, _ := json.Marshal(nilSlice)
	b2, _ := json.Marshal(emptySlice)
	fmt.Printf("nil 切片 -> %s\n", b1) // null
	fmt.Printf("空 切片 -> %s\n", b2)   // []
	fmt.Println("API 返回数组字段时 nil 会变成 null —— 需要空数组就显式初始化 []int{}")
}

// demoUnsafeString 笔记 03 第 8 节：unsafe.String/unsafe.SliceData 零拷贝及红线。
func demoUnsafeString() {
	b := []byte("hello")
	s := unsafe.String(unsafe.SliceData(b), len(b)) // 零拷贝：直接把 b 的内存当 string
	fmt.Printf("零拷贝 string=%q（与 b 共享同一块内存）\n", s)

	// 红线：string 语义上不可变 —— 修改 b 会连带改掉 s，破坏只读假设
	b[0] = 'H'
	fmt.Printf("修改 b 后 s=%q（同一块内存被改了 —— 这就是不能乱用的原因）\n", s)
}
