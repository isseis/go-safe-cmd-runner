//go:build test

package config

import (
	"errors"
	"testing"
)

func TestParsePlaceholders(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []placeholder
		wantErr  bool
		errType  error
	}{
		{
			name:  "required parameter",
			input: "${path}",
			expected: []placeholder{
				{fullMatch: "${path}", name: "path", ptype: placeholderRequired, start: 0, end: 7},
			},
		},
		{
			name:  "optional parameter",
			input: "${?verbose}",
			expected: []placeholder{
				{fullMatch: "${?verbose}", name: "verbose", ptype: placeholderOptional, start: 0, end: 11},
			},
		},
		{
			name:  "array parameter",
			input: "${@flags}",
			expected: []placeholder{
				{fullMatch: "${@flags}", name: "flags", ptype: placeholderArray, start: 0, end: 9},
			},
		},
		{
			name:     "escaped dollar",
			input:    "\\$100",
			expected: []placeholder{},
		},
		{
			name:     "escaped backslash",
			input:    "C:\\\\path",
			expected: []placeholder{},
		},
		{
			name:  "multiple placeholders",
			input: "${@flags} backup ${path}",
			expected: []placeholder{
				{fullMatch: "${@flags}", name: "flags", ptype: placeholderArray, start: 0, end: 9},
				{fullMatch: "${path}", name: "path", ptype: placeholderRequired, start: 17, end: 24},
			},
		},
		{
			name:  "placeholder in middle",
			input: "prefix${param}suffix",
			expected: []placeholder{
				{fullMatch: "${param}", name: "param", ptype: placeholderRequired, start: 6, end: 14},
			},
		},
		{
			name:     "no placeholders",
			input:    "just plain text",
			expected: []placeholder{},
		},
		{
			name:  "placeholder with underscore",
			input: "${_private_var}",
			expected: []placeholder{
				{fullMatch: "${_private_var}", name: "_private_var", ptype: placeholderRequired, start: 0, end: 15},
			},
		},
		{
			name:  "placeholder with numbers",
			input: "${var123}",
			expected: []placeholder{
				{fullMatch: "${var123}", name: "var123", ptype: placeholderRequired, start: 0, end: 9},
			},
		},
		// Error cases
		{
			name:    "unclosed placeholder",
			input:   "${path",
			wantErr: true,
			errType: &ErrUnclosedPlaceholder{},
		},
		{
			name:    "empty placeholder",
			input:   "${}",
			wantErr: true,
			errType: &ErrEmptyPlaceholder{},
		},
		{
			name:    "empty optional placeholder name",
			input:   "${?}",
			wantErr: true,
			errType: &ErrEmptyPlaceholderName{},
		},
		{
			name:    "empty array placeholder name",
			input:   "${@}",
			wantErr: true,
			errType: &ErrEmptyPlaceholderName{},
		},
		{
			name:    "invalid placeholder name - starts with number",
			input:   "${123invalid}",
			wantErr: true,
			errType: &ErrInvalidPlaceholderName{},
		},
		{
			name:    "invalid placeholder name - contains hyphen",
			input:   "${my-var}",
			wantErr: true,
			errType: &ErrInvalidPlaceholderName{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parsePlaceholders(tt.input)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				// Check error type using type switch
				switch tt.errType.(type) {
				case *ErrUnclosedPlaceholder:
					var target *ErrUnclosedPlaceholder
					if !errors.As(err, &target) {
						t.Errorf("expected ErrUnclosedPlaceholder, got %T: %v", err, err)
					}
				case *ErrEmptyPlaceholder:
					var target *ErrEmptyPlaceholder
					if !errors.As(err, &target) {
						t.Errorf("expected ErrEmptyPlaceholder, got %T: %v", err, err)
					}
				case *ErrEmptyPlaceholderName:
					var target *ErrEmptyPlaceholderName
					if !errors.As(err, &target) {
						t.Errorf("expected ErrEmptyPlaceholderName, got %T: %v", err, err)
					}
				case *ErrInvalidPlaceholderName:
					var target *ErrInvalidPlaceholderName
					if !errors.As(err, &target) {
						t.Errorf("expected ErrInvalidPlaceholderName, got %T: %v", err, err)
					}
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(result) != len(tt.expected) {
				t.Fatalf("expected %d placeholders, got %d", len(tt.expected), len(result))
			}

			for i, exp := range tt.expected {
				got := result[i]
				if got.fullMatch != exp.fullMatch {
					t.Errorf("placeholder[%d] fullMatch: expected %q, got %q", i, exp.fullMatch, got.fullMatch)
				}
				if got.name != exp.name {
					t.Errorf("placeholder[%d] name: expected %q, got %q", i, exp.name, got.name)
				}
				if got.ptype != exp.ptype {
					t.Errorf("placeholder[%d] ptype: expected %v, got %v", i, exp.ptype, got.ptype)
				}
				if got.start != exp.start {
					t.Errorf("placeholder[%d] start: expected %d, got %d", i, exp.start, got.start)
				}
				if got.end != exp.end {
					t.Errorf("placeholder[%d] end: expected %d, got %d", i, exp.end, got.end)
				}
			}
		})
	}
}

