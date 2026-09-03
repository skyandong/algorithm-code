// # 设计原则实验（笔记 01）
//
// 对应笔记：notes/design-pattern/01-设计原则的Go式解读.md
//
// 运行：go run ./experiments/ principles
//
// 实验项：
//
//	第1节：需求演进三步对照（v1 具体依赖 → v2 接口参数 → v3 双维度开闭）
//	第2节：嵌入 vs 继承——嵌入方法无法覆写，换行为只能换内层实例
//	第3节：消费者侧小接口——隐式满足 + 免费 mock
package main

import (
	"bytes"
	"fmt"
	"strings"
)

// RunPrinciplesExperiments 演示笔记 01 的设计原则。
func RunPrinciplesExperiments() {
	fmt.Println("========== 第1节: 需求演进三步对照 ==========")
	p1Evolution()

	fmt.Println("\n========== 第2节: 嵌入不是继承 ==========")
	p2Embedding()

	fmt.Println("\n========== 第3节: 消费者侧接口 ==========")
	p3Consumer()
}

// ---------- 第1节：v1 → v2 → v3 ----------

func p1Evolution() {
	// v1: 写死文件——换目的地要改函数体（违开闭）
	fmt.Println("v1: GenerateFile 写死 os.Create —— 新目的地 = 改函数体（散弹修改）")

	// v2: 目的地维度开闭——收 io.Writer，文件/内存/网络随便传
	var buf bytes.Buffer
	GenerateV2(&buf)
	fmt.Printf("v2: GenerateV2(io.Writer) —— 传内存 buffer 即可, 输出: %q\n", buf.String())

	// v3: 双维度开闭——目的地(W) + 格式(Formatter) 都可扩展
	buf.Reset()
	GenerateV3(&buf, upperFormatter{})
	fmt.Printf("v3: GenerateV3(w, upperFormatter) —— 加格式不改旧码, 输出: %q\n", buf.String())

	buf.Reset()
	GenerateV3(&buf, prefixFormatter{"[json] "})
	fmt.Printf("v3: 换 prefixFormatter —— 新类型新文件即可, 输出: %q\n", buf.String())
}

// Formatter v3 的格式扩展点（策略接口）。
type Formatter interface {
	Format(b string) string
}

type upperFormatter struct{}

func (upperFormatter) Format(s string) string { return strings.ToUpper(s) }

type prefixFormatter struct{ p string }

func (f prefixFormatter) Format(s string) string { return f.p + s }

// GenerateV2 v2：目的地维度开闭。
func GenerateV2(w interface{ WriteString(string) (int, error) }) {
	_, _ = w.WriteString(bill())
}

// GenerateV3 v3：双维度开闭。
func GenerateV3(w interface{ WriteString(string) (int, error) }, f Formatter) {
	_, _ = w.WriteString(f.Format(bill()))
}

func bill() string { return "order-42: amount=100" }

// ---------- 第2节：嵌入 vs 继承 ----------

type baseLogger struct{ prefix string }

func (b *baseLogger) Log(msg string) { fmt.Printf("%s%s\n", b.prefix, msg) }

// p2Embedding 嵌入无法覆写：inner.Log 永远是内层实现。
func p2Embedding() {
	// 嵌入 = has-a + 方法转发。没有虚函数：调的是谁的实现，写死在转发目标上
	svc := struct {
		*baseLogger // 嵌入：借 Log 方法
	}{&baseLogger{"[svc] "}}
	svc.Log("嵌入调用 —— 转发给内层 baseLogger")

	// 想换行为：不是"覆写"，是换内层实例（依赖注入）
	quiet := &baseLogger{prefix: "[quiet] "}
	wrapped := quietService{quiet}
	wrapped.Log("换掉内层实例 —— 行为随之改变")
	fmt.Println("结论: 嵌入没有虚函数, 换行为 = 换内层(注入); 多态唯一来源是接口")
}

type quietService struct {
	inner *baseLogger
}

func (q quietService) Log(msg string) { q.inner.Log("(静音) " + msg) }

// ---------- 第3节：消费者侧接口 ----------

// logger 消费者侧最小视角：包内私有、单方法（生产者不知道它存在）。
type logger interface{ Log(msg string) }

// RunWith 接受任何会 Log 的东西——标准实现/测试 stub 隐式满足。
func RunWith(l logger) { l.Log("consumer-side interface in action") }

type stdLogger struct{}

func (stdLogger) Log(msg string) { fmt.Printf("std: %s\n", msg) }

type fakeLogger struct{ msgs []string }

func (f *fakeLogger) Log(msg string) { f.msgs = append(f.msgs, msg) } // 3 行 mock，无需框架

func p3Consumer() {
	RunWith(stdLogger{}) // 真实实现

	fake := &fakeLogger{}
	RunWith(fake) // 测试 stub
	fmt.Printf("fake 收到: %v —— 生产者不知道接口存在, mock 免费\n", fake.msgs)
}
