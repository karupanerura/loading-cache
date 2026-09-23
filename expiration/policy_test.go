package expiration_test

import (
	"math"
	"math/rand/v2"
	"sync"
	"testing"
	"time"

	"github.com/karupanerura/loading-cache/expiration"
)

func TestGeneralExpirationPolicy(t *testing.T) {
	t.Parallel()

	policy := expiration.GeneralExpirationPolicy{}
	now := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		expiresAt time.Time
		want      bool
	}{
		{
			name:      "not expired when expiry is in future",
			expiresAt: now.Add(1),
			want:      false,
		},
		{
			name:      "expired when expiry is exactly now",
			expiresAt: now,
			want:      true,
		},
		{
			name:      "expired when expiry is in past",
			expiresAt: now.Add(-1),
			want:      true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := policy.IsExpired(now, tt.expiresAt); got != tt.want {
				t.Errorf("GeneralExpirationPolicy.IsExpired() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNeverExpirationPolicy(t *testing.T) {
	t.Parallel()

	policy := expiration.NeverExpirationPolicy{}
	now := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		expiresAt time.Time
	}{
		{
			name:      "not expired when expiry is in future",
			expiresAt: now.Add(1),
		},
		{
			name:      "not expired when expiry is exactly now",
			expiresAt: now,
		},
		{
			name:      "not expired even when expiry is in past",
			expiresAt: now.Add(-1),
		},
		{
			name:      "not expired even when expiry is far in past",
			expiresAt: now.Add(-1000 * time.Hour),
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := policy.IsExpired(now, tt.expiresAt); got != false {
				t.Errorf("NeverExpirationPolicy.IsExpired() = %v, want false", got)
			}
		})
	}
}

func TestEarlyExpirationPolicy(t *testing.T) {
	t.Parallel()

	now := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)
	earlyDuration := 10 * time.Minute

	t.Run("use default random generator", func(t *testing.T) {
		t.Parallel()

		policy := &expiration.EarlyExpirationPolicy{
			Duration:   earlyDuration,
			Percentage: 0.5,
		}

		// Can't test random behavior deterministically, so just call to ensure no panic
		policy.IsExpired(now, now.Add(5*time.Minute))
	})

	t.Run("random above percentage threshold - behave like general policy", func(t *testing.T) {
		t.Parallel()

		random := rand.New(rand.NewPCG(1, 2)) // deterministic random generator
		policy := &expiration.EarlyExpirationPolicy{
			Duration:   earlyDuration,
			Percentage: 0.3,
			Random:     random,
		}

		// Should behave like general expiration policy
		if policy.IsExpired(now, now.Add(5*time.Minute)) {
			t.Error("Should not be expired when random > percentage and expiry is in future")
		}

		if !policy.IsExpired(now, now.Add(-5*time.Minute)) {
			t.Error("Should be expired when random > percentage and expiry is in past")
		}
	})

	t.Run("random below percentage threshold - apply early expiration", func(t *testing.T) {
		t.Parallel()

		random := rand.New(rand.NewPCG(1, 2))
		policy := &expiration.EarlyExpirationPolicy{
			Duration:   earlyDuration,
			Percentage: 0.8,
			Random:     random,
		}

		// Should apply early expiration
		// Example: now = 12:00, expiry = 12:15, early duration = 10 min
		// When applying early expiration: now + 10min = 12:10, which is before expiry
		if policy.IsExpired(now, now.Add(15*time.Minute)) {
			t.Error("Should not be expired when expiry is beyond early window")
		}

		// Now + 10min = 12:10, which is after expiry at 12:05
		if !policy.IsExpired(now, now.Add(5*time.Minute)) {
			t.Error("Should be expired when expiry falls within early window")
		}
	})

	t.Run("edge cases", func(t *testing.T) {
		t.Parallel()

		mockRand := rand.New(rand.NewPCG(1, 2))
		policy := &expiration.EarlyExpirationPolicy{
			Duration:   earlyDuration,
			Percentage: 0.5,
			Random:     mockRand,
		}

		// Test with percentage = 0 (never early expire)
		policy.Percentage = 0
		if policy.IsExpired(now, now.Add(5*time.Minute)) {
			t.Error("With 0% chance, should never apply early expiration")
		}

		// Test with percentage = 1 (always early expire)
		policy.Percentage = 1
		if !policy.IsExpired(now, now.Add(9*time.Minute)) {
			t.Error("With 100% chance, should always apply early expiration")
		}

		// Test with zero duration: expired at expiresAt, like the general policy
		policy.Duration = 0
		policy.Percentage = 1
		if !policy.IsExpired(now, now) {
			t.Error("With zero early duration, should behave like general policy at the boundary")
		}
	})
}

// TestEarlyExpirationPolicy_ConcurrentIsExpired verifies that a policy with a
// user-provided random generator is safe to use from multiple goroutines.
// Cache storages call IsExpired concurrently, but *rand.Rand is not
// goroutine-safe by itself, so the policy must synchronize access to it.
// This test is effective when run with the race detector enabled.
func TestEarlyExpirationPolicy_ConcurrentIsExpired(t *testing.T) {
	t.Parallel()

	policy := &expiration.EarlyExpirationPolicy{
		Duration:   30 * time.Second,
		Percentage: 0.5,
		Random:     rand.New(rand.NewPCG(1, 2)),
	}

	now := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)
	expiresAt := now.Add(time.Minute)

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 1000 {
				policy.IsExpired(now, expiresAt)
			}
		}()
	}
	wg.Wait()
}

// constSource is a rand.Source that always returns the same value,
// which makes the branch chosen by EarlyExpirationPolicy deterministic.
type constSource uint64

func (s constSource) Uint64() uint64 { return uint64(s) }

func TestEarlyExpirationPolicy_Boundary(t *testing.T) {
	t.Parallel()

	now := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)
	const earlyDuration = 10 * time.Minute

	tests := []struct {
		name      string
		random    rand.Source
		duration  time.Duration
		deadline  time.Time // the time at which the entry becomes expired
		wantEarly bool
	}{
		{
			// Float64() is almost 1, which is above Percentage.
			name:     "normal branch",
			random:   constSource(math.MaxUint64),
			duration: earlyDuration,
			deadline: now,
		},
		{
			// Float64() is 0, which is not above Percentage.
			name:     "early branch",
			random:   constSource(0),
			duration: earlyDuration,
			deadline: now.Add(earlyDuration),
		},
		{
			name:     "early branch with zero duration",
			random:   constSource(0),
			duration: 0,
			deadline: now,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			policy := &expiration.EarlyExpirationPolicy{
				Duration:   tt.duration,
				Percentage: 0.5,
				Random:     rand.New(tt.random),
			}
			general := expiration.GeneralExpirationPolicy{}
			for _, c := range []struct {
				label     string
				expiresAt time.Time
				want      bool
			}{
				{"before the boundary", tt.deadline.Add(time.Nanosecond), false},
				{"at the boundary", tt.deadline, true},
				{"after the boundary", tt.deadline.Add(-time.Nanosecond), true},
			} {
				if got := policy.IsExpired(now, c.expiresAt); got != c.want {
					t.Errorf("%s: IsExpired(now, %v) = %v, want %v", c.label, c.expiresAt, got, c.want)
				}
				// The same boundary as GeneralExpirationPolicy, shifted by Duration in the early branch.
				shiftedNow := now.Add(tt.deadline.Sub(now))
				if got := general.IsExpired(shiftedNow, c.expiresAt); got != c.want {
					t.Errorf("%s: general policy disagrees: got %v, want %v", c.label, got, c.want)
				}
			}
		})
	}
}
