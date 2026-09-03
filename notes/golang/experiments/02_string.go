// # string 底层实验
//
// 对应笔记：notes/golang/02-string底层.md
//
// 运行（接入 main.go 后）：
//
//	go run ./experiments/ string
//
// 实验项：
//
//	第1节：二字段头 + 子串零拷贝（unsafe.StringData 指针相同）
//	第2节：不可变契约与零拷贝的红线（unsafe.String 修改源会连坐）
//	第3节：string ↔ []byte 拷贝成本（MemStats 观察）+ map 索引免分配特例
//	第4节：拼接三种写法对比（+ 循环 O(n²) vs Builder+Grow）
//	第5节：len 字节 vs rune 字符、for range 字节偏移、截断碎码
//	第6节：map key：string 可比 / []byte 不可比 / [N]byte 按值
package main

import (
	"fmt"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"
	"unsafe"
)

// RunStringExperiments 演示笔记 2 的 string 底层行为。
func RunStringExperiments() {
	fmt.Println("========== 第1节: 二字段头与子串零拷贝 ==========")
	strHeader()

	fmt.Println("\n========== 第2节: 不可变契约与零拷贝红线 ==========")
	strImmutable()

	fmt.Println("\n========== 第3节: 互转拷贝成本与 map 索引特例 ==========")
	strConvert()

	fmt.Println("\n========== 第4节: 拼接性能对比 ==========")
	strConcat()

	fmt.Println("\n========== 第5节: len 的歧义与 UTF-8 ==========")
	strRune()

	fmt.Println("\n========== 第6节: string 与 map key ==========")
	strMapKey()
}

// strHeader 第1节：string 是 (data, len) 头；子串共享底层内存。
func strHeader() {
	s := "hello, world"
	sub := s[:5] // 只造新头，不拷贝数据

	p1 := unsafe.StringData(s)
	p2 := unsafe.StringData(sub)
	fmt.Printf("s=%q sub=%q，底层数据指针相同: %v（子串零拷贝）\n", s, sub, p1 == p2)
	fmt.Printf("len(s)=%d len(sub)=%d（无 \\0，长度显式存储）\n", len(s), len(sub))

	// 持有 sub 会钉住整块大字符串
	fmt.Println("推论: sub 活着 → 整个 s 的底层内存无法 GC；长生命周期用 strings.Clone 拷贝出小串")
	cloned := strings.Clone(sub)
	fmt.Printf("strings.Clone(sub)=%q 独立内存（指针相同: %v）\n", cloned, unsafe.StringData(cloned) == p1)
}

// strImmutable 第2节：不可变是契约；unsafe.String 的零拷贝产物跟着源变。
func strImmutable() {
	// 编译器拦截：s[0] = 'H' // ✗ cannot assign to s[0]（注释演示）
	fmt.Println("s[0] = 'H' → 编译错误（不可变由编译器静态保证）")

	// 零拷贝视图：契约交到你手里
	b := []byte("hello")
	s := unsafe.String(unsafe.SliceData(b), len(b)) // Go 1.20+ 标准写法
	b[0] = 'H'                                       // 改的是源 []byte
	fmt.Printf("unsafe.String 零拷贝后改源 b: s=%q（s 跟着变了！红线：绝不修改零拷贝产物的底层）\n", s)

	// 驻留：编译期相同字面量共享只读段
	fmt.Println("字面量驻留: 相同常量串共享一份只读内存，修改零拷贝视图可能污染常量区 → 崩溃")
}

// strConvert 第3节：互转默认拷贝；不逃逸时免拷贝；map[string(b)] 索引免分配。
var strSink [][]byte // 全局 sink，强迫 []byte(s) 逃逸

