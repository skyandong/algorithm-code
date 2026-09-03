package leetcode

// 最小栈
// https://leetcode.cn/problems/min-stack/description/
type MinStack struct {
	stack    []int
	minStack []int // 单调非增,栈顶即当前最小值
}

func Constructor() MinStack {
	return MinStack{}
}

func (this *MinStack) Push(value int) {
	this.stack = append(this.stack, value)
	// 等于也入 minStack,保证重复最小值弹出后仍能正确追踪
	if len(this.minStack) == 0 || value <= this.minStack[len(this.minStack)-1] {
		this.minStack = append(this.minStack, value)
	}
}

func (this *MinStack) Pop() {
	if len(this.stack) == 0 {
		return
	}
	top := this.stack[len(this.stack)-1]
	this.stack = this.stack[:len(this.stack)-1]
	if len(this.minStack) > 0 && this.minStack[len(this.minStack)-1] == top {
		this.minStack = this.minStack[:len(this.minStack)-1]
	}
}

func (this *MinStack) Top() int {
	if len(this.stack) == 0 {
		return 0
	}
	return this.stack[len(this.stack)-1]
}

func (this *MinStack) GetMin() int {
	if len(this.minStack) == 0 {
		return 0
	}
	return this.minStack[len(this.minStack)-1]
}
