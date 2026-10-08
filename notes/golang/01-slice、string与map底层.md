# slice、string 与 map 底层

> **核心认知：** slice、string、map 都不是「容器」，而是指向底层存储的**视图句柄**——复制它们拷贝的是头，不是数据。坑只有两类：**多个头共享同一块存储**，以及**运行时什么时候替你换一块新存储**（扩容、搬迁）。

本文基于 Go 1.26（本机 1.26.3）的实际行为编写，不涉及历史版本对比。

---

## 目录

1. [slice 与 string 的头结构](#1-slice-与-string-的头结构)
2. [共享底层数组：三个经典坑](#2-共享底层数组三个经典坑)
3. [扩容：换数组 + 搬元素](#3-扩容换数组--搬元素)
4. [切片的内存泄漏点](#4-切片的内存泄漏点)
5. [string 不可变：契约与三个推论](#5-string-不可变契约与三个推论)
6. [string ↔ []byte：拷贝的必然与例外](#6-string--byte拷贝的必然与例外)
7. [string 拼接：+ / Builder / 预分配](#7-string-拼接--builder--预分配)
8. [rune 与 UTF-8：len 的歧义](#8-rune-与-utf-8len-的歧义)
9. [string 编译器优化清单](#9-string-编译器优化清单)
10. [string 与 map：为什么是天生 key](#10-string-与-map为什么是天生-key)
11. [map 底层：Swiss Table](#11-map-底层swiss-table)
12. [map 无序与 for range 语义](#12-map-无序与-for-range-语义)
13. [map 并发读写：fatal error，不可 recover](#13-map-并发读写fatal-error不可-recover)
14. [空切片 vs nil 切片](#14-空切片-vs-nil-切片)
15. [unsafe：零拷贝的桥梁与红线](#15-unsafe零拷贝的桥梁与红线)

---

## 1. slice 与 string 的头结构

slice 在运行时就是一个三字段结构体：

```go
// src/runtime/slice.go
type slice struct {
	array unsafe.Pointer // 指向底层数组
	len   int
	cap   int
}
```

string 同理，只少一个 cap：

```go
// src/runtime/string.go
type stringStruct struct {
	str unsafe.Pointer // 指向底层只读字节数组
	len int            // 字节数，不是字符数
}
```

差别就在这个 cap 上。slice 的元素可以改写、`append` 还可能扩容，所以它的头必须记住容量；string 头则没有容量字段——内容是**不可变的 `[]byte` 视图**，没有 `\0` 终止符，`len` 就是字节数，任何「修改」都是新分配。

slice.go 这个文件就是 slice 的运行时中枢：`make` 分配（makeslice 系列）、`append` 扩容（growslice 系列，策略见第 3 节）、`copy`（slicecopy）全在这里。

`s[1:]` 这类切片表达式只是新造一个头——`array` 前移一个元素、`len`/`cap` 各减 1，旧数组不动：

```go
s := []int{10, 20, 30}
sub := s[1:]          // 新 header，共享同一数组
sub[0] = 99
fmt.Println(s)         // [10 99 30] ← s 被一起改了
```

两索引 `s[low:high]` 的规则只有三条：

1. **边界**：`0 <= low <= high <= cap(s)`，上界看 cap 不看 len（len=0 cap=4 时 `s[3:4]` 合法）；
2. **省略**：low 默认 0；high 默认 `len(s)` 而不是 cap——所以 len=0 时 `s[1:]` 会 panic，想取到 cap 末尾得写 `s[1:cap(s)]`；
3. **结果**：len = high − low，cap = cap(s) − low。

以 `s := make([]int, 0, 4)`（len=0 cap=4）为例：

| 表达式 | len | cap |
|--------|-----|-----|
| `s[:1]` | 1 | 4 |
| `s[1:2]` | 1 | 3 |
| `s[1:1]` | 0 | 3 |
| `s[3:4]` | 1 | 1 |
| `s[1:3:3]` | 2 | 2 |

**三索引 `s[low:high:max]`：把 cap 钉死为 `max - low`。**

- 目的只有一个：后续 `append` 必然超 cap、分配新数组，不写原数组——交出子切片又不许对方误伤时，比完整 `copy` 更省（零拷贝，只改头）；
- 约束 `high <= max <= cap(s)`，**只有 low 可省**：`s[1::4]` 直接编译错误（`middle index required`）；
- 第三个索引只管 cap，读写越界与否仍由 len 决定。

string 的子串同理零拷贝：

```go
s := "hello world"
sub := s[:5] // 零拷贝
p1, p2 := unsafe.StringData(s), unsafe.StringData(sub)
fmt.Println(p1 == p2) // true —— 共享同一块底层内存
```

没有 `\0` 意味着 string 可存**任意二进制数据**（包括 `\0` 本身）。注意持有子串会钉住整个大字符串——长生命周期场景该拷贝就用 `strings.Clone`。

string 的完整展开（不可变契约、互转拷贝、拼接、rune/UTF-8）见第 5~10 节。

---

## 2. 共享底层数组：三个经典坑

### 2.1 未扩容时 append 互相覆盖

```go
a := make([]int, 3, 6) // len=3 cap=6，还有空位
b := append(a, 99)     // 未超 cap：b 仍指向 a 的数组
c := append(a, 100)    // 写同一个槽位
fmt.Println(b[3], c[3]) // 100 100 ← c 把 b 的 99 顶掉了
```

WHY：`b`、`c` 与 `a` 共享底层数组，两次 append 都写到 `a` 数组的第 4 格。**只有当 cap 不够、触发扩容分配新数组后，才真正分离。**

### 2.2 扩容后才分离

```go
d := append(c, 1, 2, 3, 4) // 需要 len=8 > cap=6：分配新数组
// &c[0] != &d[0]，cap 6 -> 12；此后改 d 不影响 c
```

### 2.3 函数内 append 不影响调用方

```go
func appendInside(s []int) { s = append(s, 7) } // 只改了「副本头」的 len

s := make([]int, 0, 4)
appendInside(s)
fmt.Println(len(s)) // 0
fmt.Println(s[:1])   // [7] ← 数据确实写进共享数组了，只是调用方 len 没变
```

WHY：参数 `s` 是 header 的值拷贝，append 改的是副本的 `len` 字段。正确姿势：**返回新 slice**（`return append(s, x)`，标准库 `appendBytes` 风格），或传 `*[]int`。

坑位速查：

| 场景 | 行为 | 根因 |
| --- | --- | --- |
| `b := append(a, x)` 且未超 cap | b 与 a 共享数组 | 只写了数组，没换 header 的 array |
| `a[i] = v` 透过子切片改值 | 所有视图可见 | 共享 array 指针 |
| 函数内 append | 调用方 len 不变 | header 值传递 |
| `s = append(s, x)` 后旧引用 | 旧 slice 仍指旧数组 | 扩容换新数组，旧 header 不知情 |

---

## 3. 扩容：换数组 + 搬元素

底层数组**长度固定**，所以「扩容」这个名字本身就是误导——它做的不是把数组撑大，而是分配一块更大的新数组，把旧元素搬过去，再返回一个新 header。

`runtime.growslice` 去掉容量策略后的三条主干：

```go
// src/runtime/slice.go
p = mallocgc(capmem, et, ...)    // ① 新分配一块数组
memmove(p, oldPtr, lenmem)       // ② 搬旧元素：lenmem = oldLen × 元素宽度
return slice{p, newLen, newcap}  // ③ 返回新 header：新 ptr、新 cap
```

由此得到三条**不随版本变**的结论：

- **是拷贝，不是原地扩张。** 新 slice 指向新数组，旧 slice 仍指向旧数组——扩容前建立的共享关系到此断开（第 2.2 节）。
- **搬的是元素值，即浅拷贝。** `memmove` 只按元素宽度逐位拷贝 `len` 个元素：元素是 `int` 这类标量就彻底独立；元素是 `*T` / `map` / `slice` / `interface` 这类含指针的值，拷过来的是**指针本身**，指向的对象**仍然共享**——扩容不会替你深拷贝。
- **容量给多少是实现细节，不保证。** 给少了要频繁拷贝（退化成 O(n²)），给多了浪费内存，所以策略永远在「摊销常数时间」和「空间浪费」之间权衡；而且分配器还会把字节数向上对齐到自己的规格，观察到的 cap 往往不是你能算出来的数——具体值随版本、元素大小、是否逃逸而变。**要确定容量就 `make([]T, 0, n)` 预分配，不要赌倍数。**

---

## 4. 切片的内存泄漏点

**整段清空：`a = a[:0]` / `a = nil` / `clear(a)` 语义不同**：

```go
a = a[:0] // len=0、cap 不变：底层数组保留复用，后续 append 不再分配
a = nil   // len=cap=0：断开引用，无别名时整个数组可回收；下次 append 重新分配
clear(a)  // len 不变：元素全部归零；要的是"归零"而不是"变短"时用
```

- `a = nil` 只断开**这一个 slice 头**：子 slice、其他别名仍钉住整块数组；且它不清数据；
- `a[:0]` 复用是缓冲区高频优化，但元素含指针时残留引用会阻止 GC——重置前先 `clear(a[:oldLen])`；
- 含敏感数据（token、密钥）的 slice，卫生清理靠 `clear`；`a[:0]` 和 `a = nil` 都只是改引用。

**大数组切小 slice 的泄漏**（读大文件、解析大 buffer 后只留一小段时高频出现）：

```go
big := make([]byte, 1<<20)      // 1 MiB
tail := big[len(big)-2:]         // 只留末尾 2 字节
// tail 的 array 指针指进 big 的分配块内部 → 整个分配对象都算存活，1 MiB 全收不掉
fixed := make([]byte, len(tail))
copy(fixed, tail)               // 修法：copy 出独立小数组；string 对应 strings.Clone
```

---

## 5. string 不可变：契约与三个推论

`s[i] = 'x'` 会直接编译错误。不可变不是「建议」，而是**运行时全局契约**：编译器、GC、map 哈希、包级字面量驻留，全都依赖它。

**推论一：零成本共享。** 同一个 string 值随便传、随便存，没有深拷贝的心智负担（对比 `[]byte` 传递时要担心谁改它）。

**推论二：字面量驻留（intern）。** 编译期可确定的相同字符串字面量在只读段共享一份内存。`"hello" == "hello"` 两处引用的是同一块只读数据。

**推论三：写 = 新分配。** 所有「修改」API（`strings.Replace`、`+`、`ToUpper`）都返回新串。循环里 `s += x` 是 O(n²) 事故的根源（第 7 节）。

**零拷贝的边界（unsafe）**——`unsafe.String` 造出的 string 指向可变内存，**契约在你手里而非编译器手里**：改源 `[]byte` 会连带改 string（可能污染驻留字面量、破坏 map key 哈希一致性）；产物生命周期与源绑定，源被复用/回收后就是悬挂视图。写法与红线见第 15 节。

---

## 6. string ↔ []byte：拷贝的必然与例外

```go
b := []byte(s)  // 拷贝（除非编译器证明不逃逸且不被修改）
s2 := string(b) // 拷贝（同上）
```

**为什么必须拷贝？** string 不可变而 []byte 可变——共享内存意味着改 b 就「改了」s，违反契约。所以语义上互转就是复制数据。

**编译器逃生通道**（证明安全时免拷贝）：

- `[]byte(s)` 后 b **不逃逸、不被修改、s 非零拷贝产物**，且发生在同一表达式/函数内 → 编译器直接引用 s 的底层数组（`-gcflags="-m"` 能看到 `[]byte(...) does not escape`）；
- 典型受益：`for _, c := range []byte(s)`、`len([]byte(s))`（这种写法本身就该用 len(s)）。

**map 索引特例**：`m[string(b)]` **不分配**——编译器把 []byte 临时串的比较就地做掉（mapaccess_fast）。

**工程判断**：热路径上频繁 `[]byte(s)`/`string(b)` 各一次 = 每个消息两份拷贝。解法按顺序：

1. **统一内部类型**——整个流水线全用 []byte（或全用 string），只在 API 边界转一次；
2. 标准库已有 []byte 版 API 就别用 string 版：`bytes.Contains` vs `strings.Contains`、`bufio.Scanner` 的 `Bytes()`；
3. 最后才是 unsafe 零拷贝（第 15 节红线全数适用）。

```go
// 快速判断成员：map 索引免拷贝特例
m := map[string]int{"ping": 1}
key := []byte("ping")
_ = m[string(key)] // 不分配（编译器优化，实验演示）
```

---

## 7. string 拼接：+ / Builder / 预分配

```go
// ✗ O(n²)：每次 + 都新分配一个串，把旧内容拷进去
s := ""
for i := 0; i < n; i++ {
    s += parts[i]
}

// ✓ strings.Builder：内部 []byte，Append 直写，String() 零拷贝
var b strings.Builder
b.Grow(totalSize)        // 能预估就 Grow，一次性分配
for _, p := range parts {
    b.WriteString(p)
}
s := b.String()          // unsafe.String 零拷贝包装（Builder 防再次写入）
```

`strings.Builder`为什么快：

1. `WriteString` 就是 `append` 到内部 buf（拷贝检查由 `copyCheck` 保证 Builder 不被值传递滥用）；
2. `String()` 用 `unsafe.String` **零拷贝**导出——这是标准库里 unsafe 的正面示范（Builder 用 `addr` 字段记住自己，防止拷贝后两个 Builder 指向同一 buf）；
3. `Grow(n)` 预分配 = slice 扩容治理的 string 版（第 3 节同源）。

| 写法 | 复杂度 | 适用 |
|---|---|---|
| `+` 循环 | O(n²) | 禁止出现在循环里 |
| `+` 一次性 `a + b + c` | O(n)，编译器优化成一次分配 | 少量固定段，最简洁 |
| `fmt.Sprintf` | O(n) 但慢（反射+装箱） | 格式化需求，非纯拼接 |
| `strings.Builder` + Grow | O(n) | 循环拼接的标准答案 |
| `bytes.Buffer` | O(n) | 需要字节/字符串混写或读回 |

---

## 8. rune 与 UTF-8：len 的歧义

Go 源码是 UTF-8，string 存的是 **UTF-8 字节序列**（也允许存任意非法字节——string 不做校验，这是「字符串即字节」派的立场）。

```go
s := "你好Go"
len(s)                      // 8 —— 字节数（你3字节 + 好3字节 + G + o）
utf8.RuneCountInString(s)   // 4 —— 字符（码点）数
[]rune(s)                   // 拷贝+解码成 []rune{20320, 22909, 71, 111}
```

**for range string 按码点迭代**——两个变量时每轮解码一个 UTF-8 序列并交出 rune，索引是**字节偏移**（不是第几个字符）：

```go
for i, r := range "你好Go" {
    fmt.Println(i, r) // 0 你 / 3 好 / 6 G / 7 o —— 索引跳变
}

// 只写一个变量时不解码，只给按 rune 边界对齐的字节索引
for i := range "你好Go" {
    fmt.Println(i) // 0 / 3 / 6 / 7
}
```

注意 `for i := range s`（单变量）**不会**解码出 rune——它只用内部解码来定位下一个边界，rune 本身不交给你。需要手动解码时用 `utf8.DecodeRuneInString(s)`，它返回 `(rune, size)`，本身零拷贝，但调用方要自己 `s = s[size:]` 前进。

三个高频坑：

1. **按下标取的是字节不是字符**：`s[0]` 是 `0xE4`（「你」的首字节），不是「你」；
2. **截断切碎多字节字符**：`s[:4]` 把「你」和「好」的首字节切进来，产出非法 UTF-8；按字符截断要 `[]rune(s)[:n]`（有拷贝）或 `utf8.RuneStart` 定位；
3. **`[]rune(s)` 分配一个 rune 切片**——中文字符串按下标随机访问 `[]rune(s)[2]` 每次调用都重新解码分配，热路径先转一次存起来。

rune 本质：`type rune = int32`（类型别名不是新类型），表示一个 Unicode 码点。

---

## 9. string 编译器优化清单

面试加分项——「string 慢」多数是没吃到的免费优化：

1. **常量折叠**：`"a" + "b" + "c"` 编译期合成 `"abc"`，零运行时成本；
2. **`+` 多元拼接**：`a + b + c`（一个表达式）编译成 `runtime.concatstrings` 一次算总长、一次分配；
3. **比较短路**：`==` 先比长度再比内容；`len` 不同立即 false；
4. **map[string(b)] 免分配**（第 6 节）；
5. **`[]byte(s)` 不逃逸时免拷贝**：只要只遍历、不把结果留存，编译器就让切片直接借用原字符串的字节（`-m` 会打印 `zero-copy string->[]byte conversion`）。所以 `for range []byte(s)` 也是 0 分配，它和 `for i := range s` 的唯一区别是按字节还是按 rune 边界（第 8 节）。一旦逃逸，就变回真拷贝（第 6 节）；
6. **子串零拷贝**（第 1 节）；
7. **switch string**：编译器按长度+首字符建跳转表，不是线性全比。

验证手段统一是 `go build -gcflags="-m"` 看逃逸/内联判定（`08`/`10` 篇的工具链）。

---

## 10. string 与 map：为什么是天生 key

map key 的要求：**comparable**。string 完美满足且高效：

1. `==` 是**按字节 memcmp**（长度不同直接不等，长度相同比内容）——O(n) 但现实中哈希先分流，冲突时才全比；
2. **不可变 → 哈希值稳定**：key 进桶后其内容永不变，哈希一致性自动维持（可变类型做 key 是灾难：改了内容 = 哈希变了 = 丢失在旧桶）；
3. 哈希种子每进程随机（第 11 节），同内容跨进程哈希不同。

对照记忆：

| 候选 key | 能否 | 原因 |
|---|---|---|
| string | ✓ | 不可变 + memcmp |
| []byte | ✗ | slice 三字段头不可比（不可 comparable） |
| [N]byte | ✓ | 定长数组按值可比（比 string 少一次间接，网络库常用来当零分配 key） |
| 含 slice/map 字段的结构体 | ✗ | 不可比 |

`[]byte` 想当 key 的标准姿势就是 `string(b)` 转一次（第 6 节的 map 索引免拷贝特例正为此设计）。

---

## 11. map 底层：Swiss Table

三层结构，全部在 `src/internal/runtime/maps/`：

```text
Map ── 目录：按 hash 高位选表
  └── table ── 定长 group 数组
        └── group ── ctrls（控制字）+ slots（槽位）
```

### 11.1 查找路径

`hash(key)` → 用 hash 高位在目录里选表 → 表内按低位定位 group → **一次定宽比较筛掉整组里不可能命中的槽** → 只在控制字对得上的槽上真正比 key。

为什么快：Swiss Table 的核心就是把「这一组里谁可能命中」压成一次定宽比较，无效 key 的逐槽比对几乎归零。控制字存的是 hash 的一小段，够筛即可，不必比全量 hash。

小 map 优化：元素少到只占一个 group 时，目录退化为直接指向它，没有间接层。

### 11.2 负载因子：超了就 rehash

`已用槽位 + 墓碑数 > 负载因子 × 容量` → rehash：容量翻倍，或目录允许时分裂成两张表。

**负载因子是「探测长度」与「空间浪费」的折中**：开放寻址靠连续探测，留一点空槽，探测链才不至于越拖越长；留多了白占内存。单张表容量有上限，到顶只能分裂——**表故意做小，rehash 的成本天然摊薄，不会出现一次性搬大表的尖刺**。

### 11.3 删除打墓碑

删除**不清空槽位**，只在控制字上打墓碑标记——直接清成空会打断探测序列，同组后面那些 key 就再也找不到了。墓碑也计入负载，否则表会被「删出来的空槽」塞满、提前 rehash。

**容量只增不减**：删元素不释放内存，要真正释放只能换一个新 map 重建（TestMapDeleteMemory 实测）。

### 11.4 可见语义

无序遍历（顺序被刻意随机化）、for range 中删除安全、并发读写 fatal——全部由语言规范保证（第 12/13 节）。

---

## 12. map 无序与 for range 语义

**为什么每次 range 顺序都不一样**：顺序被**刻意随机化**，语言不保证任何顺序。WHY：官方有意为之，防止用户依赖顺序——一旦依赖，runtime 换实现就会炸你的代码。要顺序就显式排序 key。

range 中的修改语义（语言规范保证）：

- **删除**：安全；尚未遍历到的 entry 删除后**不会**再产出；
- **新增**：**不保证**——可能被本次遍历产出，也可能被跳过（实验三次运行分别得到 false/true/false）；
- **修改**：读到的可能是新值。

```go
for k := range m {
	visited = append(visited, k)
	if len(visited) == 1 {
		for j := 0; j < 10; j++ { delete(m, j) } // 删光其余
	}
}
// len(visited) == 1：被删的确实不再产出
```

**迭代变量每次迭代独立、且始终是值副本**：闭包与 `go` 捕获的是各自那一份，所以 `for _, v := range s { go f(v) }` 不必再写 `v := v`。但**副本这一层不会变**——对 `v` 取地址、或用指针接收者方法改它，都只改副本，原切片元素不动（实验 `TestRangeClosureValueCopy`）。

---

## 13. map 并发读写：fatal error，不可 recover

```go
m := map[int]int{}
go func() { for { m[1] = 1 } }()
go func() { for { _ = m[1] } }()
// fatal error: concurrent map read and map write
```

三个要点：

1. **这是 throw 不是 panic**：runtime 直接 `fatal`，不走 panic 机制，`defer/recover` 拦不住，进程整个退出。WHY：map 并发写会破坏内部结构（链表/控制字状态不一致），runtime 选择宁可崩溃也不带着坏数据继续跑；
2. **该检测是尽力而为**（实现里是 `writing` 标志，读写前检查并 XOR 置位）——检测不到时可能先被写坏的指针拖成 `fatal error: fault`，`-race` 才是完备工具；
3. 检测到的是「正在写时读/写」，两个 goroutine 错峰写不报错但同样危险。

**怎么让 map 并发安全**：

| 方案 | 原理 | 适用 |
| --- | --- | --- |
| `sync.Mutex` + map | 单锁 | 通用，竞争小时最简单 |
| 分片锁（N 个 shard） | key hash 到 shard，锁冲突概率降 ~1/N | 高并发写、key 分布均匀；代价：遍历/动态扩容要自己实现 |
| `sync.Map` | 见 `06` 篇第 7 节 | 读多写少 |

---

## 14. 空切片 vs nil 切片

```go
var nilSlice []int     // header: {nil, 0, 0}
emptySlice := []int{}  // header: {指向零长分配, 0, 0}
```

| 判断 | nil 切片 | 空切片 |
| --- | --- | --- |
| `len(s) == 0` | true | true（**判空用这个，两者等价**） |
| `s == nil` | true | false |
| `reflect.ValueOf(s).IsNil()` | true | false |
| `json.Marshal` | `null` | `[]` |

JSON 差异是真实业务坑：API 响应的数组字段，nil 切片序列化成 `null`，前端拿到的不是空数组会报错。需要 `[]` 就显式初始化 `[]int{}` 或 marshal 前兜底。reflect 能区分二者是因为 nil 切片的 array 指针为 nil——这也说明「nil 切片」本质是合法 slice，append、range、len 都正常工作。

---

## 15. unsafe：零拷贝的桥梁与红线

`unsafe.Pointer` 是编译器唯一认可的「任意指针 ↔ 具体指针类型」转换桥梁；`uintptr` 只是无 GC 语义的整数，**不能拿来暂存指针**。它关掉的是类型系统与 GC 语义，**合法出场只有两类**：

1. **类型双关**——把一块内存按另一种布局解释（`unsafe.String` / `unsafe.Slice` 本身就是这个用途的正解）；手写 `*(*T)(unsafe.Pointer(&x))` 只在对接 C 或固定布局协议时用，布局必须逐字段一致；
2. **零拷贝**——string 与 []byte 之间免掉那次必然的拷贝。

**两个标准 API**：

```go
b := []byte("hello")
s := unsafe.String(unsafe.SliceData(b), len(b))  // []byte → string，共享内存
bs := unsafe.Slice(unsafe.StringData(s), len(s)) // string → []byte，共享内存
bs[0] = 'H'                                      // s 变成 "Hello"
```

**别用 `reflect.StringHeader` / `reflect.SliceHeader`**：两个类型在当前版本已标 `Deprecated: Use unsafe.String or unsafe.StringData instead`（`reflect/value.go`），且它们的 `Data` 字段是 `uintptr`。

**uintptr 的唯一规矩**：`unsafe.Pointer → uintptr` 之后必须在**同一个表达式内**算完并转回指针；跨语句暂存等于让 GC 失去线索（对象可能在两句之间被搬走或回收）。正解用 `unsafe.Add`：

```go
(*T)(unsafe.Add(p, off)) // p 全程是 unsafe.Pointer，GC 始终能追踪
```

**四条红线**：

| 红线 | 后果 |
| --- | --- |
| 改零拷贝 string 的底层 | 污染驻留字面量、破坏 map key 哈希一致性（实验：改 b 后 s 跟着变） |
| 产物生命周期与源脱钩 | 悬挂视图，读到他人数据或崩溃 |
| uintptr 跨语句暂存指针 | GC 移动/回收后指针失效 |
| 散落在业务代码里 | 内存安全无法静态检查，只能靠测试兜底 |

**工程决策路径**（能停在前面就别往下走）：热路径先统一内部类型（全 string 或全 []byte，只在 API 边界转一次）→ 用标准库现成的 []byte 版 API → 最后才轮到 unsafe，且收敛在边界库的少数文件里、写满测试。

标准库正面示范：`strings.Builder.String()` 就是 `unsafe.String(unsafe.SliceData(b.buf), len(b.buf))`（`strings/builder.go`）；Builder 另用 `addr` 字段记住自己，防止按值拷贝后两个 Builder 写同一块 buf。

## 实验

对应代码：[experiments/01_1_slice_test.go](experiments/01_1_slice_test.go)、[experiments/01_2_string_test.go](experiments/01_2_string_test.go)、[experiments/01_3_map_test.go](experiments/01_3_map_test.go)（均为断言式单测，无打印）

```bash
cd notes/golang
go test ./experiments/          # 全部实验（约 4 秒）
```