func strConvert() {
	var before, after runtime.MemStats
	s := strings.Repeat("x", 64)
	const n = 100000

	// 场景一：b 不逃逸、不被修改 → 编译器直接引用 s 底层，零分配
	runtime.GC()
	runtime.ReadMemStats(&before)
	total := 0
	for i := 0; i < n; i++ {
		b := []byte(s)
		total += len(b) // 局部使用，不逃逸
	}
	runtime.ReadMemStats(&after)
	noEscape := after.TotalAlloc - before.TotalAlloc

	// 场景二：b 逃逸（存进全局）→ 每次真拷贝
	strSink = make([][]byte, 0, n) // 计量前预分配，排除 slice 头数组本身的噪音
	runtime.GC()
	runtime.ReadMemStats(&before)
	for i := 0; i < n; i++ {
		strSink = append(strSink, []byte(s)) // 逃逸 → 数据拷贝
	}
	runtime.ReadMemStats(&after)
	escaped := after.TotalAlloc - before.TotalAlloc
	strSink = nil

	fmt.Printf("[]byte(s) ×10万(64B): 不逃逸分配 %d B（编译器免拷贝） vs 逃逸分配 %d KB（真拷贝，理论值 10万×64B≈6250KB）\n",
		noEscape, escaped/1024)
	fmt.Println("（小整数 0~255 有静态缓存同理——装箱是否分配要看逃逸与取值范围）")

	// map 索引特例：m[string(b)] 编译器免分配
	m := map[string]int{"ping": 1, "pong": 2}
	runtime.GC()
	runtime.ReadMemStats(&before)
	hits := 0
	for i := 0; i < n; i++ {
		key := []byte("ping")
		if _, ok := m[string(key)]; ok { // 此处不分配
			hits++
		}
	}
	runtime.ReadMemStats(&after)
	fmt.Printf("m[string(b)] ×10万: 分配 %d B（编译器特例：就地比较，map 索引免分配）\n",
		after.TotalAlloc-before.TotalAlloc)

	fmt.Println("工程解法: 流水线统一类型只在边界转一次；bytes/strings 各有 []byte 版 API")
	_ = total
	_ = hits
}

// strConcat 第4节：+ 循环 vs Builder vs Grow 预分配。
func strConcat() {
	parts := make([]string, 2000)
	for i := range parts {
		parts[i] = "abcdefgh"
	}

	// ✗ O(n²)：每次 + 全量拷贝旧内容
	start := time.Now()
	plus := ""
	for _, p := range parts {
		plus += p
	}
	plusDur := time.Since(start)

	// ✓ Builder：不 Grow（依赖内部 slice 扩容）
	start = time.Now()
	var b strings.Builder
	for _, p := range parts {
		b.WriteString(p)
	}
	_ = b.String()
	builderDur := time.Since(start)

	// ✓✓ Builder + Grow：一次到位
	start = time.Now()
	var bg strings.Builder
	bg.Grow(len(parts) * len(parts[0])) // 预估总长
	for _, p := range parts {
		bg.WriteString(p)
	}
	_ = bg.String()
	growDur := time.Since(start)

	fmt.Printf("2000 段×8B 拼接: +循环=%v Builder=%v Builder+Grow=%v（+ 循环 O(n²)，循环拼接禁用）\n",
		plusDur.Round(time.Microsecond), builderDur.Round(time.Microsecond), growDur.Round(time.Microsecond))
	fmt.Println("String() 内部是 unsafe.String 零拷贝导出——标准库 unsafe 的正面示范")
}

// strRune 第5节：len 是字节数；range 按码点、索引是字节偏移；按字节截断碎码。
func strRune() {
	s := "你好Go"
	fmt.Printf("s=%q: len=%d（字节数） RuneCountInString=%d（字符数）\n",
		s, len(s), utf8.RuneCountInString(s))

	fmt.Print("for range 迭代（索引=字节偏移）: ")
	for i, r := range s {
		fmt.Printf("[%d=%c]", i, r) // 0你 3好 6G 7o —— 索引跳变
	}
	fmt.Println()

	fmt.Printf("s[0]=0x%X（首字节，不是 '你'）；[]rune(s)=%v（解码+分配）\n",
		s[0], []rune(s))

	cut := s[:4] // 按字节硬切，切碎「好」
	fmt.Printf("s[:4]=%q（合法字节但非法 UTF-8，碎码）\n", cut)
	fmt.Println("按字符截断: []rune(s)[:2] 或 utf8.RuneStart 辅助定位（热路径先转一次存起来）")
}

// strMapKey 第6节：key 可比性对照。
func strMapKey() {
	m := map[string]int{"a": 1} // string: 不可变 + memcmp，天生 key
	m[string([]byte("b"))] = 2  // []byte 转 string 当 key
	fmt.Println("map[string]T: ✓（== 是 memcmp，长度不同直接不等）")

	// m2 := map[[]byte]int{} // ✗ 编译错误: invalid map key type []byte
	fmt.Println("map[[]byte]T: ✗ 编译错误（slice 头不可比较）")

	arr := map[[8]byte]int{} // 定长数组按值可比
	arr[[8]byte{1, 2}] = 3
	fmt.Println("map[[8]byte]T: ✓（定长数组按值比较，网络库零分配 key 的惯用法）")

	fmt.Printf("结果: string key=%v [8]byte key=%v\n", m["b"], arr[[8]byte{1, 2}])
}
