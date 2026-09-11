//go:build test

package bootstrap

import (
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/logging"
	"github.com/isseis/go-safe-cmd-runner/internal/redaction"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/config"
	tu "github.com/isseis/go-safe-cmd-runner/internal/testutil"
	"github.com/isseis/go-safe-cmd-runner/internal/testutil/identitymutationguard"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestValidateIdentifierRedaction covers the five checks by shape: names the
// word list rewrites, names only the value detector rewrites, and names no
// configured transformation touches.
//
// The allowed-host rows are deliberately a pair. They fail an implementation
// that uses redaction.DefaultConfig() and one that treats a nil Config as "skip
// the check". They do NOT fail an implementation that rebuilds the Config with
// NewConfig(WithWebhookHost(...)): AddSlackHandlers passes only that option and
// the normalized host is already written back to cfg.Global, so a rebuilt
// Config behaves identically today. Only the startup-order AST guard catches
// the rebuild, which is why both exist.
func TestValidateIdentifierRedaction(t *testing.T) {
	const (
		awsKeyID  = "AKIAIOSFODNN7EXAMPLE"
		githubPAT = "ghp_aBcDeFgHiJkLmNoPqRsTuVwXyZ0123456789ab"
		jwt       = "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.abc"
		hostURL   = "https://hooks.slack.com/services/example"
	)

	withAllowedHost, err := redaction.NewConfig(redaction.WithWebhookHost("hooks.slack.com"))
	require.NoError(t, err)

	tests := []struct {
		name            string
		groupName       string
		commandName     string
		redactionConfig *redaction.Config
		wantErr         bool
	}{
		{
			name:            "ordinary names pass",
			groupName:       "backup",
			commandName:     "pg_dump",
			redactionConfig: redaction.DefaultConfig(),
		},
		{
			name:            "word match in a group name",
			groupName:       "monkey",
			redactionConfig: redaction.DefaultConfig(),
			wantErr:         true,
		},
		{
			name:            "word match in a command name",
			groupName:       "backup",
			commandName:     "rotate_api_key",
			redactionConfig: redaction.DefaultConfig(),
			wantErr:         true,
		},
		{
			name:            "aws key id as a group name",
			groupName:       awsKeyID,
			redactionConfig: redaction.DefaultConfig(),
			wantErr:         true,
		},
		{
			name:            "aws key id as a command name",
			groupName:       "backup",
			commandName:     awsKeyID,
			redactionConfig: redaction.DefaultConfig(),
			wantErr:         true,
		},
		{
			name:            "github token as a group name",
			groupName:       githubPAT,
			redactionConfig: redaction.DefaultConfig(),
			wantErr:         true,
		},
		{
			name:            "github token as a command name",
			groupName:       "backup",
			commandName:     githubPAT,
			redactionConfig: redaction.DefaultConfig(),
			wantErr:         true,
		},
		{
			// The JWT and URL shapes are command-name rows only:
			// GroupNamePattern rejects their '.' ':' '/' before this check runs
			// on the production path.
			name:            "jwt as a command name",
			groupName:       "backup",
			commandName:     jwt,
			redactionConfig: redaction.DefaultConfig(),
			wantErr:         true,
		},
		{
			name:            "url on the allowed host as a command name",
			groupName:       "backup",
			commandName:     hostURL,
			redactionConfig: withAllowedHost,
			wantErr:         true,
		},
		{
			name:            "url on an unconfigured host as a command name",
			groupName:       "backup",
			commandName:     hostURL,
			redactionConfig: redaction.DefaultConfig(),
		},
		{
			// nil is what SetupSlackLogging returns when Slack is disabled;
			// the handler chain still runs with the default transformation.
			name:            "nil config still checks word matches",
			groupName:       "monkey",
			redactionConfig: nil,
			wantErr:         true,
		},
		{
			name:            "nil config still checks value formats",
			groupName:       "backup",
			commandName:     awsKeyID,
			redactionConfig: nil,
			wantErr:         true,
		},
		{
			name:            "nil config knows no allowed host",
			groupName:       "backup",
			commandName:     hostURL,
			redactionConfig: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &runnertypes.ConfigSpec{
				Groups: []runnertypes.GroupSpec{{
					Name:     tt.groupName,
					Commands: []runnertypes.CommandSpec{{Name: tt.commandName}},
				}},
			}

			err := ValidateIdentifierRedaction(cfg, tt.redactionConfig)

			if !tt.wantErr {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.ErrorIs(t, err, config.ErrIdentifierRedacted)
		})
	}
}

// TestValidateIdentifierRedactionDoesNotEchoValues pins that a rejected
// identifier never reaches the error detail or the stderr report. The values
// this check rejects are exactly the ones redaction would rewrite, and
// HandlePreExecutionError writes its detail to stderr without redaction, so
// echoing the value would print the secret to the terminal and process log.
func TestValidateIdentifierRedactionDoesNotEchoValues(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{name: "aws key id", value: "AKIAIOSFODNN7EXAMPLE"},
		{name: "github token", value: "ghp_aBcDeFgHiJkLmNoPqRsTuVwXyZ0123456789ab"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &runnertypes.ConfigSpec{
				Groups: []runnertypes.GroupSpec{{
					Name:     "backup",
					Commands: []runnertypes.CommandSpec{{Name: tt.value}},
				}},
			}
			err := ValidateIdentifierRedaction(cfg, redaction.DefaultConfig())
			require.ErrorIs(t, err, config.ErrIdentifierRedacted)

			preExecErr, ok := errors.AsType[*logging.PreExecutionError](err)
			require.True(t, ok, "error should be a *logging.PreExecutionError")

			// Position and check name must be there for the user to locate the
			// setting, and the rejected value must not be.
			assert.Contains(t, preExecErr.Detail(), "groups[0].commands[0]")
			assert.Contains(t, preExecErr.Detail(), "redaction")
			assert.NotContains(t, preExecErr.Detail(), tt.value)

			stdout, stderr := capturePreExecutionReport(t, preExecErr)
			assert.Contains(t, stderr, "groups[0].commands[0]")
			assert.Contains(t, stderr, "redaction")
			assert.NotContains(t, stderr, tt.value, "stderr must not echo the rejected identifier")
			assert.NotContains(t, stdout, tt.value, "stdout must not echo the rejected identifier")
		})
	}
}

