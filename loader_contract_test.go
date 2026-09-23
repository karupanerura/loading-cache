package loadingcache_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	loadingcache "github.com/karupanerura/loading-cache"
	"github.com/karupanerura/loading-cache/loader/pureloader"
	"github.com/karupanerura/loading-cache/loader/singleflightloader"
	"github.com/karupanerura/loading-cache/source"
	"github.com/karupanerura/loading-cache/storage"
	"github.com/karupanerura/loading-cache/storage/memstorage"
)

func TestLoaderRejectsInvalidSourceResults(t *testing.T) {
	t.Parallel()
	for _, singleflight := range []bool{false, true} {
		for _, scenario := range []string{"Short", "Long", "Reordered"} {
			name := "Pure/" + scenario
			if singleflight {
				name = "SingleFlight/" + scenario
			}
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
				defer cancel()
				var calls, stores atomic.Int32
				src := source.GetMultiFunctionSource[int, int](func(_ context.Context, keys []int) ([]*loadingcache.CacheEntry[int, int], error) {
					n := len(keys)
					invalid := calls.Add(1) == 1
					if invalid {
						switch scenario {
						case "Short":
							n--
						case "Long":
							n++
						}
					}
					entries := make([]*loadingcache.CacheEntry[int, int], n)
					for i := range entries {
						entries[i] = &loadingcache.CacheEntry[int, int]{Entry: loadingcache.Entry[int, int]{Key: i + 1, Value: 42}, ExpiresAt: time.Now().Add(time.Hour)}
					}
					if invalid && scenario == "Reordered" {
						entries[0], entries[1] = entries[1], entries[0]
					}
					return entries, nil
				})
				st := &storage.FunctionsStorage[int, int]{SetMultiFunc: func(context.Context, []*loadingcache.CacheEntry[int, int]) error { stores.Add(1); return nil }}
				loader := newContractLoader(singleflight, st, src)
				if _, err := loader.LoadAndStoreMulti(ctx, []int{1, 2}); !errors.Is(err, loadingcache.ErrInvalidSourceResult) {
					t.Fatalf("invalid result: got %v, want ErrInvalidSourceResult", err)
				}
				if stores.Load() != 0 {
					t.Fatal("invalid source results were stored")
				}
				entries, err := loader.LoadAndStoreMulti(ctx, []int{1, 2})
				if err != nil || len(entries) != 2 {
					t.Fatalf("retry: %v, %v", entries, err)
				}
				for i, entry := range entries {
					if entry == nil || entry.Key != i+1 || entry.Value != 42 {
						t.Errorf("retry entry %d: %v", i, entry)
					}
				}
				if stores.Load() != 1 {
					t.Fatalf("successful retry stored %d times", stores.Load())
				}
			})
		}
	}
}

func TestLoaderRejectsInvalidSingleSourceKey(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"Pure", "SingleFlight"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			var calls, stores atomic.Int32
			src := &source.FunctionsSource[int, int]{GetFunc: func(_ context.Context, key int) (*loadingcache.CacheEntry[int, int], error) {
				if calls.Add(1) == 1 {
					key++
				}
				return &loadingcache.CacheEntry[int, int]{Entry: loadingcache.Entry[int, int]{Key: key, Value: 42}, ExpiresAt: time.Now().Add(time.Hour)}, nil
			}}
			st := &storage.FunctionsStorage[int, int]{SetFunc: func(context.Context, *loadingcache.CacheEntry[int, int]) error { stores.Add(1); return nil }}
			loader := newContractLoader(name == "SingleFlight", st, src)
			if _, err := loader.LoadAndStore(ctx, 1); !errors.Is(err, loadingcache.ErrInvalidSourceResult) {
				t.Fatalf("invalid key: got %v, want ErrInvalidSourceResult", err)
			}
			if stores.Load() != 0 {
				t.Fatal("entry for another key was stored")
			}
			entry, err := loader.LoadAndStore(ctx, 1)
			if err != nil || entry == nil || entry.Key != 1 || entry.Value != 42 || stores.Load() != 1 {
				t.Fatalf("retry: entry=%v, err=%v, stores=%d", entry, err, stores.Load())
			}
		})
	}
}

