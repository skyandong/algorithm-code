// 实验 03（interface 与反射），断言验证
// 对应笔记：notes/golang/02-interface与反射.md
// 源码对照：src/runtime/runtime2.go
package main

import (
	"encoding/json"
	"fmt"
	"reflect"
	"runtime"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/assert"
)

// ===== 实验实现（原 03_interface_reflection.go，已并入本文件）=====

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

// ===== 断言用例 =====

func TestInterfacePairIsValueCopy(t *testing.T) {
	var i any = point{1, 2}
	assert.Equal(t, point{1, 2}, i)
	assert.Equal(t, "main.point", reflect.TypeOf(i).String(), "接口持有动态类型")

	p := point{X: 10, Y: 20}
	var boxed any = p
	p.X = 999
	assert.Equal(t, point{10, 20}, boxed, "装箱是值拷贝，改原值不影响接口内副本")
}

func TestBoxingAllocates(t *testing.T) {
	type big struct{ buf [64]byte }
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

func TestNilInterfaceTrap(t *testing.T) {
	err := doBad()
	assert.True(t, err != nil, "有类型无值的接口 != nil——经典事故")
	assert.Nil(t, (*myError)(nil), "裸指针本身是 nil")

	// 注意：reflect 视角下这个接口是「nil」的，testify 的 assert.NotNil 也会被骗——
	// 所以判 nil 一律用 err != nil，别用反射。
	v := reflect.ValueOf(err)
	assert.Equal(t, reflect.Ptr, v.Kind())
	assert.True(t, v.IsNil(), "IsNil 看的是底层指针，所以先判 Kind 再判 IsNil")
}

func TestTypeAssertion(t *testing.T) {
	var i any = 42

	assert.Equal(t, 42, i.(int))

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

func TestReflectFields(t *testing.T) {
	u := user{Name: "tal", age: 30}
	v := reflect.ValueOf(&u).Elem()

	assert.True(t, v.Type().Field(0).IsExported())
	assert.True(t, v.Field(0).CanSet())

	assert.False(t, v.Type().Field(1).IsExported())
	assert.False(t, v.Field(1).CanSet(), "非导出字段不可 Set")
	assert.False(t, v.Field(1).CanInterface())
	assert.Equal(t, 30, int(v.Field(1).Int()), "非导出字段 readable")

	v.Field(0).SetString("review")
	assert.Equal(t, "review", u.Name, "Set 直接写回原变量")

	assert.Panics(t, func() { v.Field(1).SetInt(31) })
}

func TestJSONNilVsEmptySlice(t *testing.T) {
	var nilSlice []int
	emptySlice := []int{}

	b1, err := json.Marshal(nilSlice)
	assert.NoError(t, err)
	assert.Equal(t, "null", string(b1), "nil 切片序列化为 null")

	b2, err := json.Marshal(emptySlice)
	assert.NoError(t, err)
	assert.Equal(t, "[]", string(b2), "空切片序列化为 []")
}

func TestUnsafeStringZeroCopy(t *testing.T) {
	b := []byte("hello")
	s := unsafe.String(unsafe.SliceData(b), len(b))

	assert.Equal(t, "hello", s)
	assert.True(t, unsafe.StringData(s) == unsafe.SliceData(b), "与 b 共享同一块内存")

	b[0] = 'H'
	assert.Equal(t, "Hello", s, "改 b 连带改 s——零拷贝红线")
}

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
