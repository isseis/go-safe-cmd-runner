package common

import (
	"testing"
)

func TestParseEnvVariable(t *testing.T) {
	tests := []struct {
		name        string
		env         string
		expectedKey string
		expectedVal string
		expectedOk  bool
	}{
		{
			name:        "valid environment variable",
			env:         "PATH=/usr/bin:/bin",
			expectedKey: "PATH",
			expectedVal: "/usr/bin:/bin",
			expectedOk:  true,
		},
		{
			name:        "variable with empty value",
			env:         "EMPTY=",
			expectedKey: "EMPTY",
			expectedVal: "",
			expectedOk:  true,
		},
		{
			name:        "variable with equals in value",
			env:         "CONFIG=key=value",
			expectedKey: "CONFIG",
			expectedVal: "key=value",
			expectedOk:  true,
		},
		{
			name:        "missing equals sign",
			env:         "INVALID",
			expectedKey: "",
			expectedVal: "",
			expectedOk:  false,
		},
		{
			name:        "empty string",
			env:         "",
			expectedKey: "",
			expectedVal: "",
			expectedOk:  false,
		},
		{
			name:        "only equals sign",
			env:         "=",
			expectedKey: "",
			expectedVal: "",
			expectedOk:  false,
		},
		{
			name:        "equals sign with value but empty key",
			env:         "=somevalue",
			expectedKey: "",
			expectedVal: "",
			expectedOk:  false,
		},
		{
			name:        "multiple equals signs",
			env:         "DB_URL=postgres://user:pass=secret@host/db",
			expectedKey: "DB_URL",
			expectedVal: "postgres://user:pass=secret@host/db",
			expectedOk:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, val, ok := ParseKeyValue(tt.env)

			if key != tt.expectedKey {
				t.Errorf("ParseEnvVariable() key = %v, expected %v", key, tt.expectedKey)
			}
			if val != tt.expectedVal {
				t.Errorf("ParseEnvVariable() val = %v, expected %v", val, tt.expectedVal)
			}
			if ok != tt.expectedOk {
				t.Errorf("ParseEnvVariable() ok = %v, expected %v", ok, tt.expectedOk)
			}
		})
	}
}
