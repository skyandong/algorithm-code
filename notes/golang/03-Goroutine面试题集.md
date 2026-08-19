# Golang Goroutine 面试题集

> 题目来源：[interview-go](https://github.com/lifei6671/interview-go) · [鸟窝 - Go 并发编程小测验](https://colobu.com/2019/04/28/go-concurrency-quizzes/)
>
> 本文按仓库 `go.mod` 声明的 Go 1.26 编写。涉及 `for` 循环变量时，默认采用 Go 1.22 起的语义；如果模块声明的是 `go 1.21` 或更低版本，需要单独考虑旧的循环变量闭包行为。
>
> 文中的 runtime 源码分析以本机 Go 1.26.3 为参考。runtime 内部结构可能随版本变化，语言规范和标准库文档才是稳定契约。

---

## 一、编程手写题

### 题 1：Goroutine + Channel 基础

写代码实现两个 goroutine：一个产生五个随机数并写入 channel，另一个从 channel 中读取数字并打印到标准输出。所有 goroutine 正常退出后，主 goroutine 才退出。

要点：

- 启动 goroutine 不会阻塞当前 goroutine，但 goroutine 自身可以阻塞；
- 无缓冲 channel 的发送和接收需要配对；
- 发送方负责关闭 channel；
- 接收方可以使用 `for range`，channel 关闭且数据读完后循环结束；
- 使用同步机制等待 goroutine 完成。

**答案：**

```go
package main

import (
	"fmt"
	"math/rand"
	"sync"
)

func main() {
	out := make(chan int)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		defer close(out)

		for i := 0; i < 5; i++ {
			out <- rand.Intn(100)
		}
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

**解析：**

`close(out)` 必须由发送方负责，因为发送方知道后续不会再有数据。接收方不能通过关闭 channel 来表示“我已经读完”，否则可能导致发送方向已关闭 channel 发送并 panic。

Go 1.25+ 也可以使用 `WaitGroup.Go` 简化任务启动：

```go
var wg sync.WaitGroup

wg.Go(func() {
	defer close(out)
	for i := 0; i < 5; i++ {
		out <- rand.Intn(100)
	}
})
```

但传统的 `Add`、`Done` 写法仍然有效，也更适合展示 WaitGroup 的基本原理。

---

### 题 2：阻塞读并发安全 Map

实现一个并发安全的 Map：

```go
type SP interface {
	Out(key string, val any)             // 写入 key/val，不阻塞
	Rd(key string, timeout time.Duration) any // key 不存在时等待，超时返回 nil
}
```

要求：

- key 已存在时，`Rd` 立即返回；
- key 不存在时，`Rd` 阻塞等待；
- `Out` 不阻塞；
- 多个 goroutine 等待同一个 key 时，都能被唤醒；
- 等待支持超时；
- mutex 保护共享状态，channel 只负责通知。

**答案：**

```go
package main

import (
	"sync"
	"time"
)

type waiter struct {
	done chan struct{}
}

type blockingMap struct {
	mu      sync.Mutex
	values  map[string]any
	waiters map[string]map[*waiter]struct{}
}

func newBlockingMap() *blockingMap {
	return &blockingMap{
		values:  make(map[string]any),
		waiters: make(map[string]map[*waiter]struct{}),
	}
}

func (m *blockingMap) Out(key string, value any) {
	m.mu.Lock()
	m.values[key] = value

	for w := range m.waiters[key] {
		close(w.done)
	}
	delete(m.waiters, key)
	m.mu.Unlock()
}

func (m *blockingMap) Rd(key string, timeout time.Duration) any {
	m.mu.Lock()
	if value, ok := m.values[key]; ok {
		m.mu.Unlock()
		return value
	}

	w := &waiter{done: make(chan struct{})}
	if m.waiters[key] == nil {
		m.waiters[key] = make(map[*waiter]struct{})
	}
	m.waiters[key][w] = struct{}{}
	m.mu.Unlock()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-w.done:
		m.mu.Lock()
		value := m.values[key]
		m.mu.Unlock()
		return value

	case <-timer.C:
		m.mu.Lock()
		if _, waiting := m.waiters[key][w]; waiting {
			delete(m.waiters[key], w)
			if len(m.waiters[key]) == 0 {
				delete(m.waiters, key)
			}
			m.mu.Unlock()
			return nil
		}
		value := m.values[key]
		m.mu.Unlock()
		return value
	}
}
```

**解析：**

- `mu` 保护 `values` 和 `waiters`，避免并发读写 Map；
- 每个等待者拥有独立的 `done` channel；
- `Out` 写入值后关闭所有等待者的 channel，关闭 channel 会唤醒所有接收者；
- 超时路径重新获取锁，确认等待者是否已经被 `Out` 移除，避免通知和超时同时发生时留下脏 waiter；
- 这个接口用 `nil` 表示超时，因此如果业务允许写入 `nil`，最好改成返回 `(any, bool)`，用布尔值区分“值为 nil”和“超时”。

`Out` 虽然在实现中短暂持有锁，但不会等待其他 goroutine 的通知完成；关闭 channel 本身不会阻塞，因此满足“不等待读取者”的要求。

---

### 题 3：高并发 IP 限流

场景：模拟 100 个 IP，每个 IP 并发访问 1000 次；每个 IP 三分钟内只允许一次访问，要求最终输出：

```text
success: 100
```

原代码的问题包括：

1. 原生 Map 并发读写不安全；
2. 检查和写入不是一个原子操作；
3. `success++` 不是原子操作；
4. 没有等待 goroutine 完成就打印结果；
5. 原代码只判断 key 是否存在，没有实现三分钟 TTL；
6. Go 1.22+ 已经改变了循环变量的默认语义，但显式传参仍然更清晰。

**答案：**

```go
package main

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

