package redaction

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// uniqueCachePattern returns a regex pattern that no other test in this package
// compiles, so a test can reason about the cache state for that pattern
// deterministically regardless of what earlier tests cached.
func uniqueCachePattern(suffix string) string {
	return `(?i)(__regex_cache_test_` + suffix + `__)(=)(\S+)`
}

// TestRegexCache_ReturnsSameCompiledRegex verifies that repeated compilation of
// the same pattern string returns the same cached *regexp.Regexp.
func TestRegexCache_ReturnsSameCompiledRegex(t *testing.T) {
	pattern := uniqueCachePattern("same_pointer")

	first := compileRedactionRegex(pattern, nil)
	require.NotNil(t, first)

	second := compileRedactionRegex(pattern, nil)
	require.NotNil(t, second)

	assert.Same(t, first, second, "a repeated pattern must be served from the cache")
}

// TestRegexCache_LimitStopsCaching verifies that once the cache has reached its
// entry limit, new patterns are compiled on every call (never cached) yet still
// redact correctly, while patterns cached before the limit continue to be
// served by identity. The limit is simulated by raising the entry counter
// directly instead of enqueueing maxRegexCacheEntries distinct patterns.
func TestRegexCache_LimitStopsCaching(t *testing.T) {
	preCached := uniqueCachePattern("pre_cached")
	preCachedRe := compileRedactionRegex(preCached, nil)
	require.NotNil(t, preCachedRe)

	savedCount := regexCacheCount.Load()
	regexCacheCount.Store(maxRegexCacheEntries)
	t.Cleanup(func() { regexCacheCount.Store(savedCount) })

	assert.Same(t, preCachedRe, compileRedactionRegex(preCached, nil),
		"a pattern cached before the limit must still be served from the cache")

	const marker = "__regex_cache_test_limit__"
	pattern := `(?i)(` + marker + `)(=)(\S+)`

	first := compileRedactionRegex(pattern, nil)
	require.NotNil(t, first)

	second := compileRedactionRegex(pattern, nil)
	require.NotNil(t, second)

	assert.NotSame(t, first, second, "patterns beyond the limit must be compiled on every call")

	config := &Config{
		Placeholder:      "[REDACTED]",
		KeyValuePatterns: []KeyValuePattern{{Literal: marker, Kind: PatternKindKeyedValue}},
	}
	assert.Equal(t, marker+"=[REDACTED]", config.RedactText(marker+"=secret123"),
		"an uncached pattern must still redact correctly through the production path")
}

// TestRegexCache_CompileFailureIsNotCached verifies the fail-secure path: an
// uncompilable pattern returns nil, the failure is not cached (a later call
// still runs through the same compile path and the entry count does not grow),
// and the failure does not pollute the cache for other patterns of the same
// Config. The failure path is exercised by calling compileRedactionRegex
// directly with an invalid pattern, because all three RedactText routes build
// their regex from regexp.QuoteMeta-escaped fragments, so no KeyValuePatterns
// entry can produce an uncompilable pattern.
func TestRegexCache_CompileFailureIsNotCached(t *testing.T) {
	const badPattern = `(?i)(` // unterminated group, always a compile error

	countBefore := regexCacheCount.Load()

	assert.Nil(t, compileRedactionRegex(badPattern, nil), "fail-secure: nil instead of a partial regex")

	if _, ok := regexCache.Load(badPattern); ok {
		t.Fatal("a failed compilation must not be stored in the cache")
	}
	assert.Nil(t, compileRedactionRegex(badPattern, nil), "a second call still takes the compile-failure path")
	assert.Equal(t, countBefore, regexCacheCount.Load(), "failed compilations must not grow the entry count")

	config := DefaultConfig()
	assert.Equal(t, "password=[REDACTED]", config.RedactText("password=secret123"),
		"an unrelated key must be unaffected by the failed compilation")
}

// TestRegexCache_ConcurrentAccess verifies that RedactText remains correct when
// many goroutines compile and use patterns at once. The first phase runs all
// goroutines on the same input, exercising concurrent LoadOrStore of the same
// patterns; the second phase gives each goroutine its own key, exercising
// concurrent insertion of distinct patterns and the entry-count increments that
// come with them. Both phases run under -race by the test runner.
func TestRegexCache_ConcurrentAccess(t *testing.T) {
	config := DefaultConfig()
	const input = "password=secret123 token=abc123"
	want := config.RedactText(input)

	const goroutines = 32
	results := make([]string, goroutines)
	var wg sync.WaitGroup
	for i := range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = config.RedactText(input)
		}()
	}
	wg.Wait()

	for i, got := range results {
		assert.Equal(t, want, got, "goroutine %d must produce the single-threaded result", i)
	}

	const distinctGoroutines = 16
	distinctResults := make([]string, distinctGoroutines)
	var distinctWG sync.WaitGroup
	for i := range distinctGoroutines {
		key := fmt.Sprintf("__regex_cache_test_distinct_%d__", i)
		cfg := &Config{
			Placeholder:      "[REDACTED]",
			KeyValuePatterns: []KeyValuePattern{{Literal: key, Kind: PatternKindKeyedValue}},
		}
		distinctWG.Add(1)
		go func() {
			defer distinctWG.Done()
			distinctResults[i] = cfg.RedactText(key + "=secret123")
		}()
	}
	distinctWG.Wait()

	for i, got := range distinctResults {
		assert.Equal(t, fmt.Sprintf("__regex_cache_test_distinct_%d__=[REDACTED]", i), got,
			"distinct-pattern goroutine %d must redact its own input correctly", i)
	}
}
