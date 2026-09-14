package identifier

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIdentifier(t *testing.T) {
	tests := []struct {
		name       string
		identifier Identifier
		want       string
	}{
		{name: "named identifier", identifier: NewIdentifier("rotate_api_key"), want: "rotate_api_key"},
		{name: "empty name", identifier: NewIdentifier(""), want: ""},
		{name: "zero value", identifier: Identifier{}, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.identifier.Name())
			assert.Equal(t, tt.want, tt.identifier.String())

			value := tt.identifier.LogValue()
			assert.Equal(t, slog.KindString, value.Kind())
			assert.Equal(t, tt.want, value.String())

			attrValue := slog.Any("command", tt.identifier).Value
			assert.Equal(t, slog.KindLogValuer, attrValue.Kind())
			assert.Equal(t, tt.want, attrValue.String())
		})
	}
}
