// 实验 02（interface 与反射），断言验证
// 对应笔记：notes/golang/02-interface与反射.md
// 源码对照：src/runtime/runtime2.go
package main

import (
	"fmt"
	"reflect"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

// point 用于观察装箱拷贝。
type point struct {
	X, Y int
}

// TestInterfacePairIsValueCopy 第 1 节：接口是 (类型, 值副本) 二元组
func TestInterfacePairIsValueCopy(t *testing.T) {
	var i any = point{1, 2}
	assert.Equal(t, "main.point", reflect.TypeOf(i).String(), "接口持有动态类型")

	p := point{X: 10, Y: 20}
	var boxed any = p
	p.X = 999
	assert.Equal(t, point{10, 20}, boxed, "装箱是值拷贝，改原值不影响接口内副本")
}

// receiver 同时具备值接收者与指针接收者方法。
type receiver struct{}

// ByValue 值接收者方法。
func (receiver) ByValue() {}

// ByPointer 指针接收者方法。
func (*receiver) ByPointer() {}

// TestMethodSet 第 2 节：T 的方法集不含指针接收者方法
func TestMethodSet(t *testing.T) {
	v := any(receiver{})
	p := any(&receiver{})

	_, valueImplValue := v.(interface{ ByValue() })
	_, valueImplPtr := v.(interface{ ByPointer() })
	_, ptrImplValue := p.(interface{ ByValue() })
	_, ptrImplPtr := p.(interface{ ByPointer() })

	assert.True(t, valueImplValue)
	assert.False(t, valueImplPtr, "T 的方法集不含指针接收者方法")
	assert.True(t, ptrImplValue, "*T 的方法集含值接收者方法")
	assert.True(t, ptrImplPtr)
}

// myError 模拟线上事故：返回 *myError(nil) 时接口非 nil。
type myError struct {
	code int
}

// Error 实现 error 接口（指针接收者）
func (e *myError) Error() string {
	return fmt.Sprintf("code %d", e.code)
}

// doBad 反面教材：返回一个「有类型无值」的接口。
func doBad() error {
	var p *myError = nil
	return p // iface: tab != nil, data == nil
}

// TestNilInterfaceTrap 第 3 节：有类型无值的接口 != nil
func TestNilInterfaceTrap(t *testing.T) {
	err := doBad()
	assert.True(t, err != nil, "有类型无值的接口 != nil——经典事故（裸指针是 nil，装进接口就不是了）")

	// 注意：reflect 视角下这个接口是「nil」的，testify 的 assert.NotNil 也会被骗——
	// 所以判 nil 一律用 err != nil，别用反射。
	v := reflect.ValueOf(err)
	assert.Equal(t, reflect.Ptr, v.Kind())
	assert.True(t, v.IsNil(), "IsNil 看的是底层指针，所以先判 Kind 再判 IsNil")
}

// boxingSink 强制装箱对象逃逸（不写全局的话编译器可能优化掉装箱）。
var boxingSink any

// TestBoxingAllocates 第 4 节：装箱逃逸后每次都要堆分配
func TestBoxingAllocates(t *testing.T) {
	type big struct {
		buf [64]byte
	}
	const n = 100000

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	for k := 0; k < n; k++ {
		var b big
		b.buf[0] = byte(k)
		boxingSink = b // 装箱并逃逸：每次分配一个装箱对象
	}
	runtime.ReadMemStats(&after)
	boxingSink = nil

	assert.GreaterOrEqual(t, after.TotalAlloc-before.TotalAlloc, uint64(n*64),
		"64B 结构体装箱逃逸：每次搬 64B 到堆")
}

// TestTypeAssertion 第 5 节：单值断言 panic，comma-ok 与 type switch 不 panic
func TestTypeAssertion(t *testing.T) {
	var i any = 42

	s, ok := i.(string)
	assert.False(t, ok)
	assert.Empty(t, s, "comma-ok 失败不 panic")

	assert.Panics(t, func() { _ = i.(string) }, "单值断言失败 panic")

	hit := ""
	switch x := i.(type) {
	case int:
		hit = "int"
		assert.Equal(t, 42, x)
	default:
		hit = "default"
	}
	assert.Equal(t, "int", hit, "type switch 命中 int 分支")
}

// user 含导出与非导出字段，用于 reflect 观察。
type user struct {
	Name string // 导出字段
	age  int    // 非导出字段
}

// TestReflectFields 第 6 节：可设置性由可寻址性与导出性共同决定
func TestReflectFields(t *testing.T) {
	u := user{Name: "tal", age: 30}
	v := reflect.ValueOf(&u).Elem()

	assert.True(t, v.Field(0).CanSet())
	assert.False(t, reflect.ValueOf(u).Field(0).CanSet(), "值不可寻址：连导出字段也 Set 不了")

	assert.False(t, v.Field(1).CanSet(), "非导出字段不可 Set")
	assert.False(t, v.Field(1).CanInterface())
	assert.Equal(t, 30, int(v.Field(1).Int()), "非导出字段 readable")

	v.Field(0).SetString("review")
	assert.Equal(t, "review", u.Name, "Set 直接写回原变量")

	assert.Panics(t, func() { v.Field(1).SetInt(31) })
}

// reflectPerfUser 反射开销对照用：字段必须导出，否则 SetInt 会 panic。
type reflectPerfUser struct {
	Name string
	Age  int
}

// BenchmarkReflectSetInt 第 7 节：直接赋值 / 缓存 Value / 全路径三档开销
func BenchmarkReflectSetInt(b *testing.B) {
	u := &reflectPerfUser{}

	b.Run("direct", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			u.Age = i
		}
	})
	b.Run("cached-value", func(b *testing.B) {
		fv := reflect.ValueOf(u).Elem().FieldByName("Age")
		for i := 0; i < b.N; i++ {
			fv.SetInt(int64(i))
		}
	})
	b.Run("full-path", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			reflect.ValueOf(u).Elem().FieldByName("Age").SetInt(int64(i))
		}
	})
}
