package loadingcache_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	loadingcache "github.com/karupanerura/loading-cache"
	"github.com/karupanerura/loading-cache/loader/pureloader"
	"github.com/karupanerura/loading-cache/source"
	"github.com/karupanerura/loading-cache/storage"
)

func TestSingleFlightCacheLoader_GetOrLoad(t *testing.T) {
	t.Parallel()

	storageErr := errors.New("storage error")
	sourceErr := errors.New("source error")
	tests := []struct {
		name           string
		key            uint8
		storageGet     func(context.Context, uint8) (*loadingcache.CacheEntry[uint8, string], error)
		sourceGet      func(context.Context, uint8) (*loadingcache.CacheEntry[uint8, string], error)
		expectedValue  string
		expectedError  error
		expectedStored []*loadingcache.CacheEntry[uint8, string]
	}{
		{
			name: "GetOrLoad returns value from source",
			key:  1,
			storageGet: func(_ context.Context, key uint8) (*loadingcache.CacheEntry[uint8, string], error) {
				return nil, nil
			},
			sourceGet: func(_ context.Context, key uint8) (*loadingcache.CacheEntry[uint8, string], error) {
				if key == 1 {
					return &loadingcache.CacheEntry[uint8, string]{
						Entry:     loadingcache.Entry[uint8, string]{Key: key, Value: "value1"},
						ExpiresAt: time.Date(2025, 1, 1, 2, 30, 45, 0, time.UTC),
					}, nil
				}
				return nil, nil
			},
			expectedValue: "value1",
			expectedError: nil,
			expectedStored: []*loadingcache.CacheEntry[uint8, string]{
				{
					Entry: loadingcache.Entry[uint8, string]{
						Key:   1,
						Value: "value1",
					},
					ExpiresAt: time.Date(2025, 1, 1, 2, 30, 45, 0, time.UTC),
				},
			},
		},
		{
			name: "GetOrLoad returns missing key from source",
			key:  2,
			storageGet: func(_ context.Context, key uint8) (*loadingcache.CacheEntry[uint8, string], error) {
				return nil, nil
			},
			sourceGet: func(_ context.Context, key uint8) (*loadingcache.CacheEntry[uint8, string], error) {
				return nil, nil
			},
			expectedValue:  "",
			expectedError:  nil,
			expectedStored: []*loadingcache.CacheEntry[uint8, string]{},
		},
		{
			name: "GetOrLoad returns missing key from source with negative cache",
			key:  2,
			storageGet: func(_ context.Context, key uint8) (*loadingcache.CacheEntry[uint8, string], error) {
				return nil, nil
			},
			sourceGet: func(_ context.Context, key uint8) (*loadingcache.CacheEntry[uint8, string], error) {
				return &loadingcache.CacheEntry[uint8, string]{
					Entry: loadingcache.Entry[uint8, string]{
						Key: key,
					},
					NegativeCache: true,
					ExpiresAt:     time.Date(2025, 1, 1, 2, 30, 45, 0, time.UTC),
				}, nil
			},
			expectedValue: "",
			expectedError: nil,
			expectedStored: []*loadingcache.CacheEntry[uint8, string]{
				{
					Entry: loadingcache.Entry[uint8, string]{
						Key: 2,
					},
					NegativeCache: true,
					ExpiresAt:     time.Date(2025, 1, 1, 2, 30, 45, 0, time.UTC),
				},
			},
		},
		{
			name: "GetOrLoad returns missing key from storage as negative cache",
			key:  2,
			storageGet: func(_ context.Context, key uint8) (*loadingcache.CacheEntry[uint8, string], error) {
				return &loadingcache.CacheEntry[uint8, string]{
					Entry: loadingcache.Entry[uint8, string]{
						Key: key,
					},
					NegativeCache: true,
					ExpiresAt:     time.Date(2025, 1, 1, 2, 30, 45, 0, time.UTC),
				}, nil
			},
			sourceGet: func(_ context.Context, key uint8) (*loadingcache.CacheEntry[uint8, string], error) {
				return &loadingcache.CacheEntry[uint8, string]{
					Entry: loadingcache.Entry[uint8, string]{
						Key: key,
					},
					NegativeCache: true,
					ExpiresAt:     time.Date(2025, 1, 1, 2, 30, 45, 0, time.UTC),
				}, nil
			},
			expectedValue:  "",
			expectedError:  nil,
			expectedStored: []*loadingcache.CacheEntry[uint8, string]{},
		},
		{
			name: "GetOrLoad returns value from cache",
			key:  3,
			storageGet: func(_ context.Context, key uint8) (*loadingcache.CacheEntry[uint8, string], error) {
				return &loadingcache.CacheEntry[uint8, string]{
					Entry: loadingcache.Entry[uint8, string]{
						Key:   key,
						Value: "cachedValue",
					},
					ExpiresAt: time.Now().Add(time.Hour),
				}, nil
			},
			sourceGet: func(_ context.Context, key uint8) (*loadingcache.CacheEntry[uint8, string], error) {
				return nil, errors.New("should not be called")
			},
			expectedValue:  "cachedValue",
			expectedError:  nil,
			expectedStored: []*loadingcache.CacheEntry[uint8, string]{},
		},
		{
			name: "GetOrLoad handles storage error",
			key:  4,
			storageGet: func(_ context.Context, key uint8) (*loadingcache.CacheEntry[uint8, string], error) {
				return nil, storageErr
			},
			sourceGet: func(_ context.Context, key uint8) (*loadingcache.CacheEntry[uint8, string], error) {
				return nil, errors.New("should not be called")
			},
			expectedValue:  "",
			expectedError:  storageErr,
			expectedStored: []*loadingcache.CacheEntry[uint8, string]{},
		},
		{
			name: "GetOrLoad handles source error",
			key:  5,
			storageGet: func(_ context.Context, key uint8) (*loadingcache.CacheEntry[uint8, string], error) {
				return nil, nil
			},
			sourceGet: func(_ context.Context, key uint8) (*loadingcache.CacheEntry[uint8, string], error) {
				return nil, sourceErr
			},
			expectedValue:  "",
			expectedError:  sourceErr,
			expectedStored: []*loadingcache.CacheEntry[uint8, string]{},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mu := sync.Mutex{}
			entries := make([]*loadingcache.CacheEntry[uint8, string], 0, 1)
			mockStorage := &storage.FunctionsStorage[uint8, string]{
				GetFunc: tt.storageGet,
				SetFunc: func(_ context.Context, entry *loadingcache.CacheEntry[uint8, string]) error {
					mu.Lock()
					defer mu.Unlock()
					entries = append(entries, entry)
					return nil
				},
			}

			src := &source.FunctionsSource[uint8, string]{
				GetFunc: tt.sourceGet,
			}

			loader := pureloader.NewPureLoader(mockStorage, src)
			loadingCache := loadingcache.LoadingCache[uint8, string]{
				Loader:  loader,
				Storage: mockStorage,
			}

			result, err := loadingCache.GetOrLoad(t.Context(), tt.key)
			if err != nil && tt.expectedError == nil {
				t.Errorf("unexpected error: %v", err)
			} else if err == nil && tt.expectedError != nil {
				t.Errorf("expected error: %v, got: nil", tt.expectedError)
			} else if err != nil && tt.expectedError != nil && !errors.Is(err, tt.expectedError) {
				t.Errorf("expected error: %v, got: %v", tt.expectedError, err)
			}

			var value string
			if result != nil {
				value = result.Value
			}
			if value != tt.expectedValue {
				t.Errorf("expected value: %v, got: %v", tt.expectedValue, value)
			}

			if df := cmp.Diff(tt.expectedStored, entries); df != "" {
				t.Errorf("unexpected stored entries: %s", df)
			}
		})
	}
}