type Ban struct {
	mu       sync.Mutex
	visitIPs map[string]time.Time
}

func NewBan() *Ban {
	return &Ban{visitIPs: make(map[string]time.Time)}
}

// visit 返回 true 表示本次访问被限制，false 表示本次访问成功。
func (b *Ban) visit(ip string, now time.Time) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	if last, ok := b.visitIPs[ip]; ok && now.Sub(last) < 3*time.Minute {
		return true
	}

	b.visitIPs[ip] = now
	return false
}

func main() {
	ban := NewBan()
	var success atomic.Int64

	var wg sync.WaitGroup
	wg.Add(100 * 1000)

	for i := 0; i < 1000; i++ {
		for j := 0; j < 100; j++ {
			ip := fmt.Sprintf("192.168.1.%d", j)
			go func(ip string) {
				defer wg.Done()
				if !ban.visit(ip, time.Now()) {
					success.Add(1)
				}
			}(ip)
		}
	}

	wg.Wait()
	fmt.Println("success:", success.Load())
}
```

**解析：**

所有访问都在短时间内发生，因此每个 IP 的第一次访问成功，之后的访问都在三分钟窗口内被限制，最终成功次数为 100。

`visit` 中的检查和写入必须在同一把锁内：

```go
if last, ok := b.visitIPs[ip]; ok && now.Sub(last) < 3*time.Minute {
	return true
}
b.visitIPs[ip] = now
```

如果把检查和写入拆开，即使 Map 本身加锁，也可能有两个 goroutine 同时检查到“未访问”，然后都成功通过。

`success` 使用 `atomic.Int64`，也可以改用另一把锁保护。`WaitGroup` 确保所有 goroutine 完成后再打印。

Go 1.22+ 的普通 `for` 循环变量已经按迭代创建，不再有旧版本中所有闭包捕获同一个 `j` 的经典问题；这里仍然显式计算 `ip` 并作为参数传入，使生命周期和意图更清晰。

生产实现还需要清理长期不访问的 IP，或使用带过期能力的缓存。时间函数最好抽象出来，方便测试三分钟过期逻辑。

---

### 题 4：定时调用与 panic 恢复

实现以下逻辑：每秒调用一次 `proc`，即使 `proc` panic，程序也不能退出。

```go
func proc() {
	panic("ok")
}
```

**答案：**

```go
package main

import (
	"fmt"
	"time"
)

func main() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for range ticker.C {
		callSafely(proc)
	}
}

func callSafely(f func()) {
	defer func() {
		if value := recover(); value != nil {
			fmt.Println("recovered:", value)
		}
	}()

	f()
}

