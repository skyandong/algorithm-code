# Channel Internals and nil Channel Semantics

本文将 `hchan` 底层结构、nil channel、channel 关闭行为，以及在 `select` 中通过 nil 禁用和恢复分支的内容统一整理在一起。

## 1. 四条基本规则

| 状态 | 操作 | 行为 |
| --- | --- | --- |
| `nil channel` | 阻塞发送 | 永久阻塞 |
| `nil channel` | 阻塞接收 | 永久阻塞 |
| `nil channel` | 非阻塞发送/接收 | 立即失败，`select` 走 `default` |
| 已关闭 channel | 发送 | `panic` |
| 已关闭且仍有缓冲数据 | 接收 | 先正常返回缓冲区中的数据，`ok == true` |
| 已关闭且缓冲区为空 | 接收 | 立即返回元素类型零值，`ok == false` |
| `close(nil)` | 关闭 | `panic` |

最重要的区别是：

> `close` 关闭的是发送入口，不会清空 buffered channel 中已有的数据；接收者可以继续读取这些数据，直到缓冲区排空。

---

## 2. `hchan` 结构

Go 的 channel 变量可以简化理解为一个指向 runtime 内部 `hchan` 的引用。

```go
type hchan struct {
	qcount   uint           // 当前缓冲区中的元素数量
	dataqsiz uint           // 缓冲区容量
	buf      unsafe.Pointer // 环形缓冲区
	elemsize uint16         // 元素大小
	closed   uint32         // 0 表示未关闭，1 表示已关闭

	elemtype *_type

	sendx uint // 下一次发送的位置
	recvx uint // 下一次接收的位置

	recvq waitq // 等待接收的 goroutine 队列
	sendq waitq // 等待发送的 goroutine 队列

	lock mutex // 保护 hchan 状态
}
```

可以把 channel 理解成：

```text
channel
  └── *hchan
        ├── closed
        ├── buf
        ├── qcount
        ├── dataqsiz
        ├── sendx
        ├── recvx
        ├── sendq
        ├── recvq
        └── lock
```

字段作用：

- `closed`：channel 是否已经关闭；
- `qcount`：当前缓冲区中的元素数量；
- `dataqsiz`：缓冲区容量；
- `buf`：buffered channel 的环形数组；
- `sendx`：下一次写入缓冲区的位置；
- `recvx`：下一次读取缓冲区的位置；
- `sendq`：发送无法继续时等待的 goroutine 队列；
- `recvq`：接收无法继续时等待的 goroutine 队列；
- `lock`：保护 channel 内部状态。

注意：runtime 内部结构属于实现细节，版本之间可能变化。本文中的源码逻辑以 Go 1.26.3 为参考，稳定的行为应以 Go 语言规范为准。

---

## 3. nil channel 没有 `hchan`

```go
var ch chan int
```

此时：

```go
ch == nil
```

它没有指向任何 `hchan`：

```text
ch ───> nil
```

因此不存在：

- 缓冲区 `buf`；
- 发送等待队列 `sendq`；
- 接收等待队列 `recvq`；
- channel 锁；
- `closed` 状态。

而正常创建 channel：

```go
ch := make(chan int, 2)
```

才会得到一个实际的 `hchan`：

```text
ch ───> hchan
          ├── closed
          ├── buf
          ├── sendq
          ├── recvq
          └── lock
```

因此，nil channel 不是一个“已经关闭的 channel”，而是一个没有通信端点的合法零值。

---

## 4. nil channel 为什么收发都永久阻塞？

### 4.1 向 nil channel 发送

```go
var ch chan int
ch <- 1
```

runtime 发送路径会先检查 channel 指针，逻辑可以简化为：

```go
func chansend(c *hchan, elem unsafe.Pointer, block bool, pc uintptr) bool {
	if c == nil {
		if !block {
			return false
		}

		gopark(nil, nil, waitReasonChanSendNilChan, traceBlockForever, 2)
		throw("unreachable")
	}

	// 正常 channel 的发送逻辑
}
```

