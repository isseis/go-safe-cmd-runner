package common

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func contextGroupValue(attrs ...slog.Attr) slog.Value {
	return slog.GroupValue(attrs...)
}

func scopeAttr(name string) slog.Attr {
	return slog.String(NotificationContextAttrs.Scope, name)
}

func groupAttr(name string) slog.Attr {
	return slog.String(NotificationContextAttrs.Group, name)
}

func commandAttr(name string) slog.Attr {
	return slog.String(NotificationContextAttrs.Command, name)
}

func TestNotificationContext_ZeroValueIsGlobalScope(t *testing.T) {
	var zero NotificationContext

	assert.Equal(t, ScopeGlobal, zero.Scope())
	assert.Equal(t, "", zero.GroupName())
	assert.Equal(t, "", zero.CommandName())
	assert.Equal(t, GlobalScope(), zero)
	assert.Equal(t, GlobalScope().LogValue(), zero.LogValue())
	assert.Equal(t, GlobalScope().LogAttr(), zero.LogAttr())
}

func TestNotificationContext_LogValueEncoding(t *testing.T) {
	tests := []struct {
		name string
		ctx  NotificationContext
		want slog.Value
	}{
		{
			name: "zero value encodes as a global scope",
			ctx:  NotificationContext{},
			want: contextGroupValue(scopeAttr(NotificationScopeNames.Global), groupAttr("")),
		},
		{
			name: "global scope always carries the group key",
			ctx:  GlobalScope(),
			want: contextGroupValue(scopeAttr(NotificationScopeNames.Global), groupAttr("")),
		},
		{
			name: "group scope omits the command key",
			ctx:  GroupScope("backup"),
			want: contextGroupValue(scopeAttr(NotificationScopeNames.Group), groupAttr("backup")),
		},
		{
			name: "group scope with an empty name still encodes its scope",
			ctx:  GroupScope(""),
			want: contextGroupValue(scopeAttr(NotificationScopeNames.Group), groupAttr("")),
		},
		{
			name: "command scope carries all three keys",
			ctx:  CommandScope("backup", "pg_dump"),
			want: contextGroupValue(
				scopeAttr(NotificationScopeNames.Command),
				groupAttr("backup"),
				commandAttr("pg_dump"),
			),
		},
		{
			name: "command scope with an empty command omits the command key",
			ctx:  CommandScope("backup", ""),
			want: contextGroupValue(scopeAttr(NotificationScopeNames.Command), groupAttr("backup")),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.ctx.LogValue())
		})
	}
}

func TestNotificationContext_Accessors(t *testing.T) {
	global := GlobalScope()
	assert.Equal(t, ScopeGlobal, global.Scope())
	assert.Equal(t, "", global.GroupName())
	assert.Equal(t, "", global.CommandName())

	group := GroupScope("backup")
	assert.Equal(t, ScopeGroup, group.Scope())
	assert.Equal(t, "backup", group.GroupName())
	assert.Equal(t, "", group.CommandName())

	command := CommandScope("backup", "pg_dump")
	assert.Equal(t, ScopeCommand, command.Scope())
	assert.Equal(t, "backup", command.GroupName())
	assert.Equal(t, "pg_dump", command.CommandName())
}

func TestNotificationContext_LogAttrUsesSharedKey(t *testing.T) {
	attr := GroupScope("backup").LogAttr()
	assert.Equal(t, NotificationContextAttrs.Key, attr.Key)
}

func TestDecodeNotificationContext_RoundTrip(t *testing.T) {
	tests := []struct {
		name string
		ctx  NotificationContext
	}{
		{name: "zero value", ctx: NotificationContext{}},
		{name: "global scope", ctx: GlobalScope()},
		{name: "group scope", ctx: GroupScope("backup")},
		{name: "command scope", ctx: CommandScope("backup", "pg_dump")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DecodeNotificationContext(tt.ctx.LogValue())
			require.NoError(t, err)
			assert.Equal(t, tt.ctx, got)
		})
	}
}

func TestGroupScope_EmptyNameIsInvalidAtDisplayBoundary(t *testing.T) {
	ctx := GroupScope("")

	// Construction must succeed; validation belongs to the display boundary.
	assert.Equal(t, ScopeGroup, ctx.Scope())
	assert.Equal(t, "", ctx.GroupName())
	assert.Equal(
		t,
		contextGroupValue(scopeAttr(NotificationScopeNames.Group), groupAttr("")),
		ctx.LogValue(),
	)

	_, err := DecodeNotificationContext(ctx.LogValue())
	require.ErrorIs(t, err, ErrInvalidNotificationContext)
}

func TestNotificationContext_UnknownScopeEncodesEmptyAndIsRejected(t *testing.T) {
	// Only an in-package value can hold an out-of-range scope; it encodes as
	// an empty scope name so the decoder rejects it instead of seeing a valid
	// global scope.
	ctx := NotificationContext{scope: NotificationScope(99), group: "backup"}

	assert.Equal(
		t,
		contextGroupValue(scopeAttr(""), groupAttr("backup")),
		ctx.LogValue(),
	)

	_, err := DecodeNotificationContext(ctx.LogValue())
	require.ErrorIs(t, err, ErrInvalidNotificationContext)
}