func TestSingleFlightCacheLoader_GetOrLoadMulti(t *testing.T) {
	t.Parallel()

	sourceErr := errors.New("source error")
	storageErr := errors.New("storage error")
	tests := []struct {
		name            string
		keys            []uint8
		storageGetMulti func(context.Context, []uint8) ([]*loadingcache.CacheEntry[uint8, string], error)
		sourceGetMulti  func(context.Context, []uint8) ([]*loadingcache.CacheEntry[uint8, string], error)
		expectedEntries []*loadingcache.Entry[uint8, string]
		expectedError   error
		expectedStored  []*loadingcache.CacheEntry[uint8, string]
	}{
		{
			name: "GetOrLoadMulti returns values from source",
			keys: []uint8{1, 2},
			storageGetMulti: func(_ context.Context, keys []uint8) ([]*loadingcache.CacheEntry[uint8, string], error) {
				entries := make([]*loadingcache.CacheEntry[uint8, string], len(keys))
				return entries, nil
			},
			sourceGetMulti: func(_ context.Context, keys []uint8) ([]*loadingcache.CacheEntry[uint8, string], error) {
				entries := make([]*loadingcache.CacheEntry[uint8, string], len(keys))
				for i, key := range keys {
					if key == 1 {
						entries[i] = &loadingcache.CacheEntry[uint8, string]{Entry: loadingcache.Entry[uint8, string]{Key: key, Value: "value1"}, ExpiresAt: time.Date(2025, 1, 1, 2, 30, 45, 0, time.UTC)}
					} else if key == 2 {
						entries[i] = &loadingcache.CacheEntry[uint8, string]{Entry: loadingcache.Entry[uint8, string]{Key: key, Value: "value2"}, ExpiresAt: time.Date(2025, 1, 1, 3, 30, 45, 0, time.UTC)}
					} else {
						entries[i] = nil
					}
				}
				return entries, nil
			},
			expectedEntries: []*loadingcache.Entry[uint8, string]{
				{Key: 1, Value: "value1"},
				{Key: 2, Value: "value2"},
			},
			expectedError: nil,
			expectedStored: []*loadingcache.CacheEntry[uint8, string]{
				{
					Entry: loadingcache.Entry[uint8, string]{
						Key:   1,
						Value: "value1",
					},
					ExpiresAt: time.Date(2025, 1, 1, 2, 30, 45, 0, time.UTC),
				},
				{
					Entry: loadingcache.Entry[uint8, string]{
						Key:   2,
						Value: "value2",
					},
					ExpiresAt: time.Date(2025, 1, 1, 3, 30, 45, 0, time.UTC),
				},
			},
		},
		{
			name: "GetOrLoadMulti returns values from cache",
			keys: []uint8{1, 2},
			storageGetMulti: func(_ context.Context, keys []uint8) ([]*loadingcache.CacheEntry[uint8, string], error) {
				entries := make([]*loadingcache.CacheEntry[uint8, string], len(keys))
				for i, key := range keys {
					if key == 1 {
						entries[i] = &loadingcache.CacheEntry[uint8, string]{Entry: loadingcache.Entry[uint8, string]{Key: key, Value: "value1"}, ExpiresAt: time.Date(2025, 1, 1, 2, 30, 45, 0, time.UTC)}
					} else if key == 2 {
						entries[i] = &loadingcache.CacheEntry[uint8, string]{Entry: loadingcache.Entry[uint8, string]{Key: key, Value: "value2"}, ExpiresAt: time.Date(2025, 1, 1, 3, 30, 45, 0, time.UTC)}
					} else {
						entries[i] = nil
					}
				}
				return entries, nil
			},
			sourceGetMulti: func(_ context.Context, keys []uint8) ([]*loadingcache.CacheEntry[uint8, string], error) {
				return nil, errors.New("should not be called")
			},
			expectedEntries: []*loadingcache.Entry[uint8, string]{
				{Key: 1, Value: "value1"},
				{Key: 2, Value: "value2"},
			},
			expectedError:  nil,
			expectedStored: []*loadingcache.CacheEntry[uint8, string]{},
		},
		{
			name: "GetOrLoadMulti handles partial cache miss",
			keys: []uint8{1, 2, 3},
			storageGetMulti: func(_ context.Context, keys []uint8) ([]*loadingcache.CacheEntry[uint8, string], error) {
				entries := make([]*loadingcache.CacheEntry[uint8, string], len(keys))
				for i, key := range keys {
					if key == 1 {
						entries[i] = &loadingcache.CacheEntry[uint8, string]{Entry: loadingcache.Entry[uint8, string]{Key: key, Value: "value1"}, ExpiresAt: time.Date(2025, 1, 1, 3, 30, 45, 0, time.UTC)}
					} else {
						entries[i] = nil
					}
				}
				return entries, nil
			},
			sourceGetMulti: func(_ context.Context, keys []uint8) ([]*loadingcache.CacheEntry[uint8, string], error) {
				entries := make([]*loadingcache.CacheEntry[uint8, string], len(keys))
				for i, key := range keys {
					if key == 2 {
						entries[i] = &loadingcache.CacheEntry[uint8, string]{Entry: loadingcache.Entry[uint8, string]{Key: key, Value: "value2"}, ExpiresAt: time.Date(2025, 1, 1, 3, 30, 45, 0, time.UTC)}
					} else if key == 3 {
						entries[i] = &loadingcache.CacheEntry[uint8, string]{Entry: loadingcache.Entry[uint8, string]{Key: key, Value: "value3"}, ExpiresAt: time.Date(2025, 1, 1, 4, 30, 45, 0, time.UTC)}
					} else {
						entries[i] = nil
					}
				}
				return entries, nil
			},
			expectedEntries: []*loadingcache.Entry[uint8, string]{
				{Key: 1, Value: "value1"},
				{Key: 2, Value: "value2"},
				{Key: 3, Value: "value3"},
			},
			expectedError: nil,
			expectedStored: []*loadingcache.CacheEntry[uint8, string]{
				{
					Entry: loadingcache.Entry[uint8, string]{
						Key:   2,
						Value: "value2",
					},
					ExpiresAt: time.Date(2025, 1, 1, 3, 30, 45, 0, time.UTC),
				},
				{
					Entry: loadingcache.Entry[uint8, string]{
						Key:   3,
						Value: "value3",
					},
					ExpiresAt: time.Date(2025, 1, 1, 4, 30, 45, 0, time.UTC),
				},
			},
		},
		{
			name: "GetOrLoadMulti handles partial cache miss with negative cache",
			keys: []uint8{1, 2, 3, 4, 5},
			storageGetMulti: func(_ context.Context, keys []uint8) ([]*loadingcache.CacheEntry[uint8, string], error) {
				entries := make([]*loadingcache.CacheEntry[uint8, string], len(keys))
				for i, key := range keys {
					if key == 1 {
						entries[i] = &loadingcache.CacheEntry[uint8, string]{Entry: loadingcache.Entry[uint8, string]{Key: key, Value: "value1"}, ExpiresAt: time.Date(2025, 1, 1, 3, 30, 45, 0, time.UTC)}
					} else if key == 2 {
						entries[i] = &loadingcache.CacheEntry[uint8, string]{Entry: loadingcache.Entry[uint8, string]{Key: key}, NegativeCache: true, ExpiresAt: time.Date(2025, 1, 1, 3, 30, 45, 0, time.UTC)}
					} else {
						entries[i] = nil
					}
				}
				return entries, nil
			},
			sourceGetMulti: func(_ context.Context, keys []uint8) ([]*loadingcache.CacheEntry[uint8, string], error) {
				entries := make([]*loadingcache.CacheEntry[uint8, string], len(keys))
				for i, key := range keys {
					if key == 2 {
						entries[i] = &loadingcache.CacheEntry[uint8, string]{Entry: loadingcache.Entry[uint8, string]{Key: key, Value: "value2"}, ExpiresAt: time.Date(2025, 1, 1, 3, 30, 45, 0, time.UTC)}
					} else if key == 3 {
						entries[i] = &loadingcache.CacheEntry[uint8, string]{Entry: loadingcache.Entry[uint8, string]{Key: key, Value: "value3"}, ExpiresAt: time.Date(2025, 1, 1, 4, 30, 45, 0, time.UTC)}
					} else if key == 4 {
						entries[i] = &loadingcache.CacheEntry[uint8, string]{Entry: loadingcache.Entry[uint8, string]{Key: key}, NegativeCache: true, ExpiresAt: time.Date(2025, 1, 1, 4, 30, 45, 0, time.UTC)}
					} else {
						entries[i] = nil
					}
				}
				return entries, nil
			},
			expectedEntries: []*loadingcache.Entry[uint8, string]{
				{Key: 1, Value: "value1"},
				nil,
				{Key: 3, Value: "value3"},
				nil,
				nil,
			},
			expectedError: nil,
			expectedStored: []*loadingcache.CacheEntry[uint8, string]{
				{
					Entry: loadingcache.Entry[uint8, string]{
						Key:   3,
						Value: "value3",
					},
					ExpiresAt: time.Date(2025, 1, 1, 4, 30, 45, 0, time.UTC),
				},
				{
					Entry: loadingcache.Entry[uint8, string]{
						Key: 4,
					},
					NegativeCache: true,
					ExpiresAt:     time.Date(2025, 1, 1, 4, 30, 45, 0, time.UTC),
				},
			},
		},
		{
			name: "GetOrLoadMulti returns error from cache",
			keys: []uint8{1, 2},
			storageGetMulti: func(_ context.Context, keys []uint8) ([]*loadingcache.CacheEntry[uint8, string], error) {
				return nil, storageErr
			},
			sourceGetMulti: func(_ context.Context, keys []uint8) ([]*loadingcache.CacheEntry[uint8, string], error) {
				return nil, sourceErr
			},
			expectedEntries: nil,
			expectedError:   storageErr,
			expectedStored:  []*loadingcache.CacheEntry[uint8, string]{},
		},
		{
			name: "GetOrLoadMulti returns error from source",
			keys: []uint8{1, 2},
			storageGetMulti: func(_ context.Context, keys []uint8) ([]*loadingcache.CacheEntry[uint8, string], error) {
				entries := make([]*loadingcache.CacheEntry[uint8, string], len(keys))
				return entries, nil
			},
			sourceGetMulti: func(_ context.Context, keys []uint8) ([]*loadingcache.CacheEntry[uint8, string], error) {
				return nil, sourceErr
			},
			expectedEntries: nil,
			expectedError:   sourceErr,
			expectedStored:  []*loadingcache.CacheEntry[uint8, string]{},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mu := sync.Mutex{}
			entries := make([]*loadingcache.CacheEntry[uint8, string], 0, len(tt.keys))
			mockStorage := &storage.FunctionsStorage[uint8, string]{
				GetMultiFunc: tt.storageGetMulti,
				SetMultiFunc: func(_ context.Context, e []*loadingcache.CacheEntry[uint8, string]) error {
					mu.Lock()
					defer mu.Unlock()
					for _, entry := range e {
						if entry != nil {
							entries = append(entries, entry)
						}
					}
					return nil
				},
			}

			src := &source.FunctionsSource[uint8, string]{
				GetMultiFunc: tt.sourceGetMulti,
			}

			loader := pureloader.NewPureLoader(mockStorage, src)
			loadingCache := loadingcache.LoadingCache[uint8, string]{
				Loader:  loader,
				Storage: mockStorage,
			}

			result, err := loadingCache.GetOrLoadMulti(t.Context(), tt.keys)
			if err != tt.expectedError {
				t.Errorf("expected error: %v, got: %v", tt.expectedError, err)
			}
			if df := cmp.Diff(tt.expectedEntries, result); df != "" {
				t.Errorf("unexpected got entries: %s", df)
			}

			if df := cmp.Diff(tt.expectedStored, entries); df != "" {
				t.Errorf("unexpected stored entries: %s", df)
			}
		})
	}
}
