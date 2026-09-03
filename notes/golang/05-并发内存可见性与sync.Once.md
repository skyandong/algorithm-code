# Concurrency Memory Visibility and `sync.Once`

本文讨论 Go 并发程序中的 data race、内存可见性、happens-before、编译器/CPU 重排，以及 `sync.Once` 的正确使用方式。

本文按 Go 1.26 语义说明。

## 1. 核心模型

一个 goroutine 中的源代码顺序，不等于另一个 goroutine 观察到的顺序。

如果多个 goroutine 访问同一个内存位置，至少一个访问是写入，并且访问之间没有通过 mutex、channel、atomic 等机制建立同步关系，就会发生 data race。

例如：

```go
package main

import "fmt"

var data int
var ready bool

func writer() {
	data = 42
	ready = true
}

func reader() {
	for !ready {
	}
	fmt.Println(data)
}

func main() {
	go writer()
	reader()
}
```

这段程序中：

- `writer` 写入 `data`，`reader` 读取 `data`；
- `writer` 写入 `ready`，`reader` 读取 `ready`；
- 没有任何同步关系。

因此不能从代码顺序推出 `reader` 一定打印 `42`。程序可能读取旧值、观察到不一致状态、一直等待，或者在不同运行环境中表现不同。

> data race 不是一种可以依赖的并发语义。不要通过“这次运行结果正常”判断代码安全。

使用 race detector 检查实际发生的竞态：

```bash
go run -race main.go
go test -race ./...
go build -race
```

race detector 不能证明程序不存在所有并发错误，但能发现许多实际发生的 data race。

---

## 2. happens-before：并发正确性的关键

并发程序需要的不只是“单次读写不被打断”，还需要明确的可见性和顺序关系。

如果事件 A happens-before 事件 B，则 B 可以观察到 A 之前已经完成的相关写入。

常见的同步关系包括：

- mutex 的 `Unlock` happens-before 之后成功获得的 `Lock`；
- 无缓冲 channel 的第 k 次接收 happens-before 第 k 次发送完成；有缓冲 channel 的第 k 次发送 happens-before 第 k 次接收完成；
- 关闭 channel happens-before 因关闭而返回零值的接收；
- atomic 操作按照其内存顺序建立同步关系；
- `WaitGroup.Wait` 返回前，相关任务必须已经完成；
- `sync.Once.Do` 返回时，初始化函数已经完成执行。

没有 happens-before 关系时，不能用源代码的上下顺序推断另一个 goroutine 的可见顺序。

---

## 3. 重排、缓存和编译器优化

### 3.1 什么是重排？

源代码可能是：

```go
data = 42
ready = true
```

但在没有同步的程序中，编译器和 CPU 可以改变指令执行、提交或对其他 CPU 可见的时机。另一个 goroutine 观察到的效果可能类似于：

```text
ready = true  // 先被观察到
data = 42     // 后被观察到
```

于是 reader 可能观察到：

```text
ready == true
data == 0
```

这不一定意味着编译器真的交换了两行源代码，也可能来自：

- 编译器调整了机器指令顺序；
- CPU 乱序执行；
- 写入暂时停留在写缓冲区；
- 不同 CPU 以不同的时机观察到写入。

重排是帮助理解“为什么没有同步就不能依赖观察顺序”的模型，不是对某次运行结果的必然预测。

### 3.2 什么是 CPU 缓存可见性？

不同 CPU 核心通常拥有自己的缓存和写缓冲区：

```text
CPU 1 / writer                 CPU 2 / reader
      │                              │
      ▼                              ▼
  CPU 1 Cache                    CPU 2 Cache
```

writer 对普通变量的写入不一定会在同一时刻被另一个核心观察到。缓存一致性协议会维护硬件层面的规则，但它不会替 Go 程序建立所需的 goroutine 同步关系。

因此不能依赖：

```go
var ready bool

// goroutine A
ready = true

// goroutine B
for !ready {
}
```

来实现安全的状态通知。

### 3.3 什么是编译器优化？

对于：

```go
for !ready {
}
```

