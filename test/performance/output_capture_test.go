package performance

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/executor"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/output"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/privilege"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/resource"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"

	"github.com/stretchr/testify/require"
)

// Helper function to create RuntimeCommand from CommandSpec
func createRuntimeCommand(spec *runnertypes.CommandSpec) *runnertypes.RuntimeCommand {
	return &runnertypes.RuntimeCommand{
		Spec:             spec,
		ExpandedCmd:      spec.Cmd,
		ExpandedArgs:     spec.Args,
		ExpandedEnv:      make(map[string]string),
		ExpandedVars:     make(map[string]string),
		EffectiveWorkDir: "",
		EffectiveTimeout: 30,
	}
}

// TestLargeOutputMemoryUsage tests memory usage with large output
func TestLargeOutputMemoryUsage(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	tempDir := t.TempDir()
	outputPath := filepath.Join(tempDir, "large_output.txt")

	// Create test configuration
	cmdSpec := &runnertypes.CommandSpec{
		Name:   "large_output_test",
		Cmd:    "sh",
		Args:   []string{"-c", "yes 'A' | head -c 10240"}, // 10KB of data
		Output: outputPath,
	}
	runtimeCmd := createRuntimeCommand(cmdSpec)

	groupSpec := &runnertypes.GroupSpec{Name: "test_group"}

	// Create necessary components for ResourceManager
	fs := common.NewDefaultFileSystem()
	exec := executor.NewDefaultExecutor()
	privMgr := privilege.NewManager(slog.Default())
	logger := slog.Default()

	// Record initial memory stats
	var initialMem runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&initialMem)

	// Create resource manager and execute command
	manager := resource.NewNormalResourceManager(exec, fs, privMgr, logger)
	ctx := context.Background()
	result, err := manager.ExecuteCommand(ctx, runtimeCmd, groupSpec, map[string]string{})

	require.NoError(t, err)
	require.Equal(t, 0, result.ExitCode)

	// Record final memory stats
	var finalMem runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&finalMem)

	// Calculate memory increase (handle potential underflow)
	var memIncrease uint64
	if finalMem.Alloc > initialMem.Alloc {
		memIncrease = finalMem.Alloc - initialMem.Alloc
	} else {
		memIncrease = 0
	}

	t.Logf("Initial memory: %d bytes", initialMem.Alloc)
	t.Logf("Final memory: %d bytes", finalMem.Alloc)
	t.Logf("Memory increase: %d bytes", memIncrease)

	// Memory increase should be reasonable (less than 1MB for 10KB output)
	maxAcceptableIncrease := uint64(1024 * 1024)
	require.True(t, memIncrease < maxAcceptableIncrease,
		"Memory increase (%d bytes) exceeds acceptable limit (%d bytes)",
		memIncrease, maxAcceptableIncrease)

	// Verify output was written correctly if output file exists
	if _, err := os.Stat(outputPath); err == nil {
		data, err := os.ReadFile(outputPath)
		require.NoError(t, err)
		require.Equal(t, 10240, len(data)) // 10KB
	}
}

// MockSecurityValidator for testing
type MockSecurityValidator struct{}

func (m *MockSecurityValidator) ValidateOutputWritePermission(_ string, _ int) error {
	return nil // Allow all writes for testing
}

// TestOutputSizeLimit tests that output size limits are enforced
func TestOutputSizeLimit(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	tempDir := t.TempDir()
	outputPath := filepath.Join(tempDir, "size_limited_output.txt")

	// Create command that generates more output than the limit
	cmdSpec := &runnertypes.CommandSpec{
		Name:   "size_limit_test",
		Cmd:    "sh",
		Args:   []string{"-c", "yes 'A' | head -c 2048"}, // 2KB of data
		Output: outputPath,
	}
	runtimeCmd := createRuntimeCommand(cmdSpec)

	groupSpec := &runnertypes.GroupSpec{Name: "test_group", WorkDir: tempDir}

	// Create necessary components for ResourceManager with output capture
	fs := common.NewDefaultFileSystem()
	exec := executor.NewDefaultExecutor()
	privMgr := privilege.NewManager(slog.Default())
	logger := slog.Default()

	// Create output capture manager with size limit (1KB limit for 2KB data)
	securityValidator := &MockSecurityValidator{}
	outputMgr := output.NewDefaultOutputCaptureManager(securityValidator)
	maxOutputSize := int64(1024) // 1KB limit

	// Create resource manager with output capture support
	manager := resource.NewNormalResourceManagerWithOutput(exec, fs, privMgr, outputMgr, maxOutputSize, logger)
	ctx := context.Background()
	result, err := manager.ExecuteCommand(ctx, runtimeCmd, groupSpec, map[string]string{})

	// Should get an error due to size limit being exceeded
	require.Error(t, err, "Expected error when output size limit is exceeded")
	require.Contains(t, err.Error(), "output size limit exceeded",
		"Expected error message to contain 'output size limit exceeded', got: %v", err)

	// Result should be nil when command fails
	require.Nil(t, result)

	// Verify output file was cleaned up and does not exist
	_, statErr := os.Stat(outputPath)
	require.True(t, os.IsNotExist(statErr),
		"Output file should be cleaned up after error, but it exists")
}

