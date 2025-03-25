package loadingcache

import (
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
