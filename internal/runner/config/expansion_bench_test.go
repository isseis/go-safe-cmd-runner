package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/environment"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

// Note: LoadConfig is not accessible from this package in the same way as tests.
// We need to use the public API or change this test file to config_test package.

// BenchmarkExpandGlobalEnv measures the performance of Global.Env expansion
func BenchmarkExpandGlobalEnv(b *testing.B) {
	// Setup: Create a GlobalConfig with various env variables
	cfg := &runnertypes.GlobalConfig{
		Env: []string{
			"BASE_DIR=/opt/app",
			"LOG_LEVEL=info",
			"PATH=/opt/tools/bin:${PATH}",
			"DATA_DIR=${BASE_DIR}/data",
			"CONFIG_DIR=${BASE_DIR}/config",
		},
		EnvAllowlist: []string{"HOME", "USER", "PATH"},
	}

	// Set system environment variables
	os.Setenv("PATH", "/usr/bin:/bin")
	os.Setenv("HOME", "/home/testuser")
	defer func() {
		os.Unsetenv("PATH")
		os.Unsetenv("HOME")
	}()

	// Create expander
	filter := environment.NewFilter(cfg.EnvAllowlist)
	expander := environment.NewVariableExpander(filter)
	autoEnv := map[string]string{} // Empty auto env for benchmark

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Clear ExpandedEnv for each iteration
		cfg.ExpandedEnv = nil

		err := ExpandGlobalEnv(cfg, expander, autoEnv)
		if err != nil {
			b.Fatalf("ExpandGlobalEnv failed: %v", err)
		}
	}
}

// BenchmarkExpandGroupEnv measures the performance of Group.Env expansion
func BenchmarkExpandGroupEnv(b *testing.B) {
	// Setup: Create a GlobalConfig and CommandGroup
	globalEnv := map[string]string{
		"BASE_DIR":  "/opt/app",
		"LOG_LEVEL": "info",
	}
	globalAllowlist := []string{"HOME", "USER"}

	group := &runnertypes.CommandGroup{
		Name: "test_group",
		Env: []string{
			"APP_DIR=${BASE_DIR}/myapp",
			"DB_HOST=localhost",
			"DB_PORT=5432",
			"DB_DATA=${APP_DIR}/data",
			"LOG_DIR=${APP_DIR}/logs",
		},
		EnvAllowlist: nil, // Inherit from Global
	}

	// Set system environment variables
	os.Setenv("HOME", "/home/testuser")
	os.Setenv("USER", "testuser")
	defer func() {
		os.Unsetenv("HOME")
		os.Unsetenv("USER")
	}()

	// Create expander
	filter := environment.NewFilter(globalAllowlist)
	expander := environment.NewVariableExpander(filter)
	autoEnv := map[string]string{} // Empty auto env for benchmark

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Clear ExpandedEnv for each iteration
		group.ExpandedEnv = nil

		err := ExpandGroupEnv(group, expander, autoEnv, globalEnv, globalAllowlist)
		if err != nil {
			b.Fatalf("ExpandGroupEnv failed: %v", err)
		}
	}
}

// BenchmarkExpandCommandEnv measures the performance of Command.Env expansion
func BenchmarkExpandCommandEnv(b *testing.B) {
	// Setup: Create a Command with env variables
	globalEnv := map[string]string{
		"BASE_DIR": "/opt/app",
	}
	groupEnv := map[string]string{
		"APP_DIR": "/opt/app/myapp",
	}

	cmd := &runnertypes.Command{
		Name: "test_cmd",
		Cmd:  "/bin/echo",
		Env: []string{
			"LOG_DIR=${APP_DIR}/logs",
			"DATA_DIR=${APP_DIR}/data",
			"CONFIG_FILE=${LOG_DIR}/config.json",
		},
	}

	// Create expander
	globalAllowlist := []string{"HOME"}
	filter := environment.NewFilter(globalAllowlist)
	expander := environment.NewVariableExpander(filter)
	autoEnv := map[string]string{}    // Empty auto env for benchmark
	groupAllowlist := globalAllowlist // Use same allowlist for simplicity

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Clear ExpandedEnv for each iteration
		cmd.ExpandedEnv = nil

		err := ExpandCommandEnv(cmd, expander, autoEnv, globalEnv, globalAllowlist, groupEnv, groupAllowlist, "test_group")
		if err != nil {
			b.Fatalf("ExpandCommandEnv failed: %v", err)
		}
	}
}