func TestCompactGetOnlySourceCachesNegativeEntries(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"Pure", "SingleFlight"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			var calls atomic.Int32
			src := &source.CompactSource[int, int]{Source: &source.FunctionsSource[int, int]{GetFunc: func(_ context.Context, key int) (*loadingcache.CacheEntry[int, int], error) {
				calls.Add(1)
				return &loadingcache.CacheEntry[int, int]{Entry: loadingcache.Entry[int, int]{Key: key}, NegativeCache: true, ExpiresAt: time.Now().Add(time.Hour)}, nil
			}}}
			st := memstorage.NewInMemoryStorage[int, int]()
			loader := newContractLoader(name == "SingleFlight", st, src)
			cache := loadingcache.LoadingCache[int, int]{Loader: loader, Storage: st}
			for range 3 {
				entry, err := cache.GetOrLoad(ctx, 1)
				if err != nil || entry != nil {
					t.Fatalf("GetOrLoad: %v, %v", entry, err)
				}
				entries, err := cache.GetOrLoadMulti(ctx, []int{1, 1})
				if err != nil || len(entries) != 2 || entries[0] != nil || entries[1] != nil {
					t.Fatalf("GetOrLoadMulti: %v, %v", entries, err)
				}
			}
			if calls.Load() != 1 {
				t.Fatalf("source called %d times; negative cache was not reused", calls.Load())
			}
		})
	}
}

func TestPureLoaderAlreadyCanceled(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	calls := 0
	src := &source.FunctionsSource[int, int]{GetFunc: func(context.Context, int) (*loadingcache.CacheEntry[int, int], error) {
		calls++
		return nil, nil
	}}
	loader := pureloader.NewPureLoader[int, int](memstorage.NewInMemoryStorage[int, int](), src)
	if _, err := loader.LoadAndStore(ctx, 1); !errors.Is(err, context.Canceled) {
		t.Fatalf("LoadAndStore: %v, want context.Canceled", err)
	}
	if _, err := loader.LoadAndStoreMulti(ctx, []int{1, 2}); !errors.Is(err, context.Canceled) {
		t.Fatalf("LoadAndStoreMulti: %v, want context.Canceled", err)
	}
	if calls != 0 {
		t.Fatalf("source called %d times for canceled loads", calls)
	}
}

// omittingSource is a source whose Get follows the LoadingSource contract,
// but whose GetMulti omits missing keys, as CompactSource allows.
// Key 9 does not exist.
type omittingSource struct {
	getMultiCalls atomic.Int32
}

func (s *omittingSource) Get(_ context.Context, key int) (*loadingcache.CacheEntry[int, int], error) {
	if key == 9 {
		return nil, nil
	}
	return &loadingcache.CacheEntry[int, int]{Entry: loadingcache.Entry[int, int]{Key: key, Value: key * 10}, ExpiresAt: time.Now().Add(time.Hour)}, nil
}

func (s *omittingSource) GetMulti(ctx context.Context, keys []int) ([]*loadingcache.CacheEntry[int, int], error) {
	s.getMultiCalls.Add(1)
	entries := make([]*loadingcache.CacheEntry[int, int], 0, len(keys))
	for _, key := range keys {
		entry, err := s.Get(ctx, key)
		if err != nil {
			return nil, err
		}
		if entry != nil {
			entries = append(entries, entry)
		}
	}
	return entries, nil
}