func proc() {
	panic("ok")
}
```

**解析：**

`recover` 只有在发生 panic 的同一个 goroutine 中调用才有效。因此 `callSafely(proc)` 和 `proc` 必须在同一个 goroutine 中执行。

如果每次 tick 都这样写：

```go
for range ticker.C {
	go callSafely(proc)
}
```

那么每次调用都在新的 goroutine 中执行，调用可能重叠；这适合允许并发执行的任务，不适合要求严格串行的任务。

如果使用后台 goroutine，则主 goroutine 需要通过 context 或其他退出信号管理它的生命周期。`time.NewTicker` 创建后应在不再使用时调用 `Stop`；不要无边界地创建 ticker。

---

### 题 5：WaitGroup 支持 WaitTimeout

为 `sync.WaitGroup` 增加超时等待功能：

```go
func WaitTimeout(wg *sync.WaitGroup, timeout time.Duration) bool
```

要求：

- WaitGroup 自然结束时返回 `false`；
- 超时时返回 `true`；
- 超时返回后，调用方仍然需要负责取消实际工作。

**答案：**

```go
package main

import (
	"sync"
	"time"
)

func WaitTimeout(wg *sync.WaitGroup, timeout time.Duration) bool {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-done:
		return false
	case <-timer.C:
		return true
	}
}
```

使用示例：

```go
var wg sync.WaitGroup
stop := make(chan struct{})

for i := 0; i < 10; i++ {
	wg.Add(1)
	go func() {
		defer wg.Done()
		select {
		case <-stop:
		case <-time.After(10 * time.Second):
		}
	}()
}

if WaitTimeout(&wg, 5*time.Second) {
	close(stop)
}
```

**解析：**

`WaitGroup` 没有内置 timeout。实现方式是让一个 goroutine 等待 `wg.Wait()`，然后用 timer 和 `select` 竞争“任务完成”和“超时”两个事件。

`WaitTimeout` 只负责等待，不负责取消任务。超时后，调用方必须通过 context、channel 或其他机制让 worker 退出，否则 worker 仍可能继续运行。

Go 1.25+ 可以使用：

```go
wg.Go(func() {
	// task
})
```

简化任务的 `Add`、启动和 `Done` 配对，但 `WaitGroup.Go` 不提供超时能力，也不能修复 `Add` 与 `Wait` 的错误生命周期关系。

---

### 题 6：多协程查询切片与 context 取消

给定一个很大的 `[]int`，使用多个 goroutine 查找目标值：

- 找到目标后输出 `Found it!`，取消其他 worker；
- 5 秒内找不到则输出 `Timeout! Not Found`；
- 所有 worker 都应最终退出。

**答案：**

```go
package main

import (
	"context"
	"fmt"
	"sync"
	"time"
)

func find(values []int, target, workers int) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	found := make(chan struct{}, 1)
	var wg sync.WaitGroup

	if workers > len(values) {
		workers = len(values)
	}
	if workers == 0 {
		fmt.Println("Timeout! Not Found")
		return
	}

	for i := 0; i < workers; i++ {
		start := len(values) * i / workers
		end := len(values) * (i + 1) / workers
		part := values[start:end]

		wg.Go(func() {
			for _, value := range part {
				select {
				case <-ctx.Done():
					return
				default:
				}

				if value == target {
					select {
					case found <- struct{}{}:
						cancel()
					default:
					}
					return
				}
			}
		})
	}

	finished := make(chan struct{})
	go func() {
		wg.Wait()
		close(finished)
	}()

	select {
	case <-found:
		fmt.Println("Found it!")
		cancel()
		<-finished
	case <-finished:
		select {
		case <-found:
			fmt.Println("Found it!")
		default:
			if ctx.Err() == context.DeadlineExceeded {
				fmt.Println("Timeout! Not Found")
			} else {
				fmt.Println("Not Found")
			}
		}
	}
}
```

**解析：**

context 取消是协作式的，不会强制杀死 goroutine。worker 必须在循环中检查 `ctx.Done()`，并且可能阻塞的发送也应使用带取消分支的 `select`。

这里用 `found` 只传递一次“找到”事件，用 `finished` 表示所有 worker 已退出。找到结果后调用 `cancel`，其他 worker 在下一次检查时退出。

实际项目中可以按 CPU 数量、数据大小和查询成本选择 worker 数量；不要盲目启动大量 goroutine。

---

## 二、选择题

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
	chain += " --> A"
	B()
}

func B() {
	chain += " --> B"
	C()
}

func C() {
	mu.Lock()
	defer mu.Unlock()
	chain += " --> C"
}
```

