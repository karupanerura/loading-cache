package memstorage_test

import (
	"fmt"
	"math/rand/v2"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/sync/errgroup"

	loadingcache "github.com/karupanerura/loading-cache"
	"github.com/karupanerura/loading-cache/expiration"
	"github.com/karupanerura/loading-cache/storage/memstorage"
)

// TestStress_ConcurrentSetGetExpiration hammers the storage with concurrent
// Set/SetMulti, Get/GetMulti (with duplicate keys) and a moving clock so that
// reads keep racing the lazy expired-entry deletion against writes restoring
// the same keys. Values are derived from their keys, so any cross-key mixup or
// stale-delete anomaly is observable; the race detector covers the rest.
func TestStress_ConcurrentSetGetExpiration(t *testing.T) {
	t.Parallel()

	policies := map[string]expiration.ExpirationPolicy{
		"GeneralPolicy": expiration.GeneralExpirationPolicy{},
		"EarlyPolicy":   &expiration.EarlyExpirationPolicy{Duration: 500 * time.Millisecond, Percentage: 0.5},
	}
	bucketsSizes := map[string]int{
		"SingleBucket":   1,
		"MultipleBucket": 8,
	}

	for policyName, policy := range policies {
		for sizeName, bucketsSize := range bucketsSizes {
			t.Run(fmt.Sprintf("%s/%s", policyName, sizeName), func(t *testing.T) {
				t.Parallel()

				base := time.Date(2024, time.March, 16, 0, 0, 0, 0, time.UTC)
				var offset atomic.Int64
				clock := loadingcache.ClockFunc(func() time.Time {
					return base.Add(time.Duration(offset.Load()))
				})

				storage := memstorage.NewInMemoryStorage(
					memstorage.WithBucketsSize[uint8, int8](bucketsSize),
					memstorage.WithClock[uint8, int8](clock),
					memstorage.WithExpirationPolicy[uint8, int8](policy),
				)

				const (
					numKeys       = 16
					numWriters    = 2
					numReaders    = 4
					numIterations = 300
					ttl           = time.Second
				)

				entryFor := func(key uint8) *loadingcache.CacheEntry[uint8, int8] {
					return &loadingcache.CacheEntry[uint8, int8]{
						Entry:     loadingcache.Entry[uint8, int8]{Key: key, Value: int8(key)},
						ExpiresAt: clock.Now().Add(ttl),
					}
				}
				verify := func(key uint8, entry *loadingcache.CacheEntry[uint8, int8]) error {
					if entry == nil {
						return nil // expired or not yet written
					}
					if entry.Key != key || entry.Value != int8(key) {
						return fmt.Errorf("unexpected entry for key %d: %+v", key, entry)
					}
					return nil
				}

				var eg errgroup.Group
				eg.Go(func() error { // clock advancer: expires everything twice per TTL
					for i := 0; i < 20; i++ {
						offset.Add(int64(ttl / 2))
						time.Sleep(time.Millisecond)
					}
					return nil
				})
				for w := 0; w < numWriters; w++ {
					eg.Go(func() error {
						for i := 0; i < numIterations; i++ {
							if i%2 == 0 {
								if err := storage.Set(t.Context(), entryFor(uint8(rand.IntN(numKeys)))); err != nil {
									return err
								}
							} else {
								entries := make([]*loadingcache.CacheEntry[uint8, int8], 1+rand.IntN(4))
								for j := range entries {
									entries[j] = entryFor(uint8(rand.IntN(numKeys))) // duplicates are intentional
								}
								if err := storage.SetMulti(t.Context(), entries); err != nil {
									return err
								}
							}
						}
						return nil
					})
				}
				for r := 0; r < numReaders; r++ {
					eg.Go(func() error {
						for i := 0; i < numIterations; i++ {
							if i%2 == 0 {
								key := uint8(rand.IntN(numKeys))
								entry, err := storage.Get(t.Context(), key)
								if err != nil {
									return err
								}
								if err := verify(key, entry); err != nil {
									return err
								}
							} else {
								key := uint8(rand.IntN(numKeys))
								keys := []uint8{key, uint8(rand.IntN(numKeys)), key, uint8(rand.IntN(numKeys))} // duplicates are intentional
								entries, err := storage.GetMulti(t.Context(), keys)
								if err != nil {
									return err
								}
								if len(entries) != len(keys) {
									return fmt.Errorf("expected %d entries, got %d", len(keys), len(entries))
								}
								for j, key := range keys {
									if err := verify(key, entries[j]); err != nil {
										return err
									}
								}
							}
						}
						return nil
					})
				}
				if err := eg.Wait(); err != nil {
					t.Fatal(err)
				}

				// After the churn no pending lazy deletion may swallow a fresh write.
				if err := storage.Set(t.Context(), entryFor(1)); err != nil {
					t.Fatal(err)
				}
				entry, err := storage.Get(t.Context(), 1)
				if err != nil {
					t.Fatal(err)
				}
				if entry == nil || entry.Key != 1 || entry.Value != 1 {
					t.Errorf("a fresh write was lost after the churn: %+v", entry)
				}
			})
		}
	}
}