func TestCompactSourceDuplicateRequests(t *testing.T) {
	t.Parallel()
	for _, singleflight := range []bool{false, true} {
		name := "Pure"
		if singleflight {
			name = "SingleFlight"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			inner := &omittingSource{}
			src := &source.CompactSource[int, int]{Source: inner}
			st := memstorage.NewInMemoryStorage[int, int]()
			loader := newContractLoader(singleflight, st, src)
			cache := loadingcache.LoadingCache[int, int]{Loader: loader, Storage: st}
			keys := []int{1, 1, 9, 3, 3}
			for _, phase := range []string{"Cold", "Warm"} {
				entries, err := cache.GetOrLoadMulti(t.Context(), keys)
				if err != nil || len(entries) != len(keys) {
					t.Fatalf("%s: %v, %v", phase, entries, err)
				}
				for i, key := range keys {
					if key == 9 {
						if entries[i] != nil {
							t.Fatalf("%s: missing key: %+v", phase, entries[i])
						}
					} else if entries[i] == nil || entries[i].Key != key || entries[i].Value != key*10 {
						t.Fatalf("%s: entry %d: %+v", phase, i, entries[i])
					}
				}
			}
			// Missing keys are not negatively cached, so the warm phase loads key 9 again.
			if got := inner.getMultiCalls.Load(); got != 2 {
				t.Errorf("GetMulti calls: got %d, want 2", got)
			}

			// Get delegates to the underlying Get, which follows the contract on its own.
			for _, key := range []int{5, 9} {
				entry, err := cache.GetOrLoad(t.Context(), key)
				if err != nil {
					t.Fatalf("GetOrLoad(%d): %v", key, err)
				}
				if key == 9 {
					if entry != nil {
						t.Errorf("GetOrLoad(9): got %+v, want nil", entry)
					}
				} else if entry == nil || entry.Key != key || entry.Value != key*10 {
					t.Errorf("GetOrLoad(%d): got %+v", key, entry)
				}
			}
		})
	}
}

// storeCountingStorage counts the calls that store entries.
type storeCountingStorage struct {
	loadingcache.CacheStorage[int, int]
	stores atomic.Int32
}

func (s *storeCountingStorage) Set(ctx context.Context, entry *loadingcache.CacheEntry[int, int]) error {
	s.stores.Add(1)
	return s.CacheStorage.Set(ctx, entry)
}

func (s *storeCountingStorage) SetMulti(ctx context.Context, entries []*loadingcache.CacheEntry[int, int]) error {
	s.stores.Add(1)
	return s.CacheStorage.SetMulti(ctx, entries)
}

