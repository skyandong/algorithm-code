package main

import (
	"runtime"
	"sync"
	"time"
)

func main() {
	cpuNum := runtime.NumCPU()
	for i := 0; i < cpuNum; i++ {
		go func() {
			var num int64
			for {
				num++
			}
		}()
	}
	time.Sleep(time.Minute)
}

func test(nums []int, n int) int {
	ch := make(chan int, n)

	wg := sync.WaitGroup{}
	part := len(nums) / n
	more := len(nums) % n

	for i := 0; i < n; i++ {
		start := i * part
		end := part * (i + 1)
		if i == n-1 {
			end += more
		}
		tempNums := nums[start:end]
		wg.Go(func() {
			var sum int
			for i := 0; i < len(tempNums); i++ {
				sum += tempNums[i]
			}
			ch <- sum
		})
	}
	wg.Wait()

	var sum int
	for i := 0; i < n; i++ {
		sum += <-ch
	}
	return sum
}
