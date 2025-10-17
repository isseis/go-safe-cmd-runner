//go:build test

package variable

// NewAutoVarProviderWithClock creates a new AutoVarProvider with the specified clock.
func NewAutoVarProviderWithClock(clock Clock) AutoVarProvider {
	return &autoVarProvider{
		clock: clock,
	}
}