编译器可以基于当前 goroutine 的代码分析读取行为。由于程序存在 data race，不能要求编译器每次循环都重新从共享内存读取 `ready`，也不能要求它保留某种特定的读取顺序。

这不是说 Go 编译器一定会把代码优化成：

```go
value := ready
for !value {
}
```

而是说：一旦程序已经违反并发访问规则，就不能依赖普通变量的跨 goroutine 可见性。

---

## 4. 使用 atomic 发布状态

如果使用一个状态变量发布另一个变量，可以使用 atomic：

```go
package main

import (
	"fmt"
	"sync/atomic"
)

var data atomic.Int64
var ready atomic.Bool

func main() {
	go func() {
		data.Store(42)
		ready.Store(true)
	}()

	for !ready.Load() {
	}

	fmt.Println(data.Load())
}
```

这里的意图是：

```text
先写入 data
再 Store ready = true

reader 先 Load ready
确认 ready == true 后，再 Load data
```

atomic 操作解决了对这些变量的并发访问问题，并提供相应的内存顺序保证。

不过忙等会消耗 CPU。如果目标只是等待某项工作完成，通常更适合使用 channel 或 WaitGroup：

```go
package main

import "fmt"

func main() {
	done := make(chan struct{})
	data := 0

	go func() {
		data = 42
		close(done)
	}()

	<-done
	fmt.Println(data)
}
```

关闭 `done` 前的写入，对收到关闭通知之后的代码可见。

---

## 5. 使用 Mutex 建立同步关系

```go
package main

import (
	"fmt"
	"sync"
)

var mu sync.Mutex
var data int
var ready bool

func writer() {
	mu.Lock()
	data = 42
	ready = true
	mu.Unlock()
}

func reader() {
	mu.Lock()
	defer mu.Unlock()

	if ready {
		fmt.Println(data)
	}
}
```

锁的作用不只是防止同时修改数据，还建立了可见性关系：

```text
writer:
    data = 42
    ready = true
    Unlock()
           │
           │ happens-before
           ▼
reader:
    Lock()
    读取 ready
    读取 data
```

如果只用 mutex 保护写入，却在读取时不加锁，读取仍然可能与写入形成 data race。

---

## 6. 错误的双重检查 `Once`

下面的实现试图通过双重检查执行一次初始化：

```go
package doublecheck

import "sync"

type Once struct {
	mu   sync.Mutex
	done uint32
}

func (o *Once) Do(f func()) {
	if o.done == 1 {
		return
	}

	o.mu.Lock()
	defer o.mu.Unlock()

	if o.done == 0 {
		o.done = 1
		f()
	}
}
```

### 6.1 外层无锁读取构成 data race

下面的读取：

```go
if o.done == 1 {
	return
}
```

没有锁，也没有 atomic，而其他 goroutine 可能同时执行：

```go
o.done = 1
```

所以 mutex 只保护内层临界区，不能保护外层读取。

### 6.2 完成标志设置得太早

代码顺序是：

```go
o.done = 1
f()
```

假设 `f` 执行时间较长：

```go
func f() {
	time.Sleep(time.Second)
	fmt.Println("初始化完成")
}
```

另一个 goroutine 可能先看到：

```text
done == 1
```

然后直接返回，但此时 `f` 还没有执行完。因此“done”应该表示初始化已经完成，而不是“准备开始初始化”。

### 6.3 不要简单归因于 CPU 缓存

这段代码的根本问题是违反了 Go 的并发访问规则，表现不能依赖。CPU cache、编译器优化和指令重排可以帮助解释为什么无同步读写不可靠，但不能用来推导 `f` 必然执行多次或某个固定结果。

---

## 7. 正确使用 `sync.Once`

实际开发中直接使用标准库：

```go
package main

import "sync"

var once sync.Once
var singleton *Service

type Service struct{}

func service() *Service {
	once.Do(func() {
		singleton = &Service{}
	})
	return singleton
}
```

`sync.Once` 保证：

- 多个 goroutine 并发调用时，初始化函数只执行一次；
- 调用 `Do` 的 goroutine 会等待正在执行初始化的 goroutine；
- 初始化函数返回后，后续调用才能观察到初始化结果；
- 初始化函数 panic 后，`Once` 仍被视为已经执行过，后续调用不会再次执行该函数。

