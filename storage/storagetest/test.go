// storagetest package provides generic test cases for cache storage implementations.
package storagetest

import (
	"fmt"
	"math/rand/v2"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	loadingcache "github.com/karupanerura/loading-cache"
	"golang.org/x/sync/errgroup"
)

// BenchmarkSet benchmarks the Set method of the cache storage.
func BenchmarkSet[K loadingcache.KeyConstraint, V loadingcache.ValueConstraint](b *testing.B, storage loadingcache.CacheStorage[K, V], keys []K) {
	var zero V
	expiresAt := time.Now().Add(time.Hour)
	ctx := b.Context()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		storage.Set(ctx, &loadingcache.CacheEntry[K, V]{
			Entry:     loadingcache.Entry[K, V]{Key: keys[i%len(keys)], Value: zero},
			ExpiresAt: expiresAt,
		})
	}
}

type TestClonerStruct struct {
	value int8
}

func (s *TestClonerStruct) Clone() *TestClonerStruct {
	return &TestClonerStruct{value: s.value}
}

// TestCloneStruct tests the cloning behavior of the cache storage.
func TestCloneStruct(t *testing.T, provider func() (loadingcache.CacheStorage[uint8, *TestClonerStruct], func())) {
	t.Run("CloneStruct", func(t *testing.T) {
		t.Parallel()

		storage, release := provider()
		defer release()

		original := &loadingcache.CacheEntry[uint8, *TestClonerStruct]{
			Entry: loadingcache.Entry[uint8, *TestClonerStruct]{
				Key:   1,
				Value: &TestClonerStruct{value: 1},
			},
			ExpiresAt: time.Now().Add(time.Hour),
		}
		if err := storage.Set(t.Context(), original); err != nil {
			t.Fatal(err)
		}

		got, err := storage.Get(t.Context(), 1)
		if err != nil {
			t.Fatal(err)
		}

		if original == got || original.Value == got.Value {
			t.Error("struct must be cloned, but got same that")
		}
		if df := cmp.Diff(original, got, cmp.AllowUnexported(TestClonerStruct{})); df != "" {
			t.Errorf("struct diff=%s", df)
		}

		before := got
		got, err = storage.Get(t.Context(), 1)
		if err != nil {
			t.Fatal(err)
		}
		if before == got || before.Value == got.Value {
			t.Error("struct must be cloned, but got same that")
		}
		if df := cmp.Diff(before, got, cmp.AllowUnexported(TestClonerStruct{})); df != "" {
			t.Errorf("struct diff=%s", df)
		}
	})
}

type TestDeepCopyerStruct struct {
	value int8
}

func (s *TestDeepCopyerStruct) DeepCopy() *TestDeepCopyerStruct {
	return &TestDeepCopyerStruct{value: s.value}
}

func TestDeepCopyStruct(t *testing.T, provider func() (loadingcache.CacheStorage[uint8, *TestDeepCopyerStruct], func())) {
	t.Run("DeepCopyStruct", func(t *testing.T) {
		t.Parallel()

		storage, release := provider()
		defer release()

		original := &loadingcache.CacheEntry[uint8, *TestDeepCopyerStruct]{
			Entry: loadingcache.Entry[uint8, *TestDeepCopyerStruct]{
				Key:   1,
				Value: &TestDeepCopyerStruct{value: 1},
			},
			ExpiresAt: time.Now().Add(time.Hour),
		}
		if err := storage.Set(t.Context(), original); err != nil {
			t.Fatal(err)
		}

		got, err := storage.Get(t.Context(), 1)
		if err != nil {
			t.Fatal(err)
		}

		if original == got || original.Value == got.Value {
			t.Error("struct must be cloned, but got same that")
		}
		if df := cmp.Diff(original, got, cmp.AllowUnexported(TestDeepCopyerStruct{})); df != "" {
			t.Errorf("struct diff=%s", df)
		}

		before := got
		got, err = storage.Get(t.Context(), 1)
		if err != nil {
			t.Fatal(err)
		}
		if before == got || before.Value == got.Value {
			t.Error("struct must be cloned, but got same that")
		}
		if df := cmp.Diff(before, got, cmp.AllowUnexported(TestDeepCopyerStruct{})); df != "" {
			t.Errorf("struct diff=%s", df)
		}
	})
}