关键是：

```go
gopark(...)
```

`gopark` 会把当前 goroutine 挂起并让出线程，不是让线程执行忙等循环：

```text
G1 执行 ch <- 1
        │
        ▼
发现 ch == nil
        │
        ▼
G1 被 park
        │
        ▼
没有 hchan，也没有唤醒队列
        │
        ▼
永久阻塞
```

之所以永久不会被唤醒，是因为 nil channel 没有：

- `hchan`；
- `sendq`；
- `recvq`；
- 可以匹配当前发送的接收者；
- 可以修改等待状态并唤醒 goroutine 的 channel 对象。

所以“永久阻塞”表示 goroutine 进入等待状态后永远无法恢复执行，不表示它一直占用 CPU。

### 4.2 从 nil channel 接收

```go
var ch chan int
v := <-ch
```

接收路径也会检查 `c == nil`：

```go
if c == nil {
	if !block {
		return
	}

	gopark(nil, nil, waitReasonChanReceiveNilChan, traceBlockForever, 2)
	throw("unreachable")
}
```

接收者等待一个永远不存在的发送者，因此同样永久阻塞。

### 4.3 nil channel 不会直接 panic

nil channel 是合法的 channel 零值，不是非法对象。Go 将它定义成“永远不会就绪的通信端点”，这使它可以安全参与 `select`。

如果发送 nil channel 直接 panic，就无法使用 nil channel 动态禁用某个 `select` case。

---

## 5. nil channel 在 `select` 中的作用

### 5.1 nil 接收 case 永远不会被选中

```go
var ch chan int

select {
case v := <-ch:
	fmt.Println(v)
case <-time.After(time.Second):
	fmt.Println("timeout")
}
```

从 nil channel 接收永远不会完成，因此第一个 case 永远不会就绪，最终执行 timeout 分支。

### 5.2 nil 发送 case 永远不会被选中

```go
var output chan int

select {
case output <- 1:
	fmt.Println("sent")
case <-time.After(time.Second):
	fmt.Println("timeout")
}
```

`output == nil` 时，发送分支永远无法完成，另一个 case 仍然可以被选择。

### 5.3 配合 `default` 实现非阻塞操作

```go
var ch chan int

select {
case ch <- 1:
	fmt.Println("发送成功")
default:
	fmt.Println("发送失败")
}
```

输出：

```text
发送失败
```

runtime 的 `block` 参数决定行为：

```go
if c == nil {
	if !block {
		return false
	}

	gopark(...)
}
```

因此：

- 阻塞发送/接收：永久阻塞；
- 非阻塞发送/接收：立即失败；
- `select` 中没有 `default`：该 nil case 不会就绪；
- `select` 中有 `default`：立即选择 `default`。

---

## 6. 通过 nil 动态禁用和恢复 `select` 分支

### 6.1 `ch = nil` 只是改变变量指向

```go
ch := make(chan int, 1)
saved := ch

ch = nil
```

此时：

```text
ch    ───> nil
saved ───> 原来的 hchan
```

原来的 channel 没有被关闭，也没有被销毁。重新赋值：

```go
ch = saved
```

之后：

```text
ch    ───> 原来的 hchan
saved ───> 原来的 hchan
```

因此：

```go
ch = nil
```

是暂时禁用这个变量对应的通信分支，不是关闭 channel。

### 6.2 禁用一个 `select` 分支

```go
package main

import "fmt"

func main() {
	ch := make(chan int, 1)
	var input <-chan int = ch

	select {
	case v := <-input:
		fmt.Println(v)
	default:
		fmt.Println("没有数据")
	}

	input = nil

	select {
	case v := <-input:
		fmt.Println(v)
	default:
		fmt.Println("input 已禁用")
	}
}
```

当：

```go
input = nil
```

下面的 case：

```go
case v := <-input:
```

永远不会就绪。

### 6.3 恢复被 nil 禁用的分支

只要原 channel 仍然开放，并且还保存着它的引用，就可以重新赋值：

