package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/cyrano-janus/osb-checker/test"
	"github.com/cyrano-janus/osb-checker/test/config"
)

// version wird beim Release ueber -ldflags gesetzt.
var version = "dev"

func main() {
	configFile := flag.String("f", "configs/config.yaml", "Path to configuration file")
	verbose := flag.Bool("v", false, "Enable verbose output")
	showVersion := flag.Bool("version", false, "Print the version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("osb-checker %s\n", version)
		return
	}

	cfg, err := config.LoadConfig(*configFile)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Was geprueft wird, gehoert in die Ausgabe: sonst laesst sich ein
	// gruener Bericht nicht von einem unterscheiden, der gegen den falschen
	// Broker oder den falschen Service lief.
	fmt.Printf("osb-checker %s -> %s (OSB %s", version, cfg.BrokerURL, cfg.APIVersion)
	if cfg.ServiceID != "" {
		fmt.Printf(", service %s", cfg.ServiceID)
	}
	if cfg.AcceptsAsync {
		fmt.Print(", async")
	}
	fmt.Println(")")
	if cfg.Insecure {
		fmt.Fprintln(os.Stderr, "osb-checker: WARNING insecure=true disables broker certificate verification; never use this in CI")
	}

	suite, err := test.NewTestSuite(cfg, *verbose)
	if err != nil {
		log.Fatalf("Failed to build test suite: %v", err)
	}

	results := suite.Run()
	printResults(results)

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
