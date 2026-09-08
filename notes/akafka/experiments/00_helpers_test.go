// akafka 实验的测试公共辅助（跨案例复用）
//
// 两类用例混在一个包里：
//  1. 纯逻辑用例：不连 Kafka，任何环境都能跑 —— CI 里靠它们兜底
//  2. 集成用例：连 localhost:9092，开头调 requireKafka，连不上就 t.Skip，
//     所以在没起 Kafka 的机器上 go test 也不会失败（只是跳过）
//
// 跑法：
//
//	make up     # 起 Kafka（docker compose）
//	make test   # 全量；无 Kafka 时集成用例自动跳过
package main

import (
	"bytes"
	"io"
	"net"
	"os"
	"sync"
	"testing"
	"time"
)

// kafkaProbe 只探测一次，结果缓存给整个包复用。
var kafkaProbe struct {
	once      sync.Once
	reachable bool
}

// requireKafka 探测本地 Kafka；不可用时跳过当前用例而不是判失败。
// 这样 CI（没有 broker）跑全量测试也能保持绿色。
// 另外支持 go test -short：只想跑纯逻辑时用它一键跳过所有集成用例。
func requireKafka(t *testing.T) {
	t.Helper()

	if testing.Short() {
		t.Skip("跳过集成用例（-short 只跑纯逻辑）")
	}

	kafkaProbe.once.Do(func() {
		conn, err := net.DialTimeout("tcp", brokers, 500*time.Millisecond)
		if err == nil {
			kafkaProbe.reachable = true
			conn.Close()
		}
	})

	if !kafkaProbe.reachable {
		t.Skipf("跳过：%s 上没有 Kafka，先执行 make up", brokers)
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

// sumCounts 把「分区 → 消息数」的 map 求和，用于对比生产前后总量。
func sumCounts(counts map[int32]int64) int64 {
	var total int64
	for _, c := range counts {
		total += c
	}
	return total
}

// maxCount 返回分区消息数的最大值；空 map 返回 0。
func maxCount(counts map[int32]int64) int64 {
	var max int64
	for _, c := range counts {
		if c > max {
			max = c
		}
	}
	return max
}

// minCount 返回分区消息数的最小值；空 map 返回 0。
func minCount(counts map[int32]int64) int64 {
	min := int64(-1)
	for _, c := range counts {
		if min == -1 || c < min {
			min = c
		}
	}
	if min == -1 {
		return 0
	}
	return min
}
