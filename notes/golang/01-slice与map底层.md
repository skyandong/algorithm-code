# Slice、string 与 Map 底层

> **核心认知：** slice 和 map 都不是「容器」，而是指向底层存储的**视图句柄**——slice 是 (ptr, len, cap) 三字段头，string 是只读的 (ptr, len) 二字段头，map 是指向 runtime 哈希表的指针。它们的坑全部来自两件事：**共享底层状态**（多个头写同一块存储）和**运行时行为**（何时分配新数组、何时扩容搬迁）。工程口诀：slice/string 赋值/传参拷贝的是头不是数据；string 不可变是全局契约，拼接用 Builder + Grow、热路径互转单向设计、零拷贝只在边界库；map 的可见语义（无序、扩容、并发 fatal）由 runtime 保证，不要依赖任何实现细节。

本文基于 Go 1.26（本机 1.26.3）的实际行为编写，不涉及历史版本对比。

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

差别就在这个 cap 上：slice 的元素可写、append 可能扩容，所以要记容量；string 是**不可变的 []byte 视图**——没有 `\0` 终止符、`len` 是字节数、任何「修改」都是新分配。

slice.go 这个文件就是 slice 的运行时中枢：`make` 分配（makeslice 系列）、`append` 扩容（growslice 系列，策略见第 3 节）、`copy`（slicecopy）全在这里。

`s[1:]` 这类切片表达式只是新造一个头——`array` 前移一个元素、`len`/`cap` 各减 1，旧数组不动：

```go
s := []int{10, 20, 30}
sub := s[1:]          // 新 header，共享同一数组
sub[0] = 99
fmt.Println(s)         // [10 99 30] ← s 被一起改了
```

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

## 3. 扩容规则

`runtime.nextslicecap`（Go 1.26.3 源码）：

```go
const threshold = 256
if newLen > 2*oldCap { return newLen } // 一次要很多，按需给
if oldCap < threshold  { return 2*oldCap } // 小 slice 翻倍
// 大 slice：从 2x 平滑过渡到 1.25x
newcap += (newcap + 3*threshold) >> 2
```

- `cap < 256`：翻倍；
- 之后：`newcap += (newcap + 768) / 4`，增长系数随容量增大从 2x **平滑滑向 1.25x**；
- 最后还要过 `roundupsize` 按 **sizeclass 对齐**（分配器规格），观察值会进一步修正。

实测序列（Go 1.26.3，`[]int` 逐个 append）：

```text
cap: 1 2 4 8 16 32 64 128 256 512 848 1280 1792 2560 3408 5120 7168 9216 12288 ...
                     ↑ 翻倍段结束   ↑ 1.5x 1.4x ...（sizeclass 对齐后的观察值）
```

两个补充事实：

- **Go 1.26 新优化**：非逃逸的「append 到空 slice」且总字节数 ≤32 时，编译器直接分配**栈上背衬数组**（cap = 32/elemSize，如 `[]int` 得 4、`[]byte` 得 32），所以本地小 slice 首次 append 的 cap 可能是 4 而不是 1（实验里用包级变量强制逃逸规避了它）；
- 含指针的元素（scan 对象）sizeclass 对齐还要多算 8 字节 malloc header，序列与 `[]int` 不同。

**工程结论：不要依赖具体扩容倍数。** 它随版本、元素大小、是否逃逸而变。需要确定容量就 `make([]T, 0, n)` 预分配——这也是性能优化第一课（避免反复扩容拷贝）。

---

## 4. 删除元素：三种写法与内存泄漏点

