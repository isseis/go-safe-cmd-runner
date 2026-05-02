// Package bootstrap provides application initialization and setup functionality.
package bootstrap

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/logging"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/config"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/resource"
	"github.com/isseis/go-safe-cmd-runner/internal/verification"
)

// ErrInvalidSlackAllowedHost is a sentinel error returned when the slack_allowed_host value is invalid.
var ErrInvalidSlackAllowedHost = errors.New("slack_allowed_host must be a valid hostname or IP address without port or whitespace")

// rfc1123LabelRE matches the RFC 1123 §2.1 character pattern for a single DNS label:
// starts and ends with a letter or digit; interior may contain hyphens.
// Note: the 63-character per-label and 253-character total-hostname length limits
// defined by RFC 1123 are not enforced here.
var rfc1123LabelRE = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9\-]*[A-Za-z0-9])?$`)

// normalizeSlackAllowedHost converts host to a normalized allowed host value.
// Returns ("", nil) when host is empty (Slack disabled).
// Accepts hostnames and IPv4 literals whose dot-separated labels match the RFC 1123
// character pattern, and IPv6 literals in brackets (e.g. "[::1]").
// Length limits (63 chars per label, 253 chars total) are not enforced.
// Returns an error for any other value (port, scheme, path, query, fragment, whitespace, etc.).
func normalizeSlackAllowedHost(host string) (string, error) {
	if host == "" {
		return "", nil
	}
	// IPv6 literal: "[<addr>]" — delegate bracket-stripping to url.Parse.
	if strings.HasPrefix(host, "[") {
		u, err := url.Parse("https://" + host + "/")
		if err != nil || u.Hostname() == "" || u.Port() != "" {
			return "", fmt.Errorf("%w (got %q)", ErrInvalidSlackAllowedHost, host)
		}
		return u.Hostname(), nil // bare address e.g. "::1"
	}

	// Plain hostname or IPv4 literal: validate the character pattern of each dot-separated label.
	// IPv4 addresses (e.g. "192.0.2.1") pass because each octet matches the label regex.
	for label := range strings.SplitSeq(host, ".") {
		if !rfc1123LabelRE.MatchString(label) {
			return "", fmt.Errorf("%w (got %q)", ErrInvalidSlackAllowedHost, host)
		}
	}
	return strings.ToLower(host), nil
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
