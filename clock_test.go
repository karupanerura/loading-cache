package loadingcache_test

import (
	"math/rand/v2"
	"testing"
	"time"

	loadingcache "github.com/karupanerura/loading-cache"
)

func TestRandomizedClock_Now(t *testing.T) {
	t.Parallel()

	fixedTime := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)
	fixedClock := loadingcache.ClockFunc(func() time.Time {
		return fixedTime
	})
	duration := 10 * time.Minute

	t.Run("When percentage is 0, always returns original time", func(t *testing.T) {
		t.Parallel()

		clock := &loadingcache.RandomizedClock{
			Clock:      fixedClock,
			Duration:   duration,
			Percentage: 0,
		}

		// Call multiple times to ensure consistent behavior
		for i := 0; i < 100; i++ {
			result := clock.Now()
			if !result.Equal(fixedTime) {
				t.Errorf("Expected time %v, got %v", fixedTime, result)
			}
		}
	})

	t.Run("When percentage is 1, always returns randomized time", func(t *testing.T) {
		t.Parallel()

		// Use seeded random for deterministic tests
		r := rand.New(rand.NewPCG(42, 54))

		clock := &loadingcache.RandomizedClock{
			Clock:      fixedClock,
			Duration:   duration,
			Percentage: 1.0,
			Random:     r,
		}

		result := clock.Now()
		if !result.Equal(fixedTime.Add(duration)) {
			t.Errorf("Time %v must be after %v", result, duration)
		}
	})

	t.Run("With custom random source", func(t *testing.T) {
		t.Parallel()

		// Create two clocks with the same seed to get deterministic but different values
		r := rand.New(rand.NewPCG(42, 54))

		clock := &loadingcache.RandomizedClock{
			Clock:      fixedClock,
			Duration:   duration,
			Percentage: 1.0,
			Random:     r,
		}

		result := clock.Now()
		expected := fixedTime.Add(duration)

		if !result.Equal(expected) {
			t.Errorf("With seeded random, expected %v but got %v", expected, result)
		}
	})

	t.Run("With percentage between 0 and 1", func(t *testing.T) {
		t.Parallel()

		// Use 0.5 as percentage and count how many times we get original vs randomized
		clock := &loadingcache.RandomizedClock{
			Clock:      fixedClock,
			Duration:   duration,
			Percentage: 0.5,
			Random:     rand.New(rand.NewPCG(42, 54)),
		}

		originalCount := 0
		futureCount := 0
		iterations := 1000

		for i := 0; i < iterations; i++ {
			result := clock.Now()
			if result.Equal(fixedTime) {
				originalCount++
			} else {
				futureCount++

				// Verify the future time
				if !result.Equal(fixedTime.Add(duration)) {
					t.Errorf("Time %v must be after %v", result, duration)
				}
			}
		}

		// With enough iterations, we should see approximately 50% original and 50% randomized
		// Allow for some statistical variation
		tolerance := 0.1 * float64(iterations)
		expected := 0.5 * float64(iterations)

		if float64(originalCount) < expected-tolerance || float64(originalCount) > expected+tolerance {
			t.Errorf("Expected roughly %v original times, got %v out of %v iterations",
				expected, originalCount, iterations)
		}
	})
}
