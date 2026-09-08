// golang 实验的测试公共辅助（跨实验复用）
//
// 两类用例混在一个包里：
//  1. 纯逻辑用例：直接断言实验里的可复用函数，任何环境都能跑 —— CI 里靠它们兜底
//  2. 冒烟用例：requireDemoRun 开头调 it，完整跑一遍实验并断言关键输出；
//     `go test -short` 会跳过（全套 demo 含 GC/并发/定时 sleep，整体约 1 分钟）
//
// 跑法：
//
//	go test ./experiments/ -v            # 全量（纯逻辑 + demo 冒烟）
//	go test ./experiments/ -short -v     # 只跑纯逻辑
package main

import (
	"bytes"
	"io"
	"os"
	"testing"
)

// requireDemoRun demo 冒烟用例的门槛：-short 时跳过当前用例而不是跑满全套。
// golang 实验不依赖外部服务，跳过的原因只有「省时间」——与 akafka 的 requireKafka 同位。
func requireDemoRun(t *testing.T) {
	t.Helper()

	if testing.Short() {
		t.Skip("跳过 demo 冒烟（-short 只跑纯逻辑）")
	}
}

// captureStdout 捕获 fn 执行期间打印到 os.Stdout 的内容。
// 实验代码大量用 fmt.Println 输出结论，断言输出是一种低成本的冒烟手段。
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("创建 pipe 失败: %v", err)
	}

	orig := os.Stdout
	os.Stdout = w

	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()

	fn()

	os.Stdout = orig
	_ = w.Close()
	out := <-done
	_ = r.Close()
	return out
}