选项：

- A：不能编译；
- B：输出 `main --> A --> B --> C`；
- C：输出 `main`；
- D：发生死锁，runtime 可能报告 fatal error。

**答案：D。**

**解析：**

`A` 获取 `mu` 后调用 `C`，而 `C` 再次尝试获取同一个 `sync.Mutex`。Go 的 mutex 不可重入，因此 `C` 永久阻塞，`A` 也无法执行 defer 解锁。

这不是通常意义上可以被 `recover` 捕获的普通 panic。若所有 goroutine 都无法继续运行，runtime 通常报告：

```text
fatal error: all goroutines are asleep - deadlock!
```

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

选项：

- A：不能编译；
- B：输出 `1`；
- C：程序一直阻塞，最终可能报告 fatal deadlock；
- D：立即发生普通 panic。

**答案：C。**

**解析：**

`A` 先持有读锁，然后休眠。主 goroutine 随后等待写锁。`RWMutex` 在写锁等待时会阻塞新的读锁请求，避免写者无限饥饿，因此 `C` 中的第二次 `RLock` 可能无法获得。

形成等待链：

```text
A 持有读锁，等待 C 返回
C 等待新的读锁
main 等待写锁
```

程序会阻塞，通常不是普通 panic。

---

### 题 9：WaitGroup 生命周期

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

选项：

- A：不能编译；
- B：一定无输出并正常退出；
- C：一定永久阻塞；
- D：一定 panic；
- E：WaitGroup 使用错误，结果依赖调度，可能正常退出，也可能触发 misuse panic。

**答案：E。**

**解析：**

`Done` 可能将计数器减到 0，`Wait` 可能因此返回；此时 goroutine 又调用 `Add(1)`，违反了 WaitGroup 的生命周期约束。

结果不能简单断言为某一个固定行为：

- `Wait` 已经返回后，后续 `Add(1)` 可能成功，但会留下未完成计数；
- `Add` 与计数器归零、`Wait` 返回过程并发时，可能触发：

```text
sync: WaitGroup misuse: Add called concurrently with Wait
```

正数 `Add` 应该在对应的 `Wait` 开始前完成。Go 1.25+ 的 `WaitGroup.Go` 能减少常见的 `Add`/`Done` 配对错误，但不能让错误的任务生命周期变正确。

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

选项：

- A：不能编译；
- B：可以编译，正确实现了单例；
- C：可以编译，但存在 data race，不能认为正确实现了 Once；
- D：可以编译，必然因为 CPU 缓存导致 `f` 多次执行。

**答案：C。**

**解析：**

外层读取：

```go
if o.done == 1
```

没有加锁，也没有使用 atomic；另一个 goroutine 可能同时写入 `o.done`，因此存在 data race。

mutex 只保护加锁路径，不能使外层无锁读取安全。不要把问题简单归因于“CPU 缓存不同步”，根本原因是程序没有建立 Go 内存模型要求的同步关系。

锁内的第二次检查通常能避免多个 goroutine 执行 `f`，但整个实现仍然不是正确的 `Once`。此外，代码在 `f` 执行前就设置 `done = 1`，慢速初始化期间其他 goroutine 可能误以为初始化已经完成。

实际开发应直接使用：

```go
var once sync.Once
once.Do(f)
```

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
	mu2 := mu
	mu.count++
	mu.Unlock()

	mu2.Lock()
	mu2.count++
	mu2.Unlock()
	fmt.Println(mu.count, mu2.count)
}
```

选项：

- A：不能编译；
- B：输出 `1, 1`；
- C：输出 `1, 2`；
- D：复制已使用的 Mutex，`mu2.Lock()` 会阻塞，程序可能报告 fatal deadlock。

**答案：D。**

**解析：**

`mu2 := mu` 会复制整个结构体，也复制了 mutex 当时的锁状态。复制发生时 `mu` 已经加锁，所以 `mu2` 看起来也是 locked 状态。

之后调用：

```go
mu2.Lock()
```

会永久等待。包含 mutex 的结构体不应复制，通常应传递指针，并遵守“使用过的锁不能复制”的规则。`go vet` 也可能通过 `copylocks` 检查提示该问题。

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

var pool = sync.Pool{
	New: func() any { return new(bytes.Buffer) },
}

func main() {
	go func() {
		for {
			processRequest(1 << 28) // 256 MiB
		}
	}()

	for i := 0; i < 1000; i++ {
		go func() {
			for {
				processRequest(1 << 10) // 1 KiB
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
	time.Sleep(time.Millisecond)
}
```