```go
package main

import "fmt"

func main() {
	ch := make(chan int, 1)
	var input <-chan int = ch

	input = nil

	select {
	case v := <-input:
		fmt.Println(v)
	default:
		fmt.Println("input 已禁用")
	}

	input = ch
	ch <- 42

	select {
	case v := <-input:
		fmt.Println(v)
	default:
		fmt.Println("没有数据")
	}
}
```

输出：

```text
input 已禁用
42
```

### 6.4 循环中动态暂停和恢复

```go
func consume(
	ch <-chan int,
	pause, resume, done <-chan struct{},
) {
	input := ch

	for {
		select {
		case v := <-input:
			fmt.Println("收到：", v)

		case <-pause:
			input = nil

		case <-resume:
			input = ch

		case <-done:
			return
		}
	}
}
```

`input = nil` 后，接收分支被禁用；`input = ch` 后，该分支恢复参与 `select`。

### 6.5 关闭后的 channel 为什么常常设置为 nil？

处理多个输入 channel 时，经常使用：

```go
for ch1 != nil || ch2 != nil {
	select {
	case v, ok := <-ch1:
		if !ok {
			ch1 = nil
			continue
		}
		fmt.Println(v)

	case v, ok := <-ch2:
		if !ok {
			ch2 = nil
			continue
		}
		fmt.Println(v)
	}
}
```

已关闭且排空的 channel 会立即返回：

```text
v  = 零值
ok = false
```

如果不把它设置为 nil，这个 case 可能不断被选中，导致循环空转。设置：

```go
ch1 = nil
```

就是把已经结束的分支从 `select` 中移除。

### 6.6 并发修改 channel 变量的注意事项

channel 本身可以被多个 goroutine 并发收发，但保存 channel 的变量不一定可以被并发读写：

```go
var ch chan int

go func() {
	ch = nil
}()

go func() {
	select {
	case <-ch:
	}
}()
```

这里可能发生 data race。

安全做法是：

- 由执行 `select` 的同一个 goroutine 修改 channel 变量；
- 通过另一个 channel 传递更新指令；
- 使用 `sync.Mutex` 保护变量；
- 使用合适的 atomic 类型保护状态。

---

## 7. 正常 channel 的发送路径

创建 buffered channel：

```go
ch := make(chan int, 2)
```

初始状态可以简化为：

```text
hchan:
    dataqsiz = 2
    qcount   = 0
    closed   = 0
    sendx    = 0
    recvx    = 0
    buf      = 环形缓冲区
```

发送时，runtime 大致按照以下顺序处理：

```go
lock(&c.lock)

if c.closed != 0 {
	unlock(&c.lock)
	panic("send on closed channel")
}

if sg := c.recvq.dequeue(); sg != nil {
	// 有等待中的接收者，直接交接
	send(c, sg, ep, ...)
	return true
}

if c.qcount < c.dataqsiz {
	// 缓冲区有空间，写入环形队列
	qp := chanbuf(c, c.sendx)
	typedmemmove(c.elemtype, qp, ep)

	c.sendx++
	c.qcount++
	unlock(&c.lock)
	return true
}

// 缓冲区已满，当前 goroutine 加入 sendq 并阻塞
```

发送有三种情况：

1. 有等待中的接收者：直接交接数据；
2. buffered channel 有空间：写入缓冲区；
3. 没有接收者且缓冲区已满：进入 `sendq` 阻塞。

---

## 8. 无缓冲 channel：直接交接

```go
ch := make(chan int)
```

无缓冲 channel 的：

```go
dataqsiz = 0
```

它没有保存元素的缓冲区。如果接收者已经等待：

```go
go func() {
	v := <-ch
	fmt.Println(v)
}()

ch <- 42
```

发送者会从 `recvq` 中找到接收者：

```go
if sg := c.recvq.dequeue(); sg != nil {
	send(c, sg, ep, ...)
	return true
}
```

数据不会先写入 buffer，而是直接复制到接收者的目标变量：

