package loadingcache_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"
	loadingcache "github.com/karupanerura/loading-cache"
	"github.com/karupanerura/loading-cache/index"
	"github.com/karupanerura/loading-cache/storage"
)

// recordingLoader is a SourceLoader that records the keys it is asked to load
// and returns an entry with the value key*10 for each of them.
type recordingLoader struct {
	mu    sync.Mutex
	calls [][]uint8
}

func (l *recordingLoader) LoadAndStore(ctx context.Context, key uint8) (*loadingcache.Entry[uint8, int], error) {
	entries, err := l.LoadAndStoreMulti(ctx, []uint8{key})
	if err != nil {
		return nil, err
	}
	return entries[0], nil
}

func (l *recordingLoader) LoadAndStoreMulti(_ context.Context, keys []uint8) ([]*loadingcache.Entry[uint8, int], error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls = append(l.calls, append([]uint8(nil), keys...))
	entries := make([]*loadingcache.Entry[uint8, int], len(keys))
	for i, key := range keys {
		entries[i] = &loadingcache.Entry[uint8, int]{Key: key, Value: int(key) * 10}
	}
	return entries, nil
}

func (l *recordingLoader) recordedCalls() [][]uint8 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.calls
}

func storageReturning(entries []*loadingcache.CacheEntry[uint8, int], err error) *storage.FunctionsStorage[uint8, int] {
	return &storage.FunctionsStorage[uint8, int]{
		GetMultiFunc: func(context.Context, []uint8) ([]*loadingcache.CacheEntry[uint8, int], error) {
			return entries, err
		},
	}
}

func cached(key uint8, value int) *loadingcache.CacheEntry[uint8, int] {
	return &loadingcache.CacheEntry[uint8, int]{Entry: loadingcache.Entry[uint8, int]{Key: key, Value: value}}
}

func TestGetOrLoadMulti_InvalidStorageResultLength(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name    string
		entries []*loadingcache.CacheEntry[uint8, int]
		wantMsg string
	}{
		{name: "nil", entries: nil, wantMsg: "returned 0 entries for 2 keys"},
		{name: "short", entries: make([]*loadingcache.CacheEntry[uint8, int], 1), wantMsg: "returned 1 entries for 2 keys"},
		{name: "long", entries: make([]*loadingcache.CacheEntry[uint8, int], 3), wantMsg: "returned 3 entries for 2 keys"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			loader := &recordingLoader{}
			cache := &loadingcache.LoadingCache[uint8, int]{
				Storage: storageReturning(tt.entries, nil),
				Loader:  loader,
			}
			got, err := cache.GetOrLoadMulti(t.Context(), []uint8{1, 2})
			if err == nil {
				t.Fatalf("GetOrLoadMulti: got %v, want an error", got)
			}
			if got != nil {
				t.Errorf("GetOrLoadMulti: got %v, want nil entries", got)
			}
			if msg := err.Error(); !strings.Contains(msg, "CacheStorage.GetMulti") || !strings.Contains(msg, tt.wantMsg) {
				t.Errorf("error message %q should name the operation and contain %q", msg, tt.wantMsg)
			}
			if errors.Is(err, loadingcache.ErrInvalidSourceResult) {
				t.Errorf("a storage result error must not be classified as ErrInvalidSourceResult: %v", err)
			}
			if calls := loader.recordedCalls(); len(calls) != 0 {
				t.Errorf("loader calls: got %v, want none", calls)
			}
		})
	}
}

func TestGetOrLoadMulti_ValidStorageResultLength(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name      string
		keys      []uint8
		entries   []*loadingcache.CacheEntry[uint8, int]
		want      []*loadingcache.Entry[uint8, int]
		wantCalls [][]uint8
	}{
		{
			name:      "all missing",
			keys:      []uint8{1, 2},
			entries:   make([]*loadingcache.CacheEntry[uint8, int], 2),
			want:      []*loadingcache.Entry[uint8, int]{{Key: 1, Value: 10}, {Key: 2, Value: 20}},
			wantCalls: [][]uint8{{1, 2}},
		},
		{
			name:      "duplicated keys with hits and misses",
			keys:      []uint8{1, 2, 1},
			entries:   []*loadingcache.CacheEntry[uint8, int]{cached(1, 1), nil, cached(1, 1)},
			want:      []*loadingcache.Entry[uint8, int]{{Key: 1, Value: 1}, {Key: 2, Value: 20}, {Key: 1, Value: 1}},
			wantCalls: [][]uint8{{2}},
		},
		{
			name:    "empty input with nil result",
			keys:    []uint8{},
			entries: nil,
			want:    []*loadingcache.Entry[uint8, int]{},
		},
		{
			name:    "empty input with empty result",
			keys:    nil,
			entries: []*loadingcache.CacheEntry[uint8, int]{},
			want:    []*loadingcache.Entry[uint8, int]{},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			loader := &recordingLoader{}
			cache := &loadingcache.LoadingCache[uint8, int]{
				Storage: storageReturning(tt.entries, nil),
				Loader:  loader,
			}
			got, err := cache.GetOrLoadMulti(t.Context(), tt.keys)
			if err != nil {
				t.Fatalf("GetOrLoadMulti: %v", err)
			}
			if df := cmp.Diff(tt.want, got); df != "" {
				t.Errorf("entries (-want +got):\n%s", df)
			}
			if df := cmp.Diff(tt.wantCalls, loader.recordedCalls()); df != "" {
				t.Errorf("loader calls (-want +got):\n%s", df)
			}
		})
	}
}