func TestConsistency(t *testing.T, provider func() (loadingcache.CacheStorage[uint8, int8], func())) {
	t.Run("Consistency", func(t *testing.T) {
		t.Parallel()

		t.Run("SetAndGet", func(t *testing.T) {
			t.Parallel()

			storage, release := provider()
			defer release()

			expiresAt := time.Now().Add(time.Hour)
			patterns := []loadingcache.Entry[uint8, int8]{
				{0, 1},
				{1, 2},
				{2, 3},
				{3, 4},
				{4, 5},
				{251, 124},
				{252, 125},
				{253, 126},
				{254, 127},
				{255, -128},
			}
			rand.Shuffle(len(patterns), func(i, j int) {
				patterns[i], patterns[j] = patterns[j], patterns[i]
			})
			var eg errgroup.Group
			for _, pattern := range patterns {
				pattern := pattern
				eg.Go(func() error {
					entry, err := storage.Get(t.Context(), pattern.Key)
					if err != nil {
						return err
					} else if entry != nil {
						return fmt.Errorf("unexpected exists value for key %d", pattern.Key)
					}
					return nil
				})
			}
			if err := eg.Wait(); err != nil {
				t.Fatal(err)
			}

			eg = errgroup.Group{}
			for _, pattern := range patterns {
				pattern := pattern
				eg.Go(func() error {
					return storage.Set(t.Context(), &loadingcache.CacheEntry[uint8, int8]{
						Entry:     pattern,
						ExpiresAt: expiresAt,
					})
				})
			}
			if err := eg.Wait(); err != nil {
				t.Fatal(err)
			}

			eg = errgroup.Group{}
			entries := make([]*loadingcache.CacheEntry[uint8, int8], len(patterns))
			for i, pattern := range patterns {
				i := i
				pattern := pattern
				eg.Go(func() error {
					entry, err := storage.Get(t.Context(), pattern.Key)
					if err != nil {
						return err
					}
					entries[i] = entry
					return nil
				})
			}
			if err := eg.Wait(); err != nil {
				t.Fatal(err)
			}

			for i, pattern := range patterns {
				if df := cmp.Diff(pattern, entries[i].Entry); df != "" {
					t.Errorf("pattern[%d] key=%d entry diff=%s", i, pattern.Key, df)
				}
			}
		})
	})

	t.Run("SetMultiAndGetMulti", func(t *testing.T) {
		t.Parallel()

		storage, release := provider()
		defer release()

		expiresAt := time.Now().Add(time.Hour)
		patterns := []struct {
			pairs []*loadingcache.CacheEntry[uint8, int8]
		}{
			{
				[]*loadingcache.CacheEntry[uint8, int8]{
					{Entry: loadingcache.Entry[uint8, int8]{Key: 0, Value: 1}, ExpiresAt: expiresAt},
				},
			},
			{
				[]*loadingcache.CacheEntry[uint8, int8]{
					{Entry: loadingcache.Entry[uint8, int8]{Key: 1, Value: 2}, ExpiresAt: expiresAt},
					{Entry: loadingcache.Entry[uint8, int8]{Key: 2, Value: 3}, ExpiresAt: expiresAt},
				},
			},
			{
				[]*loadingcache.CacheEntry[uint8, int8]{
					{Entry: loadingcache.Entry[uint8, int8]{Key: 4, Value: 5}, ExpiresAt: expiresAt},
					{Entry: loadingcache.Entry[uint8, int8]{Key: 5, Value: 6}, ExpiresAt: expiresAt},
					{Entry: loadingcache.Entry[uint8, int8]{Key: 6, Value: 7}, ExpiresAt: expiresAt},
				},
			},
			{
				[]*loadingcache.CacheEntry[uint8, int8]{
					{Entry: loadingcache.Entry[uint8, int8]{Key: 7, Value: 8}, ExpiresAt: expiresAt},
					{Entry: loadingcache.Entry[uint8, int8]{Key: 8, Value: 9}, ExpiresAt: expiresAt},
					{Entry: loadingcache.Entry[uint8, int8]{Key: 9, Value: 10}, ExpiresAt: expiresAt},
					{Entry: loadingcache.Entry[uint8, int8]{Key: 10, Value: 11}, ExpiresAt: expiresAt},
				},
			},
			{
				[]*loadingcache.CacheEntry[uint8, int8]{
					{Entry: loadingcache.Entry[uint8, int8]{Key: 251, Value: 124}, ExpiresAt: expiresAt},
					{Entry: loadingcache.Entry[uint8, int8]{Key: 252, Value: 125}, ExpiresAt: expiresAt},
					{Entry: loadingcache.Entry[uint8, int8]{Key: 253, Value: 126}, ExpiresAt: expiresAt},
					{Entry: loadingcache.Entry[uint8, int8]{Key: 254, Value: 127}, ExpiresAt: expiresAt},
					{Entry: loadingcache.Entry[uint8, int8]{Key: 255, Value: -128}, ExpiresAt: expiresAt},
				},
			},
		}
		rand.Shuffle(len(patterns), func(i, j int) {
			patterns[i], patterns[j] = patterns[j], patterns[i]
		})

		var eg errgroup.Group
		for _, pattern := range patterns {
			pattern := pattern
			eg.Go(func() error {
				return storage.SetMulti(t.Context(), pattern.pairs)
			})
		}
		if err := eg.Wait(); err != nil {
			t.Fatal(err)
		}

		eg = errgroup.Group{}
		mu := sync.Mutex{}
		results := make([][]*loadingcache.CacheEntry[uint8, int8], len(patterns))
		for i, pattern := range patterns {
			i := i
			results[i] = make([]*loadingcache.CacheEntry[uint8, int8], len(pattern.pairs))

			keys := make([]uint8, len(pattern.pairs))
			for j, pair := range pattern.pairs {
				keys[j] = pair.Key
			}
			eg.Go(func() error {
				r, err := storage.GetMulti(t.Context(), keys)
				if err != nil {
					return err
				}

				mu.Lock()
				defer mu.Unlock()
				results[i] = r
				return nil
			})
		}
		if err := eg.Wait(); err != nil {
			t.Fatal(err)
		}

		for i, pattern := range patterns {
			if df := cmp.Diff(pattern.pairs, results[i]); df != "" {
				t.Errorf("pattern[%d] entry diff=%s", i, df)
			}
		}
	})
}