// TestLoaderRejectsNegativeEntryWithoutKey verifies that a negative-cache entry
// whose Key is left unset is rejected instead of being cached for key 0, and that
// a valid negative-cache entry for the requested key 0 is accepted.
func TestLoaderRejectsNegativeEntryWithoutKey(t *testing.T) {
	t.Parallel()

	// entriesFor returns positional results in which key 5 is a negative-cache
	// entry without Key, and every other key is found.
	entriesFor := func(keys []int) []*loadingcache.CacheEntry[int, int] {
		entries := make([]*loadingcache.CacheEntry[int, int], len(keys))
		for i, key := range keys {
			if key == 5 {
				entries[i] = &loadingcache.CacheEntry[int, int]{NegativeCache: true, ExpiresAt: time.Now().Add(time.Hour)}
			} else if key == 0 {
				entries[i] = &loadingcache.CacheEntry[int, int]{Entry: loadingcache.Entry[int, int]{Key: 0}, NegativeCache: true, ExpiresAt: time.Now().Add(time.Hour)}
			} else {
				entries[i] = &loadingcache.CacheEntry[int, int]{Entry: loadingcache.Entry[int, int]{Key: key, Value: key * 10}, ExpiresAt: time.Now().Add(time.Hour)}
			}
		}
		return entries
	}
	positional := source.GetMultiFunctionSource[int, int](func(_ context.Context, keys []int) ([]*loadingcache.CacheEntry[int, int], error) {
		return entriesFor(keys), nil
	})
	sources := map[string]loadingcache.LoadingSource[int, int]{
		"Positional": &source.FunctionsSource[int, int]{
			GetFunc: func(ctx context.Context, key int) (*loadingcache.CacheEntry[int, int], error) {
				return entriesFor([]int{key})[0], nil
			},
			GetMultiFunc: positional,
		},
		"Compact": &source.CompactSource[int, int]{Source: &source.FunctionsSource[int, int]{
			GetFunc: func(ctx context.Context, key int) (*loadingcache.CacheEntry[int, int], error) {
				return entriesFor([]int{key})[0], nil
			},
			GetMultiFunc: positional,
		}},
	}
	for srcName, src := range sources {
		for _, singleflight := range []bool{false, true} {
			name := srcName + "/Pure"
			if singleflight {
				name = srcName + "/SingleFlight"
			}
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
				defer cancel()
				st := &storeCountingStorage{CacheStorage: memstorage.NewInMemoryStorage[int, int]()}
				cache := loadingcache.LoadingCache[int, int]{Loader: newContractLoader(singleflight, st, src), Storage: st}

				if entries, err := cache.GetOrLoadMulti(ctx, []int{5, 6}); entries != nil || !errors.Is(err, loadingcache.ErrInvalidSourceResult) {
					t.Errorf("GetOrLoadMulti([5 6]): got %v, %v; want nil, ErrInvalidSourceResult", entries, err)
				}
				if entry, err := cache.GetOrLoad(ctx, 5); entry != nil || !errors.Is(err, loadingcache.ErrInvalidSourceResult) {
					t.Errorf("GetOrLoad(5): got %v, %v; want nil, ErrInvalidSourceResult", entry, err)
				}
				if got := st.stores.Load(); got != 0 {
					t.Errorf("stores for invalid results: got %d, want 0", got)
				}

				// A cached hit for key 6 must not turn the failure into a partial result.
				if _, err := cache.GetOrLoad(ctx, 6); err != nil {
					t.Fatalf("GetOrLoad(6): %v", err)
				}
				stores := st.stores.Load()
				if entries, err := cache.GetOrLoadMulti(ctx, []int{6, 5}); entries != nil || !errors.Is(err, loadingcache.ErrInvalidSourceResult) {
					t.Errorf("GetOrLoadMulti([6 5]) with a cached hit: got %v, %v; want nil, ErrInvalidSourceResult", entries, err)
				}
				if got := st.stores.Load(); got != stores {
					t.Errorf("stores for invalid results with a cached hit: got %d, want %d", got, stores)
				}

				// Nothing was cached for key 0 by the invalid results.
				if cached, err := st.Get(ctx, 0); err != nil || cached != nil {
					t.Errorf("cached entry for key 0: got %+v, %v; want nil", cached, err)
				}

				// A negative-cache entry for the requested key 0 is valid.
				if entries, err := cache.GetOrLoadMulti(ctx, []int{0, 6}); err != nil || len(entries) != 2 || entries[0] != nil || entries[1] == nil {
					t.Errorf("GetOrLoadMulti([0 6]): got %v, %v", entries, err)
				}
				if cached, err := st.Get(ctx, 0); err != nil || cached == nil || !cached.NegativeCache {
					t.Errorf("cached entry for key 0: got %+v, %v; want a negative-cache entry", cached, err)
				}
			})
		}
	}
}

func TestLoadingCacheAlreadyCanceled(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	// Already-canceled calls must stop before consulting storage.
	st := &storage.FunctionsStorage[int, int]{
		GetFunc: func(context.Context, int) (*loadingcache.CacheEntry[int, int], error) {
			t.Error("storage was called with an already-canceled context")
			return nil, nil
		},
		GetMultiFunc: func(context.Context, []int) ([]*loadingcache.CacheEntry[int, int], error) {
			t.Error("storage was called with an already-canceled context")
			return nil, nil
		},
	}
	cache := loadingcache.LoadingCache[int, int]{Storage: st, Loader: pureloader.NewPureLoader[int, int](st, &source.FunctionsSource[int, int]{GetFunc: func(context.Context, int) (*loadingcache.CacheEntry[int, int], error) {
		t.Error("source was called with an already-canceled context")
		return nil, nil
	}})}
	if entry, err := cache.GetOrLoad(ctx, 1); entry != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("GetOrLoad: %v, %v", entry, err)
	}
	for _, keys := range [][]int{nil, {1, 2}} {
		if entries, err := cache.GetOrLoadMulti(ctx, keys); entries != nil || !errors.Is(err, context.Canceled) {
			t.Fatalf("GetOrLoadMulti(%v): %v, %v", keys, entries, err)
		}
	}
}

func newContractLoader(singleflight bool, st loadingcache.CacheStorage[int, int], src loadingcache.LoadingSource[int, int]) loadingcache.SourceLoader[int, int] {
	if singleflight {
		return singleflightloader.NewSingleFlightLoader(st, src)
	}
	return pureloader.NewPureLoader(st, src)
}