```go
// 写法一：截断 —— O(1)
a = a[:len(a)-1]
// 坑：底层数组末尾仍持有该元素（元素含指针时 = 阻止 GC），补救两种姿势：
a[len(a)-1] = nil          // 姿势一：截断「前」清槽位（SliceTricks 标准写法）
a[:cap(a)][len(a)] = nil   // 姿势二：截断「后」借 reslice 到 cap 清 —— 直接写 a[len(a)] = nil 会 panic
// WHY：截断后 len 已减 1，索引上限是 len-1，直接 a[len(a)] 越界（实测 index out of range）；
//      a[:cap(a)] 把 len 重新扩到 cap，原被删元素落在新 len 之内，才能索引到
// 注意：= nil 只对含指针的元素编译得过（指针/接口/string/slice/含指针字段的结构体）；
//      值类型 []int 等赋不了 nil，但也本来就不阻止 GC，无需补救；通用形式 var zero T
// clear(a[len(a):cap(a)]) 截断后一步清光整个尾部（姿势二的批量版，任意元素类型通用）

// 写法二：copy 覆盖 —— 保序，O(n)
a = append(a[:i], a[i+1:]...)
// 坑：同上，新尾之后的旧值仍残留（a[:cap(a)] 可见）

// 写法三：swap-delete —— O(1)，不保序
a[i] = a[len(a)-1]
a = a[:len(a)-1]
// 优点：被删槽位立刻被覆盖，GC 不泄漏；注意 a[len(a)] 处留有被移元素的副本（a[:cap(a)] 可读）
// 集合语义（去重、缓存淘汰）首选
```

| 写法 | 复杂度 | 保序 | 尾部残留 |
| --- | --- | --- | --- |
| `a = a[:len-1]` | O(1) | 尾部删除无所谓 | **有**（泄漏点） |
| `append(a[:i], a[i+1:]...)` | O(n) | ✅ | 有（同上） |
| swap-delete | O(1) | ❌ | 无泄漏（新尾槽位留有副本，可读） |

**整段清空：`a = a[:0]` / `a = nil` / `clear(a)` 语义不同**：

```go
a = a[:0] // len=0、cap 不变：底层数组保留复用，后续 append 不再分配
a = nil   // len=cap=0：断开引用，无别名时整个数组可回收；下次 append 重新分配
clear(a)  // len 不变：元素全部归零；要的是"归零"而不是"变短"时用
```

- `a = nil` 只断开**这一个 slice 头**：子 slice、其他别名仍钉住整块数组；且它不清数据；
- `a[:0]` 复用是缓冲区高频优化，但元素含指针时泄漏点同上——重置前先 `clear(a[:oldLen])`；
- 含敏感数据（token、密钥）的 slice，卫生清理靠 `clear`；截断和置 nil 都只是改引用。

**大数组切小 slice 的泄漏**（高频线上问题）：

```go
big := make([]byte, 1<<20)      // 1 MiB
tail := big[len(big)-2:]         // 只用末尾 2 字节
// 只要 tail 活着，整块 1 MiB 都无法被 GC 回收
fixed := make([]byte, len(tail))
copy(fixed, tail)               // 修复：copy 出独立小数组
```

WHY：GC 以**分配对象**为回收单位，`tail` 的 array 指针指进 `big` 的分配块中间，整块都活着。读文件、解析大 buffer 后只留一小段时必做 copy（string 场景对应 `strings.Clone`，见第 1 节）。

---

## 5. string 不可变：契约与三个推论

`s[i] = 'x'` 直接编译错误。不可变不是「建议」，是**运行时全局契约**——编译器、GC、map 哈希、包级字面量驻留全部依赖它。

**推论一：零成本共享。** 同一个 string 值随便传、随便存，没有深拷贝的心智负担（对比 `[]byte` 传递时要担心谁改它）。

**推论二：字面量驻留（intern）。** 编译期可确定的相同字符串字面量在只读段共享一份内存。`"hello" == "hello"` 两处引用的是同一块只读数据。

**推论三：写 = 新分配。** 所有「修改」API（`strings.Replace`、`+`、`ToUpper`）都返回新串。循环里 `s += x` 是 O(n²) 事故的根源（第 7 节）。

**零拷贝的边界（unsafe）**——`unsafe.String` 造出的 string 指向可变内存，**契约在你手里而非编译器手里**：

```go
b := []byte("hello")
s := unsafe.String(unsafe.SliceData(b), len(b)) // 标准写法
b[0] = 'H'
fmt.Println(s) // "Hello" —— s 跟着变了！
```

这就是 `02` 篇红线的 string 版：**绝不能修改零拷贝 string 的底层数据**（可能污染驻留字面量、破坏 map key 哈希一致性）；且产物生命周期与源绑定，b 被复用/回收后 s 是悬挂视图。只在序列化/网络库的边界热路径用。

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
3. 最后才是 unsafe 零拷贝（`02` 篇红线全数适用）。

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

