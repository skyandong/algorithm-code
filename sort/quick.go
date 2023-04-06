package sort

import (
	"github.com/skyandong/go-code/stack"
)

// partSort1 左右指针法
func partSort1(array []int, left int, right int) (begin int) {
	begin = left
	key := array[right]
	for end := right; begin < end; {
		for begin < end && array[begin] <= key {
			begin++
		}
		for begin < end && array[end] >= key {
			end--
		}
		if begin < end {
			array[begin], array[end] = array[end], array[begin]
		}
	}
	array[begin], array[right] = array[right], array[begin]
	return
}

// partSort2 挖坑法
func partSort2(array []int, left int, right int) (begin int) {
	begin = left
	key := array[right]
	for end := right; begin < end; {
		for begin < end && array[begin] <= key {
			begin++
		}
		array[end] = array[begin]
		for begin < end && array[end] >= key {
			end--
		}
		array[begin] = array[end]
	}
	array[begin] = key
	return
}

// partSort3 前后指针法
func partSort3(array []int, left int, right int) (prev int) {
	prev = left - 1
	key := array[right]
	for cur := left; cur < right; cur++ {
		for array[cur] < key {
			if prev++; prev != cur {
				array[cur], array[prev] = array[prev], array[cur]
			} else {
				break
			}
		}
	}
	prev++
	array[prev], array[right] = array[right], array[prev]
	return
}

// SortNonR 快排非递归
func SortNonR(array []int, begin int, end int) {
	s := stack.NewStack()
	s.Push(begin)
	s.Push(end)
	for !s.Empty() {
		end = s.Top().(int)
		s.Pop()
		begin = s.Top().(int)
		s.Pop()
		if begin < end {
			dev := partSort3(array, begin, end)
			s.Push(begin)
			s.Push(dev - 1)
			s.Push(dev + 1)
			s.Push(end)
		}
	}
}

// sort 快排
func sort(array []int, begin int, end int) {
	if begin >= end {
		return
	}
	dev := partSort3(array, begin, end)
	sort(array, begin, dev-1)
	sort(array, dev+1, end)
}

// QuickSort 快排
func QuickSort(array []int) {
	//sort(array, 0, len(array) - 1)
	SortNonR(array, 0, len(array)-1)
}
