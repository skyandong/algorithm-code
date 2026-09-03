# Interface 与反射

> **核心认知：** interface 变量是「**(动态类型, 动态值)**」的二元组——空接口 `any` 是 eface（类型指针+数据指针），非空接口是 iface（itab+数据指针）。所有经典事故都源于忘记这个二元组：`nil != nil` 的坑是「有类型无值」，方法集的坑是「T 与 *T 不同」，装箱的坑是「拷贝+逃逸」。反射则是把这个二元组及其类型信息暴露给运行时——**用运行时信息换取通用性，代价是失去编译期检查和优化**。

按 Go 1.26 语义说明。

---

## 1. iface / eface：接口的底层表示

```go
// runtime/runtime2.go（Go 1.26）
type iface struct {          // 非空接口（有方法集的）
	tab  *itab               // 类型 + 方法表
	data unsafe.Pointer      // 指向动态值（或其副本）
}

type eface struct {          // 空接口 any / interface{}
	_type *abi.Type
	data  unsafe.Pointer
}

// itab（编译期生成、全局只读、runtime 缓存复用）
// type itab struct {
//   inter  *interfacetype // 接口类型
//   _type  *_type         // 动态具体类型
//   hash   uint32         // type switch 的快路径
//   fun    [1]uintptr     // 变长方法表（按接口方法名排序）
// }
```

要点：

- `var i Iface = x` 装箱后，`i` 是 2 个机器字的胖指针：**itab 回答「你是谁、有哪些方法」，data 回答「值在哪」**；
- itab 全局唯一且只读（`(接口类型, 动态类型)` 二元组决定），断言/调用走内存间接寻址——这是接口调用比直接调用慢的根源；
- `data` 是**指针**：小值直接存指针指向的副本，大值同样拷贝一份再存地址（见 §4）。

内存布局示意：

```text
var i error = &MyErr{42}

  i (iface)                    itab                        MyErr 实例
 ┌──────────┐    ┌──────────────────────┐    ┌─────────────────┐
 │ tab  ────┼───▶│ inter  → error 类型   │    │ code: 42        │
 │ data ────┼───▶│ _type  → *MyErr 类型  │    └─────────────────┘
 └──────────┘    │ hash   → 快路径      │                ▲
                 │ fun[0] → (*MyErr).Error│    data ────────┘
                 └──────────────────────┘
```

推论：`i == nil` 比较的是「tab 和 data 双双为 nil」——这个事实是 §3 所有事故的根源。

---

## 2. 值接收者 vs 指针接收者：方法集

语言规范：

- **T 的方法集**：仅含接收者为 `T` 的方法；
- **\*T 的方法集**：含接收者为 `T` 和 `*T` 的全部方法。

```go
type R struct{}

func (R) ByValue()  {}
func (*R) ByPointer() {}

v := any(R{})
p := any(&R{})
_, ok1 := v.(interface{ ByValue() })   // true
_, ok2 := v.(interface{ ByPointer() })  // false ← T 不满足含指针方法的接口
_, ok3 := p.(interface{ ByValue() })   // true
_, ok4 := p.(interface{ ByPointer() }) // true
```

WHY：装箱进接口的 `T` 值是**不可寻址的副本**，编译器无法安全地替它取址调用 `*T` 方法（`*T` 方法可能修改原对象，改副本没有意义还骗你）；而 `*T` 装箱后，通过指针既能调值方法（自动解引用）也能调指针方法。

语法糖提示：可寻址变量调用指针接收者方法 `r.ByPointer()` 时，编译器自动 `(&r).ByPointer()`，所以**普通调用从不出错，只有接口赋值/断言时才暴露方法集差异**——这是坑的隐蔽之处。

工程规则：实现了 `Error()/String()` 这类指针接收者方法的类型，返回值必须一直用 `*T`（标准库 `func (e *MyErr) Error()` → 永远 `return &MyErr{}` 或 `return nil`）。

---

## 3. nil 接口坑（线上事故经典）

```go
type MyErr struct{ code int }
func (e *MyErr) Error() string { return "boom" }

func do() error {
	var p *MyErr = nil
	return p // 「有类型无值」的接口
}

err := do()
fmt.Println(err != nil) // true ！
```

