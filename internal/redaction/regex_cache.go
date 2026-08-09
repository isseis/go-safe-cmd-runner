package redaction

import (
	"sync"
	"sync/atomic"
)

// maxRegexCacheEntries is the upper bound on the number of compiled patterns
// held by regexCache. Each key in KeyValuePatterns is routed to exactly one of
// the three compile routes (colon, space, or key-value pattern), so the default
// configuration compiles 12 patterns in the steady state (12 keys × one route
// each). Extending KeyValuePatterns via configuration adds exactly 1 entry per
// additional key, so 256 leaves more than 20x headroom over the default. The
// cache never evicts: once the limit is reached, patterns that have not been
// seen before are compiled on every call, matching the behavior before the
// cache existed.
const maxRegexCacheEntries = 256

// regexCache stores compiled redaction patterns keyed by the regex string they
// were compiled from. compileRedactionRegex is the single compilation point for
// all three key-value routes, so every route benefits. RedactText may be called
// concurrently from multiple goroutines; sync.Map makes the cache safe under
// that concurrency, and the cached *regexp.Regexp values are themselves safe
// for concurrent use.
var regexCache sync.Map

// regexCacheCount is the number of entries currently held by regexCache. It is
// incremented only by a goroutine whose LoadOrStore actually inserted a new
// entry, so concurrent callers of the same pattern do not double-count it.
var regexCacheCount atomic.Int64