**for range string 按码点迭代**——每轮解码一个 UTF-8 序列，索引是**字节偏移**（不是第几个字符）：

```go
for i, r := range "你好Go" {
    fmt.Println(i, r) // 0 你 / 3 好 / 6 G / 7 o —— 索引跳变
}
```

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
5. **range string 免 []byte 化**：`for i := range s` 直接在字节上迭代，`utf8.DecodeRuneInString` 零拷贝解码（对比 `for range []byte(s)` 需要先拷贝）；
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

三个核心类型：`Map`（顶层）→ `table`（单张表）→ `group`（一组 8 槽），全部在 `$GOROOT/src/internal/runtime/maps/`。

### 11.1 三层结构

```go
// src/internal/runtime/maps/map.go（顶层，节选）
type Map struct {
	used        uint64         // 已用槽位数，len() 直接读它（必须放首字段，编译器依赖）
	seed        uintptr        // hash 种子（每次创建随机）
	dirPtr      unsafe.Pointer // 目录：正常为 *[dirLen]*table；小 map 直接指向单个 group
	dirLen      int
	globalDepth uint8          // 目录查找用的 hash 位数
	globalShift uint8
	// writing uint8 并发写标志（读写前检查，见第 13 节）
}

// src/internal/runtime/maps/table.go（单张表，节选）
type table struct {
	used       uint16 // 本表已用槽位
	capacity   uint16 // 总槽位（恒为 2^N）
	growthLeft uint16 // 距下次 rehash 还能填的空槽数（tombstone 也计入）
	localDepth uint8
	index      int    // 本表在目录中的首个索引（可连续占多个条目）
	// groups 是定长的 group 数组
}

// src/internal/runtime/maps/group.go（一组 8 槽，key/elem 编译期按类型展开）
type group struct {
	ctrls ctrlGroup                // 8 个控制字：hash 低 7 位 H2 + 空闲/已删标记
	slots [abi.MapGroupSlots]slot  // abi.MapGroupSlots = 8
}
```

查找路径一句话：`hash(seed)` → **高 globalDepth 位**在 directory 里选 table → 表内按 hash 低位定 group → **SIMD 一次比对 8 个控制字（H2）** → 命中控制字再比 key。为什么快：控制字把「8 个槽谁可能命中」压缩成一次宽比较，无效 key 的比对几乎归零。

小 map 优化：元素 ≤8 时整张 map 就一个 group——`dirLen == 0`，`dirPtr` 直接指向它，零间接层。

### 11.2 负载因子 7/8 触发 rehash

单表 `used + tombstones > loadFactor × capacity`（`maxAvgGroupLoad = 7`，每 8 槽平均最多占 7）→ rehash：容量翻倍，或目录允许时**分裂成两张表**。WHY 7/8：每组平均留 1 槽空，是探测长度与空间浪费的折中。单表容量上限 1024 槽（`maxTableCapacity`），到顶只能分裂——单张表小，rehash 成本天然摊薄，没有一次性搬大表的尖刺。

### 11.3 删除打 tombstone

删除不清槽位，控制字打成 `ctrlDeleted`（`0b11111110`）——不能直接清成 empty，否则打断探测序列；插入优先复用 tombstone 槽。`growthLeft` 把 tombstone 一并计入，防止表被删空槽塞满。**容量只增不减**：要真正释放内存，换新 map 重建（TestMapDeleteMemory 实测）。

### 11.4 可见语义

无序遍历（起点随机化）、for range 中删除安全、并发读写 fatal——全部由语言规范保证（第 12/13 节）。

---

## 12. map 无序与 for range 语义

**为什么每次 range 顺序都不一样**：遍历起点（起始桶/group 和槽位偏移）由随机数决定。WHY：官方有意为之，防止用户依赖顺序——一旦依赖，runtime 换实现就会炸你的代码。要顺序就显式排序 key。

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
2. **该检测是尽力而为**（实现里是 `writing` 标志，读写前检查并 XOR 置位）——是「尽力检测」而非保证抓到每一个竞态，`-race` 才是完备工具；
3. 检测到的是「正在写时读/写」，两个 goroutine 错峰写不报错但同样危险。

**怎么让 map 并发安全**：

