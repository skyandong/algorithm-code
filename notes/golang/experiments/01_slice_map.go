// # Slice 与 Map 底层实验
//
// 对应笔记：notes/golang/01-slice与map底层.md
//
// 运行：
//
//	go run ./experiments/ slicemap
//
// 实验项：
//
//	Exp1：slice 头结构（ptr/len/cap）与子切片共享底层数组
//	Exp2：append 未扩容互相覆盖 / 扩容后分离 / 函数内 append 不影响调用方
//	Exp3：扩容 cap 序列（Go 1.18+ 规则 + sizeclass 对齐）
//	Exp4：从 slice 删元素的三种写法与内存泄漏点（含指针元素清槽位 + a[len(a)] 越界反面教材）
//	Exp4.4：大数组切小 slice 的内存泄漏（copy 修复）
//	Exp5：map 底层 —— 预分配避免 rehash、delete 不缩容（换新 map 才释放）
//	Exp6：map 无序 — 随机遍历起点
//	Exp7：for range 中删除安全 / 新增不保证可见
//	Exp8：map 并发读写 fatal（受开关控制，默认关闭）
//	Exp9：sync.Map —— 读多写少 vs Mutex+map、持续写新 key 的性能对照
//	Exp10：空切片 vs nil 切片（len==0 等价、reflect 可区分）
//
// 注意：Exp8 打开开关后程序会以 fatal error 崩溃（throw，无法 recover），
// 这是预期行为，不是 bug。
package main

import (
	"fmt"
	"reflect"
	"runtime"
	"slices"
	"sync"
	"time"
)

// RunSliceMapExperiments 演示笔记 01 的核心语义。
func RunSliceMapExperiments() {
	fmt.Println("========== 第1节: slice 头结构（ptr/len/cap）与共享底层数组 ==========")
	demoSliceHeader()

	fmt.Println("\n========== 第2节: append 的共享/分离与函数内 append ==========")
	demoSharedArray()
	demoFuncAppend()

	fmt.Println("\n========== 第3节: 扩容 cap 序列（Go 1.18+）==========")
	demoGrowthCap()

	fmt.Println("\n========== 第4节: 删除元素的三种写法与泄漏点 ==========")
	demoDeleteWays()

	fmt.Println("\n========== 第4.4节: 大数组切小 slice 的内存泄漏 ==========")
	demoSubsliceLeak()

	fmt.Println("\n========== 第5节: map 底层 —— 预分配避免 rehash 与删除不缩容 ==========")
	demoMapInternals()

	fmt.Println("\n========== 第6节: map 无序 — 随机遍历起点 ==========")
	demoMapUnordered()

	fmt.Println("\n========== 第7节: for range 中的删除与新增 ==========")
	demoRangeMutation()

	fmt.Println("\n========== 第8节: map 并发读写 fatal（默认关闭）==========")
	demoConcurrentMapFatal()

	fmt.Println("\n========== 第9节: sync.Map —— 读多写少大胜；写新 key 的结论随版本变 ==========")
	demoSyncMapLayers()

	fmt.Println("\n========== 第10节: 空切片 vs nil 切片 ==========")
	demoNilEmptySlice()
}

// demoSliceHeader 笔记 01 第 1 节：slice 是 (ptr, len, cap) 三字段头，赋值/传参拷贝的是头。
func demoSliceHeader() {
	s := []int{10, 20, 30} // 字面量：len=cap=3
	fmt.Printf("s     len=%d cap=%d &s[0]=%p\n", len(s), cap(s), &s[0])

	// 子切片：新 header，指向同一底层数组的中间
	sub := s[1:]
	fmt.Printf("s[1:] len=%d cap=%d &sub[0]=%p\n", len(sub), cap(sub), &sub[0])
	fmt.Printf("&s[1]=%p，&sub[0]==&s[1]: %v（同一块内存）\n", &s[1], &sub[0] == &s[1])

	// 修改 sub 会改到 s：两个 header 共享一个数组
	sub[0] = 99
	fmt.Printf("sub[0]=99 之后 s=%v（s[1] 被一起改了）\n", s)
}

// demoSharedArray 笔记 01 第 2 节：append 未超 cap 时写共享数组；超 cap 才分配新数组。
func demoSharedArray() {
	a := make([]int, 3, 6) // len=3 cap=6，还有空位

	b := append(a, 99) // 未超 cap：b 仍指向 a 的底层数组
	fmt.Printf("&a[0]=%p &b[0]=%p 共享=%v\n", &a[0], &b[0], &a[0] == &b[0])

	c := append(a, 100) // 同样写第 4 个槽位：把 b 刚写入的 99 顶掉
	fmt.Printf("b[3]=%d c[3]=%d（c 把 b 的 99 覆盖了）\n", b[3], c[3])

	d := append(c, 1, 2, 3, 4) // 需要 len=8 > cap=6：扩容，分配新数组
	fmt.Printf("扩容后 &c[0]=%p &d[0]=%p 共享=%v（cap %d -> %d）\n",
		&c[0], &d[0], &c[0] == &d[0], cap(c), cap(d))
}

