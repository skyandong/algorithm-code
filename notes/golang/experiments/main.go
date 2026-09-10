package main

import (
	"fmt"
	"os"
)

// entries 实验入口表：名字 -> 入口函数（供 main 与测试共用）。
// 注：实验 01（slice/string/map）已全部改为断言式单元测试，
// 见 01-1_slice_test.go、01-2_map_test.go、01-3_string_test.go。
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
	{"interface", RunInterfaceReflectionExperiments},
	{"sync", RunSyncExperiments},
	{"context", RunContextExperiments},
	{"performance", RunPerformanceExperiments},
	{"generics", RunGenericsExperiments},
	{"tdd", RunTDDExperiments},
}

// usage 打印用法（列出全部实验名）。
func usage() {
	fmt.Println("用法: go run ./experiments/ [all|visibility|channel|interview|masters|gmp|gcmemory|interface|sync|context|performance|generics|tdd]")
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