// TestRejectedIdentifiersAreRedactedByHandler demonstrates why the setting
// boundary rejects these names: a rejected name logged as an attribute value
// reaches the handler chain as the redaction placeholder, so the notification
// scope would no longer point at the configured group or command.
func TestRejectedIdentifiersAreRedactedByHandler(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{name: "word match", value: "monkey"},
		{name: "aws key id", value: "AKIAIOSFODNN7EXAMPLE"},
		{name: "webhook host url", value: "https://hooks.slack.com/services/example"},
		{name: "github token", value: "ghp_aBcDeFgHiJkLmNoPqRsTuVwXyZ0123456789ab"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			redactionConfig, err := redaction.NewConfig(redaction.WithWebhookHost("hooks.slack.com"))
			require.NoError(t, err)

			recorder := tu.NewLogRecorder(nil)
			handler := redaction.NewRedactingHandler(recorder, redactionConfig,
				slog.New(slog.NewTextHandler(io.Discard, nil)))
			slog.New(handler).Info("identifier", "command", tt.value)

			record := recorder.RequireRecord(t, slog.LevelInfo, "identifier")
			masked, ok := record.Attrs["command"].(string)
			require.True(t, ok, "command attribute should be a string")
			// The whole value becomes the placeholder except for a webhook URL,
			// where the detector keeps the scheme and host and masks the path:
			// either way the value is rewritten.
			assert.NotEqual(t, tt.value, masked)
			assert.Contains(t, masked, redaction.DefaultPlaceholder)
		})
	}
}

// TestSampleConfigsPassIdentifierRedaction prevents a bundled sample from
// reintroducing a name the redaction check rejects. Without it, the rejection
// would surface only when the e2e targets read the file.
func TestSampleConfigsPassIdentifierRedaction(t *testing.T) {
	root := identitymutationguard.RepositoryRoot(t)
	paths, err := filepath.Glob(filepath.Join(root, "sample", "*.toml"))
	require.NoError(t, err)
	require.NotEmpty(t, paths, "no sample TOML files found under %s", root)

	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			content, err := os.ReadFile(path)
			require.NoError(t, err)

			loader := config.NewLoaderForTest()
			cfg, err := loader.LoadConfigForTest(content)
			require.NoErrorf(t, err, "sample %s must load", path)

			require.NoError(t, ValidateIdentifierRedaction(cfg, redaction.DefaultConfig()),
				"sample %s contains an identifier redaction would rewrite", path)
		})
	}
}

// capturePreExecutionReport runs HandlePreExecutionError with os.Stdout and
// os.Stderr redirected to pipes and returns what it wrote to each. It captures
// the fmt-based report; the structured slog line goes through the handler
// installed at startup.
func capturePreExecutionReport(t *testing.T, preExecErr *logging.PreExecutionError) (stdout, stderr string) {
	t.Helper()

	outReader, outWriter, err := os.Pipe()
	require.NoError(t, err)
	errReader, errWriter, err := os.Pipe()
	require.NoError(t, err)

	origStdout, origStderr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = outWriter, errWriter
	t.Cleanup(func() {
		os.Stdout, os.Stderr = origStdout, origStderr
		_ = outWriter.Close()
		_ = errWriter.Close()
		_ = outReader.Close()
		_ = errReader.Close()
	})

	logging.HandlePreExecutionError(preExecErr)

	os.Stdout, os.Stderr = origStdout, origStderr
	require.NoError(t, outWriter.Close())
	require.NoError(t, errWriter.Close())

	outBytes, err := io.ReadAll(outReader)
	require.NoError(t, err)
	errBytes, err := io.ReadAll(errReader)
	require.NoError(t, err)
	return string(outBytes), string(errBytes)
}
