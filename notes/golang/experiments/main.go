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

	// 单个实验名 -> 入口
	entries := []struct {
		name string
		run  func()
	}{
		{"visibility", RunVisibilityExperiments},
		{"channel", RunChannelExperiments},
		{"interview", RunInterviewExperiments},
		{"masters", RunMastersExperiments},
		{"gmp", RunGMPExperiments},
		{"gcmemory", RunGCMemoryExperiments},
		{"slicemap", RunSliceMapExperiments},
		{"interface", RunInterfaceReflectionExperiments},
		{"sync", RunSyncExperiments},
		{"context", RunContextExperiments},
		{"performance", RunPerformanceExperiments},
		{"string", RunStringExperiments},
		{"generics", RunGenericsExperiments},
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

	fmt.Println("用法: go run ./experiments/ [all|visibility|channel|interview|masters|gmp|gcmemory|slicemap|interface|sync|context|performance|string|generics]")
}