// demoFuncAppend 笔记 01 第 2.3 节：函数内 append 只改副本 header 的 len。
func demoFuncAppend() {
	s := make([]int, 0, 4)
	appendInside(s)
	fmt.Printf("函数内 append 后 len=%d（调用方看不到）\n", len(s))
	fmt.Printf("偷看底层数组 s[:1]=%v（数据其实写进去了，只是 len 没变）\n", s[:1])

	s = appendAndReturn(s)
	fmt.Printf("返回新 header 后 len=%d s=%v\n", len(s), s)
}

// appendInside 修改的是参数副本（header 拷贝）的 len，调用方无感。
func appendInside(s []int) {
	s = append(s, 7)
}

// appendAndReturn 正确姿势：把新 header 返回（或传 *[]int）。
func appendAndReturn(s []int) []int {
	return append(s, 7)
}

// growthSink 包级变量：让 append 结果逃逸，走 runtime.growslice 真实路径
// （非逃逸的小 append 在 Go 1.26 会被编译器直接分配 32 字节内的栈上背衬数组，观察不到首轮规律）。
var growthSink []int

// demoGrowthCap 笔记 01 第 3 节：cap<256 翻倍；之后增长系数平滑过渡到 1.25x，再被 sizeclass 对齐。
func demoGrowthCap() {
	growthSink = make([]int, 0)
	prev := cap(growthSink)
	fmt.Printf("初始 cap=%d\n", prev)
	for i := 1; i <= 10000; i++ {
		growthSink = append(growthSink, i)
		if cap(growthSink) != prev {
			fmt.Printf("len=%-6d cap: %-5d -> %-5d（x%.3f）\n", len(growthSink), prev, cap(growthSink), float64(cap(growthSink))/float64(prev))
			prev = cap(growthSink)
		}
	}
	fmt.Println("结论：倍数只是观察值，随元素大小/版本变化 —— 工程上不要依赖具体扩容倍数")
}

// demoDeleteWays 笔记 01 第 4 节：三种删除写法各有泄漏点与复杂度取舍。
func demoDeleteWays() {
	// 写法一：截断 —— O(1)，但尾部元素仍留在底层数组里
	a := []int{1, 2, 3, 4}
	a = a[:len(a)-1]
	fmt.Printf("截断后 a=%v，底层数组实际还是 %v（4 仍被引用，指针元素会泄漏）\n", a, a[:cap(a)])

	// 写法二：copy 覆盖（保序，O(n)）
	b := []int{1, 2, 3, 4, 5}
	i := 1
	b = append(b[:i], b[i+1:]...)
	fmt.Printf("copy 覆盖后 b=%v（保序；底层数组尾部残留旧值 %v）\n", b, b[:cap(b)][len(b):])

	// 写法三：swap-delete（O(1)，不保序）
	c := []int{1, 2, 3, 4, 5}
	j := 1
	c[j] = c[len(c)-1]
	c = c[:len(c)-1]
	fmt.Printf("swap-delete 后 c=%v（不保序；被删元素立即被覆盖，无尾部残留）\n", c)

	// 泄漏修复：指针元素截断必须手动清槽位（int 无所谓，指针会让被删对象 GC 不到）
	p := make([]*int, 0, 4)
	v := 42
	for i := 0; i < 4; i++ {
		p = append(p, &v)
	}
	p[len(p)-1] = nil // 姿势一：截断「前」清槽位（SliceTricks 标准写法）
	p = p[:len(p)-1]
	fmt.Printf("截断+清槽位后，偷看尾部槽位=%v（需 reslice p[:cap(p)] 才能看到）\n", p[:cap(p)][len(p)])

	// 反面教材：截断「后」直接写 a[len(a)] = nil —— 索引上限是 len-1，直接越界 panic
	func() {
		defer func() { fmt.Printf("截断后直接 a[len(a)]=nil → panic: %v\n", recover()) }()
		q := make([]*int, 2, 2)
		q = q[:1]
		q[len(q)] = nil // index out of range [1] with length 1
	}()
}

// demoSubsliceLeak 笔记 01 第 4.4 节：小 slice 挂住整个大数组，copy 出独立小片才释放。
func demoSubsliceLeak() {
	big := make([]byte, 1<<20) // 1 MiB
	tail := big[len(big)-2:]   // 只要末尾 2 字节
	fmt.Printf("big 起点=%p，tail 起点=%p（tail 指向 big 内部）\n", &big[0], &tail[0])
	fmt.Println("只要 tail 活着，整块 1 MiB 都无法被 GC 回收（共享底层数组）")

	fixed := make([]byte, len(tail))
	copy(fixed, tail) // 修复：copy 出独立分配的小数组，big 随后可被回收
	fmt.Printf("copy 修复后 fixed 起点=%p（与 big 无关）\n", &fixed[0])

	runtime.KeepAlive(tail) // 保证上面的观察期间 tail（连带 big）确实存活
}