WHY：接口的 nil 判断比较的是**二元组整体**。`var p *MyErr = nil; var err error = p` 之后，iface 的 `tab` 指向 `(*MyErr, error)` 的 itab（非 nil），`data == nil`。`err != nil` 为 true，因为它有类型。**只有 tab 和 data 都为 nil，接口才 == nil。**

事故形态：错误分支里 `var p *MyErr; ...; return p`，调用方 `if err != nil` 永真，走进「有错误」的逻辑；更糟的是后续 `err.Error()` 触发 nil 指针解引用 panic。

正确写法：

```go
// 写法一（根治）：想返回 nil 就写 nil
func do() error {
	var p *MyErr
	if bad {
		return nil // 而不是 return p
	}
	return &MyErr{code: 42}
}

// 写法二（调用方兜底）：先判 Kind 再 IsNil（非指针/chan/map/slice/func 的 Kind 调 IsNil 会 panic）
v := reflect.ValueOf(err)
if v.Kind() == reflect.Ptr && v.IsNil() { /* 实际是 nil 错误 */ }
```

变体坑：把 `*T` 存进 `map[K]Iface`、塞进结构体字段再判 nil，同因同果。原则：**接口变量的 nil 判断只在「直接赋 nil」时可信**。

---

## 4. 接口装箱：值拷贝与逃逸

装箱 = 把值**拷贝一份**，让 `data` 指向它：

```go
p := point{X: 10, Y: 20}
var boxed any = p
p.X = 999
fmt.Println(boxed) // {10 20} ← 接口里是装箱那一刻的副本
```

三层成本：

1. **拷贝**：大结构体装箱 = 一次全量拷贝（1 KiB 的值按值装箱，比传指针多搬上百倍字节）；
2. **逃逸**：装进接口的值通常逃逸到堆（`data` 指针的生命周期超出栈帧）。实证：

```text
$ go build -gcflags=-m
./main.go:9:17:  b escapes to heap      ← 传给 fmt.Println(...any)
./main.go:10:14: b does not escape      ← 仅本地 concrete 使用
```

3. **GC 压力**：`fmt.Println(x)` 的 `...any` 参数，天然把每个实参装箱 + 逃逸——热路径日志里的结构体就是这么进堆的（这也是 slog `LogValuer`、zap `ObjectMarshaler` 想解决的问题之一）。

工程对策：热路径避免装箱——泛型（1.18+，编译期特化零装箱）、传具体类型、小对象值传递前先想一次拷贝成本。

---

## 5. 类型断言 / type switch 的开销

```go
v := i.(T)        // 失败 → panic（运行时错误，可 recover）
v, ok := i.(T)    // comma-ok：失败不 panic，推荐
switch x := i.(type) { case T1: ... } // type switch
```

```go
var i any = 42

n := i.(int)              // 42，直接用
if s, ok := i.(string); ok {
	fmt.Println(s)        // 不会进
} else {
	fmt.Println(ok)       // false ← comma-ok：失败优雅降级
}

// 反例：单值断言失败 → panic: interface conversion: interface {} is int, not string
// recover 能接住，但用 recover 做控制流是最差的写法

switch x := i.(type) {    // x 在每个 case 里被自动断言为该类型
case string:
	fmt.Println("string:", x)
case int:
	fmt.Println("int:", x) // 命中
}
```

开销构成：

- eface→具体类型：比较 `_type` 指针，几个 ns，接近免费；
- iface→具体类型：比较 itab 里的 `_type`，同样便宜；
- iface→另一个接口：需要查「目标接口 × 动态类型」的 itab，runtime 有全局 itab 缓存（哈希表），命中快但仍是内存间接；
- type switch 用 itab 里的 `hash`（动态类型哈希的副本）先快速排除不匹配的 case。

结论：**有快路径，但非零成本**（内存间接 + 潜在缓存查找 + 编译器无法内联）。日常代码随便用；纳秒级热路径（每秒千万次）里，改用泛型或具体类型分发。

---

## 6. reflect：TypeOf / ValueOf 与 Value 的能力

```go
t := reflect.TypeOf(i)  // 拿 *rtype（类型的元数据）：NumField/Field/Kind...
v := reflect.ValueOf(i) // 拿值的句柄：Int()/String()/Set.../Interface()
```

