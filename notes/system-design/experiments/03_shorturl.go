package main

import (
	"fmt"
	"hash/fnv"
)

// 实验 03：短链服务——base62 编解码 + 简易布隆过滤器
// 锚点: base62 编解码往返一致; 布隆插入 10 万 key 后,
//       实测误判率落在理论值 ±20% 区间内。

const base62Alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// base62Encode: 数字 → 短码（7 位定长, 高位补 0）
func base62Encode(n uint64, width int) string {
	if n == 0 {
		b := make([]byte, width)
		for i := range b {
			b[i] = '0'
		}
		return string(b)
	}
	buf := make([]byte, 0, 11)
	for n > 0 {
		buf = append(buf, base62Alphabet[n%62])
		n /= 62
	}
	// 反转
	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}
	// 补齐到定宽
	if len(buf) < width {
		pad := make([]byte, width-len(buf))
		for i := range pad {
			pad[i] = '0'
		}
		buf = append(pad, buf...)
	}
	return string(buf)
}

// base62Decode: 短码 → 数字
func base62Decode(s string) (uint64, error) {
	var n uint64
	for i := 0; i < len(s); i++ {
		c := s[i]
		var v int
		switch {
		case c >= '0' && c <= '9':
			v = int(c - '0')
		case c >= 'A' && c <= 'Z':
			v = int(c-'A') + 10
		case c >= 'a' && c <= 'z':
			v = int(c-'a') + 36
		default:
			return 0, fmt.Errorf("非法字符 %q", c)
		}
		n = n*62 + uint64(v)
	}
	return n, nil
}

// bloomFilter: 简易布隆过滤器（k 个哈希 + 位数组）
type bloomFilter struct {
	bits []uint64 // 位存储, 64bit/word
	m    uint64   // 总位数
	k    int      // 哈希个数
}

func newBloomFilter(m uint64, k int) *bloomFilter {
	words := m / 64
	if m%64 != 0 {
		words++
	}
	return &bloomFilter{bits: make([]uint64, words), m: m, k: k}
}

// hashes: 对 key 生成 k 个位置（双重哈希: h1 + i*h2, 避免计算 k 次）
func (b *bloomFilter) positions(key string) []uint64 {
	h := fnv.New64a()
	h.Write([]byte(key))
	h1 := h.Sum64()

	h2 := h1
	h2 ^= h2 >> 33
	h2 *= 0xff51afd7ed558ccd
	h2 ^= h2 >> 33

	pos := make([]uint64, b.k)
	for i := 0; i < b.k; i++ {
		pos[i] = (h1 + uint64(i)*h2) % b.m
	}
	return pos
}

func (b *bloomFilter) add(key string) {
	for _, p := range b.positions(key) {
		b.bits[p/64] |= 1 << (p % 64)
	}
}

func (b *bloomFilter) mightContain(key string) bool {
	for _, p := range b.positions(key) {
		if b.bits[p/64]&(1<<(p%64)) == 0 {
			return false // 有任一位为 0 → 一定不存在
		}
	}
	return true // 全 1 → 可能存在（可能误判）
}

func (b *bloomFilter) setBits() uint64 {
	var c uint64
	for _, w := range b.bits {
		for ; w != 0; w &= w - 1 {
			c++
		}
	}
	return c
}

func RunShortURLExperiments() {
	fmt.Println("== 实验 03: base62 编解码 + 布隆过滤器 ==")

	// ---- Part 1: base62 往返 ----
	fmt.Println("--- Part 1: base62 编解码往返 ---")
	samples := []uint64{0, 1, 61, 62, 3843, 1000000, 3521614606207} // 最后一个 = 62^7-1 的量级
	allOK := true
	for _, n := range samples {
		code := base62Encode(n, 7)
		back, err := base62Decode(code)
		ok := err == nil && back == n
		allOK = allOK && ok
		fmt.Printf("  %-14d → %-8s → %-14d %s\n", n, code, back, mark(ok))
	}
	fmt.Printf("  62^7 = %d ≈ 3.5 万亿, 7 位短码容量充足 → %s\n", pow62(7), mark(allOK))

	// ---- Part 2: 布隆过滤器误判率实测 vs 理论 ----
	fmt.Println("\n--- Part 2: 布隆过滤器 (插入 10 万 key) ---")
	const n = 100000
	// 按 1% 误判率配置: m = -n·lnp/(ln2)² ≈ 958506 bit, k=7
	const p = 0.01
	const m = 958506
	const k = 7

	bf := newBloomFilter(m, k)
	inserted := make(map[string]bool, n)
	for i := 0; i < n; i++ {
		key := fmt.Sprintf("t.cn/%s", base62Encode(uint64(i), 7))
		bf.add(key)
		inserted[key] = true
	}

	// ① 已插入的 key 应该全部返回"可能存在"（布隆无假阴性）
	fn := 0
	for i := 0; i < n; i++ {
		key := fmt.Sprintf("t.cn/%s", base62Encode(uint64(i), 7))
		if !bf.mightContain(key) {
			fn++
		}
	}
	fmt.Printf("  假阴性: %d 个 (布隆保证为 0) → %s\n", fn, mark(fn == 0))

	// ② 未插入的随机 key: 统计误判率
	const probe = 100000
	fp := 0
	for i := 0; i < probe; i++ {
		key := fmt.Sprintf("t.cn/%s", base62Encode(uint64(n+i*7+3), 7)) // 未插入的区间
		if bf.mightContain(key) {
			fp++
		}
	}
	actual := float64(fp) / probe
	// 理论误判率: p = (1 - e^(-kn/m))^k
	theory := falsePositiveRate(k, n, m)
	fmt.Printf("  理论误判率  : p=(1-e^(-kn/m))^k = %.4f%%\n", theory*100)
	fmt.Printf("  实测误判率  : %d/%d = %.4f%%\n", fp, probe, actual*100)

	lo, hi := theory*0.8, theory*1.2
	fmt.Printf("  实测落在理论 ±20%% 区间 [%.4f%%, %.4f%%] → %s\n", lo*100, hi*100, mark(actual >= lo && actual <= hi))

	fmt.Printf("  内存占用    : %d bit ≈ %.1f KB\n", m, float64(m)/8/1024)
	fmt.Printf("  置位率      : %d/%d = %.1f%%\n", bf.setBits(), m, float64(bf.setBits())/float64(m)*100)
}

func pow62(e int) uint64 {
	r := uint64(1)
	for i := 0; i < e; i++ {
		r *= 62
	}
	return r
}

// falsePositiveRate: 理论误判率 p = (1 - e^(-kn/m))^k
// e^x 用泰勒展开计算
func falsePositiveRate(k int, n int, m uint64) float64 {
	x := -float64(k) * float64(n) / float64(m) // -kn/m
	// e^x = 1 + x + x²/2! + ...
	term := 1.0
	sum := 1.0
	for i := 1; i <= 20; i++ {
		term *= x / float64(i)
		sum += term
	}
	inner := 1 - sum // 1 - e^(-kn/m)
	r := 1.0
	for i := 0; i < k; i++ {
		r *= inner
	}
	return r
}
