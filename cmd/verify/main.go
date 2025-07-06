// Package main provides the verify command for the go-safe-cmd-runner.
// It verifies file integrity using previously recorded hashes.
package main

import (
	"fmt"
	"os"

	"github.com/isseis/go-safe-cmd-runner/internal/cmdcommon"
)

func main() {
	// Parse command line flags
	config, err := cmdcommon.ParseFlags()
	if err != nil {
		cmdcommon.PrintUsage()
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Create validator with SHA256 hasher
	validator, err := cmdcommon.CreateValidator(config.HashDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating validator: %v\n", err)
		os.Exit(1)
	}

	// Verify file hash
	err = validator.Verify(config.File)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Verification failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("OK: %s\n", config.File)
}
