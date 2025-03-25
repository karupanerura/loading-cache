package expiration

import (
	"math/rand/v2"
	"time"
)

// ExpirationPolicy is the interface for the expiration time checker.
// Implementations determine when cached values should be considered expired.
type ExpirationPolicy interface {
	// IsExpired returns true if the value is expired.
	// The now parameter represents the current time, and expiresAt is the value's expiration time.
	IsExpired(now, expiresAt time.Time) bool
}

// GeneralExpirationPolicy is a policy that expires a value at a specific time.
// It implements the standard time-based expiration check where a value is
// considered expired if the current time is after the expiration time.
type GeneralExpirationPolicy struct{}

var _ ExpirationPolicy = GeneralExpirationPolicy{}

// IsExpired returns true if the current time is after the specified expiration time.
// This is the standard expiration check: a value is expired when now >= expiresAt.
func (GeneralExpirationPolicy) IsExpired(now, expiresAt time.Time) bool {
	return !expiresAt.After(now)
}

// NeverExpirationPolicy is a policy that never expires a value.
// This is useful for permanent cache entries that should remain valid indefinitely.
type NeverExpirationPolicy struct{}

var _ ExpirationPolicy = NeverExpirationPolicy{}

// IsExpired always returns false, indicating that values never expire.
// This policy ignores the expiration time completely.
func (NeverExpirationPolicy) IsExpired(now, expiresAt time.Time) bool {
	return false
}

// EarlyExpirationPolicy is a policy that can expire a value before its actual expiration time.
// This policy is useful for preventing cache stampedes by introducing randomness in the
// expiration process, causing different cache clients to refresh their values at different times.
type EarlyExpirationPolicy struct {
	// Duration is how much earlier the value can expire.
	// For example, if set to 30 seconds, the value might expire up to 30 seconds
	// before its actual expiration time, depending on the Percentage.
	Duration time.Duration

	// Percentage is the chance (between 0 and 1) that the value will expire early.
	// A value of 0 means never expire early, while 1 means always expire early.
	// For example, 0.5 means there's a 50% chance of early expiration.
	Percentage float64

	// Random is the random number generator to decide early expiration.
	// If not set, the default system random generator is used.
	// This can be set to a specific random generator for deterministic behavior in tests.
	Random *rand.Rand
}

var _ ExpirationPolicy = (*EarlyExpirationPolicy)(nil)

// IsExpired checks if the value is expired.
// This method has two behaviors:
// 1. With probability (1-Percentage): behaves like GeneralExpirationPolicy, checking if now > expiresAt
// 2. With probability Percentage: checks if (now + Duration) > expiresAt, causing early expiration
//
// By using this policy, different cache clients will likely refresh their caches at
// different times, preventing multiple simultaneous refresh operations (thundering hard).
func (p *EarlyExpirationPolicy) IsExpired(now, expiresAt time.Time) bool {
	if p.randFloat64() > p.Percentage {
		return now.After(expiresAt)
	}
	return now.Add(p.Duration).After(expiresAt)
}

func (p *EarlyExpirationPolicy) randFloat64() float64 {
	if p.Random == nil {
		return rand.Float64()
	}
	return p.Random.Float64()
}
