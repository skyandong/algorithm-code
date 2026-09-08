package main

import (
	"fmt"
	"os"
)

// entries 实验入口表：名字 -> 入口函数（供 main 与测试共用）。
var entries = []struct {
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
	{"tdd", RunTDDExperiments},
}

// usage 打印用法（列出全部实验名）。
func usage() {
	fmt.Println("用法: go run ./experiments/ [all|visibility|channel|interview|masters|gmp|gcmemory|slicemap|interface|sync|context|performance|string|generics|tdd]")
}

func main() {
	exp := "all"
	if len(os.Args) > 1 {
		exp = os.Args[1]
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

	usage()
}
