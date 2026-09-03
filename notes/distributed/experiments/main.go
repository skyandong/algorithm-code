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
		{"cap", RunCAPExperiments},
		{"raft", RunRaftExperiments},
		{"localmsg", RunLocalMsgExperiments},
		{"locks", RunLocksExperiments},
		{"sharding", RunShardingExperiments},
		{"clock", RunClockExperiments},
		{"phi", RunPhiExperiments},
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

	fmt.Println("用法: go run ./experiments/ [all|cap|raft|localmsg|locks|sharding|clock|phi]")
}
