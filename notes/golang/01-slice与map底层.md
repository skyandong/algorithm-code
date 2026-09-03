# Slice 与 Map 底层

> **核心认知：** slice 和 map 都不是「容器」，而是指向底层存储的**视图句柄**——slice 是 (ptr, len, cap) 三字段头，map 是指向 runtime 哈希表的指针。它们的坑全部来自两件事：**共享底层状态**（多个 slice 头写同一块数组）和**运行时行为**（何时分配新数组、何时扩容搬迁）。工程口诀：slice 赋值/传参拷贝的是头不是数据；map 的可见语义（无序、扩容、并发 fatal）由 runtime 保证，不要依赖任何实现细节。

按 Go 1.26 语义说明，版本分界处单独标注。

---

## 1. slice 头结构

slice 在运行时就是一个三字段结构体（`runtime.slice`；旧代码里的 `reflect.SliceHeader` 已于 Go 1.20 起废弃，改用 `unsafe.Slice`/`unsafe.SliceData`）：

```go
type slice struct {
	array unsafe.Pointer // 指向底层数组
	len   int
	cap   int
}
```

关键推论：

- `s = s2`、传参、赋值，**拷贝的是这三个字**，不是数据（WHY：这就是 slice 「轻」的原因，也一切坑的源头）；
- `s[i]` 越界检查只看 `len`；`append` 看的是 `cap`；
- `s[1:]` 只是新造了一个头：`array = 原数组+1×elemSize`，`len-1`，`cap-1`——旧数组还在。

```go
s := []int{10, 20, 30}
sub := s[1:]          // 新 header，共享同一数组
sub[0] = 99
fmt.Println(s)         // [10 99 30] ← s 被一起改了
```

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

## 3. 扩容规则（Go 1.18+）

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

版本分界：**Go 1.17 及以前是 `<1024 翻倍、≥1024 每次加 1/4`**；Go 1.18 改成上面的 256 阈值平滑公式。

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
// 坑：底层数组末尾仍持有该元素（指针元素 = 阻止 GC），补救（截断前或后皆可）：
a[len(a)] = nil // 截断后 len 已减 1，这里正是被删元素的原槽位

// 写法二：copy 覆盖 —— 保序，O(n)
a = append(a[:i], a[i+1:]...)
// 坑：同上，新尾之后的旧值仍残留（a[:cap(a)] 可见）

// 写法三：swap-delete —— O(1)，不保序
a[i] = a[len(a)-1]
a = a[:len(a)-1]
// 优点：被删元素立刻被覆盖，无尾部残留；集合语义（去重、缓存淘汰）首选
```

| 写法 | 复杂度 | 保序 | 尾部残留 |
| --- | --- | --- | --- |
| `a = a[:len-1]` | O(1) | 尾部删除无所谓 | **有**（泄漏点） |
| `append(a[:i], a[i+1:]...)` | O(n) | ✅ | 有（同上） |
| swap-delete | O(1) | ❌ | 无 |

**大数组切小 slice 的泄漏**（高频线上问题）：

```go
big := make([]byte, 1<<20)      // 1 MiB
tail := big[len(big)-2:]         // 只用末尾 2 字节
// 只要 tail 活着，整块 1 MiB 都无法被 GC 回收
fixed := make([]byte, len(tail))
copy(fixed, tail)               // 修复：copy 出独立小数组
```

WHY：GC 以**分配对象**为回收单位，`tail` 的 array 指针指进 `big` 的分配块中间，整块都活着。读文件、解析大 buffer 后只留一小段时必做 copy（string 场景对应 `strings.Clone`）。

---

## 5. map 底层

### 5.1 经典实现：hmap + 桶（Go ≤1.23，面试基准模型）

```go
type hmap struct {
	count     int          // 元素数，len() 直接读它
	B         uint8        // 桶数 = 2^B
	hash0     uint32       // hash 种子（每次创建随机）
	buckets   unsafe.Pointer // 桶数组
	oldbuckets unsafe.Pointer // 扩容期间的旧桶
	noverflow uint16       // overflow 桶近似计数
	// ...flags、nevacuate 等
}

