package graph_theory

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCanFinish(t *testing.T) {
	// LeetCode 示例 1: 2 门课,0→1,可完成
	assert.Equal(t, true, canFinish(2, [][]int{{1, 0}}))

	// LeetCode 示例 2: 2 门课,0↔1 形成环,不可完成
	assert.Equal(t, false, canFinish(2, [][]int{{1, 0}, {0, 1}}))

	// 无任何前置,全部可完成
	assert.Equal(t, true, canFinish(3, [][]int{}))

	// 单门课无前置
	assert.Equal(t, true, canFinish(1, [][]int{}))

	// 线性链 0→1→2→3,可完成
	assert.Equal(t, true, canFinish(4, [][]int{{1, 0}, {2, 1}, {3, 2}}))

	// 三节点环 0→1→2→0,不可完成
	assert.Equal(t, false, canFinish(3, [][]int{{1, 0}, {2, 1}, {0, 2}}))

	// 两个独立连通分量,均无环,可完成
	assert.Equal(t, true, canFinish(5, [][]int{{1, 0}, {3, 2}}))

	// 两个独立连通分量,其中一个有环,不可完成
	assert.Equal(t, false, canFinish(4, [][]int{{1, 0}, {2, 3}, {3, 2}}))

	// 自环:课程 0 是自己的前置,不可完成
	assert.Equal(t, false, canFinish(2, [][]int{{0, 0}}))
}