// demoMapInternals 笔记 01 第 5 节：预分配避免反复 rehash；delete 不缩容，释放要换新 map。
func demoMapInternals() {
	const n = 300000

	// 1) 预分配 vs 动态增长：负载因子超限就 rehash 搬迁（老实现 6.5 触发翻倍，1.24+ Swiss 表 7/8）
	start := time.Now()
	m1 := make(map[int]int)
	for i := 0; i < n; i++ {
		m1[i] = i // 无 size hint：边插边扩容，多轮 rehash 搬迁
	}
	noHint := time.Since(start)

	start = time.Now()
	m := make(map[int]int, n) // 已知规模：make 的 size hint 一次到位，基本不触发 rehash
	for i := 0; i < n; i++ {
		m[i] = i
	}
	withHint := time.Since(start)
	fmt.Printf("构建 %d 元素 map: 无提示=%v（多次 rehash 搬迁） vs 预分配=%v\n", n, noHint, withHint)

	// 2) 删除与内存：部分删除不缩容（桶数组保留）；删空时 1.24+ Swiss 实现重置释放，老 hmap 不释放
	var ms1, ms2, ms3, ms4 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&ms1)
	heapWithMap := ms1.HeapAlloc / (1 << 20)

	for i := 0; i < 2*n/3; i++ { // 删 2/3，留 1/3
		delete(m, i)
	}
	runtime.GC()
	runtime.ReadMemStats(&ms2)
	heapPartial := ms2.HeapAlloc / (1 << 20)

	for k := range m { // 删光
		delete(m, k)
	}
	runtime.GC()
	runtime.ReadMemStats(&ms3)
	heapEmpty := ms3.HeapAlloc / (1 << 20)

	m = nil // 换新 map / 置 nil：整体释放的最稳姿势（任意版本都成立）
	runtime.GC()
	runtime.ReadMemStats(&ms4)
	heapAfterDrop := ms4.HeapAlloc / (1 << 20)

	fmt.Printf("装满=%d MB → 部分删除=%d MB（不缩容！）→ 删空=%d MB（1.24+ Swiss 重置释放）→ m=nil=%d MB\n",
		heapWithMap, heapPartial, heapEmpty, heapAfterDrop)
	fmt.Println("工程推论: 高峰过后的缓存别指望逐个 delete 还内存 —— 要么删空重置（新版本）、要么整体重建（m = make(...)）")
}

// demoSyncMapLayers 笔记 01 第 8 节：读多写少 sync.Map 赢，持续写新 key 反而输。
func demoSyncMapLayers() {
	const workers = 4
	const per = 100000
	wait := func(n int, fn func(id int)) {
		var wg sync.WaitGroup
		wg.Add(n)
		for i := 0; i < n; i++ {
			go func(id int) {
				defer wg.Done()
				fn(id)
			}(i)
		}
		wg.Wait()
	}

	// 场景一：读多写少（key 集合稳定）—— sync.Map 读路径无锁，赢
	var sm sync.Map
	for i := 0; i < 16; i++ {
		sm.Store(i, i)
	}
	start := time.Now()
	wait(workers, func(id int) {
		for i := 0; i < per; i++ {
			sm.Load(i % 16)
		}
	})
	smRead := time.Since(start)

	m := make(map[int]int, 16)
	for i := 0; i < 16; i++ {
		m[i] = i
	}
	var mu sync.RWMutex
	start = time.Now()
	wait(workers, func(id int) {
		for i := 0; i < per; i++ {
			mu.RLock()
			_ = m[i%16]
			mu.RUnlock()
		}
	})
	rwRead := time.Since(start)

	// 场景二：持续写新 key —— sync.Map 写路径开销更大（1.24 前: 写 dirty + miss 提升；HashTrieMap 后差距缩小）
	start = time.Now()
	wait(workers, func(id int) {
		for i := 0; i < per; i++ {
			sm.Store(id*per+i, i) // 全是新 key
		}
	})
	smWrite := time.Since(start)

	start = time.Now()
	wait(workers, func(id int) {
		for i := 0; i < per; i++ {
			mu.Lock()
			m[id*per+i] = i
			mu.Unlock()
		}
	})
	rwWrite := time.Since(start)

	fmt.Printf("读多写少 ×%d万次: sync.Map=%v  RWMutex+map=%v\n", workers*per/10000, smRead, rwRead)
	fmt.Printf("持续写新 key 同规模: sync.Map=%v  RWMutex+map=%v\n", smWrite, rwWrite)
	fmt.Println("选型: 读多写少/集合稳定 → sync.Map 没有争议（无锁读大胜全局锁）")
	fmt.Println("版本分界: 经典 read/dirty 模型（≤1.23）持续写新 key 是短板（miss 提升 + 整体搬运）；")
	fmt.Println("实测 Go 1.26 换 HashTrieMap 后写新 key 也优于 Mutex+map —— 面试答选型口诀时务必注版本")
}