```text
发送者栈上的 42
        │
        │ 直接复制
        ▼
接收者栈上的 v
```

因此无缓冲 channel 更像一次同步握手：

```text
发送者和接收者必须同时到场
双方完成一次直接交接
```

如果发送者先到，它进入 `sendq`；如果接收者先到，它进入 `recvq`。另一方到达后，runtime 将二者配对并唤醒等待者。

---

## 9. buffered channel：环形队列

```go
ch := make(chan int, 3)
```

缓冲区可以表示为：

```text
buf:
+-----+-----+-----+
|  0  |  1  |  2  |
+-----+-----+-----+
```

发送 `10`：

```text
buf:
+-----+-----+-----+
| 10  |     |     |
+-----+-----+-----+

sendx  = 1
recvx  = 0
qcount = 1
```

继续发送 `20` 和 `30`：

```text
buf:
+-----+-----+-----+
| 10  | 20  | 30  |
+-----+-----+-----+

sendx  = 0
recvx  = 0
qcount = 3
```

接收一次，取出 `recvx` 位置的 `10`：

```text
buf:
+-----+-----+-----+
|     | 20  | 30  |
+-----+-----+-----+

sendx  = 0
recvx  = 1
qcount = 2
```

再次发送 `40`，`sendx` 绕回 0：

```text
buf:
+-----+-----+-----+
| 40  | 20  | 30  |
+-----+-----+-----+

sendx  = 1
recvx  = 1
qcount = 3
```

字段作用：

- `sendx`：下一次写入的位置；
- `recvx`：下一次读取的位置；
- `qcount`：当前元素数量。

---

## 10. `close` 的底层语义

### 10.1 `close` 不是销毁 channel

关闭 channel 的核心动作不是销毁对象，而是把 `hchan.closed` 设置为 1：

```go
func closechan(c *hchan) {
	if c == nil {
		panic("close of nil channel")
	}

	lock(&c.lock)

	if c.closed != 0 {
		unlock(&c.lock)
		panic("close of closed channel")
	}

	c.closed = 1

	// 唤醒等待中的接收者
	// 唤醒等待中的发送者

	unlock(&c.lock)
}
```

关闭后的状态是：

```text
关闭发送入口
保留接收出口
保留缓冲区中的已有数据
```

### 10.2 为什么向已关闭 channel 发送会 panic？

发送路径在真正发送前会检查：

```go
lock(&c.lock)

if c.closed != 0 {
	unlock(&c.lock)
	panic("send on closed channel")
}
```

只要：

```go
c.closed == 1
```

发送就会 panic，与下面这些因素无关：

- channel 是否有缓冲；
- 缓冲区是否还有空间；
- 是否有等待中的接收者；
- channel 是否刚刚关闭。

```go
ch := make(chan int, 1)
close(ch)
ch <- 1 // panic: send on closed channel
```

关闭表示发送方宣布以后不会再发送数据。关闭后继续发送违反了 channel 通信协议。

### 10.3 为什么 `close(nil)` 会 panic？

发送和接收是等待通信完成：

```text
发送 nil channel → 等待不存在的接收者
接收 nil channel → 等待不存在的发送者
```

而关闭是修改 channel 状态：

```go
c.closed = 1
```

nil channel 没有 `hchan`，没有 `closed` 字段可以修改，因此：

```go
var ch chan int
close(ch) // panic: close of nil channel
```

可以这样记忆：

| 操作 | nil channel 的含义 |
| --- | --- |
| 发送 | 等待一个永远不存在的接收者 |
| 接收 | 等待一个永远不存在的发送者 |
| 关闭 | 试图关闭不存在的 channel，因此 panic |

---

## 11. 从已关闭 channel 接收

### 11.1 缓冲区有数据：先正常读取

```go
ch := make(chan int, 2)
ch <- 10
ch <- 20
close(ch)
```

关闭时：

```text
closed = 1
qcount = 2
```

`close` 不会清空 `buf`。接收路径会先判断是否还有缓冲数据：

