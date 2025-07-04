// Package main is the entry point for the go-safe-cmd-runner application.
// This tool provides secure command execution with file validation capabilities.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/isseis/go-safe-cmd-runner/internal/filevalidator"
)

const (
	// hashDirPermissions defines the permissions for hash directories.
	hashDirPermissions = 0o750
)

func main() {
	var (
		command = flag.String("command", "", "Command to execute: 'record' or 'verify'")
		file    = flag.String("file", "", "Path to the file to record/verify")
		hashDir = flag.String("hash-dir", "", "Directory to store hash files (default: current directory)")
	)
	flag.Parse()

	if *command == "" || *file == "" {
		fmt.Fprintf(os.Stderr, "Usage: %s -command <record|verify> -file <path> [-hash-dir <dir>]\n", os.Args[0])
		flag.PrintDefaults()
		os.Exit(1)
	}

	// Set default hash directory to current directory if not specified
	if *hashDir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting current directory: %v\n", err)
			os.Exit(1)
		}
		*hashDir = cwd
	}

	// Create hash directory if it doesn't exist
	if err := os.MkdirAll(*hashDir, hashDirPermissions); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating hash directory: %v\n", err)
		os.Exit(1)
	}

	// Create validator
	validator, err := filevalidator.New(&filevalidator.SHA256{}, *hashDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating validator: %v\n", err)
		os.Exit(1)
	}

	// Execute command
	switch *command {
	case "record":
		if err := validator.Record(*file); err != nil {
			fmt.Fprintf(os.Stderr, "Error recording file: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Successfully recorded hash for file: %s\n", *file)
		
		// Show hash file location
		hashFilePath, err := validator.GetHashFilePath(*file)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting hash file path: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Hash file created at: %s\n", hashFilePath)

	case "verify":
		if err := validator.Verify(*file); err != nil {
			fmt.Fprintf(os.Stderr, "Verification failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("File verification successful: %s\n", *file)

	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", *command)
		fmt.Fprintf(os.Stderr, "Supported commands: record, verify\n")
		os.Exit(1)
	}
}