| 方案 | 原理 | 适用 |
| --- | --- | --- |
| `sync.Mutex` + map | 单锁 | 通用，竞争小时最简单 |
| 分片锁（N 个 shard） | key hash 到 shard，锁冲突概率降 ~1/N | 高并发写、key 分布均匀；代价：遍历/动态扩容要自己实现 |
| `sync.Map` | 见第 14 节 | 读多写少 |

---

## 14. sync.Map：并发哈希 trie

```go
// src/sync/map.go（sync.Map 本体就是 HashTrieMap 的薄包装）
type Map struct {
	_ noCopy
	m isync.HashTrieMap[any, any]
}

// src/internal/sync/hashtriemap.go（节选）
type HashTrieMap[K comparable, V any] struct {
	inited   atomic.Uint32
	initMu   Mutex
	root     atomic.Pointer[indirect[K, V]] // 根节点：按 hash 前缀逐层下行
	keyHash  hashFunc
	valEqual equalFunc
	seed     uintptr
}
```

内部是**并发哈希 trie（HAMT 变体）**：按 key hash 的前缀逐层定位子树，**读路径无锁**（沿 atomic 指针下行，不碰任何锁），**写锁粒度到子树级**——不相干的 key 各写各的子树，互不竞争。

| 场景 | 表现 | WHY |
| --- | --- | --- |
| 读多写少、key 集合稳定 | 优于锁 + map | Load 无锁 |
| 多 goroutine 写不相干的 key | 同样占优（实验有对照数据） | 锁在子树级，分片状写入无竞争 |
| Range 遍历 | 无快照保证 | 最终一致遍历 |
| 值类型是指针 | 需 LoadOrStore 等原子组合操作时注意竞态 | 读到的指针仍需原子更新（`atomic.Pointer[T]` 是常见搭档） |

选型：**写同一批热点 key / 写读混合** → Mutex/分片；**key 写一次读多次、或多 goroutine 分片状写入** → sync.Map。没有 len()。

---

## 15. 空切片 vs nil 切片

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

## 16. 面试高频

**Q1：slice/string 的底层结构？**
slice 是 (ptr, len, cap) 三字段头，string 是 (ptr, len) 二字段头（无 `\0`、长度显式存储）。子串/子切片 s[i:j] 零拷贝（只造新头）；string 可存任意二进制。

**Q2：为什么 string 设计成不可变？**
三个受益方：共享安全（传值零风险）、字面量驻留（相同字面量一份内存）、map key 哈希稳定（不可变→哈希值不变）。代价是所有修改操作都是新分配，循环拼接必须用 Builder。

**Q3：string 和 []byte 怎么选？**
要不可变/做 map key/读：string；要修改/流式处理：[]byte。互转默认拷贝（不可变契约）；例外：编译器证明不逃逸不改时 []byte(s) 免拷贝、m[string(b)] 索引免分配。热路径原则：流水线统一类型，只在边界转一次。

**Q4：为什么循环里 s += x 是 O(n²)？**
每次 + 分配新串并全量拷贝旧内容，n 次共拷贝 O(n²) 字节。正解 strings.Builder + Grow 预分配；String() 用 unsafe.String 零拷贝导出。

**Q5：len(s) 返回什么？**
字节数。字符（码点）数是 utf8.RuneCountInString(s)。for range string 按码点迭代、索引是字节偏移；s[0] 取的是字节。截断按字节会切碎多字节字符。

**Q6：[]byte 为什么不能做 map key 而 string 可以？**
slice 头不可比较（comparable 不满足）；string 不可变、== 是 memcmp、哈希稳定。[]byte 当 key 用 string(b) 转一次（map 索引处编译器免分配）。

**Q7：unsafe.String 零拷贝什么时候能用？**
标准库示范是 strings.Builder.String()。红线：绝不修改底层（污染驻留字面量/破坏哈希）；生命周期与源绑定（源回收即悬挂视图）；只集中在边界库少数文件。uintptr 运算必须单表达式内完成。

**Q8：持有大字符串/大数组的子串会有什么问题？**
零拷贝共享整块底层内存——只要子串活着，整个大对象无法 GC。长生命周期持有时用 strings.Clone（string）/ copy（slice）拷出独立小对象。

---

本篇对应实验：experiments/01_slice_map.go、experiments/02_string.go
