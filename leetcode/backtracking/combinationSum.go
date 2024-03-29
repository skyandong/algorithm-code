package backtracking

func combinationSum(candidates []int, target int) [][]int {
	numMap := make(map[int]bool)
	for _, data := range candidates {
		numMap[data] = true
	}

	var res [][]int
	work(numMap, target, &res, &[]int{})
	return res
}

func work(dataMap map[int]bool, target int, s *[][]int, stack *[]int) {
	num := 2
	if len(*stack) > 0 {
		num = (*stack)[len(*stack)-1]
	}

	for ; num <= target; num++ {
		if !dataMap[num] {
			continue
		}

		// 入栈
		*stack = append(*stack, num)
		target -= num
		if target == 0 {
			r := make([]int, len(*stack))
			copy(r, *stack)
			*s = append((*s), r)
		}
		work(dataMap, target, s, stack)
		// 出栈
		*stack = (*stack)[:len(*stack)-1]
		// 出栈时
		target += num
	}
	return
}
