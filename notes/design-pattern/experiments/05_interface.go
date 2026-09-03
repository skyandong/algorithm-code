// # 接口设计实验（笔记 05）
//
// 对应笔记：notes/design-pattern/05-接口设计模式.md
//
// 运行：go run ./experiments/ interface
//
// 实验项：
//
//	第1节：io.Reader 生态 —— 单方法接口连接一切（文件→压缩→哈希 的内存版）
//	第2节：实现 Reader 的三条契约
//	第3节：小接口组合 —— Reader/Writer/Closer 按需要求
package main

import (
	"bytes"
	"crypto/md5"
	"fmt"
	"io"
	"strings"
)

// RunInterfaceExperiments 演示笔记 05 的接口设计。
func RunInterfaceExperiments() {
	fmt.Println("========== 第1节: io.Reader 生态 ==========")
	i1Ecosystem()

	fmt.Println("\n========== 第2节: Read 三契约 ==========")
	i2Contracts()

	fmt.Println("\n========== 第3节: 按需要求小接口 ==========")
	i3Compose()
}

// i1Ecosystem 一个函数（io.Copy）连接所有实现。
func i1Ecosystem() {
	src := strings.NewReader("hello interface ecology") // 内存当文件

	var buf bytes.Buffer                            // 目的地是内存
	hasher := md5.New()                              // 目的地是哈希

	n1, _ := io.Copy(&buf, src)                      // reader → writer（内存→内存）
	src.Reset("hello interface ecology")
	n2, _ := io.Copy(hasher, src)                    // 同一个 Copy，喂给哈希

	fmt.Printf("io.Copy -> buffer: %d 字节, 内容=%q\n", n1, buf.String())
	fmt.Printf("io.Copy -> md5:   %d 字节, sum=%x\n", n2, hasher.Sum(nil))
	fmt.Println("同一个 io.Copy 连接 strings/bytes/md5——单方法接口的生态威力")
}

// i2Contracts 实现方遵守 vs 违反契约的对照。
type shortReader struct {
	r      io.Reader
	shortN int // 每次只读这么多（模拟 n<len(p) 合法）
}

func (s *shortReader) Read(p []byte) (int, error) {
	if len(p) > s.shortN {
		p = p[:s.shortN] // 契约①：n<len(p) 完全合法
	}
	return s.r.Read(p)
}

type badReader struct{}

func (badReader) Read(p []byte) (int, error) {
	return 0, nil // 契约③：(0,nil) 是死循环陷阱——io.Copy 会永远转
}

func i2Contracts() {
	// 契约①：短读不丢数据，io.Copy 靠循环兜底
	sr := &shortReader{r: strings.NewReader("0123456789"), shortN: 3}
	var out bytes.Buffer
	n, err := io.Copy(&out, sr)
	fmt.Printf("每次最多读3字节: io.Copy 仍凑齐 %d 字节 = %q, err=%v（调用方不假设读满）\n",
		n, out.String(), err)

	_ = badReader{} // (0,nil) 版本不真跑 io.Copy——会死循环；契约的违反方式
	fmt.Println("badReader 返回 (0,nil): 不演示真跑（io.Copy 死循环）——这就是契约③要防的")
}

// i3Compose 消费者按需收窄依赖面。
type memFile struct {
	data []byte
	pos  int
}

func (m *memFile) Read(p []byte) (int, error) { // 只实现 Read
	if m.pos >= len(m.data) {
		return 0, io.EOF
	}
	n := copy(p, m.data[m.pos:])
	m.pos += n
	return n, nil
}

// Process 只要求 Reader——能传的东西最多。
func Process(r io.Reader) (string, error) {
	b, err := io.ReadAll(r)
	return string(b), err
}

// ProcessRC 要求 Read+Close——能做的多了（负责关闭），能传的少了。
func ProcessRC(rc io.ReadCloser) error {
	defer rc.Close() // 拿到就要负责关
	_, err := io.ReadAll(rc)
	return err
}

type nopCloser struct{ io.Reader }

func (nopCloser) Close() error { return nil }

func i3Compose() {
	f := &memFile{data: []byte("mem-file content")}

	s, err := Process(f) // memFile 只有 Read，也能传
	fmt.Printf("Process(io.Reader): %q err=%v（依赖面最小, 可传最多）\n", s, err)

	err = ProcessRC(nopCloser{f}) // 包一层 Closer 才能满足更窄的签名
	fmt.Printf("ProcessRC(io.ReadCloser): err=%v（要求多了, 可传的少了——能力换依赖）\n", err)

	fmt.Println("io.ReadCloser = Reader 组合 Closer——多能力用组合, 不造大接口")
}
