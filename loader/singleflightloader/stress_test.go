package singleflightloader_test

import (
	"context"
	"fmt"
	"math/rand/v2"
	"sync"
	"testing"
	"time"

	"golang.org/x/sync/errgroup"

	loadingcache "github.com/karupanerura/loading-cache"
	"github.com/karupanerura/loading-cache/loader/singleflightloader"
	"github.com/karupanerura/loading-cache/source"
	"github.com/karupanerura/loading-cache/storage"
	"github.com/karupanerura/loading-cache/storage/memstorage"
)

type mutableValue struct {
	N int
}

func (v *mutableValue) Clone() *mutableValue {
	return &mutableValue{N: v.N}
}

// TestStress_MixedOperations hammers the whole stack (LoadingCache +
// SingleFlightLoader + memstorage) with overlapping keys, short TTLs, mutable
// values, duplicate keys and canceled contexts. Every caller asserts the value
// it received and then destroys it in place: if any two callers (or a caller
// and the cache) shared the same value object, the assertion or the race
// detector would catch it.
func TestStress_MixedOperations(t *testing.T) {
	t.Parallel()

	const (
		numRegularKeys = 8
		missingKey     = 8 // the source returns no entry for this key
		negativeKey    = 9 // the source returns a negative cache entry for this key
		numWorkers     = 8
		numIterations  = 400
		ttl            = 2 * time.Millisecond
	)

	src := &source.FunctionsSource[int, *mutableValue]{
		GetFunc: func(_ context.Context, key int) (*loadingcache.CacheEntry[int, *mutableValue], error) {
			switch key {
			case missingKey:
				return nil, nil
			case negativeKey:
				return &loadingcache.CacheEntry[int, *mutableValue]{
					Entry:         loadingcache.Entry[int, *mutableValue]{Key: key},
					NegativeCache: true,
					ExpiresAt:     time.Now().Add(time.Hour),
				}, nil
			default:
				return &loadingcache.CacheEntry[int, *mutableValue]{
					Entry:     loadingcache.Entry[int, *mutableValue]{Key: key, Value: &mutableValue{N: key * 100}},
					ExpiresAt: time.Now().Add(ttl),
				}, nil
			}
		},
		GetMultiFunc: func(_ context.Context, keys []int) ([]*loadingcache.CacheEntry[int, *mutableValue], error) {
			entries := make([]*loadingcache.CacheEntry[int, *mutableValue], len(keys))
			for i, key := range keys {
				switch key {
				case missingKey:
					entries[i] = nil
				case negativeKey:
					entries[i] = &loadingcache.CacheEntry[int, *mutableValue]{
						Entry:         loadingcache.Entry[int, *mutableValue]{Key: key},
						NegativeCache: true,
						ExpiresAt:     time.Now().Add(time.Hour),
					}
				default:
					entries[i] = &loadingcache.CacheEntry[int, *mutableValue]{
						Entry:     loadingcache.Entry[int, *mutableValue]{Key: key, Value: &mutableValue{N: key * 100}},
						ExpiresAt: time.Now().Add(ttl),
					}
				}
			}
			return entries, nil
		},
	}

	st := memstorage.NewInMemoryStorage[int, *mutableValue]()
	loader := singleflightloader.NewSingleFlightLoader(st, src)
	cache := &loadingcache.LoadingCache[int, *mutableValue]{Loader: loader, Storage: st}

	verifyEntry := func(key int, entry *loadingcache.Entry[int, *mutableValue]) error {
		if key == missingKey || key == negativeKey {
			if entry != nil {
				return fmt.Errorf("expected nil entry for key %d, got %+v", key, entry)
			}
			return nil
		}
		if entry == nil {
			return fmt.Errorf("expected entry for key %d, got nil", key)
		}
		if entry.Key != key {
			return fmt.Errorf("expected key %d, got %d", key, entry.Key)
		}
		if entry.Value == nil {
			return fmt.Errorf("expected value for key %d, got nil", key)
		}
		if got := entry.Value.N; got != key*100 {
			return fmt.Errorf("expected value %d for key %d, got %d (the value object is shared with another receiver)", key*100, key, got)
		}
		// Destroy the received value in place. If the cache or another caller
		// shares this object, a subsequent assertion or the race detector
		// will catch it.
		entry.Value.N = -1
		return nil
	}

	randKey := func() int { return rand.IntN(numRegularKeys + 2) }
	randKeys := func() []int {
		keys := make([]int, 1+rand.IntN(6))
		for i := range keys {
			keys[i] = randKey() // duplicates are intentionally allowed
		}
		return keys
	}

	var eg errgroup.Group
	for w := 0; w < numWorkers; w++ {
		eg.Go(func() error {
			for i := 0; i < numIterations; i++ {
				switch i % 4 {
				case 0:
					key := randKey()
					entry, err := cache.GetOrLoad(t.Context(), key)
					if err != nil {
						return err
					}
					if err := verifyEntry(key, entry); err != nil {
						return err
					}
				case 1:
					keys := randKeys()
					entries, err := cache.GetOrLoadMulti(t.Context(), keys)
					if err != nil {
						return err
					}
					if len(entries) != len(keys) {
						return fmt.Errorf("expected %d entries, got %d", len(keys), len(entries))
					}
					for j, key := range keys {
						if err := verifyEntry(key, entries[j]); err != nil {
							return err
						}
					}
				case 2:
					key := randKey()
					entry, err := loader.LoadAndStore(t.Context(), key)
					if err != nil {
						return err
					}
					if err := verifyEntry(key, entry); err != nil {
						return err
					}
				case 3:
					keys := randKeys()
					entries, err := loader.LoadAndStoreMulti(t.Context(), keys)
					if err != nil {
						return err
					}
					if len(entries) != len(keys) {
						return fmt.Errorf("expected %d entries, got %d", len(keys), len(entries))
					}
					for j, key := range keys {
						if err := verifyEntry(key, entries[j]); err != nil {
							return err
						}
					}
				}

				// Exercise the abandoned-waiter path with an already canceled context.
				if i%16 == 15 {
					ctx, cancel := context.WithCancel(t.Context())
					cancel()
					key := randKey()
					if entry, err := loader.LoadAndStore(ctx, key); err == nil {
						// The result may have been ready before the cancellation was observed.
						if err := verifyEntry(key, entry); err != nil {
							return err
						}
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

// TestLoadAndStore_ValueIsolation pins the fix for the value-sharing race in
// sendEntry/sendEntries: the uncloned original value may only be handed to the
// last receiver, after the clones for all other receivers have been made.
// A receiver may start mutating its value as soon as it receives it, so if an
// earlier receiver got the original while later clones were still being made
// from it, the clones would observe the mutation. The slow cloner makes that
// window wide enough to fail deterministically.
func TestLoadAndStore_ValueIsolation(t *testing.T) {
	t.Parallel()

	const numWaiters = 3
	release := make(chan struct{})
	src := &source.FunctionsSource[int, *mutableValue]{
		GetFunc: func(_ context.Context, key int) (*loadingcache.CacheEntry[int, *mutableValue], error) {
			<-release // hold the load until all waiters are registered
			return &loadingcache.CacheEntry[int, *mutableValue]{
				Entry:     loadingcache.Entry[int, *mutableValue]{Key: key, Value: &mutableValue{N: 42}},
				ExpiresAt: time.Now().Add(time.Hour),
			}, nil
		},
	}
	st := &storage.FunctionsStorage[int, *mutableValue]{
		SetFunc: func(context.Context, *loadingcache.CacheEntry[int, *mutableValue]) error { return nil },
	}
	slowCloner := loadingcache.ValueClonerFunc[*mutableValue](func(v *mutableValue) *mutableValue {
		time.Sleep(10 * time.Millisecond)
		return &mutableValue{N: v.N}
	})
	loader := singleflightloader.NewSingleFlightLoader(st, src, singleflightloader.WithCloner[int](slowCloner))

	var wg sync.WaitGroup
	got := make([]int, numWaiters)
	wg.Add(numWaiters)
	for i := 0; i < numWaiters; i++ {
		go func(i int) {
			defer wg.Done()
			entry, err := loader.LoadAndStore(t.Context(), 1)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			got[i] = entry.Value.N
			entry.Value.N = -1 // mutate the received value immediately
		}(i)
	}

	time.Sleep(100 * time.Millisecond) // let all waiters join the waitlist
	close(release)
	wg.Wait()

	for i, n := range got {
		if n != 42 {
			t.Errorf("waiter %d observed %d instead of 42: its value was shared with another receiver", i, n)
		}
	}
}
