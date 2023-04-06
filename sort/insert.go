package sort

// InsertSort 插入排序
func InsertSort(array []int) {
	for begin := 0; begin < len(array)-1; begin++ {
		cur := begin
		tmp := array[cur+1]
		for cur >= 0 {
			if array[cur] > tmp {
				array[cur+1] = array[cur]
				cur--
			} else {
				break
			}
		}
		array[cur+1] = tmp
	}
}

// ShallSort 希尔排序
func ShallSort(array []int) {
	for gap := 8; gap > 1; {
		gap = gap/3 + 1
		for begin := 0; begin < len(array)-gap; begin++ {
			cur := begin
			tmp := array[cur+gap]
			for cur >= 0 {
				if array[cur] > tmp {
					array[cur+gap] = array[cur]
					cur -= gap
				} else {
					break
				}
			}
			array[cur+gap] = tmp
		}
	}
}
