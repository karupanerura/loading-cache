package expiration_test

import (
	"math/rand/v2"
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

		// Test with zero duration
		policy.Duration = 0
		policy.Percentage = 1
		if policy.IsExpired(now, now) {
			t.Error("With zero early duration, should behave like general policy")
		}
	})
}
