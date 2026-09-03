# string 底层

> **核心认知：** string 是「**(数据指针, 长度)**」二字段头——**没有 `\0` 终止符，长度显式存储，内容不可变**。不可变是全局契约：它让 string 可以安全地做 map key、被任意共享、编译器放心地驻留（intern）字面量；代价是每次「改」字符串都是新分配。string 的一切坑都来自两个转换：**string ↔ []byte 拷贝**（何时逃不出拷贝）和**零拷贝视图**（unsafe 何时安全）。工程口诀：拼接用 Builder + Grow，热路径转换单向设计，零拷贝只在边界库、绝不写。

按 Go 1.26 语义说明，版本分界处单独标注。前置知识：slice 三字段头见 `01-slice与map底层.md`，unsafe 红线见 `03-interface与反射.md` §8。

---

## 目录

1. [string 头结构：无 \0、显式长度](#1-string-头结构无-0显式长度)
2. [不可变：契约与三个推论](#2-不可变契约与三个推论)
3. [string ↔ []byte：拷贝的必然与例外](#3-string--byte拷贝的必然与例外)
4. [拼接：+ / Builder / 预分配](#4拼接--builder--预分配)
5. [rune 与 UTF-8：len 的歧义](#5-rune-与-utf-8len-的歧义)
6. [string 与 map：为什么是天生 key](#6-string-与-map为什么是天生-key)
7. [编译器优化清单](#7-编译器优化清单)
8. [面试高频](#8-面试高频)

---

## 1. string 头结构：无 \0、显式长度

```go
// runtime 里 string 就是一个二字段结构（reflect.StringHeader 已废弃，
// 观察用 unsafe.StringData / unsafe.Slice）
type stringHeader struct {
    data unsafe.Pointer // 指向底层字节数组
    len  int            // 字节数（不是字符数！）
}
```

与 slice 头（ptr/len/cap）只差一个 cap——**string 是「不可变的 []byte 视图」**。三个直接推论：

1. `len(s)` 是**字节数**，多字节 UTF-8 字符会让它大于「字符数」（§5 展开）；
2. 子串 `s[i:j]` **不拷贝数据**：新头的 data = 原指针偏移 i×1，len = j-i。大字符串上反复切小片是零成本的（对比 `[]byte` 切片同样共享，但 []byte 还能写回）；
3. 因为没有 `\0`，string 可以存**任意二进制数据**（包括 `\0` 本身）——`string(b)` 与 `[]byte(s)` 是安全的二进制管道。

```go
s := "hello world"
sub := s[:5] // 零拷贝
p1, p2 := unsafe.StringData(s), unsafe.StringData(sub)
fmt.Println(p1 == p2) // true —— 共享同一块底层内存（实验演示）
```

推论 2 的工程价值：从大 payload 里提取字段用切片是 O(1)；但注意持有 `sub` 会让整个大字符串无法 GC——**长生命周期场景该拷贝时还是要拷贝**（`strings.Clone`，Go 1.18+）。

---

## 2. 不可变：契约与三个推论

`s[i] = 'x'` 直接编译错误。不可变不是「建议」，是**运行时全局契约**——编译器、GC、map 哈希、包级字面量驻留全部依赖它。

**推论一：零成本共享。** 同一个 string 值随便传、随便存，没有深拷贝的心智负担（对比 `[]byte` 传递时要担心谁改它）。

**推论二：字面量驻留（intern）。** 编译期可确定的相同字符串字面量在只读段共享一份内存。`"hello" == "hello"` 两处引用的是同一块只读数据。

**推论三：写 = 新分配。** 所有「修改」API（`strings.Replace`、`+`、`ToUpper`）都返回新串。循环里 `s += x` 是 O(n²) 事故的根源（§4）。

**零拷贝的边界（unsafe）**——`unsafe.String` 造出的 string 指向可变内存，**契约在你手里而非编译器手里**：

```go
b := []byte("hello")
s := unsafe.String(unsafe.SliceData(b), len(b)) // Go 1.20+ 标准写法
b[0] = 'H'
fmt.Println(s) // "Hello" —— s 跟着变了！
```

这就是 `03` 篇红线的 string 版：**绝不能修改零拷贝 string 的底层数据**（可能污染驻留字面量、破坏 map key 哈希一致性）；且产物生命周期与源绑定，b 被复用/回收后 s 是悬挂视图。只在序列化/网络库的边界热路径用。

---

## 3. string ↔ []byte：拷贝的必然与例外

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
3. 最后才是 unsafe 零拷贝（`03` 篇红线全数适用）。

```go
// 快速判断成员：map 索引免拷贝特例
m := map[string]int{"ping": 1}
key := []byte("ping")
_ = m[string(key)] // 不分配（编译器优化，实验演示）
```

---

## 4. 拼接：+ / Builder / 预分配

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

`strings.Builder`（Go 1.10+）为什么快：

1. `WriteString` 就是 `append` 到内部 buf（拷贝检查由 `copyCheck` 保证 Builder 不被值传递滥用）；
2. `String()` 用 `unsafe.String` **零拷贝**导出——这是标准库里 unsafe 的正面示范（Builder 用 `addr` 字段记住自己，防止拷贝后两个 Builder 指向同一 buf）；
3. `Grow(n)` 预分配 = slice 扩容治理的 string 版（`01` 篇 §3 同源）。

| 写法 | 复杂度 | 适用 |
|---|---|---|
| `+` 循环 | O(n²) | 禁止出现在循环里 |
| `+` 一次性 `a + b + c` | O(n)，编译器优化成一次分配 | 少量固定段，最简洁 |
| `fmt.Sprintf` | O(n) 但慢（反射+装箱） | 格式化需求，非纯拼接 |
| `strings.Builder` + Grow | O(n) | 循环拼接的标准答案 |
| `bytes.Buffer` | O(n) | 需要字节/字符串混写或读回 |

---

## 5. rune 与 UTF-8：len 的歧义

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

## 6. string 与 map：为什么是天生 key

map key 的要求：**comparable**。string 完美满足且高效：

1. `==` 是**按字节 memcmp**（长度不同直接不等，长度相同比内容）——O(n) 但现实中哈希先分流，冲突时才全比；
2. **不可变 → 哈希值稳定**：key 进桶后其内容永不变，哈希一致性自动维持（可变类型做 key 是灾难：改了内容 = 哈希变了 = 丢失在旧桶）；
3. 哈希种子每进程随机（`01` 篇），同内容跨进程哈希不同。

对照记忆：

| 候选 key | 能否 | 原因 |
|---|---|---|
| string | ✓ | 不可变 + memcmp |
| []byte | ✗ | slice 三字段头不可比（不可 comparable） |
| [N]byte | ✓ | 定长数组按值可比（比 string 少一次间接，网络库常用来当零分配 key） |
| 含 slice/map 字段的结构体 | ✗ | 不可比 |

`[]byte` 想当 key 的标准姿势就是 `string(b)` 转一次（§3 的 map 索引免拷贝特例正为此设计）。

---

## 7. 编译器优化清单

面试加分项——「string 慢」多数是没吃到的免费优化：

1. **常量折叠**：`"a" + "b" + "c"` 编译期合成 `"abc"`，零运行时成本；
2. **`+` 多元拼接**：`a + b + c`（一个表达式）编译成 `runtime.concatstrings` 一次算总长、一次分配；
3. **比较短路**：`==` 先比长度再比内容；`len` 不同立即 false；
4. **map[string(b)] 免分配**（§3）；
5. **range string 免 []byte 化**：`for i := range s` 直接在字节上迭代，`utf8.DecodeRuneInString` 零拷贝解码（对比 `for range []byte(s)` 需要先拷贝）；
6. **子串零拷贝**（§1）；
7. **switch string**：编译器按长度+首字符建跳转表，不是线性全比。

验证手段统一是 `go build -gcflags="-m"` 看逃逸/内联判定（`09`/`11` 篇的工具链）。

---

## 8. 面试高频

**Q1：string 的底层结构？**
(数据指针, 长度) 二字段头，指向底层字节数组。无 `\0` 终止符、长度显式存储。子串 s[i:j] 零拷贝（只造新头）；string 可存任意二进制。

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

**Q8：持有大字符串的子串会有什么问题？**
sub 零拷贝共享整块底层内存——只要 sub 活着，整个大字符串无法 GC。长生命周期持有时用 strings.Clone 拷贝出独立的小串。

---

本篇对应实验：experiments/02_string.go