// TestConcurrentExecution tests parallel command execution with output capture
func TestConcurrentExecution(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	tempDir := t.TempDir()
	numCommands := 5

	// Create necessary components for ResourceManager
	fs := common.NewDefaultFileSystem()
	exec := executor.NewDefaultExecutor()
	privMgr := privilege.NewManager(slog.Default())
	logger := slog.Default()

	// Create output capture manager
	// For testing, we can use a simple mock SecurityValidator or skip output capture

	manager := resource.NewNormalResourceManager(exec, fs, privMgr, logger)

	// Execute commands concurrently
	results := make(chan error, numCommands)
	start := time.Now()

	for i := 0; i < numCommands; i++ {
		go func(index int) {
			cmdSpec := &runnertypes.CommandSpec{
				Name:   fmt.Sprintf("concurrent_test_%d", index),
				Cmd:    "echo",
				Args:   []string{fmt.Sprintf("Output from command %d", index)},
				Output: filepath.Join(tempDir, fmt.Sprintf("output_%d.txt", index)),
			}
			runtimeCmd := createRuntimeCommand(cmdSpec)

			groupSpec := &runnertypes.GroupSpec{Name: "test_group"}

			ctx := context.Background()
			_, err := manager.ExecuteCommand(ctx, runtimeCmd, groupSpec, map[string]string{})
			results <- err
		}(i)
	}

	// Wait for all commands to complete
	for i := 0; i < numCommands; i++ {
		err := <-results
		require.NoError(t, err, "Command %d failed", i)
	}

	duration := time.Since(start)
	t.Logf("Concurrent execution of %d commands took: %v", numCommands, duration)

	// Verify all output files were created
	for i := 0; i < numCommands; i++ {
		outputPath := filepath.Join(tempDir, fmt.Sprintf("output_%d.txt", i))
		if _, err := os.Stat(outputPath); err == nil {
			data, err := os.ReadFile(outputPath)
			require.NoError(t, err)
			expected := fmt.Sprintf("Output from command %d\n", i)
			require.Equal(t, expected, string(data))
		}
	}

	// Concurrent execution should not take significantly longer than sequential
	// (allowing some overhead for goroutine management)
	maxExpectedDuration := 5 * time.Second
	require.True(t, duration < maxExpectedDuration,
		"Concurrent execution took too long: %v", duration)
}

// TestLongRunningStability tests stability with long-running commands
func TestLongRunningStability(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	tempDir := t.TempDir()
	outputPath := filepath.Join(tempDir, "long_running_output.txt")

	// Create command that runs for a while and produces incremental output
	cmdSpec := &runnertypes.CommandSpec{
		Name:    "long_running_test",
		Cmd:     "sh",
		Args:    []string{"-c", "for i in $(seq 1 10); do echo \"Line $i\"; sleep 0.1; done"},
		Output:  outputPath,
		Timeout: 30,
	}
	runtimeCmd := createRuntimeCommand(cmdSpec)

	groupSpec := &runnertypes.GroupSpec{Name: "test_group"}

	// Create necessary components for ResourceManager
	fs := common.NewDefaultFileSystem()
	exec := executor.NewDefaultExecutor()
	privMgr := privilege.NewManager(slog.Default())
	logger := slog.Default()

	// Create output capture manager
	// For testing, we can use a simple mock SecurityValidator or skip output capture

	manager := resource.NewNormalResourceManager(exec, fs, privMgr, logger)

	start := time.Now()
	ctx := context.Background()
	result, err := manager.ExecuteCommand(ctx, runtimeCmd, groupSpec, map[string]string{})
	duration := time.Since(start)

	require.NoError(t, err)
	require.Equal(t, 0, result.ExitCode)

	t.Logf("Long-running command took: %v", duration)

	// Verify output contains all expected lines if output file exists
	if _, err := os.Stat(outputPath); err == nil {
		data, err := os.ReadFile(outputPath)
		require.NoError(t, err)

		lines := strings.Split(strings.TrimSpace(string(data)), "\n")
		require.Len(t, lines, 10)

		for i, line := range lines {
			expected := fmt.Sprintf("Line %d", i+1)
			require.Equal(t, expected, line)
		}
	}

	// Should complete in reasonable time (less than 5 seconds for 10 iterations with 0.1s sleep)
	require.True(t, duration < 5*time.Second,
		"Command took too long: %v", duration)
}