选项：

- A：不能编译；
- B：可以编译，内存一定稳定；
- C：可以编译，运行时内存可能较高或出现较大峰值；
- D：大 buffer 一定会在下一次 GC 中被回收。

**答案：C。**

**解析：**

`bytes.Buffer.Grow` 扩容后不会因为下一次只需要 1 KiB 就自动缩容。256 MiB 的 buffer 放回池后，后续小请求可能暂时复用它，从而保留较大的底层数组。

但 `sync.Pool` 不是持久缓存，GC 可能清理其中的对象。因此不能断言内存一定暴涨，也不能断言一定在某个固定时间回收；实际表现受调度、GC 和池内容影响。

如果业务需要内存上限，通常在放回池之前检查容量，过大的 buffer 直接丢弃：

```go
if b.Cap() <= maxBufferSize {
	pool.Put(b)
}
```

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

	c := time.Tick(time.Second)
	for range c {
		fmt.Printf("#goroutines: %d\n", runtime.NumGoroutine())
	}
}
```

选项：

- A：不能编译；
- B：一段时间后总是输出 `#goroutines: 1`；
- C：一段时间后总是输出 `#goroutines: 2`；
- D：必然立即 panic；
- E：存在 data race，第二个 goroutine 可能拿到 nil 或已初始化的 channel，结果不确定。

**答案：E。**

**解析：**

调用：

```go
}(ch)
```

时，参数值已经被求值。第一个 goroutine 可能还没有执行到：

```go
ch = make(chan int, 1)
```

因此第二个 goroutine 可能拿到 nil；也可能拿到已经初始化的 channel。

同时，第一个 goroutine 写 `ch` 与主 goroutine 读取 `ch` 作为参数之间没有同步，构成 data race。

如果拿到 nil，第二个 goroutine 在：

```go
<-ch
```

处永久阻塞；如果拿到真实 channel，则可能正常接收并退出。因此 B、C 都不是必然结论。

此外，`time.Tick` 返回的 ticker 没有显式停止方法，长期运行的代码应使用 `time.NewTicker` 并在结束时 `Stop`。

正确做法是先初始化 channel，再启动使用它的 goroutine：

```go
ch := make(chan int, 1)

go func() {
	ch <- 1
}()

go func(ch <-chan int) {
	<-ch
}(ch)
```

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

选项：

- A：不能编译；
- B：输出 `1`；
- C：输出 `0`；
- D：`close(nil)` 触发 panic，进程退出。

**答案：D。**

**解析：**

`ch` 从未初始化，因此：

```go
close(ch)
```

等价于关闭 nil channel，会触发：

```text
panic: close of nil channel
```

需要区分：

- 向 nil channel 发送：永久阻塞；
- 从 nil channel 接收：永久阻塞；
- 关闭 nil channel：panic。

panic 发生在 goroutine 中时，如果没有在同一个 goroutine 中 recover，会导致整个进程退出。主 goroutine 的 `<-ch` 本身不会先成功，因为 nil channel 没有通信端点。

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

选项：

- A：不能编译；
- B：输出 `1`；
- C：输出 `0`；
- D：panic。

**答案：A。**

**解析：**

Go 1.26 的 `sync.Map` 仍然没有 `Len` 方法，因此：

```go
m.Len()
```

无法编译。

如果需要数量，需要额外维护计数，或使用 `Range` 自己统计：

```go
count := 0
m.Range(func(_, _ any) bool {
	count++
	return true
})
```

但 `Range` 不提供一个同时刻的全局快照语义，统计结果需要结合业务并发要求判断是否足够准确。

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

