package sort

func adjustDown(array []int, lens int, begin int) {
	parent := begin
	child := (begin * 2) + 1
	for child < lens {
		if child+1 < lens && array[child] < array[child+1] {
			child++
		}
		if array[child] > array[parent] {
			array[child], array[parent] = array[parent], array[child]
			parent = child
			child = parent*2 + 1
		} else {
			break
		}
	}
}

// HeapSort 堆排序
func HeapSort(array []int) {
	for begin := (len(array) - 2) / 2; begin >= 0; begin-- {
		adjustDown(array, len(array), begin)
	}
	for end := len(array) - 1; end > 0; end-- {
		array[0], array[end] = array[end], array[0]
		adjustDown(array, end, 0)
	}
}