// BenchmarkLoadConfigWithEnvs measures the performance of loading a complete
// configuration file with Global.Env and Group.Env
func BenchmarkLoadConfigWithEnvs(b *testing.B) {
	// Create a temporary test configuration
	tmpDir := b.TempDir()
	configPath := filepath.Join(tmpDir, "bench_test.toml")

	configContent := `[global]
env = [
    "BASE_DIR=/opt/app",
    "LOG_LEVEL=info",
    "PATH=/opt/tools/bin:${PATH}",
    "DATA_DIR=${BASE_DIR}/data",
    "CONFIG_DIR=${BASE_DIR}/config",
    "CACHE_DIR=${BASE_DIR}/cache",
    "TEMP_DIR=${BASE_DIR}/temp",
]
env_allowlist = ["HOME", "USER", "PATH"]
verify_files = ["${BASE_DIR}/verify.sh"]

[[groups]]
name = "database"
env = [
    "DB_HOST=localhost",
    "DB_PORT=5432",
    "DB_DATA=${BASE_DIR}/db-data",
    "DB_LOGS=${DB_DATA}/logs",
    "DB_BACKUP=${DB_DATA}/backup",
]
verify_files = ["${DB_DATA}/schema.sql"]

[[groups.commands]]
name = "migrate"
cmd = "${BASE_DIR}/bin/migrate"
args = ["-h", "${DB_HOST}", "-p", "${DB_PORT}"]
env = ["MIGRATION_DIR=${DB_DATA}/migrations"]

[[groups]]
name = "web"
env_allowlist = ["PORT"]
env = [
    "WEB_DIR=${BASE_DIR}/web",
    "WEB_LOGS=${WEB_DIR}/logs",
    "WEB_STATIC=${WEB_DIR}/static",
]

[[groups.commands]]
name = "start"
cmd = "${WEB_DIR}/server"
args = ["--port", "${PORT}"]
env = ["SERVER_CONFIG=${WEB_DIR}/config.json"]

[[groups]]
name = "api"
env = [
    "API_DIR=${BASE_DIR}/api",
    "API_PORT=3000",
    "API_LOGS=${API_DIR}/logs",
]

[[groups.commands]]
name = "start_api"
cmd = "${API_DIR}/api-server"
args = ["--port", "${API_PORT}"]
`

	err := os.WriteFile(configPath, []byte(configContent), 0o644)
	if err != nil {
		b.Fatalf("Failed to create test config file: %v", err)
	}

	// Set system environment variables
	os.Setenv("PATH", "/usr/bin:/bin")
	os.Setenv("HOME", "/home/testuser")
	os.Setenv("USER", "testuser")
	os.Setenv("PORT", "8080")
	defer func() {
		os.Unsetenv("PATH")
		os.Unsetenv("HOME")
		os.Unsetenv("USER")
		os.Unsetenv("PORT")
	}()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		content, err := os.ReadFile(configPath)
		if err != nil {
			b.Fatalf("Failed to read config file: %v", err)
		}
		loader := NewLoader()
		_, err = loader.LoadConfig(content)
		if err != nil {
			b.Fatalf("LoadConfig failed: %v", err)
		}
	}
}

// BenchmarkLoadConfigWithoutEnvs measures the performance of loading a configuration
// file without Global.Env and Group.Env (baseline)
func BenchmarkLoadConfigWithoutEnvs(b *testing.B) {
	// Use an existing sample file without Global.Env/Group.Env
	configPath := filepath.Join("..", "..", "..", "sample", "variable_expansion_basic.toml")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		content, err := os.ReadFile(configPath)
		if err != nil {
			b.Fatalf("Failed to read config file: %v", err)
		}
		loader := NewLoader()
		_, err = loader.LoadConfig(content)
		if err != nil {
			b.Fatalf("LoadConfig failed: %v", err)
		}
	}
}

// BenchmarkExpandGlobalEnv_LargeConfig measures performance with many variables
func BenchmarkExpandGlobalEnv_LargeConfig(b *testing.B) {
	// Setup: Create a GlobalConfig with 50 env variables
	env := make([]string, 50)
	for i := 0; i < 50; i++ {
		if i == 0 {
			env[i] = "VAR_0=value_0"
		} else {
			// Each variable references the previous one
			env[i] = "VAR_" + string(rune('0'+i%10)) + "=" + "value_" + string(rune('0'+i%10))
		}
	}

	cfg := &runnertypes.GlobalConfig{
		Env:          env,
		EnvAllowlist: []string{"HOME"},
	}

	os.Setenv("HOME", "/home/testuser")
	defer os.Unsetenv("HOME")

	filter := environment.NewFilter(cfg.EnvAllowlist)
	expander := environment.NewVariableExpander(filter)
	autoEnv := map[string]string{} // Empty auto env for benchmark

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cfg.ExpandedEnv = nil
		err := ExpandGlobalEnv(cfg, expander, autoEnv)
		if err != nil {
			b.Fatalf("ExpandGlobalEnv failed: %v", err)
		}
	}
}

// BenchmarkExpandGroupEnv_ComplexReferences measures performance with complex references
func BenchmarkExpandGroupEnv_ComplexReferences(b *testing.B) {
	// Setup: Create a GlobalConfig and Group with complex variable references
	globalEnv := map[string]string{
		"BASE_DIR":   "/opt/app",
		"DATA_DIR":   "/opt/app/data",
		"CONFIG_DIR": "/opt/app/config",
	}
	globalAllowlist := []string{"HOME", "USER"}

	group := &runnertypes.CommandGroup{
		Name: "test_group",
		Env: []string{
			"APP_DIR=${BASE_DIR}/myapp",
			"APP_DATA=${DATA_DIR}/myapp",
			"APP_CONFIG=${CONFIG_DIR}/myapp",
			"APP_LOGS=${APP_DIR}/logs",
			"APP_CACHE=${APP_DIR}/cache",
			"APP_TEMP=${APP_DIR}/temp",
			"DB_DIR=${APP_DATA}/db",
			"DB_LOGS=${APP_LOGS}/db",
			"DB_BACKUP=${DB_DIR}/backup",
			"WEB_DIR=${APP_DIR}/web",
			"WEB_STATIC=${WEB_DIR}/static",
			"WEB_TEMPLATES=${WEB_DIR}/templates",
		},
	}

	os.Setenv("HOME", "/home/testuser")
	defer os.Unsetenv("HOME")

	filter := environment.NewFilter(globalAllowlist)
	expander := environment.NewVariableExpander(filter)
	autoEnv := map[string]string{} // Empty auto env for benchmark

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		group.ExpandedEnv = nil
		err := ExpandGroupEnv(group, expander, autoEnv, globalEnv, globalAllowlist)
		if err != nil {
			b.Fatalf("ExpandGroupEnv failed: %v", err)
		}
	}
}