type FixedClock struct {
	Time time.Time
}

func (c *FixedClock) Now() time.Time {
	return c.Time
}

func TestExpiration(t *testing.T, provider func(loadingcache.Clock) (loadingcache.CacheStorage[uint8, int8], func())) {
	t.Run("Expiration", func(t *testing.T) {
		t.Parallel()

		t.Run("SetAndGet", func(t *testing.T) {
			t.Parallel()

			base := time.Now()
			clock := &FixedClock{Time: base}
			storage, release := provider(clock)
			defer release()

			cacheEntry, err := storage.Get(t.Context(), 1)
			if err != nil {
				t.Fatal(err)
			}
			if cacheEntry != nil {
				t.Error("should not exist")
			}

			expiresAt := base.Add(time.Hour)
			if err := storage.Set(t.Context(), &loadingcache.CacheEntry[uint8, int8]{
				Entry:     loadingcache.Entry[uint8, int8]{Key: 1, Value: 1},
				ExpiresAt: expiresAt,
			}); err != nil {
				t.Fatal(err)
			}

			cacheEntry, err = storage.Get(t.Context(), 1)
			if err != nil {
				t.Fatal(err)
			}
			if df := cmp.Diff(&loadingcache.CacheEntry[uint8, int8]{
				Entry:     loadingcache.Entry[uint8, int8]{Key: 1, Value: 1},
				ExpiresAt: expiresAt,
			}, cacheEntry); df != "" {
				t.Errorf("entry diff=%s", df)
			}

			clock.Time = base.Add(time.Hour - time.Second)
			cacheEntry, err = storage.Get(t.Context(), 1)
			if err != nil {
				t.Fatal(err)
			}
			if df := cmp.Diff(&loadingcache.CacheEntry[uint8, int8]{
				Entry:     loadingcache.Entry[uint8, int8]{Key: 1, Value: 1},
				ExpiresAt: expiresAt,
			}, cacheEntry); df != "" {
				t.Errorf("entry diff=%s", df)
			}

			clock.Time = base.Add(time.Hour)
			cacheEntry, err = storage.Get(t.Context(), 1)
			if err != nil {
				t.Fatal("should not get at first")
			} else if cacheEntry != nil {
				t.Error("should not exist")
			}

			clock.Time = base.Add(time.Hour + time.Second)
			cacheEntry, err = storage.Get(t.Context(), 1)
			if err != nil {
				t.Fatal("should not get again")
			} else if cacheEntry != nil {
				t.Error("should not exist")
			}
		})

		t.Run("SetMultiAndGetMulti", func(t *testing.T) {
			t.Parallel()

			base := time.Now()
			clock := &FixedClock{Time: base}
			storage, release := provider(clock)
			defer release()

			keys := []uint8{1, 2, 3}
			entries, err := storage.GetMulti(t.Context(), keys)
			if err != nil {
				t.Fatal(err)
			}
			for i, entry := range entries {
				if entry != nil {
					t.Errorf("entry[%d] should not exist", i)
				}
			}

			expiresAt := base.Add(time.Hour)
			testEntries := []*loadingcache.CacheEntry[uint8, int8]{
				{Entry: loadingcache.Entry[uint8, int8]{Key: 1, Value: 1}, ExpiresAt: expiresAt},
				{Entry: loadingcache.Entry[uint8, int8]{Key: 2, Value: 2}, ExpiresAt: expiresAt},
				{Entry: loadingcache.Entry[uint8, int8]{Key: 3, Value: 3}, ExpiresAt: expiresAt},
			}
			if err := storage.SetMulti(t.Context(), testEntries); err != nil {
				t.Fatal(err)
			}

			// Verify entries were stored
			entries, err = storage.GetMulti(t.Context(), keys)
			if err != nil {
				t.Fatal(err)
			}
			if df := cmp.Diff(testEntries, entries); df != "" {
				t.Errorf("entries diff=%s", df)
			}

			// Just before expiration
			clock.Time = base.Add(time.Hour - time.Second)
			entries, err = storage.GetMulti(t.Context(), keys)
			if err != nil {
				t.Fatal(err)
			}
			if df := cmp.Diff(testEntries, entries); df != "" {
				t.Errorf("entries diff=%s", df)
			}

			// At expiration
			clock.Time = base.Add(time.Hour)
			entries, err = storage.GetMulti(t.Context(), keys)
			if err != nil {
				t.Fatal(err)
			}
			for i, entry := range entries {
				if entry != nil {
					t.Errorf("entry[%d] should be expired at exactly expiration time", i)
				}
			}

			// After expiration
			clock.Time = base.Add(time.Hour + time.Second)
			entries, err = storage.GetMulti(t.Context(), keys)
			if err != nil {
				t.Fatal(err)
			}
			for i, entry := range entries {
				if entry != nil {
					t.Errorf("entry[%d] should be expired after expiration time", i)
				}
			}
		})
	})
}

