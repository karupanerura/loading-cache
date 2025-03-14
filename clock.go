package loadingcache

import (
	"math/rand/v2"
	"time"
)

// Clock is an interface for getting the current time.
type Clock interface {
	Now() time.Time
}

// ClockFunc is a function type that implements the Clock interface.
type ClockFunc func() time.Time

// Now calls the function.
func (f ClockFunc) Now() time.Time {
	return f()
}

// SystemClock is the default clock that uses time.Now.
var SystemClock Clock = ClockFunc(time.Now)

// RandomizedClock is a clock that randomizes the current time.
// It returns the current time or the current time plus the duration.
type RandomizedClock struct {
	// Clock is the clock that provides the current time.
	Clock Clock

	// Duration is the additional duration to the current time.
	// The current time is randomized with the duration.
	Duration time.Duration

	// Percentage is the percentage of the randomization.
	// The current time is randomized with the percentage of Duration.
	// The percentage must be in the range of [0, 1].
	Percentage float64

	// Random is the random number generator.
	// If nil, it uses system default random generator.
	Random *rand.Rand
}

// Now returns the current time.
// If the random number is less than the percentage, it returns the current time plus the duration.
// Otherwise, it returns the current time.
func (r *RandomizedClock) Now() time.Time {
	if r.randFloat64() > r.Percentage {
		return r.Clock.Now()
	}
	return r.Clock.Now().Add(r.Duration)
}

func (r *RandomizedClock) randFloat64() float64 {
	if r.Random == nil {
		return rand.Float64()
	}
	return r.Random.Float64()
}