**Value 的三个能力位**决定你能对它做什么：

| 能力 | 条件 | 典型拦截点 |
| --- | --- | --- |
| 可读（`Int()`/`String()`） | 总是可以 | —— |
| `CanInterface()` | 值不是「只读标记」（flagRO） | **非导出字段**取出的 Value 为 false，`Interface()` 会 panic |
| `CanSet()` | 可寻址 **且** 非 flagRO | `ValueOf(x)` 不可寻址；非导出字段不可 Set |

```go
u := user{Name: "tal", age: 30}           // type user struct{ Name string; age int }
v := reflect.ValueOf(&u).Elem()           // 必须经指针取 Elem() 才可寻址

v.Field(0).CanSet()        // true  （Name 导出 + 可寻址）
v.Field(1).CanSet()        // false （age 非导出 → flagRO）
v.Field(1).CanInterface()  // false
v.Field(1).Int()           // 30   （读不受限）
v.Field(0).SetString("x") // 成功，直接写回 u
v.Field(1).SetInt(31)     // panic: using value obtained using unexported field
```

WHY 非导出字段只读：reflect 是普通库，必须维持包级封装语义——否则任何外部代码都能改写私有字段。绕过手段存在（`unsafe` + `Field(i).UnsafeAddr()`），但等于放弃语言保护。

另一个高频陷阱：`reflect.ValueOf(u)` 拿到的是 `u` 的**副本**，Set 的是副本——想改原值必须 `ValueOf(&u).Elem()`。

---

## 7. reflect 的性能开销与工程应用

慢的三个原因（WHY）：

1. **分配**：ValueOf 装箱参数 → 每次调用可能产生堆分配（接口装箱 §4 的成本全继承）；
2. **间接**：读字段要经过 `Value` 结构 + offset 计算，调用方法要走 `Method(i).Call(...)` 的参数切片装箱；
3. **无内联**：编译器对反射调用完全无法内联/特化，泛型的优势全失。

工程应用（它们证明反射的价值在「边界」而非「热路径」）：

- `encoding/json`：Marshal/Unmarshal 全靠反射遍历字段 + tag 解析（性能敏感场景用 easyjson 代码生成替代，快 ~5-10 倍）；
- ORM（gorm/ent）：结构体 ↔ 表字段的映射、关系加载，启动时缓存 reflect 元数据避免重复解析；
- 依赖注入（google/wire 编译期生成、uber/fx 运行时反射装配）：按类型把依赖拼装起来；
- k8s-style 深拷贝、结构体 diff/patch。

一句话定位：**反射是用运行时信息换取通用性**。正确的性能姿势：反射只在初始化/边界做一次并缓存结果（字段 offset、方法表），热循环里全部走预编译的索引或闭包。

---

## 8. unsafe：何时出场

`unsafe.Pointer` 是编译器认可的「任意指针 ↔ 指针类型」的转换桥梁（uintptr 只是无 GC 语义的整数，**不**能用 uintptr 暂存指针跨语句传递）。合法出场：

```go
// 1. 类型双关（底层布局相同的类型互转，如 []byte ↔ 自定义类型）
// 2. string / []byte 零拷贝（Go 1.20+ 标准写法，替代社区野生写法）
b := []byte("hello")
s := unsafe.String(unsafe.SliceData(b), len(b)) // s 与 b 共享内存，零拷贝

// 反向：[]byte 视图
bs := unsafe.Slice(unsafe.StringData(s), len(s))
```

**工程红线**：

1. **绝不修改**经零拷贝得到的 string 的底层内存——string 的不可变语义是全局契约，违反可能污染共享的常量数据、破坏 map key 哈希一致性（实验演示：改 b 后 s 跟着变）；
2. 零拷贝产物**生命周期**与源绑定：b 被 GC 回收后 s 变成悬挂视图，跨函数/跨 goroutine 返回前必须确定源数据不会被复用或回收；
3. uintptr 参与的指针运算必须在一个表达式内完成（GC 可能在语句间移动或回收目标）；
4. 只在序列化/网络库这类边界热路径用、集中在少数文件、写满测试—— unsafe 破坏的是内存安全，出错即崩溃或数据损坏，没有中间态。

---

本篇对应实验: experiments/03_interface_reflection.go