// demoMapUnordered 笔记 01 第 6 节：每次 range 起点随机，两次顺序通常不同。
func demoMapUnordered() {
	m := map[int]int{}
	for i := 0; i < 10; i++ {
		m[i] = i
	}
	collect := func() []int {
		out := make([]int, 0, len(m))
		for k := range m {
			out = append(out, k)
		}
		return out
	}
	first := collect()
	second := collect()
	fmt.Println("第一次:", first)
	fmt.Println("第二次:", second)
	fmt.Printf("两次顺序相同=%v（起点随机；相同纯属巧合，小 map 概率不为零）\n", slices.Equal(first, second))
}

// demoRangeMutation 笔记 01 第 6 节：range 中删除安全（未遍历的不再产出），新增不保证可见。
func demoRangeMutation() {
	// 实验 A：遍历到第一个 key 后删光其余 —— 规范保证被删的 key 不会再出现
	m := map[int]int{}
	for i := 0; i < 10; i++ {
		m[i] = i
	}
	var visited []int
	for k := range m {
		visited = append(visited, k)
		if len(visited) == 1 {
			for j := 0; j < 10; j++ {
				if j != k {
					delete(m, j) // 删除所有「尚未遍历」的 key
				}
			}
		}
	}
	fmt.Printf("删光其余 key 后实际遍历到的元素个数=%d（应为 1；删除安全且不再产出）\n", len(visited))

	// 实验 B：遍历中新增 —— 可能被产出也可能被跳过，规范不保证
	m2 := map[int]int{}
	for i := 0; i < 10; i++ {
		m2[i] = i
	}
	seen100 := false
	for k := range m2 {
		if k == 5 {
			m2[100] = 100 // 新增一个 key
		}
		if k == 100 {
			seen100 = true
		}
	}
	fmt.Printf("新增的 100 是否被遍历到=%v（不保证，多次运行结果会变）\n", seen100)
}

// fatalDemoEnabled 打开后演示并发写 map 的崩溃：fatal error: concurrent map writes。
// 该错误由 runtime throw，不是 panic，defer/recover 都拦不住，进程直接退出 —— 崩溃是预期行为。
const fatalDemoEnabled = false

// demoConcurrentMapFatal 笔记 01 第 7 节：并发读写 map → fatal error（不可 recover）。
func demoConcurrentMapFatal() {
	if !fatalDemoEnabled {
		fmt.Println("开关 fatalDemoEnabled=false（默认），只打印说明：")
		fmt.Println("  并发读写 map 会触发 runtime 检测（1.6+），输出形如：")
		fmt.Println("  fatal error: concurrent map writes / concurrent map read and map write")
		fmt.Println("  这是 throw 不是 panic，recover 无效，整个进程直接崩溃")
		fmt.Println("  想亲眼观察：把 fatalDemoEnabled 改为 true 再运行（预期崩溃）")
		return
	}

	m := map[int]int{}
	var wg sync.WaitGroup
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			defer func() { _ = recover() }() // 演示 recover 对 fatal error 无效
			for i := 0; i < 1000; i++ {
				m[i] = w
			}
		}(w)
	}
	wg.Wait()
	fmt.Println("未触发检测（正常结束）")
}

// demoNilEmptySlice 笔记 01 第 9 节：nil 切片与空切片的判空等价、==nil 与 reflect 的差异。
func demoNilEmptySlice() {
	var nilSlice []int    // nil 切片：header {nil, 0, 0}
	emptySlice := []int{} // 空切片：header {有效指针, 0, 0}

	fmt.Printf("len(nilSlice)=%d len(emptySlice)=%d —— 判空用 len()==0，二者等价\n",
		len(nilSlice), len(emptySlice))
	fmt.Printf("nilSlice == nil: %-5v emptySlice == nil: %v\n", nilSlice == nil, emptySlice == nil)
	fmt.Printf("reflect.ValueOf(nilSlice).IsNil()=%v, reflect.ValueOf(emptySlice).IsNil()=%v\n",
		reflect.ValueOf(nilSlice).IsNil(), reflect.ValueOf(emptySlice).IsNil())
	fmt.Println("JSON 序列化差异（null vs []）见 08 实验第 6 节")
}
