// Package bootstrap provides application initialization and setup functionality.
package bootstrap

import (
	"errors"
	"fmt"
	"net/url"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/logging"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/config"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/resource"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/verification"
)

// ErrInvalidSlackAllowedHost is a sentinel error returned when the slack_allowed_host value is invalid.
var ErrInvalidSlackAllowedHost = errors.New("slack_allowed_host must be a valid hostname without port or whitespace")

// normalizeSlackAllowedHost converts host to a normalized allowed hostname.
// Returns ("", nil) when host is empty (Slack disabled).
// Returns an error for invalid values such as port numbers, schemes, paths, or whitespace.
func normalizeSlackAllowedHost(host string) (string, error) {
	if host == "" {
		return "", nil
	}
	u, err := url.Parse("https://" + host + "/")
	// u.Path is always "/" for a valid bare hostname since we append "/" in the URL.
	// A non-"/" path means the input contained a path or scheme component (e.g. "host/path" or "https://host").
	if err != nil || u.Hostname() == "" || u.Port() != "" || u.Path != "/" {
		return "", fmt.Errorf("%w (got %q)", ErrInvalidSlackAllowedHost, host)
	}
	return u.Hostname(), nil
}

// LoadAndPrepareConfig loads and verifies a configuration file.
//
// This function performs the following steps:
//  1. Verifies the configuration file's hash (TOCTOU protection)
//  2. Loads the configuration using config.Loader
//
// Note: All variable expansion (Global.EnvVars, Group.EnvVars, Command.EnvVars, Cmd, Args)
// is now performed inside config.Loader.LoadConfig(). This function only handles
// verification and loading.
//
// The returned Config is ready for execution with all variables expanded.
//
// Parameters:
//   - verificationManager: Manager for secure file verification
//   - configPath: Path to the configuration file
//   - runID: Unique identifier for this execution run
//
// Returns:
//   - *ConfigSpec: Prepared configuration ready for command execution
//   - error: Any error during verification, loading, or parsing
func LoadAndPrepareConfig(verificationManager *verification.Manager, configPath, runID string) (*runnertypes.ConfigSpec, error) {
	if configPath == "" {
		return nil, &logging.PreExecutionError{
			Type:      logging.ErrorTypeRequiredArgumentMissing,
			Message:   "Config file path is required",
			Component: string(resource.ComponentConfig),
			RunID:     runID,
		}
	}

	// Perform atomic verification and reading to prevent TOCTOU attacks
	// The verification manager reads the file once, verifies its hash, and returns the content
	content, err := verificationManager.VerifyAndReadConfigFile(configPath)
	if err != nil {
		return nil, &logging.PreExecutionError{
			Type:      logging.ErrorTypeFileAccess,
			Message:   err.Error(),
			Component: string(resource.ComponentVerification),
			RunID:     runID,
		}
	}

	// Load config from the verified content
	// All expansion (Global.EnvVars, Group.EnvVars, Command.EnvVars, Cmd, Args) is now
	// performed inside config.Loader.LoadConfig()
	// LoadConfigWithPath processes includes and merges templates from multiple files
	// Use verified template manager to ensure included files are also verified against hashes
	cfgLoader := config.NewLoader(
		common.NewDefaultFileSystem(),
		verificationManager,
	)

	cfg, err := cfgLoader.LoadConfig(configPath, content)
	if err != nil {
		return nil, &logging.PreExecutionError{
			Type:      logging.ErrorTypeConfigParsing,
			Message:   err.Error(),
			Component: string(resource.ComponentConfig),
			RunID:     runID,
		}
	}

	normalizedHost, err := normalizeSlackAllowedHost(cfg.Global.SlackAllowedHost)
	if err != nil {
		return nil, &logging.PreExecutionError{
			Type:      logging.ErrorTypeConfigParsing,
			Message:   err.Error(),
			Component: string(resource.ComponentConfig),
			RunID:     runID,
		}
	}
	cfg.Global.SlackAllowedHost = normalizedHost

	return cfg, nil
}