选项：

- A：不能编译；
- B：输出 `1`；
- C：输出 `0`；
- D：panic。

**答案：B。**

**解析：**

`f` 先执行：

```go
a = 1
```

然后等待从 `c` 接收。主 goroutine 的：

```go
c <- 0
```

必须等到 `f` 完成接收后才能完成。channel 的接收完成与发送完成建立同步关系，因此：

```text
a = 1
    happens-before
f 的接收完成
    happens-before
main 的发送完成
    happens-before
print(a)
```

所以主 goroutine 读取到 `1`。

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
	value, ok := m.m[key]
	return value, ok
}

func (m *Map) Put(key, value int) {
	m.Lock()
	defer m.Unlock()
	m.m[key] = value
}

func (m *Map) Len() int {
	return len(m.m)
}
```

一个 goroutine 循环调用 `Put`，另一个 goroutine 循环调用 `Len`。选项：

- A：不能编译；
- B：可运行且没有并发问题；
- C：`Len` 与 `Put` 并发访问原生 Map，存在 data race，运行时也可能 fatal；
- D：一定输出固定结果。

**答案：C。**

**解析：**

`Put` 加锁，但 `Len` 没有加锁：

```go
func (m *Map) Len() int {
	return len(m.m)
}
```

因此 `Len` 可能与 `Put` 同时访问原生 Map，产生 data race，并可能触发：

```text
fatal error: concurrent map read and map write
```

如果读多写少，应将结构体改为 `sync.RWMutex`：

```go
type Map struct {
	mu sync.RWMutex
	m  map[int]int
}

func (m *Map) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.m)
}
```

runtime 对 Map 的并发写检查不是同步机制，不能代替锁。

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

	ints := make([]int, 0, 1000)

	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			ints = append(ints, i)
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			ints = append(ints, i)
		}
	}()

	wg.Wait()
	fmt.Println(len(ints))
}
```

选项：

- A：不能编译；
- B：一定输出 `2000`；
- C：存在 data race，长度和内容不确定，可能不是 `2000`；
- D：一定 panic。

**答案：C。**

**解析：**

slice 由 `array`、`len`、`cap` 三个字段组成。并发 `append` 可能同时修改 slice header，也可能同时写入底层数组，因此存在 data race。

它不一定像 Map 那样由 runtime 主动检测并 fatal，但“没有 panic”不等于安全。结果可能偶然是 2000，也可能小于 2000，内容也可能丢失或重复。

常见修复方式：

1. 用 mutex 保护整个 append；
2. 每个 goroutine 写入独立 slice，最后合并；
3. 预先分配数组，让每个 goroutine 写入不重叠的索引；
4. 通过 channel 将追加操作交给一个 owner goroutine。

---

### 题 19：Goroutine 闭包与 range 变量

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
	defer wg.Done()
	t.V++
}

func (t *T) Print() {
	time.Sleep(time.Second)
	fmt.Print(t.V)
}

