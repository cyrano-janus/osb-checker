package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/example/osb-checker/test"
	"github.com/example/osb-checker/test/config"
)

func main() {
	configFile := flag.String("f", "configs/config.yaml", "Path to configuration file")
	verbose := flag.Bool("v", false, "Enable verbose output")
	flag.Parse()

	// Load configuration
	cfg, err := config.LoadConfig(*configFile)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Create test suite
	suite := test.NewTestSuite(cfg, *verbose)

	// Run all tests
	results := suite.Run()

	// Print results
	printResults(results)

	// Exit with appropriate code
	if results.Failed > 0 {
		os.Exit(1)
	}
}

func printResults(results *test.TestResults) {
	fmt.Println("\n========================================")
	fmt.Println("OSB Checker Test Results (Spec 2.17)")
	fmt.Println("========================================")
	fmt.Printf("Total Tests: %d\n", results.Total)
	fmt.Printf("Passed: %d\n", results.Passed)
	fmt.Printf("Failed: %d\n", results.Failed)
	fmt.Printf("Skipped: %d\n", results.Skipped)
	fmt.Println("========================================")

	if len(results.Failures) > 0 {
		fmt.Println("\nFAILURES:")
		fmt.Println("---------")
		for _, failure := range results.Failures {
			fmt.Printf("❌ %s\n", failure.TestName)
			fmt.Printf("   Error: %s\n", failure.Error)
			if failure.Endpoint != "" {
				fmt.Printf("   Endpoint: %s %s\n", failure.Method, failure.Endpoint)
			}
			fmt.Println()
		}
	}

	if len(results.Successes) > 0 {
		fmt.Println("\nSUCCESSES:")
		fmt.Println("----------")
		for _, success := range results.Successes {
			fmt.Printf("✅ %s\n", success.TestName)
		}
	}

	fmt.Println("\n========================================")
	if results.Failed == 0 {
		fmt.Println("🎉 All tests passed!")
	} else {
		fmt.Printf("⚠️  %d test(s) failed\n", results.Failed)
	}
	fmt.Println("========================================")
}