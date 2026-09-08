// 实验 02（string 底层）的单元测试 —— 冒烟为主（实验全是打印演示，无独立纯函数）。
package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestStringSmoke 全量冒烟：断言各节关键结论。
func TestStringSmoke(t *testing.T) {
	requireDemoRun(t)

	out := captureStdout(t, RunStringExperiments)
	assert.Contains(t, out, "子串零拷贝", "StringData 指针相同的证据必须出现")
	assert.Contains(t, out, "免拷贝", "不逃逸零分配的实测必须出现")
	assert.Contains(t, out, "m[string(b)] 免分配")
	assert.Contains(t, out, "Builder+Grow", "拼接对比必须包含预分配姿势")
	assert.Contains(t, out, "碎码", "按字节截断产生非法 UTF-8 必须演示")
	assert.Contains(t, out, "编译器优化清单", "第 7 节清单必须出现")
}