不要复制已经使用过的 `sync.Once`，也不要复制包含 `sync.Once` 的结构体。

---

## 8. 如果只是为了理解双重检查

可以用 atomic 配合 mutex 写出一个教学版本：

```go
package main

import (
	"sync"
	"sync/atomic"
)

type Once struct {
	mu   sync.Mutex
	done atomic.Uint32
}

func (o *Once) Do(f func()) {
	if o.done.Load() == 1 {
		return
	}

	o.mu.Lock()
	defer o.mu.Unlock()

	if o.done.Load() == 0 {
		f()
		o.done.Store(1)
	}
}
```

这个版本的逻辑是：

1. atomic `Load`/`Store` 访问 `done`，避免普通变量 data race；
2. mutex 保证同一时刻只有一个 goroutine 执行 `f`；
3. `Store(1)` 放在 `f` 返回后，表示初始化已经完成。

但这仍然不应该替代标准库 `sync.Once`。标准库处理了初始化完成语义、panic 行为和内部同步细节，手写版本容易遗漏边界条件。

---

## 9. 常见同步机制对比

| 机制 | 适合场景 | 主要保证 |
| --- | --- | --- |
| `sync.Mutex` | 保护共享可变状态 | 互斥访问和内存可见性 |
| `sync.RWMutex` | 读多写少的共享状态 | 读写锁同步 |
| channel | 传递数据或通知完成 | 发送/接收同步和可见性 |
| `sync/atomic` | 计数器、状态标志、无锁数据路径 | 原子访问和内存顺序 |
| `sync.WaitGroup` | 等待一组任务完成 | 任务完成后的等待关系 |
| `sync.Once` | 一次性初始化 | 初始化只执行一次且完成后可见 |

选择原则：

- 要保护一组相关字段，优先考虑 mutex；
- 要传递数据或表达任务完成，优先考虑 channel；
- 只有单个计数器或状态标志时，考虑 atomic；
- 要等待一组 goroutine 结束，使用 WaitGroup；
- 要做一次性初始化，使用 `sync.Once`。

---

## 10. Race detector

运行单个文件：

```bash
go run -race main.go
```

运行测试：

```bash
go test -race ./...
```

构建程序：

```bash
go build -race
```

检测到竞态时，通常会报告：

```text
WARNING: DATA RACE
Read at ...
Previous write at ...
```

race detector 主要检测实际执行到的内存访问。没有报告并不等于证明程序在所有调度和输入下都没有并发错误；仍然需要检查锁的粒度、任务生命周期、channel 关闭责任和取消路径。

---

## 11. 总结

```text
普通变量并发读写：data race，不可依赖
Mutex：保护共享状态并建立可见性
channel：传输数据或发送完成通知
atomic：安全地读写单个状态或计数器
WaitGroup：等待一组 goroutine 完成
sync.Once：只执行一次初始化，并在完成后发布结果
```

最重要的原则是：

> 不要使用普通变量在 goroutine 之间传递状态。必须通过 mutex、channel、atomic、WaitGroup 或其他同步机制建立明确的 happens-before 关系。

重排、CPU 缓存和编译器优化是理解可见性问题的模型；真正的修复方式是使用 Go 提供的同步原语，而不是依赖某种机器、编译器或运行时表现。

---

## 实验

对应代码：[experiments/01_memory_visibility.go](experiments/01_memory_visibility.go)

```bash
cd notes/golang
go run ./experiments/ visibility        # 常规运行
go run -race ./experiments/ visibility  # 观察 DATA RACE 报告
```

实验内容：
1. data race 演示：普通变量跨 goroutine 传递状态，-race 下报 `WARNING: DATA RACE`
2. atomic 发布状态：`Store`/`Load` 建立 happens-before
3. channel 发布：`close(done)` 通知完成，关闭前的写入可见
4. mutex 同步：`Unlock` happens-before 之后的 `Lock`，同时保证互斥与可见性
5. 错误的双重检查 Once：外层无锁读 `done` 是 data race（-race 可复现）
6. 正确的 `sync.Once`：初始化只执行一次，且完成后对所有人可见
