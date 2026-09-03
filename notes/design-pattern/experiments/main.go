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
		{"principles", RunPrinciplesExperiments},
		{"creational", RunCreationalExperiments},
		{"structural", RunStructuralExperiments},
		{"behavioral", RunBehavioralExperiments},
		{"interface", RunInterfaceExperiments},
		{"concurrency", RunConcurrencyExperiments},
		{"errors", RunErrorsExperiments},
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

	fmt.Println("用法: go run ./experiments/ [all|principles|creational|structural|behavioral|interface|concurrency|errors]")
}