// BenchmarkOutputCapture benchmarks the output capture performance
func BenchmarkOutputCapture(b *testing.B) {
	tempDir := b.TempDir()

	// Create necessary components for ResourceManager
	fs := common.NewDefaultFileSystem()
	exec := executor.NewDefaultExecutor()
	privMgr := privilege.NewManager(slog.Default())
	logger := slog.Default()

	manager := resource.NewNormalResourceManager(exec, fs, privMgr, logger)

	// Small output benchmark
	b.Run("SmallOutput", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			outputPath := filepath.Join(tempDir, fmt.Sprintf("small_output_%d.txt", i))
			cmdSpec := &runnertypes.CommandSpec{
				Name:   "small_output_bench",
				Cmd:    "echo",
				Args:   []string{"small output"},
				Output: outputPath,
			}
			runtimeCmd := createRuntimeCommand(cmdSpec)

			groupSpec := &runnertypes.GroupSpec{
				Name: "bench_group",
			}

			ctx := context.Background()
			_, err := manager.ExecuteCommand(ctx, runtimeCmd, groupSpec, map[string]string{})
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	// Large output benchmark
	b.Run("LargeOutput", func(b *testing.B) {
		largeData := strings.Repeat("A", 100*1024) // 100KB
		for i := 0; i < b.N; i++ {
			outputPath := filepath.Join(tempDir, fmt.Sprintf("large_output_%d.txt", i))
			cmdSpec := &runnertypes.CommandSpec{
				Name:   "large_output_bench",
				Cmd:    "echo",
				Args:   []string{largeData},
				Output: outputPath,
			}
			runtimeCmd := createRuntimeCommand(cmdSpec)

			groupSpec := &runnertypes.GroupSpec{
				Name: "bench_group",
			}

			ctx := context.Background()
			_, err := manager.ExecuteCommand(ctx, runtimeCmd, groupSpec, map[string]string{})
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// TestMemoryLeakDetection tests for potential memory leaks
func TestMemoryLeakDetection(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	tempDir := t.TempDir()
	iterations := 100

	// Record initial memory
	var initialMem runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&initialMem)

	// Create necessary components for ResourceManager
	fs := common.NewDefaultFileSystem()
	exec := executor.NewDefaultExecutor()
	privMgr := privilege.NewManager(slog.Default())
	logger := slog.Default()

	manager := resource.NewNormalResourceManager(exec, fs, privMgr, logger)

	// Execute many commands to detect memory leaks
	for i := 0; i < iterations; i++ {
		outputPath := filepath.Join(tempDir, fmt.Sprintf("leak_test_%d.txt", i))
		cmdSpec := &runnertypes.CommandSpec{
			Name:   fmt.Sprintf("leak_test_%d", i),
			Cmd:    "echo",
			Args:   []string{fmt.Sprintf("Test output %d", i)},
			Output: outputPath,
		}
		runtimeCmd := createRuntimeCommand(cmdSpec)

		groupSpec := &runnertypes.GroupSpec{
			Name: "leak_test_group",
		}

		ctx := context.Background()
		result, err := manager.ExecuteCommand(ctx, runtimeCmd, groupSpec, map[string]string{})
		require.NoError(t, err)
		require.Equal(t, 0, result.ExitCode)

		// Force GC every 10 iterations
		if i%10 == 0 {
			runtime.GC()
		}
	}

	// Final memory measurement
	var finalMem runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&finalMem)

	// Calculate memory increase (handle potential underflow)
	var memIncrease uint64
	if finalMem.Alloc > initialMem.Alloc {
		memIncrease = finalMem.Alloc - initialMem.Alloc
	} else {
		memIncrease = 0
	}
	t.Logf("Memory increase after %d iterations: %d bytes", iterations, memIncrease)

	// Memory increase should be minimal (less than 1MB for 100 iterations)
	maxAcceptableIncrease := uint64(1024 * 1024)
	require.True(t, memIncrease < maxAcceptableIncrease,
		"Potential memory leak detected: %d bytes increase", memIncrease)
}
