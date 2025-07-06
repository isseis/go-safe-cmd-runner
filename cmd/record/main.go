// Package main provides the record command for the go-safe-cmd-runner.
// It records file hashes for later verification.
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

	// Record file hash
	hashFile, err := validator.Record(config.File)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error recording file hash: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Recorded hash for %s in %s\n", config.File, hashFile)
}