func TestGetOrLoadMulti_StorageErrorTakesPrecedence(t *testing.T) {
	t.Parallel()

	loader := &recordingLoader{}
	cache := &loadingcache.LoadingCache[uint8, int]{
		Storage: storageReturning(make([]*loadingcache.CacheEntry[uint8, int], 5), errStorage),
		Loader:  loader,
	}
	if got, err := cache.GetOrLoadMulti(t.Context(), []uint8{1, 2}); got != nil || err != errStorage {
		t.Errorf("GetOrLoadMulti: got %v, %v; want nil, %v", got, err, errStorage)
	}
	if calls := loader.recordedCalls(); len(calls) != 0 {
		t.Errorf("loader calls: got %v, want none", calls)
	}
}

func TestGetOrLoadMulti_SilentErrorStorageResultLength(t *testing.T) {
	t.Parallel()

	t.Run("error is converted to missing entries", func(t *testing.T) {
		t.Parallel()

		var onError []error
		loader := &recordingLoader{}
		cache := &loadingcache.LoadingCache[uint8, int]{
			Storage: &storage.SilentErrorStorage[uint8, int]{
				Storage: storageReturning(nil, errStorage),
				OnError: func(err error) { onError = append(onError, err) },
			},
			Loader: loader,
		}
		got, err := cache.GetOrLoadMulti(t.Context(), []uint8{1, 2})
		if err != nil {
			t.Fatalf("GetOrLoadMulti: %v", err)
		}
		if df := cmp.Diff([]*loadingcache.Entry[uint8, int]{{Key: 1, Value: 10}, {Key: 2, Value: 20}}, got); df != "" {
			t.Errorf("entries (-want +got):\n%s", df)
		}
		if len(onError) != 1 || onError[0] != errStorage {
			t.Errorf("OnError calls: got %v, want [%v]", onError, errStorage)
		}
	})

	t.Run("invalid length without error is detected by LoadingCache", func(t *testing.T) {
		t.Parallel()

		var onError []error
		loader := &recordingLoader{}
		cache := &loadingcache.LoadingCache[uint8, int]{
			Storage: &storage.SilentErrorStorage[uint8, int]{
				Storage: storageReturning(make([]*loadingcache.CacheEntry[uint8, int], 1), nil),
				OnError: func(err error) { onError = append(onError, err) },
			},
			Loader: loader,
		}
		if got, err := cache.GetOrLoadMulti(t.Context(), []uint8{1, 2}); got != nil || err == nil {
			t.Errorf("GetOrLoadMulti: got %v, %v; want nil and an error", got, err)
		}
		if len(onError) != 0 {
			t.Errorf("OnError calls: got %v, want none", onError)
		}
		if calls := loader.recordedCalls(); len(calls) != 0 {
			t.Errorf("loader calls: got %v, want none", calls)
		}
	})
}

func TestIndexedLoadingCache_InvalidStorageResultLength(t *testing.T) {
	t.Parallel()

	idx := &index.FunctionsIndex[int, uint8]{
		GetFunc: func(context.Context, int) ([]uint8, error) {
			return []uint8{1, 2}, nil
		},
		GetMultiFunc: func(_ context.Context, sks []int) (map[int][]uint8, error) {
			return map[int][]uint8{1: {1, 2}}, nil
		},
	}
	loader := &recordingLoader{}
	cache := loadingcache.NewIndexedLoadingCache(loadingcache.LoadingCache[uint8, int]{
		Storage: storageReturning(make([]*loadingcache.CacheEntry[uint8, int], 1), nil),
		Loader:  loader,
	}, idx)

	if got, err := cache.FindBySecondaryKey(t.Context(), 1); got != nil || err == nil {
		t.Errorf("FindBySecondaryKey: got %v, %v; want nil and an error", got, err)
	}
	if got, err := cache.FindBySecondaryKeys(t.Context(), []int{1}); got != nil || err == nil {
		t.Errorf("FindBySecondaryKeys: got %v, %v; want nil and an error", got, err)
	}
	if calls := loader.recordedCalls(); len(calls) != 0 {
		t.Errorf("loader calls: got %v, want none", calls)
	}
}

// shortLoader is a SourceLoader that violates the contract by returning one
// entry fewer than requested.
type shortLoader struct{}

func (shortLoader) LoadAndStore(context.Context, uint8) (*loadingcache.Entry[uint8, int], error) {
	return nil, nil
}

func (shortLoader) LoadAndStoreMulti(_ context.Context, keys []uint8) ([]*loadingcache.Entry[uint8, int], error) {
	return make([]*loadingcache.Entry[uint8, int], len(keys)-1), nil
}

func TestGetOrLoadMulti_InvalidLoaderResultLength(t *testing.T) {
	t.Parallel()

	cache := &loadingcache.LoadingCache[uint8, int]{
		Storage: storageReturning(make([]*loadingcache.CacheEntry[uint8, int], 2), nil),
		Loader:  shortLoader{},
	}
	got, err := cache.GetOrLoadMulti(t.Context(), []uint8{1, 2})
	if err == nil {
		t.Fatalf("GetOrLoadMulti: got %v, want an error", got)
	}
	if got != nil {
		t.Errorf("GetOrLoadMulti: got %v, want nil entries", got)
	}
	if msg := err.Error(); !strings.Contains(msg, "SourceLoader.LoadAndStoreMulti") || !strings.Contains(msg, "returned 1 entries for 2 keys") {
		t.Errorf("error message %q should name the operation and the lengths", msg)
	}
}