```go
if c.closed != 0 {
	if c.qcount == 0 {
		// 已关闭且已排空，返回零值和 false
		return zero, false
	}

	// 已关闭但仍有数据，继续读取
}

if c.qcount > 0 {
	qp := chanbuf(c, c.recvx)
	typedmemmove(c.elemtype, ep, qp)
	typedmemclr(c.elemtype, qp)

	c.recvx++
	c.qcount--
	return value, true
}
```

结果是：

```text
第一次接收：10, true
第二次接收：20, true
第三次接收：0,  false
```

### 11.2 缓冲区为空：返回零值和 `ok == false`

```go
ch := make(chan int)
close(ch)

v, ok := <-ch
fmt.Println(v, ok) // 0 false
```

runtime 接收路径可以简化为：

```go
lock(&c.lock)

if c.closed != 0 && c.qcount == 0 {
	unlock(&c.lock)
	clear(receiveTarget)
	return true, false
}
```

`clear(receiveTarget)` 会把接收目标设置为元素类型的零值：

| 类型 | 零值 |
| --- | --- |
| `int` | `0` |
| `string` | `""` |
| `bool` | `false` |
| 指针 | `nil` |
| slice | `nil` |
| struct | 所有字段为零值 |

接收的第二个返回值表示是否真的收到数据：

- `ok == true`：收到真实发送的值；
- `ok == false`：channel 已关闭且没有更多数据。

因此不能只通过值判断 channel 是否关闭，因为发送方也可能发送元素类型的零值：

```go
ch := make(chan int, 1)
ch <- 0
close(ch)

v, ok := <-ch
fmt.Println(v, ok) // 0 true

v, ok = <-ch
fmt.Println(v, ok) // 0 false
```

---

## 12. 关闭时如何处理阻塞中的 goroutine？

### 12.1 阻塞中的接收者

```go
ch := make(chan int)

go func() {
	v, ok := <-ch
	fmt.Println(v, ok)
}()

time.Sleep(time.Second)
close(ch)
```

接收者没有等到发送者，会被放入：

```go
c.recvq
```

并阻塞。关闭 channel 时，runtime 遍历接收队列，将等待者标记为失败并唤醒：

```go
for {
	sg := c.recvq.dequeue()
	if sg == nil {
		break
	}

	if sg.elem.get() != nil {
		typedmemclr(c.elemtype, sg.elem.get())
		sg.elem.set(nil)
	}

	gp := sg.g
	gp.param = unsafe.Pointer(sg)
	sg.success = false
	glist.push(gp)
}
```

被唤醒的接收者得到：

```text
v  = 0
ok = false
```

### 12.2 阻塞中的发送者

```go
ch := make(chan int, 1)
ch <- 1

go func() {
	ch <- 2
	fmt.Println("发送完成")
}()

time.Sleep(time.Second)
close(ch)
```

此时缓冲区已满，发送者进入：

```go
c.sendq
```

关闭 channel 时，runtime 会唤醒等待中的发送者，但将其标记为失败：

```go
sg.success = false
```

发送者醒来后检查：

```go
closed := !mysg.success

if closed {
	if c.closed == 0 {
		throw("chansend: spurious wakeup")
	}

	panic("send on closed channel")
}
```

因此关闭 channel 不会让阻塞中的发送成功，而是让发送者醒来后发现发送失败并 panic。

---

## 13. nil channel 与 deadlock 的区别

单独运行下面的程序：

```go
package main

func main() {
	var ch chan int
	ch <- 1
}
```

发送操作本身执行的是 `gopark`，不是直接 panic。但程序中没有其他可运行的 goroutine，runtime 的死锁检测器发现所有 goroutine 都在等待，于是最终报告：

```text
fatal error: all goroutines are asleep - deadlock!
```

这不是 nil channel 发送直接 panic，而是 runtime 后续发现整个程序无法继续执行。

如果还有其他 goroutine 可以运行：

```go
func main() {
	var ch chan int

	go func() {
		for {
			// 继续运行
		}
	}()

	ch <- 1
}
```

