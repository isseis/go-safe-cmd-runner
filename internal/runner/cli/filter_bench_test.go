package cli

import (
	"fmt"
	"strings"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

func BenchmarkParseGroupNames(b *testing.B) {
	input := strings.Repeat("group,", 5) + "group"
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		ParseGroupNames(input)
	}
}

func BenchmarkValidateGroupNames(b *testing.B) {
	names := []string{"build", "test", "deploy", "verify", "cleanup"}
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = ValidateGroupNames(names)
	}
}

func BenchmarkFilterGroups(b *testing.B) {
	config := &runnertypes.ConfigSpec{
		Groups: make([]runnertypes.GroupSpec, 10),
	}
	for i := range config.Groups {
		config.Groups[i] = runnertypes.GroupSpec{Name: fmt.Sprintf("group%d", i)}
	}

	target := []string{"group1", "group5", "group9"}
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		FilterGroups(target, config)
	}
}