func main() {
	var wg sync.WaitGroup
	wg.Add(10)

	ts := make([]T, 10)
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

选项：

- A：按顺序输出 `12345678910`；
- B：按顺序输出 `0123456789`；
- C：输出十个 `9`；
- D：输出十个 `10`；
- E：在 Go 1.26 下输出 `0` 到 `9` 各一次，但顺序不确定。

**答案：E。**

**解析：**

Go 1.22 起，每次 `range` 迭代都有独立的循环变量。因此在 Go 1.26 下：

```go
for _, t := range ts {
	go t.Print()
}
```

每个 goroutine 捕获自己的 `t` 副本，打印内容是 `0` 到 `9`，但 goroutine 调度不保证顺序。

`Incr` 使用指针接收者，但 `range` 变量 `t` 本身是 `T` 的值副本，调用方法时修改的是该迭代副本，不会修改 `ts` 中的元素。第二轮重新从未被修改的 `ts` 读取，因此仍然是 `0` 到 `9`。

如果模块使用 Go 1.21 或更低版本，需要额外考虑旧的 range 变量共享语义；不能把旧版本的闭包陷阱直接套用到 Go 1.26。

这道题的输出没有分隔符，实际打印可能连成任意排列的数字字符串；“各一次、顺序不确定”是语义上的答案。

---

## 三、简答分析题

### 题 20：对已关闭的 channel 进行读写，会怎么样？为什么？

请回答：

1. 写已关闭的 channel 会发生什么？
2. 读已关闭的 channel 会发生什么？
3. 从 runtime 源码层面解释原因。

**答案：**

#### 1. 向已关闭 channel 发送

```go
ch := make(chan int)
close(ch)
ch <- 1 // panic: send on closed channel
```

channel 关闭后，runtime 将底层 `hchan.closed` 标记为已关闭。发送路径在加锁后检查该状态，如果非零就直接 panic：

```go
if c.closed != 0 {
	unlock(&c.lock)
	panic("send on closed channel")
}
```

即使 buffered channel 还有剩余空间，也不能继续发送。

#### 2. 从已关闭 channel 接收

需要区分两种情况。

如果 channel 关闭时缓冲区仍有数据：

```go
ch := make(chan int, 2)
ch <- 10
close(ch)

v, ok := <-ch // v == 10, ok == true
```

关闭不会清空缓冲区，接收方会先把已有数据读完。

如果 channel 已关闭且缓冲区为空：

```go
v, ok := <-ch // v == 0, ok == false
```

runtime 会把接收目标清零，并返回 `received == false`。对于不同类型，返回其零值：

```text
int      -> 0
string   -> ""
bool     -> false
pointer  -> nil
```

因此判断 channel 是否关闭，应检查第二个返回值：

```go
v, ok := <-ch
if !ok {
	// 已关闭且没有更多数据
}
```

只写：

```go
v := <-ch
```

只能拿到零值，无法判断这个零值是发送方真实发送的，还是关闭后的默认值。

#### 3. runtime 层面的解释

以 Go 1.26.3 的 `runtime/chan.go` 为例，关闭 channel 的关键动作是：

```go
lock(&c.lock)
if c.closed != 0 {
	unlock(&c.lock)
	panic("close of closed channel")
}
c.closed = 1
```

发送路径先检查：

```go
lock(&c.lock)
if c.closed != 0 {
	unlock(&c.lock)
	panic("send on closed channel")
}
```

接收路径先检查关闭状态和缓冲区计数：

```go
if c.closed != 0 && c.qcount == 0 {
	unlock(&c.lock)
	clear(receiveTarget)
	return true, false
}
```

如果 `c.closed != 0` 但 `c.qcount > 0`，接收路径仍会从环形缓冲区取出数据，并返回 `received == true`。

runtime 源码参考：

```text
/Users/tal/sdk/go1.26.3/src/runtime/chan.go
```

需要注意，`hchan` 字段和 runtime 函数属于实现细节，未来版本可能调整；稳定的语言层语义是：

```text
发送已关闭 channel：panic
关闭后仍有缓冲数据：继续接收，ok=true
关闭且数据耗尽：零值，ok=false
```

nil channel 是另一种状态：向 nil channel 发送或接收会永久阻塞，而 `close(nil)` 会 panic。

---

## 实验

对应代码：[experiments/03_goroutine_interview.go](experiments/03_goroutine_interview.go)

```bash
cd notes/golang
go run ./experiments/ interview
```

实验内容（编程手写题 1-6 全部可运行）：
1. 题1：Goroutine + Channel 基础 — 生产者/消费者，发送方负责 `close`
2. 题2：阻塞读并发安全 Map — `Out` 唤醒所有等待者，`Rd` 支持超时返回 nil
3. 题3：高并发 IP 限流 — 100 IP × 1000 并发，三分钟窗口，期望输出 `success: 100`
4. 题4：定时调用 + panic 恢复 — `recover` 必须在同一 goroutine 内
5. 题5：`WaitGroup` 支持 `WaitTimeout` — 超时返回 true，调用方负责取消
6. 题6：多协程查询切片 + context 取消 — 找到即取消其他 worker（含超时路径演示）
7. 题19（选择题演示）：闭包与 range 变量 — `Incr` 作用于值副本，`Print` 打印 0~9 各一次顺序不定

选择题（题 7-19）多为故意写错的代码，可作为"运行观察"练习：死锁类题（7/8/11）不要直接跑，可先阅读解析再对照；`-race` 可复现题 10/17/18 的 data race。
