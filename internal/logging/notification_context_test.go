package logging

import (
	"context"
	"log/slog"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/redaction"
	tu "github.com/isseis/go-safe-cmd-runner/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRedactingHandler_ResolvesNotificationContextLogValue verifies that the
// redacting handler resolves the NotificationContext LogValuer into its fixed
// group encoding before forwarding the record. The Slack handler reads the
// encoded group, so a regression that forwarded an unresolved LogValuer would
// make every notification context look invalid.
func TestRedactingHandler_ResolvesNotificationContextLogValue(t *testing.T) {
	tests := []struct {
		name string
		ctx  common.NotificationContext
	}{
		{name: "global scope", ctx: common.GlobalScope()},
		{name: "group scope", ctx: common.GroupScope("backup")},
		{name: "command scope", ctx: common.CommandScope("backup", "pg_dump")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var captured []slog.Record
			handler := redaction.NewRedactingHandler(
				tu.NewCallbackHandler(func(r slog.Record) {
					captured = append(captured, r)
				}),
				nil,
				nil,
			)
			logger := slog.New(handler)
			logger.LogAttrs(context.Background(), slog.LevelInfo, "notification", tt.ctx.LogAttr())

			require.Len(t, captured, 1, "the lower handler must receive exactly one record")

			value, ok := capturedNotificationContext(captured[0])
			require.True(t, ok, "the notification context attribute must survive redaction")
			require.Equal(t, slog.KindGroup, value.Kind(),
				"redaction must resolve the LogValuer before forwarding the record")

			got, err := common.DecodeNotificationContext(value)
			require.NoError(t, err, "the context must decode to the same validity as a direct encoding")
			assert.Equal(t, tt.ctx, got)
		})
	}
}

// capturedNotificationContext returns the value of the notification context
// attribute in the record.
func capturedNotificationContext(r slog.Record) (slog.Value, bool) {
	var (
		value slog.Value
		found bool
	)
	r.Attrs(func(attr slog.Attr) bool {
		if attr.Key != common.NotificationContextAttrs.Key {
			return true
		}
		value = attr.Value
		found = true
		return false
	})
	return value, found
}