type bmap struct { // 一个桶
	tophash [8]byte // 每个槽 key hash 的高 8 位，先比 tophash 再比 key
	// 后跟 8 个 key、8 个 elem（编译期按类型展开）
	// 再跟 overflow *bmap（桶满后挂链）
}
```

### 5.2 负载因子 6.5 触发翻倍扩容

`count > 6.5 × 2^B`（代码里是 `count > 13 × (2^B / 2)`）→ 扩容：B+1，桶数翻倍。WHY 6.5：8 槽桶留 1/4 余量，是「空间浪费 vs 查找速度」的实测折中。

### 5.3 增量搬迁，不是一次性 rehash

扩容不搬数据，只挂上 `oldbuckets`；**之后的每次写入顺带搬一个桶**（evacuate），读写期间新旧桶并存（读会 fallback 到旧桶）。WHY：一次性 rehash 大 map 会造成单次操作毫秒级尖刺，增量把成本摊到后续操作。

### 5.4 等量扩容（整理碎片）

删除多导致负载不高，但 overflow 链很长（`noverflow ≥ 2^min(B,15)`）→ 触发**同容量**扩容：桶数不变，只是把松散数据压回前面桶、清掉 overflow 链。WHY：查找退化成链表遍历，整理后恢复 O(1)。

### 5.5 版本分界：Go 1.24+ 已换 Swiss Table

Go 1.24 起 map 底层改为 Swiss Table（`internal/runtime/maps`），Go 1.26 即此实现：

- 桶 → **group**（8 槽 + 1 字节/槽的控制字，存 hash 低 7 位 H2，SIMD 思路一次比对 8 槽）；
- 顶层是 **directory + 多张 table**（extendible hashing，按 hash 高位选 table）；
- 单 table 容量上限 1024 槽；每表独立负载因子 **7/8**（`maxAvgGroupLoad=7`，超过即 rehash：翻倍或分裂成两张表）——**「6.5」是老实现的数字**；
- 增量性换了形态：单张 table 一次搬完，但 table 很小，扩容天然被摊薄；
- 删除打 tombstone、插入优先复用；**等量扩容已不存在**（每次 rehash 必然翻倍/分裂）。

无序遍历、增量扩容、并发 fatal 这些**可见语义完全不变**——面试答经典模型没毛病，但要能说出 1.24 的变化，这是加分项。

---

## 6. map 无序与 for range 语义

**为什么每次 range 顺序都不一样**：遍历起点（起始桶/group 和槽位偏移）由随机数决定。WHY：官方有意为之，防止用户依赖顺序——一旦依赖，实现升级（如 1.24 换 Swiss Table）就会炸你的代码。要顺序就显式排序 key。

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

## 7. map 并发读写：fatal error，不可 recover

```go
m := map[int]int{}
go func() { for { m[1] = 1 } }()
go func() { for { _ = m[1] } }()
// fatal error: concurrent map read and map write
```

三个要点：

1. **这是 throw 不是 panic**：runtime 直接 `fatal`，不走 panic 机制，`defer/recover` 拦不住，进程整个退出。WHY：map 并发写会破坏内部结构（链表/控制字状态不一致），runtime 选择宁可崩溃也不带着坏数据继续跑；
2. **Go 1.6+ 才有该检测**（hmap 的并发读写标志位；Swiss 实现里是 `writing` 标志，读写前检查）——是「尽力检测」而非保证抓到每一个竞态，`-race` 才是完备工具；
3. 检测到的是「正在写时读/写」，两个 goroutine 错峰写不报错但同样危险。

**怎么让 map 并发安全**：

| 方案 | 原理 | 适用 |
| --- | --- | --- |
| `sync.Mutex` + map | 单锁 | 通用，竞争小时最简单 |
| 分片锁（N 个 shard） | key hash 到 shard，锁冲突概率降 ~1/N | 高并发写、key 分布均匀；代价：遍历/动态扩容要自己实现 |
| `sync.Map` | 见下 | 读多写少 |

---

## 8. sync.Map：read/dirty 两层

一句话原理：**两个内建 map——read（无锁原子读）+ dirty（加锁写）**。Load 先查 read（无锁命中即返回）；read 未命中且 dirty 有新数据，加锁查 dirty；misses 次数 ≥ len(dirty) 时把 dirty 整体提升为 read。

| 场景 | 表现 | WHY |
| --- | --- | --- |
| 读多写少、key 集合稳定 | 优于锁 + map | Load 无锁走 read |
| 持续写新 key | **比 Mutex+map 还慢** | 每次 Store 加锁写 dirty + miss 计数，提升时还要整体搬运、新分配 dirty |
| 遍历 | 无快照保证 | Range 就是遍历两层 |
| 值类型是指针 | 需 LoadOrStore 等原子组合操作时注意竞态 | 读到的指针仍需原子更新（`atomic.Pointer[T]` 是常见搭档） |

选型：写多 → Mutex/分片；读多写少（缓存、连接池元数据、一次性配置）→ sync.Map。

---

## 9. 空切片 vs nil 切片

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

本篇对应实验: experiments/01_slice_map.go
