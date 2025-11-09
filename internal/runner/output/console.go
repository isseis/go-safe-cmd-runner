package output

import (
	"os"
	"sync"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/executor"
)

// ConsoleOutputWriter implements executor.OutputWriter for console output
type ConsoleOutputWriter struct {
	mu sync.Mutex
}

// NewConsoleOutputWriter creates a new console output writer
func NewConsoleOutputWriter() executor.OutputWriter {
	return &ConsoleOutputWriter{}
}

// Write implements executor.OutputWriter.Write
func (c *ConsoleOutputWriter) Write(stream executor.OutputStream, data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Write to the appropriate standard stream
	if stream == executor.StderrStream {
		_, err := os.Stderr.Write(data)
		return err
	}
	_, err := os.Stdout.Write(data)
	return err
}

// Close implements executor.OutputWriter.Close
func (c *ConsoleOutputWriter) Close() error {
	return nil
}
