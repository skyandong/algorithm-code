# Golang Goroutine 面试题集

> 题目来源：[interview-go](https://github.com/lifei6671/interview-go) · [鸟窝 - Go 并发编程小测验](https://colobu.com/2019/04/28/go-concurrency-quizzes/)

---

## 一、编程手写题

### 题 1：Goroutine + Channel 基础

写代码实现两个 goroutine，其中一个产生随机数并写入 go channel，另外一个从 channel 中读取数字并打印到标准输出。最终输出五个随机数。

**要点提示：**

- `goroutine` 在 Go 中是非阻塞的
- 无缓冲 `channel` 读写都是阻塞的，可用 `for range` 读取，管道关闭后 `for` 退出
- 可使用 `select case` 从管道读取数据
- 注意 goroutine 间的同步（`sync.WaitGroup` 或 `done channel`）

```go
// TODO: 在这里写你的代码
```

---

### 题 2：阻塞读并发安全 Map

Go 里面 Map 如何实现 key 不存在时 `get` 操作等待，直到 key 存在或者超时。要求保证并发安全，且实现以下接口：

```go
type sp interface {
    Out(key string, val interface{})                       // 存入 key/val，如果该 key 读取的 goroutine 挂起，则唤醒。此方法不会阻塞
    Rd(key string, timeout time.Duration) interface{}      // 读取一个 key，如果 key 不存在则阻塞，等待 key 存在或者超时
}
```

**要点提示：**

- 阻塞协程第一个想到 `channel`
- 并发安全需要锁
- 多个 goroutine 读同一个不存在的 key 时需要都能被唤醒
- 每个 key 可能需要一个独立的阻塞 channel

```go
// TODO: 在这里写你的代码
```

---

### 题 3：高并发 IP 限流

场景：在一个高并发的 web 服务器中，要限制 IP 的频繁访问。现模拟 100 个 IP 同时并发访问服务器，每个 IP 要重复访问 1000 次。每个 IP 三分钟之内只能访问一次。

**修改以下代码，完成该过程，要求能成功输出 `success:100`：**

```go
package main

import (
	"fmt"
	"time"
)

type Ban struct {
	visitIPs map[string]time.Time
}

func NewBan() *Ban {
	return &Ban{visitIPs: make(map[string]time.Time)}
}

func (o *Ban) visit(ip string) bool {
	if _, ok := o.visitIPs[ip]; ok {
		return true
	}
	o.visitIPs[ip] = time.Now()
	return false
}

func main() {
	success := 0
	ban := NewBan()
	for i := 0; i < 1000; i++ {
		for j := 0; j < 100; j++ {
			go func() {
				ip := fmt.Sprintf("192.168.1.%d", j)
				if !ban.visit(ip) {
					success++
				}
			}()
		}
	}
	fmt.Println("success:", success)
}
```

**需要修复的问题：**

1. `map` 并发读写不安全
2. `for` 循环中启动 goroutine 时的闭包变量捕获问题
3. `success++` 不是原子操作
4. 三分钟内同一 IP 只能访问一次（需要过期清理机制）

```go
// TODO: 在这里写你的修复后代码
```

---

### 题 4：定时调用 + panic 恢复

写出以下逻辑，要求每秒钟调用一次 `proc` 并保证程序不退出：

```go
package main

func main() {
    go func() {
        // 1. 在这里需要你写算法
        // 2. 要求每秒钟调用一次 proc 函数
        // 3. 要求程序不能退出
    }()

    select {}
}

func proc() {
    panic("ok")
}
```

**要点提示：**

- 定时执行 → `time.NewTicker`
- 捕获 panic → `recover()`
- 程序不能退出 → panic 必须在 goroutine 内部被 recover

```go
// TODO: 在这里写你的代码
```

---

### 题 5：WaitGroup 支持 WaitTimeout

为 `sync.WaitGroup` 的 `Wait` 函数支持 `WaitTimeout` 功能：

```go
package main

import (
	"fmt"
	"sync"
	"time"
)

func main() {
	wg := sync.WaitGroup{}
	c := make(chan struct{})
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(num int, close <-chan struct{}) {
			defer wg.Done()
			<-close
			fmt.Println(num)
		}(i, c)
	}

	if WaitTimeout(&wg, time.Second*5) {
		close(c)
		fmt.Println("timeout exit")
	}
	time.Sleep(time.Second * 10)
}

func WaitTimeout(wg *sync.WaitGroup, timeout time.Duration) bool {
	// 要求手写代码
	// 如果 timeout 到了超时时间返回 true
	// 如果 WaitGroup 自然结束返回 false
}
```

**要点提示：**

- `wg.Wait()` 和 `time.Timer` 都会阻塞
- 两个阻塞需要各启动一个 goroutine
- 用无缓冲 channel 来竞争"谁先完成"

```go
// TODO: 补全 WaitTimeout 函数
```

---

### 题 6：多协程查询切片 + context 取消

假设有一个超长的切片，元素类型为 `int`，元素为乱序排列。限时 5 秒，使用多个 goroutine 查找切片中是否存在给定的值。在查找到目标值或者超时后立刻结束所有 goroutine 的执行。

比如，切片 `[23, 32, 78, 43, 76, 65, 345, 762, ..., 915, 86]`，查找目标值 345：
- 如果切片中存在，输出 `"Found it!"` 并立即取消仍在执行查询任务的 goroutine
- 如果超时未查到，输出 `"Timeout! Not Found"`，同时立即取消仍在执行的 goroutine

```go
// TODO: 在这里写你的代码
```

---

## 二、选择题

> 以下题目来自 [鸟窝 - Go 并发编程小测验](https://colobu.com/2019/04/28/go-concurrency-quizzes/)

### 题 7：Mutex 死锁

```go
package main

import (
	"fmt"
	"sync"
)

var mu sync.Mutex
var chain string

func main() {
	chain = "main"
	A()
	fmt.Println(chain)
}

func A() {
	mu.Lock()
	defer mu.Unlock()
	chain = chain + " --> A"
	B()
}

func B() {
	chain = chain + " --> B"
	C()
}

func C() {
	mu.Lock()
	defer mu.Unlock()
	chain = chain + " --> C"
}
```

- A: 不能编译
- B: 输出 `main --> A --> B --> C`
- C: 输出 `main`
- D: panic

---

### 题 8：RWMutex 死锁

```go
package main

import (
	"fmt"
	"sync"
	"time"
)

var mu sync.RWMutex
var count int

func main() {
	go A()
	time.Sleep(2 * time.Second)
	mu.Lock()
	defer mu.Unlock()
	count++
	fmt.Println(count)
}

func A() {
	mu.RLock()
	defer mu.RUnlock()
	B()
}

func B() {
	time.Sleep(5 * time.Second)
	C()
}

func C() {
	mu.RLock()
	defer mu.RUnlock()
}
```

- A: 不能编译
- B: 输出 1
- C: 程序 hang 住
- D: panic

---

### 题 9：WaitGroup

```go
package main

import (
	"sync"
	"time"
)

func main() {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		time.Sleep(time.Millisecond)
		wg.Done()
		wg.Add(1)
	}()
	wg.Wait()
}
```

- A: 不能编译
- B: 无输出，正常退出
- C: 程序 hang 住
- D: panic

---

### 题 10：双检查实现单例

```go
package doublecheck

import "sync"

type Once struct {
	m    sync.Mutex
	done uint32
}

func (o *Once) Do(f func()) {
	if o.done == 1 {
		return
	}
	o.m.Lock()
	defer o.m.Unlock()
	if o.done == 0 {
		o.done = 1
		f()
	}
}
```

- A: 不能编译
- B: 可以编译，正确实现了单例
- C: 可以编译，有并发问题，f 函数可能会被执行多次
- D: 可以编译，但是程序运行会 panic

---

### 题 11：Mutex 值复制

```go
package main

import (
	"fmt"
	"sync"
)

type MyMutex struct {
	count int
	sync.Mutex
}

func main() {
	var mu MyMutex
	mu.Lock()
	var mu2 = mu
	mu.count++
	mu.Unlock()
	mu2.Lock()
	mu2.count++
	mu2.Unlock()
	fmt.Println(mu.count, mu2.count)
}
```

- A: 不能编译
- B: 输出 1, 1
- C: 输出 1, 2
- D: panic

---

### 题 12：sync.Pool 内存

```go
package main

import (
	"bytes"
	"fmt"
	"runtime"
	"sync"
	"time"
)

var pool = sync.Pool{New: func() interface{} { return new(bytes.Buffer) }}

func main() {
	go func() {
		for {
			processRequest(1 << 28) // 256MiB
		}
	}()
	for i := 0; i < 1000; i++ {
		go func() {
			for {
				processRequest(1 << 10) // 1KiB
			}
		}()
	}
	var stats runtime.MemStats
	for i := 0; ; i++ {
		runtime.ReadMemStats(&stats)
		fmt.Printf("Cycle %d: %dB\n", i, stats.Alloc)
		time.Sleep(time.Second)
		runtime.GC()
	}
}

func processRequest(size int) {
	b := pool.Get().(*bytes.Buffer)
	time.Sleep(500 * time.Millisecond)
	b.Grow(size)
	pool.Put(b)
	time.Sleep(1 * time.Millisecond)
}
```

- A: 不能编译
- B: 可以编译，运行时正常，内存稳定
- C: 可以编译，运行时内存可能暴涨
- D: 可以编译，运行时内存先暴涨，但是过一会会回收掉

---

### 题 13：Channel goroutine 泄漏

```go
package main

import (
	"fmt"
	"runtime"
	"time"
)

func main() {
	var ch chan int
	go func() {
		ch = make(chan int, 1)
		ch <- 1
	}()
	go func(ch chan int) {
		time.Sleep(time.Second)
		<-ch
	}(ch)
	c := time.Tick(1 * time.Second)
	for range c {
		fmt.Printf("#goroutines: %d\n", runtime.NumGoroutine())
	}
}
```

- A: 不能编译
- B: 一段时间后总是输出 `#goroutines: 1`
- C: 一段时间后总是输出 `#goroutines: 2`
- D: panic

---

### 题 14：关闭 nil channel

```go
package main

import "fmt"

func main() {
	var ch chan int
	var count int
	go func() {
		ch <- 1
	}()
	go func() {
		count++
		close(ch)
	}()
	<-ch
	fmt.Println(count)
}
```

- A: 不能编译
- B: 输出 1
- C: 输出 0
- D: panic

---

### 题 15：sync.Map

```go
package main

import (
	"fmt"
	"sync"
)

func main() {
	var m sync.Map
	m.LoadOrStore("a", 1)
	m.Delete("a")
	fmt.Println(m.Len())
}
```

- A: 不能编译
- B: 输出 1
- C: 输出 0
- D: panic

---

### 题 16：Happens-Before

```go
package main

var c = make(chan int)
var a int

func f() {
	a = 1
	<-c
}

func main() {
	go f()
	c <- 0
	print(a)
}
```

- A: 不能编译
- B: 输出 1
- C: 输出 0
- D: panic

---

### 题 17：自定义 Map 并发

```go
package main

import "sync"

type Map struct {
	m map[int]int
	sync.Mutex
}

func (m *Map) Get(key int) (int, bool) {
	m.Lock()
	defer m.Unlock()
	i, ok := m.m[key]
	return i, ok
}

func (m *Map) Put(key, value int) {
	m.Lock()
	defer m.Unlock()
	m.m[key] = value
}

func (m *Map) Len() int {
	return len(m.m)
}

func main() {
	var wg sync.WaitGroup
	wg.Add(2)
	m := Map{m: make(map[int]int)}
	go func() {
		for i := 0; i < 10000000; i++ {
			m.Put(i, i)
		}
		wg.Done()
	}()
	go func() {
		for i := 0; i < 10000000; i++ {
			m.Len()
		}
		wg.Done()
	}()
	wg.Wait()
}
```

- A: 不能编译
- B: 可运行，无并发问题
- C: 可运行，有并发问题（data race）
- D: panic

---

### 题 18：Slice 并发 append

```go
package main

import (
	"fmt"
	"sync"
)

func main() {
	var wg sync.WaitGroup
	wg.Add(2)
	var ints = make([]int, 0, 1000)
	go func() {
		for i := 0; i < 1000; i++ {
			ints = append(ints, i)
		}
		wg.Done()
	}()
	go func() {
		for i := 0; i < 1000; i++ {
			ints = append(ints, i)
		}
		wg.Done()
	}()
	wg.Wait()
	fmt.Println(len(ints))
}
```

- A: 不能编译
- B: 输出 2000
- C: 输出可能不是 2000
- D: panic

---

### 题 19：Goroutine 闭包陷阱

```go
package main

import (
	"fmt"
	"sync"
	"time"
)

type T struct {
	V int
}

func (t *T) Incr(wg *sync.WaitGroup) {
	t.V++
	wg.Done()
}

func (t *T) Print() {
	time.Sleep(1e9)
	fmt.Print(t.V)
}

func main() {
	var wg sync.WaitGroup
	wg.Add(10)
	var ts = make([]T, 10)
	for i := 0; i < 10; i++ {
		ts[i] = T{i}
	}
	for _, t := range ts {
		go t.Incr(&wg)
	}
	wg.Wait()
	for _, t := range ts {
		go t.Print()
	}
	time.Sleep(5 * time.Second)
}
```

- A: 输出 12345678910
- B: 输出 0123456789
- C: 输出 9999999999
- D: 输出 10101010101010101010

---

## 三、简答分析题

### 题 20：对已关闭的 channel 进行读写，会怎么样？为什么？

请回答：

1. 写已关闭的 channel 会发生什么？
2. 读已关闭的 channel 会发生什么（分两种情况讨论）？
3. 从 `runtime/chan.go` 源码层面解释原因。

---

## 参考答案

> ⚠️ 先自己写，写完再看答案！

<details>
<summary><b>题 1 答案</b></summary>

```go
package main

import (
	"fmt"
	"math/rand"
	"sync"
)

func main() {
	out := make(chan int)
	wg := sync.WaitGroup{}
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 5; i++ {
			out <- rand.Intn(100)
		}
		close(out)
	}()
	go func() {
		defer wg.Done()
		for v := range out {
			fmt.Println(v)
		}
	}()
	wg.Wait()
}
```
</details>

<details>
<summary><b>题 4 答案</b></summary>

```go
package main

import (
	"fmt"
	"time"
)

func main() {
	go func() {
		t := time.NewTicker(time.Second)
		for range t.C {
			go func() {
				defer func() {
					if err := recover(); err != nil {
						fmt.Println(err)
					}
				}()
				proc()
			}()
		}
	}()

	select {}
}

func proc() {
	panic("ok")
}
```
</details>

<details>
<summary><b>题 5 答案</b></summary>

```go
func WaitTimeout(wg *sync.WaitGroup, timeout time.Duration) bool {
	ch := make(chan bool, 1)

	go time.AfterFunc(timeout, func() {
		ch <- true
	})

	go func() {
		wg.Wait()
		ch <- false
	}()

	return <-ch
}
```
</details>

<details>
<summary><b>选择题答案</b></summary>

| 题号 | 答案 | 解析 |
|------|------|------|
| 7 | **D** | `sync.Mutex` 不可重入，`A()` lock 后在调用链中 `C()` 再次 lock → 死锁 panic |
| 8 | **D** | 写锁等待时新来的读锁被阻塞（优先写锁），`A` 的读锁又在等 `C` 完成 → 死锁 panic |
| 9 | **D** | `Wait()` 返回后不能再调用 `Add()` → panic |
| 10 | **C** | 多核 CPU 缓存导致 `done` 在不同核心间不同步，`f()` 可能被多次执行 |
| 11 | **D** | `mu2 = mu` 是值复制，锁状态也被复制，`mu2` 是已加锁状态 → 再 Lock 死锁 |
| 12 | **C** | 256MiB 的 buffer 放回 pool 后被 1KiB 的请求取走但未缩容 → 内存暴涨 |
| 13 | **C** | `ch` 为 nil，第一个 goroutine 中 `ch <- 1` 永久阻塞（向 nil channel 写），永远 2 个 goroutine |
| 14 | **D** | `ch` 为 nil，关闭 nil channel → panic |
| 15 | **A** | `sync.Map` 没有 `Len()` 方法 → 编译失败 |
| 16 | **B** | `c <- 0` 阻塞直到 `f()` 中 `<-c` 读到，`a=1` happens-before `print(a)` → 输出 1 |
| 17 | **C** | `Len()` 没加锁，与 `Put()` 并发读写 map → data race |
| 18 | **C** | 多个 goroutine 并发 `append`，slice 底层数组可能被覆盖 → `len` < 2000 |
| 19 | **D** | `for _, t := range ts` 中 `t` 是值副本，`Incr` 也是值接收者（操作副本），`ts` 原元素 `V` 不变；但 `Print` 也是值副本，打印的 `t.V` 依赖编译器对 range 变量 `t` 的处理——最终 `ts` 原数组 `V` 全为 0，但 print 的 `t` 是最后一个值 10 的副本（关键：`t.Incr` 是值接收者，操作的是副本不是原数组；而 `t.Print` 中的 `t` 可能是 range 最后的值）→ 输出 10 个 10 |
</details>

<details>
<summary><b>题 20 答案</b></summary>

### 写已关闭的 channel → panic

```go
c := make(chan int)
close(c)
c <- 1  // panic: send on closed channel
```

源码 `runtime/chan.go`：
```go
func chansend(c *hchan, ep unsafe.Pointer, block bool, callerpc uintptr) bool {
    if c.closed != 0 {
        unlock(&c.lock)
        panic(plainError("send on closed channel"))
    }
}
```

### 读已关闭的 channel

分两种情况：
1. **关闭前还有未读元素** → 正常读出元素，第二个返回值 `ok=true`
2. **元素已读完** → 非阻塞直接返回零值，`ok=false`

源码：
```go
func chanrecv(c *hchan, ep unsafe.Pointer, block bool) (selected, received bool) {
    if c.closed != 0 && c.qcount == 0 {
        // 缓冲区为空 → 返回零值
        if ep != nil {
            typedmemclr(c.elemtype, ep)
        }
        return true, false  // selected=true, received=false
    }
}
```
</details>
