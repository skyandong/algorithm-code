// 实验 03（interface 与反射）的单元测试 —— 冒烟为主 + 装箱值拷贝的直接断言。
package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestBoxingCopy 装箱是值拷贝：装箱后改原值，接口里仍是旧副本。
func TestBoxingCopy(t *testing.T) {
	p := point{X: 1, Y: 2}
	var i any = p
	p.X = 100

	assert.Equal(t, point{X: 1, Y: 2}, i, "接口持有装箱那一刻的副本")
	assert.Equal(t, 100, p.X)
}

// TestInterfaceSmoke 全量冒烟。
func TestInterfaceSmoke(t *testing.T) {
	requireDemoRun(t)

	out := captureStdout(t, RunInterfaceReflectionExperiments)
	assert.Contains(t, out, "装箱即拷贝")
	assert.Contains(t, out, "nil 接口", "经典事故必须出现")
	assert.Contains(t, out, "CanSet=false", "非导出字段不可 Set 必须出现")
	assert.Contains(t, out, "全路径(ValueOf+FieldByName)", "reflect 开销三档对比必须出现")
	assert.Contains(t, out, "null", "JSON nil vs 空切片必须出现")
}
