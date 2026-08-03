package main

import (
	"sync"
	"time"
)

var mu sync.RWMutex
var count int

func main() {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		time.Sleep(time.Millisecond)
		wg.Done()
		wg.Add(1)
	}()
	wg.Wait()

	wg.Go(func() {

	})
}