主 goroutine 会继续阻塞，但因为另一个 goroutine 仍然可运行，不会立即触发“所有 goroutine 都睡眠”的 deadlock 检测。

---

## 14. channel 状态转换

### 14.1 nil channel

```text
nil
 │
 ├── 阻塞发送 → 永久阻塞
 ├── 阻塞接收 → 永久阻塞
 ├── 非阻塞发送 → 立即失败
 ├── 非阻塞接收 → 立即失败
 └── close → panic
```

nil channel 没有 `hchan`，因此没有正常的状态转换。

### 14.2 open channel

```text
open
 │
 ├── close → closed
 ├── send → 直接交接 / 写入缓冲区 / 阻塞
 └── recv → 直接交接 / 读取缓冲区 / 阻塞
```

### 14.3 closed channel

```text
closed
 │
 ├── send → panic
 └── recv
       ├── qcount > 0 → 返回缓冲区数据
       └── qcount == 0 → 返回零值，ok=false
```

---

## 15. nil 禁用和 close 关闭的区别

| 操作 | 影响对象 | 是否可恢复 | 后续行为 |
| --- | --- | --- | --- |
| `ch = nil` | channel 变量 | 可以 | 该变量上的 `select` case 永远不就绪 |
| `close(ch)` | channel 对象对应的 `hchan` | 不可以 | 发送 panic，接收已有数据或返回零值 |
| `ch = make(chan T)` | channel 变量 | 相当于创建新 channel | 指向新的开放 `hchan` |

### 15.1 nil 禁用后可以恢复

```go
saved := ch
ch = nil
ch = saved
```

前提是 `saved` 仍然指向一个开放 channel。

### 15.2 已关闭 channel 不能重新打开

```go
ch := make(chan int)
oldCh := ch
close(ch)

ch = nil
ch = oldCh
```

此时 `ch` 仍然指向原来的已关闭 `hchan`：

```text
hchan.closed = 1
```

重新设置变量不会把：

```go
hchan.closed = 1
```

恢复为：

```go
hchan.closed = 0
```

如果需要重新使用，必须创建新的 channel：

```go
ch = make(chan int)
ch <- 1
```

这不是重新打开旧 channel，而是让变量指向一个新的 `hchan`。

---

## 16. 一份完整示例

```go
package main

import "fmt"

func main() {
	ch := make(chan int, 2)

	ch <- 10
	ch <- 20
	close(ch)

	v, ok := <-ch
	fmt.Println(v, ok) // 10 true

	v, ok = <-ch
	fmt.Println(v, ok) // 20 true

	v, ok = <-ch
	fmt.Println(v, ok) // 0 false

	v, ok = <-ch
	fmt.Println(v, ok) // 0 false
}
```

状态变化：

```text
创建：
closed = 0
qcount = 0

发送 10：
closed = 0
qcount = 1

发送 20：
closed = 0
qcount = 2

关闭：
closed = 1
qcount = 2

接收：
closed = 1
qcount = 1
返回 10, true

接收：
closed = 1
qcount = 0
返回 20, true

接收：
closed = 1
qcount = 0
返回 0, false
```

---

## 17. 总结

可以从 channel 的三个核心状态理解全部规则：

```text
nil：
    没有 hchan
    阻塞收发永久等待
    非阻塞收发立即失败
    select case 永远不会就绪
    close(nil) panic

open：
    根据 recvq、sendq、buf 和 qcount
    决定直接交接、入队、出队或阻塞

closed：
    关闭发送入口
    保留接收出口和已有缓冲数据
    缓冲区有数据就继续返回数据
    缓冲区为空就返回零值和 ok=false
```

最准确的一句话是：

> `close` 不是销毁 channel，而是把 `hchan.closed` 置为 1，并唤醒等待者；之后禁止发送，允许接收者把缓冲区中的数据排空，排空后接收零值。`ch = nil` 只是让某个变量暂时不再指向 channel，常用于动态禁用 `select` 分支；只要保存原 channel 引用，就可以重新启用该分支。
