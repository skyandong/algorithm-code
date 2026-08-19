package main

import (
	"fmt"
	"os"
)

func main() {
	exp := "visibility"
	if len(os.Args) > 1 {
		exp = os.Args[1]
	}

	switch exp {
	case "visibility":
		RunVisibilityExperiments()
	case "channel":
		RunChannelExperiments()
	case "interview":
		RunInterviewExperiments()
	case "masters":
		RunMastersExperiments()
	default:
		fmt.Println("用法: go run ./experiments/ [visibility|channel|interview|masters]")
	}
}
