package common

import "testing"

func TestContainsPathTraversalSegment(t *testing.T) {
	tests := []struct {
		name string
		path string
		want bool
	}{
		{"empty", "", false},
		{"single traversal", "..", true},
		{"relative traversal", "../etc/passwd", true},
		{"nested traversal", "a/b/../c", true},
		{"no traversal", "a/b/c.txt", false},
		{"dots in filename", "archive..zip", false},
		{"dots in segment", "a..b/c", false},
		{"leading dotfile", ".hidden/file", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ContainsPathTraversalSegment(tt.path)
			if got != tt.want {
				t.Fatalf("ContainsPathTraversalSegment(%q) = %v; want %v", tt.path, got, tt.want)
			}
		})
	}
}
