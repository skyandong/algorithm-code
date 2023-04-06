package sort

// BubbleSort 冒泡排序
func BubbleSort(array []int) {
	for end := len(array) - 1; end > 0; end-- {
		for begin := 0; begin < end; begin++ {
			if array[begin] > array[begin+1] {
				array[begin], array[begin+1] = array[begin+1], array[begin]
			}
		}
	}
}
