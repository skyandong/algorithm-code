package main

import (
	"fmt"
	"os"
)

func main() {
	exp := "all"
	if len(os.Args) > 1 {
		exp = os.Args[1]
	}

	entries := []struct {
		name string
		run  func()
	}{
		{"estimate", RunEstimateExperiments},
		{"seckill", RunSeckillExperiments},
		{"shorturl", RunShortURLExperiments},
		{"feed", RunFeedExperiments},
		{"im", RunIMExperiments},
		{"segment", RunSegmentExperiments},
		{"timewheel", RunTimeWheelExperiments},
		{"cache", RunCacheExperiments},
	}

	if exp == "all" {
		for _, e := range entries {
			fmt.Printf("\n############ 实验 %s ############\n", e.name)
			e.run()
		}
		return
	}

	for _, e := range entries {
		if e.name == exp {
			e.run()
			return
		}
	}

	fmt.Printf("未知实验名: %s（可用: ", exp)
	for i, e := range entries {
		if i > 0 {
			fmt.Print(", ")
		}
		fmt.Print(e.name)
	}
	fmt.Println(")")
	os.Exit(1)
}