func TestNegativeCache(t *testing.T, provider func(loadingcache.Clock) (loadingcache.CacheStorage[uint8, int8], func())) {
	t.Run("NegativeCache", func(t *testing.T) {
		t.Parallel()

		t.Run("SetAndGet", func(t *testing.T) {
			t.Parallel()

			base := time.Now()
			clock := &FixedClock{Time: base}
			storage, release := provider(clock)
			defer release()

			cacheEntry, err := storage.Get(t.Context(), 1)
			if err != nil {
				t.Fatal(err)
			}
			if cacheEntry != nil {
				t.Error("should not exist")
			}

			expiresAt := base.Add(time.Hour)
			if err := storage.Set(t.Context(), &loadingcache.CacheEntry[uint8, int8]{
				Entry:         loadingcache.Entry[uint8, int8]{Key: 1},
				NegativeCache: true,
				ExpiresAt:     expiresAt,
			}); err != nil {
				t.Fatal(err)
			}

			cacheEntry, err = storage.Get(t.Context(), 1)
			if err != nil {
				t.Fatal(err)
			}
			if df := cmp.Diff(&loadingcache.CacheEntry[uint8, int8]{
				Entry:         loadingcache.Entry[uint8, int8]{Key: 1},
				NegativeCache: true,
				ExpiresAt:     expiresAt,
			}, cacheEntry); df != "" {
				t.Errorf("entry diff=%s", df)
			}

			clock.Time = base.Add(time.Hour - time.Second)
			cacheEntry, err = storage.Get(t.Context(), 1)
			if err != nil {
				t.Fatal(err)
			}
			if df := cmp.Diff(&loadingcache.CacheEntry[uint8, int8]{
				Entry:         loadingcache.Entry[uint8, int8]{Key: 1},
				NegativeCache: true,
				ExpiresAt:     expiresAt,
			}, cacheEntry); df != "" {
				t.Errorf("entry diff=%s", df)
			}

			clock.Time = base.Add(time.Hour)
			cacheEntry, err = storage.Get(t.Context(), 1)
			if err != nil {
				t.Fatal("should not get at first")
			} else if cacheEntry != nil {
				t.Error("should not exist")
			}

			clock.Time = base.Add(time.Hour + time.Second)
			cacheEntry, err = storage.Get(t.Context(), 1)
			if err != nil {
				t.Fatal("should not get again")
			} else if cacheEntry != nil {
				t.Error("should not exist")
			}
		})

		t.Run("SetMultiAndGetMulti", func(t *testing.T) {
			t.Parallel()

			base := time.Now()
			clock := &FixedClock{Time: base}
			storage, release := provider(clock)
			defer release()

			keys := []uint8{1, 2, 3}
			entries, err := storage.GetMulti(t.Context(), keys)
			if err != nil {
				t.Fatal(err)
			}
			for i, entry := range entries {
				if entry != nil {
					t.Errorf("entry[%d] should not exist", i)
				}
			}

			expiresAt := base.Add(time.Hour)
			testEntries := []*loadingcache.CacheEntry[uint8, int8]{
				{Entry: loadingcache.Entry[uint8, int8]{Key: 1}, NegativeCache: true, ExpiresAt: expiresAt},
				{Entry: loadingcache.Entry[uint8, int8]{Key: 2, Value: 2}, ExpiresAt: expiresAt},
				{Entry: loadingcache.Entry[uint8, int8]{Key: 3, Value: 3}, ExpiresAt: expiresAt},
			}
			if err := storage.SetMulti(t.Context(), testEntries); err != nil {
				t.Fatal(err)
			}

			// Verify entries were stored
			entries, err = storage.GetMulti(t.Context(), keys)
			if err != nil {
				t.Fatal(err)
			}
			if df := cmp.Diff(testEntries, entries); df != "" {
				t.Errorf("entries diff=%s", df)
			}

			// Just before expiration
			clock.Time = base.Add(time.Hour - time.Second)
			entries, err = storage.GetMulti(t.Context(), keys)
			if err != nil {
				t.Fatal(err)
			}
			if df := cmp.Diff(testEntries, entries); df != "" {
				t.Errorf("entries diff=%s", df)
			}

			// At expiration
			clock.Time = base.Add(time.Hour)
			entries, err = storage.GetMulti(t.Context(), keys)
			if err != nil {
				t.Fatal(err)
			}
			for i, entry := range entries {
				if entry != nil {
					t.Errorf("entry[%d] should be expired at exactly expiration time", i)
				}
			}

			// After expiration
			clock.Time = base.Add(time.Hour + time.Second)
			entries, err = storage.GetMulti(t.Context(), keys)
			if err != nil {
				t.Fatal(err)
			}
			for i, entry := range entries {
				if entry != nil {
					t.Errorf("entry[%d] should be expired after expiration time", i)
				}
			}
		})
	})
}