func TestApplyEscapeSequences(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "dollar escape",
			input:    "\\$100",
			expected: "$100",
		},
		{
			name:     "backslash escape",
			input:    "C:\\\\path",
			expected: "C:\\path",
		},
		{
			name:     "no escape",
			input:    "normal text",
			expected: "normal text",
		},
		{
			name:     "multiple dollar escapes",
			input:    "\\$100 and \\$200",
			expected: "$100 and $200",
		},
		{
			name:     "multiple backslash escapes",
			input:    "C:\\\\path\\\\file.txt",
			expected: "C:\\path\\file.txt",
		},
		{
			name:     "mixed escapes",
			input:    "\\$100 in C:\\\\folder",
			expected: "$100 in C:\\folder",
		},
		{
			name:     "backslash not followed by escapable char",
			input:    "\\n\\t\\r",
			expected: "\\n\\t\\r",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "single backslash at end",
			input:    "text\\",
			expected: "text\\",
		},
		{
			name:     "backslash before regular char",
			input:    "\\a\\b\\c",
			expected: "\\a\\b\\c",
		},
		{
			name:     "double backslash at end",
			input:    "text\\\\",
			expected: "text\\",
		},
		{
			name:     "escaped dollar in placeholder-like context",
			input:    "\\${not_a_placeholder}",
			expected: "${not_a_placeholder}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := applyEscapeSequences(tt.input)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestParsePlaceholders_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []placeholder
		wantErr  bool
	}{
		{
			name:  "escape sequence followed by placeholder",
			input: "\\$${param}",
			expected: []placeholder{
				{fullMatch: "${param}", name: "param", ptype: placeholderRequired, start: 2, end: 10},
			},
		},
		{
			name:  "placeholder after escaped backslash",
			input: "\\\\${param}",
			expected: []placeholder{
				{fullMatch: "${param}", name: "param", ptype: placeholderRequired, start: 2, end: 10},
			},
		},
		{
			name:  "consecutive placeholders",
			input: "${a}${b}${c}",
			expected: []placeholder{
				{fullMatch: "${a}", name: "a", ptype: placeholderRequired, start: 0, end: 4},
				{fullMatch: "${b}", name: "b", ptype: placeholderRequired, start: 4, end: 8},
				{fullMatch: "${c}", name: "c", ptype: placeholderRequired, start: 8, end: 12},
			},
		},
		{
			name:  "all placeholder types",
			input: "${req}${?opt}${@arr}",
			expected: []placeholder{
				{fullMatch: "${req}", name: "req", ptype: placeholderRequired, start: 0, end: 6},
				{fullMatch: "${?opt}", name: "opt", ptype: placeholderOptional, start: 6, end: 13},
				{fullMatch: "${@arr}", name: "arr", ptype: placeholderArray, start: 13, end: 20},
			},
		},
		{
			name:     "dollar without brace",
			input:    "$100",
			expected: []placeholder{},
		},
		{
			name:     "dollar at end",
			input:    "text$",
			expected: []placeholder{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parsePlaceholders(tt.input)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(result) != len(tt.expected) {
				t.Fatalf("expected %d placeholders, got %d", len(tt.expected), len(result))
			}

			for i, exp := range tt.expected {
				got := result[i]
				if got.fullMatch != exp.fullMatch {
					t.Errorf("placeholder[%d] fullMatch: expected %q, got %q", i, exp.fullMatch, got.fullMatch)
				}
				if got.name != exp.name {
					t.Errorf("placeholder[%d] name: expected %q, got %q", i, exp.name, got.name)
				}
				if got.ptype != exp.ptype {
					t.Errorf("placeholder[%d] ptype: expected %v, got %v", i, exp.ptype, got.ptype)
				}
				if got.start != exp.start {
					t.Errorf("placeholder[%d] start: expected %d, got %d", i, exp.start, got.start)
				}
				if got.end != exp.end {
					t.Errorf("placeholder[%d] end: expected %d, got %d", i, exp.end, got.end)
				}
			}
		})
	}
}
