package main

import (
	"context"
	"fmt"
	"os"

	sloth "github.com/slok/sloth/pkg/lib"
)

func main() {
	configFromEnv, err := NewConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get Config: %v\n", err)
		os.Exit(1)
	}

	gen, err := sloth.NewPrometheusSLOGenerator(sloth.PrometheusSLOGeneratorConfig{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create Sloth generator: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()
	results := make(map[string]bool)

	for _, name := range chartOrder {
		config := charts[name]
		results[name] = processChart(ctx, gen, name, config, configFromEnv.RepoChartPath)
	}

	fmt.Println("\n--- Summary ---")
	hasFailure := false
	for _, name := range chartOrder {
		status := "OK"
		if !results[name] {
			status = "FAILED"
			hasFailure = true
		}
		fmt.Printf("  %s: %s\n", name, status)
	}

	if hasFailure {
		os.Exit(1)
	}
}