func TestDecodeNotificationContext_Validity(t *testing.T) {
	tests := []struct {
		name    string
		value   slog.Value
		want    NotificationContext
		wantErr bool
	}{
		{
			name:  "global encoding",
			value: contextGroupValue(scopeAttr(NotificationScopeNames.Global), groupAttr("")),
			want:  GlobalScope(),
		},
		{
			name: "group encoding without a command key",
			value: contextGroupValue(
				scopeAttr(NotificationScopeNames.Group),
				groupAttr("backup"),
			),
			want: GroupScope("backup"),
		},
		{
			name: "command encoding",
			value: contextGroupValue(
				scopeAttr(NotificationScopeNames.Command),
				groupAttr("backup"),
				commandAttr("pg_dump"),
			),
			want: CommandScope("backup", "pg_dump"),
		},
		{
			name: "global encoding with an explicit empty command",
			value: contextGroupValue(
				scopeAttr(NotificationScopeNames.Global),
				groupAttr(""),
				commandAttr(""),
			),
			want: GlobalScope(),
		},
		{
			name: "group encoding with an explicit empty command",
			value: contextGroupValue(
				scopeAttr(NotificationScopeNames.Group),
				groupAttr("backup"),
				commandAttr(""),
			),
			want: GroupScope("backup"),
		},
		{
			name:    "value is not a group",
			value:   slog.StringValue(NotificationScopeNames.Global),
			wantErr: true,
		},
		{
			name:    "scope key is missing",
			value:   contextGroupValue(groupAttr("")),
			wantErr: true,
		},
		{
			name:    "group key is missing",
			value:   contextGroupValue(scopeAttr(NotificationScopeNames.Global)),
			wantErr: true,
		},
		{
			name: "scope value is not a string",
			value: contextGroupValue(
				slog.Int(NotificationContextAttrs.Scope, 0),
				groupAttr(""),
			),
			wantErr: true,
		},
		{
			name: "group value is not a string",
			value: contextGroupValue(
				scopeAttr(NotificationScopeNames.Group),
				slog.Int(NotificationContextAttrs.Group, 1),
			),
			wantErr: true,
		},
		{
			name: "command value is not a string",
			value: contextGroupValue(
				scopeAttr(NotificationScopeNames.Command),
				groupAttr("backup"),
				slog.Int(NotificationContextAttrs.Command, 1),
			),
			wantErr: true,
		},
		{
			name:    "scope name is unknown",
			value:   contextGroupValue(scopeAttr("tenant"), groupAttr("")),
			wantErr: true,
		},
		{
			name: "unknown lower key",
			value: contextGroupValue(
				scopeAttr(NotificationScopeNames.Global),
				groupAttr(""),
				slog.String("tenant", "backup"),
			),
			wantErr: true,
		},
		{
			name: "scope key is duplicated",
			value: contextGroupValue(
				scopeAttr(NotificationScopeNames.Global),
				scopeAttr(NotificationScopeNames.Global),
				groupAttr(""),
			),
			wantErr: true,
		},
		{
			name: "group key is duplicated",
			value: contextGroupValue(
				scopeAttr(NotificationScopeNames.Group),
				groupAttr("backup"),
				groupAttr("backup"),
			),
			wantErr: true,
		},
		{
			name: "command key is duplicated",
			value: contextGroupValue(
				scopeAttr(NotificationScopeNames.Command),
				groupAttr("backup"),
				commandAttr("pg_dump"),
				commandAttr("pg_dump"),
			),
			wantErr: true,
		},
		{
			name: "global scope carries a group name",
			value: contextGroupValue(
				scopeAttr(NotificationScopeNames.Global),
				groupAttr("backup"),
			),
			wantErr: true,
		},
		{
			name: "global scope carries a command name",
			value: contextGroupValue(
				scopeAttr(NotificationScopeNames.Global),
				groupAttr(""),
				commandAttr("pg_dump"),
			),
			wantErr: true,
		},
		{
			name: "group scope has an empty group name",
			value: contextGroupValue(
				scopeAttr(NotificationScopeNames.Group),
				groupAttr(""),
			),
			wantErr: true,
		},
		{
			name: "group scope carries a command name",
			value: contextGroupValue(
				scopeAttr(NotificationScopeNames.Group),
				groupAttr("backup"),
				commandAttr("pg_dump"),
			),
			wantErr: true,
		},
		{
			name: "command scope has an empty group name",
			value: contextGroupValue(
				scopeAttr(NotificationScopeNames.Command),
				groupAttr(""),
				commandAttr("pg_dump"),
			),
			wantErr: true,
		},
		{
			name: "command scope has an empty command name",
			value: contextGroupValue(
				scopeAttr(NotificationScopeNames.Command),
				groupAttr("backup"),
				commandAttr(""),
			),
			wantErr: true,
		},
		{
			name: "command scope has no command key",
			value: contextGroupValue(
				scopeAttr(NotificationScopeNames.Command),
				groupAttr("backup"),
			),
			wantErr: true,
		},
		{
			name: "group name has no displayable character",
			value: contextGroupValue(
				scopeAttr(NotificationScopeNames.Group),
				groupAttr("\x01\x02"),
			),
			wantErr: true,
		},
		{
			name: "group name is only spaces",
			value: contextGroupValue(
				scopeAttr(NotificationScopeNames.Group),
				groupAttr("   "),
			),
			wantErr: true,
		},
		{
			name: "command name has no displayable character",
			value: contextGroupValue(
				scopeAttr(NotificationScopeNames.Command),
				groupAttr("backup"),
				commandAttr("\u202A\u202E"),
			),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DecodeNotificationContext(tt.value)
			if tt.wantErr {
				require.ErrorIs(t, err, ErrInvalidNotificationContext)
				assert.Equal(t, NotificationContext{}, got, "an invalid value must not produce a context")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
