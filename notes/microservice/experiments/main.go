package main

import (
	"fmt"
	"os"
)

// mark: 验收标记（跨实验共用）
func mark(ok bool) string {
	if ok {
		return "✓"
	}
	return "✗"
}

func main() {
	exp := "all"
	if len(os.Args) > 1 {
		exp = os.Args[1]
	}

	entries := []struct {
		name string
		run  func()
	}{
		{"registry", RunRegistryExperiments},
		{"config", RunConfigExperiments},
		{"circuitbreaker", RunCircuitBreakerExperiments},
		{"gateway", RunGatewayExperiments},
		{"trace", RunTraceExperiments},
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
