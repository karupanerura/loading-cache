package loadingcache_test

import (
	"context"
	"fmt"
	"math/rand/v2"
	"testing"
	"time"

	"golang.org/x/sync/errgroup"

	loadingcache "github.com/karupanerura/loading-cache"
	"github.com/karupanerura/loading-cache/index"
	"github.com/karupanerura/loading-cache/index/omcindex"
	"github.com/karupanerura/loading-cache/loader/singleflightloader"
	"github.com/karupanerura/loading-cache/source"
	"github.com/karupanerura/loading-cache/storage/memstorage"
)

// TestStress_IndexedLoadingCache hammers the full stack (IndexedLoadingCache +
// OnMemoryIndex + SingleFlightLoader + memstorage) with concurrent
// FindBySecondaryKey / FindBySecondaryKeys / Refresh calls, short TTLs,
// duplicate secondary keys and canceled contexts.
//
// The index maps sk -> [sk, sk+1], so adjacent secondary keys share a primary
// key: a FindBySecondaryKeys call covering adjacent keys receives the same
// entry under two different secondary keys, which exercises the
// original-vs-clone distribution branch in FindBySecondaryKeys. Every caller
// asserts the value it received and then destroys it in place: if any two
// receivers (or a receiver and the cache) shared the same value object, the
// assertion or the race detector would catch it.
func TestStress_IndexedLoadingCache(t *testing.T) {
	t.Parallel()

	const (
		numSecondaryKeys = 8 // sk: 0..7, the index maps sk -> [sk, sk+1]
		negativePK       = 7 // the source returns a negative cache entry for this key
		missingPK        = 8 // the source returns no entry for this key
		numWorkers       = 6
		numIterations    = 300
		numRefreshes     = 40
		ttl              = 2 * time.Millisecond
	)

	idx := omcindex.NewOnMemoryIndex[uint8, int](index.FunctionIndexSource[uint8, int](func(context.Context) (map[uint8][]int, error) {
		m := make(map[uint8][]int, numSecondaryKeys)
		for sk := uint8(0); sk < numSecondaryKeys; sk++ {
			m[sk] = []int{int(sk), int(sk) + 1}
		}
		return m, nil
	}))

	newEntry := func(key int) *loadingcache.CacheEntry[int, *TestClonerStruct] {
		switch key {
		case missingPK:
			return nil
		case negativePK:
			return &loadingcache.CacheEntry[int, *TestClonerStruct]{
				Entry:         loadingcache.Entry[int, *TestClonerStruct]{Key: key},
				NegativeCache: true,
				ExpiresAt:     time.Now().Add(time.Hour),
			}
		default:
			return &loadingcache.CacheEntry[int, *TestClonerStruct]{
				Entry:     loadingcache.Entry[int, *TestClonerStruct]{Key: key, Value: &TestClonerStruct{Value: key * 100}},
				ExpiresAt: time.Now().Add(ttl),
			}
		}
	}
	src := source.GetMultiFunctionSource[int, *TestClonerStruct](func(_ context.Context, keys []int) ([]*loadingcache.CacheEntry[int, *TestClonerStruct], error) {
		entries := make([]*loadingcache.CacheEntry[int, *TestClonerStruct], len(keys))
		for i, key := range keys {
			entries[i] = newEntry(key)
		}
		return entries, nil
	})

	st := memstorage.NewInMemoryStorage[int, *TestClonerStruct]()
	loader := singleflightloader.NewSingleFlightLoader(st, src)
	cache := loadingcache.NewIndexedLoadingCache(
		loadingcache.LoadingCache[int, *TestClonerStruct]{Loader: loader, Storage: st},
		idx,
	)

	// initialize the index so that getters do not depend on the refresher goroutine
	if err := idx.Refresh(t.Context()); err != nil {
		t.Fatal(err)
	}

	// expectEntries returns how many non-nil entries a lookup for sk must yield.
	expectEntries := func(sk uint8) int {
		if sk >= numSecondaryKeys {
			return 0
		}
		n := 0
		for _, pk := range []int{int(sk), int(sk) + 1} {
			if pk != negativePK && pk != missingPK {
				n++
			}
		}
		return n
	}

	// verifyAndDestroy asserts the entries for sk and then corrupts the values
	// in place. FindBySecondaryKey yields nil entries for missing/negative
	// primary keys while FindBySecondaryKeys omits them, so nil entries are
	// skipped but counted against expectEntries.
	verifyAndDestroy := func(sk uint8, entries []*loadingcache.Entry[int, *TestClonerStruct]) error {
		found := 0
		for _, entry := range entries {
			if entry == nil {
				continue
			}
			found++
			if entry.Key != int(sk) && entry.Key != int(sk)+1 {
				return fmt.Errorf("unexpected primary key %d for secondary key %d", entry.Key, sk)
			}
			if entry.Value == nil {
				return fmt.Errorf("expected value for primary key %d, got nil", entry.Key)
			}
			if got := entry.Value.Value; got != entry.Key*100 {
				return fmt.Errorf("expected value %d for primary key %d, got %d (the value object is shared with another receiver)", entry.Key*100, entry.Key, got)
			}
			// Destroy the received value in place. If the cache or another
			// receiver shares this object, a subsequent assertion or the race
			// detector will catch it.
			entry.Value.Value = -1
		}
		if want := expectEntries(sk); found != want {
			return fmt.Errorf("expected %d entries for secondary key %d, got %d", want, sk, found)
		}
		return nil
	}

	var eg errgroup.Group
	eg.Go(func() error {
		for i := 0; i < numRefreshes; i++ {
			if err := idx.Refresh(t.Context()); err != nil {
				return err
			}
			time.Sleep(200 * time.Microsecond)
		}
		return nil
	})
	for w := 0; w < numWorkers; w++ {
		eg.Go(func() error {
			for i := 0; i < numIterations; i++ {
				switch i % 2 {
				case 0:
					// A secondary key beyond the index range verifies the not-found path.
					sk := uint8(rand.IntN(numSecondaryKeys + 1))
					entries, err := cache.FindBySecondaryKey(t.Context(), sk)
					if err != nil {
						return err
					}
					if err := verifyAndDestroy(sk, entries); err != nil {
						return err
					}
				case 1:
					// Adjacent secondary keys share a primary key, so the same
					// entry is distributed to both keys within one result map.
					// Duplicates are intentionally allowed.
					base := uint8(rand.IntN(numSecondaryKeys - 1))
					sks := []uint8{base, base + 1, base, uint8(rand.IntN(numSecondaryKeys + 1))}
					m, err := cache.FindBySecondaryKeys(t.Context(), sks)
					if err != nil {
						return err
					}
					// verify each distinct key once: verifyAndDestroy corrupts
					// the values in place, so a value object shared between two
					// secondary keys would fail the second verification
					seen := map[uint8]struct{}{}
					for _, sk := range sks {
						if _, ok := seen[sk]; ok {
							continue
						}
						seen[sk] = struct{}{}
						if err := verifyAndDestroy(sk, m[sk]); err != nil {
							return err
						}
					}
				}

				// Already-canceled calls must return before starting a load.
				if i%16 == 15 {
					ctx, cancel := context.WithCancel(t.Context())
					cancel()
					sk := uint8(rand.IntN(numSecondaryKeys))
					if _, err := cache.FindBySecondaryKey(ctx, sk); err != context.Canceled {
						return fmt.Errorf("expected context.Canceled, got %v", err)
					}
				}

				// Let entries expire from time to time to force reload storms.
				if i%50 == 49 {
					time.Sleep(ttl / 2)
				}
			}
			return nil
		})
	}
	if err := eg.Wait(); err != nil {
		t.Fatal(err)
	}
}
