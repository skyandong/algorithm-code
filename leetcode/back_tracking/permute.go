package backtracking

func permute(nums []int) [][]int {
	res := [][]int{}
	use := make([]bool, len(nums))
	backtrackPermute(nums, []int{}, &res, use)
	return res
}

func backtrackPermute(nums, part []int, res *[][]int, use []bool) {
	if len(nums) == len(part) {
		*res = append(*res, append([]int{}, part...))
		return
	}

	for i := 0; i < len(nums); i++ {
		if use[i] == true {
			continue
		}

		part = append(part, nums[i])
		use[i] = true

		backtrackPermute(nums, part, res, use)
		part = part[:len(part)-1]
		use[i] = false
	}
}
