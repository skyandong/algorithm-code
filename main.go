package main

import (
	"runtime"
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
