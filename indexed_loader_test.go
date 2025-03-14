package loadingcache_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	loadingcache "github.com/karupanerura/loading-cache"
	"github.com/karupanerura/loading-cache/index"
	"github.com/karupanerura/loading-cache/loader/pureloader"
	"github.com/karupanerura/loading-cache/source"
	"github.com/karupanerura/loading-cache/storage"
)

var (
	errStorage = errors.New("storage error")
	errIndex   = errors.New("index error")
	errLoader  = errors.New("loader error")
)

func TestIndexedLoadingCache_FindBySecondaryKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                    string
		secondaryKey            string
		indexGet                func(context.Context, string) ([]int, error)
		storageGetMulti         func(context.Context, []int) ([]*loadingcache.CacheEntry[int, string], error)
		loaderLoadAndStoreMulti func(context.Context, []int) ([]*loadingcache.Entry[int, string], error)
		expected                []*loadingcache.Entry[int, string]
		expectedErr             error
	}{
		{
			name:         "successful lookup - all from storage",
			secondaryKey: "category1",
			indexGet: func(_ context.Context, key string) ([]int, error) {
				if key == "category1" {
					return []int{1, 2}, nil
				}
				return nil, errIndex
			},
			storageGetMulti: func(_ context.Context, keys []int) ([]*loadingcache.CacheEntry[int, string], error) {
				entries := make([]*loadingcache.CacheEntry[int, string], len(keys))
				for i, key := range keys {
					switch key {
					case 1:
						entries[i] = &loadingcache.CacheEntry[int, string]{Entry: loadingcache.Entry[int, string]{Key: 1, Value: "value1"}}
					case 2:
						entries[i] = &loadingcache.CacheEntry[int, string]{Entry: loadingcache.Entry[int, string]{Key: 2, Value: "value2"}}
					}
				}
				return entries, nil
			},
			loaderLoadAndStoreMulti: func(_ context.Context, keys []int) ([]*loadingcache.Entry[int, string], error) {
				return nil, errors.New("should not be called")
			},
			expected: []*loadingcache.Entry[int, string]{
				{Key: 1, Value: "value1"},
				{Key: 2, Value: "value2"},
			},
		},
		{
			name:         "successful lookup - partial from loader",
			secondaryKey: "category2",
			indexGet: func(_ context.Context, key string) ([]int, error) {
				if key == "category2" {
					return []int{3, 4}, nil
				}
				return nil, errIndex
			},
			storageGetMulti: func(_ context.Context, keys []int) ([]*loadingcache.CacheEntry[int, string], error) {
				entries := make([]*loadingcache.CacheEntry[int, string], len(keys))
				for i, key := range keys {
					if key == 3 {
						entries[i] = &loadingcache.CacheEntry[int, string]{Entry: loadingcache.Entry[int, string]{Key: 3, Value: "value3"}}
					}
				}
				return entries, nil
			},
			loaderLoadAndStoreMulti: func(_ context.Context, keys []int) ([]*loadingcache.Entry[int, string], error) {
				entries := make([]*loadingcache.Entry[int, string], len(keys))
				for i, key := range keys {
					if key == 4 {
						entries[i] = &loadingcache.Entry[int, string]{Key: 4, Value: "value4"}
					}
				}
				return entries, nil
			},
			expected: []*loadingcache.Entry[int, string]{
				{Key: 3, Value: "value3"},
				{Key: 4, Value: "value4"},
			},
		},
		{
			name:         "index error",
			secondaryKey: "error",
			indexGet: func(_ context.Context, key string) ([]int, error) {
				return nil, errIndex
			},
			expectedErr: errIndex,
		},
		{
			name:         "empty results",
			secondaryKey: "empty",
			indexGet: func(_ context.Context, key string) ([]int, error) {
				return []int{}, nil
			},
			expected: nil,
		},
		{
			name:         "storage error",
			secondaryKey: "storage-error",
			indexGet: func(_ context.Context, key string) ([]int, error) {
				return []int{1, 2}, nil
			},
			storageGetMulti: func(_ context.Context, keys []int) ([]*loadingcache.CacheEntry[int, string], error) {
				return nil, errStorage
			},
			expectedErr: errStorage,
		},
		{
			name:         "loader error",
			secondaryKey: "loader-error",
			indexGet: func(_ context.Context, key string) ([]int, error) {
				return []int{5}, nil
			},
			storageGetMulti: func(_ context.Context, keys []int) ([]*loadingcache.CacheEntry[int, string], error) {
				return make([]*loadingcache.CacheEntry[int, string], len(keys)), nil
			},
			loaderLoadAndStoreMulti: func(_ context.Context, keys []int) ([]*loadingcache.Entry[int, string], error) {
				return nil, errLoader
			},
			expectedErr: errLoader,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			idx := &index.FunctionsIndex[string, int]{
				GetFunc: tt.indexGet,
			}

			// Create a proper storage implementation
			s := &storage.FunctionsStorage[int, string]{
				GetMultiFunc: func(ctx context.Context, keys []int) ([]*loadingcache.CacheEntry[int, string], error) {
					if tt.storageGetMulti == nil {
						return make([]*loadingcache.CacheEntry[int, string], len(keys)), nil
					}
					return tt.storageGetMulti(ctx, keys)
				},
				SetFunc: func(_ context.Context, entry *loadingcache.CacheEntry[int, string]) error {
					return nil
				},
				SetMultiFunc: func(_ context.Context, entries []*loadingcache.CacheEntry[int, string]) error {
					return nil
				},
			}

			// Create a proper source implementation
			src := &source.FunctionsSource[int, string]{
				GetMultiFunc: func(ctx context.Context, keys []int) ([]*loadingcache.CacheEntry[int, string], error) {
					if tt.loaderLoadAndStoreMulti == nil {
						return make([]*loadingcache.CacheEntry[int, string], len(keys)), nil
					}

					entries, err := tt.loaderLoadAndStoreMulti(ctx, keys)
					if err != nil {
						return nil, err
					}

					cacheEntries := make([]*loadingcache.CacheEntry[int, string], len(entries))
					for i, entry := range entries {
						if entry != nil {
							cacheEntries[i] = &loadingcache.CacheEntry[int, string]{
								Entry:     *entry,
								ExpiresAt: time.Now().Add(time.Hour),
							}
						}
					}
					return cacheEntries, nil
				},
			}

			cache := loadingcache.NewIndexedLoadingCache(loadingcache.LoadingCache[int, string]{
				Loader:  pureloader.NewPureLoader(s, src),
				Storage: s,
			}, idx)

			result, err := cache.FindBySecondaryKey(context.Background(), tt.secondaryKey)

			if tt.expectedErr != nil {
				if err == nil || !errors.Is(err, tt.expectedErr) {
					t.Errorf("expected error %v, got %v", tt.expectedErr, err)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if diff := cmp.Diff(tt.expected, result); diff != "" {
				t.Errorf("unexpected result (-want +got):\n%s", diff)
			}
		})
	}
}

func TestIndexedLoadingCache_FindBySecondaryKeys(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                    string
		secondaryKeys           []string
		indexGetMulti           func(context.Context, []string) (map[string][]int, error)
		storageGetMulti         func(context.Context, []int) ([]*loadingcache.CacheEntry[int, string], error)
		loaderLoadAndStoreMulti func(context.Context, []int) ([]*loadingcache.Entry[int, string], error)
		expected                map[string][]*loadingcache.Entry[int, string]
		expectedErr             error
	}{
		{
			name:          "successful multi lookup",
			secondaryKeys: []string{"category1", "category2"},
			indexGetMulti: func(_ context.Context, keys []string) (map[string][]int, error) {
				result := map[string][]int{
					"category1": {1, 2},
					"category2": {2, 3},
				}
				return result, nil
			},
			storageGetMulti: func(_ context.Context, keys []int) ([]*loadingcache.CacheEntry[int, string], error) {
				entries := make([]*loadingcache.CacheEntry[int, string], len(keys))
				for i, key := range keys {
					switch key {
					case 1:
						entries[i] = &loadingcache.CacheEntry[int, string]{Entry: loadingcache.Entry[int, string]{Key: 1, Value: "value1"}}
					case 2:
						entries[i] = &loadingcache.CacheEntry[int, string]{Entry: loadingcache.Entry[int, string]{Key: 2, Value: "value2"}}
					case 3:
						entries[i] = &loadingcache.CacheEntry[int, string]{Entry: loadingcache.Entry[int, string]{Key: 3, Value: "value3"}}
					}
				}
				return entries, nil
			},
			expected: map[string][]*loadingcache.Entry[int, string]{
				"category1": {
					{Key: 1, Value: "value1"},
					{Key: 2, Value: "value2"},
				},
				"category2": {
					{Key: 2, Value: "value2"},
					{Key: 3, Value: "value3"},
				},
			},
		},
		{
			name:          "partial lookup with loader",
			secondaryKeys: []string{"category1", "category3"},
			indexGetMulti: func(_ context.Context, keys []string) (map[string][]int, error) {
				result := map[string][]int{
					"category1": {1, 2},
					"category3": {4, 5},
				}
				return result, nil
			},
			storageGetMulti: func(_ context.Context, keys []int) ([]*loadingcache.CacheEntry[int, string], error) {
				entries := make([]*loadingcache.CacheEntry[int, string], len(keys))
				for i, key := range keys {
					switch key {
					case 1:
						entries[i] = &loadingcache.CacheEntry[int, string]{Entry: loadingcache.Entry[int, string]{Key: 1, Value: "value1"}}
					case 2:
						entries[i] = &loadingcache.CacheEntry[int, string]{Entry: loadingcache.Entry[int, string]{Key: 2, Value: "value2"}}
					}
				}
				return entries, nil
			},
			loaderLoadAndStoreMulti: func(_ context.Context, keys []int) ([]*loadingcache.Entry[int, string], error) {
				entries := make([]*loadingcache.Entry[int, string], len(keys))
				for i, key := range keys {
					switch key {
					case 4:
						entries[i] = &loadingcache.Entry[int, string]{Key: 4, Value: "value4"}
					case 5:
						entries[i] = &loadingcache.Entry[int, string]{Key: 5, Value: "value5"}
					}
				}
				return entries, nil
			},
			expected: map[string][]*loadingcache.Entry[int, string]{
				"category1": {
					{Key: 1, Value: "value1"},
					{Key: 2, Value: "value2"},
				},
				"category3": {
					{Key: 4, Value: "value4"},
					{Key: 5, Value: "value5"},
				},
			},
		},
		{
			name:          "index error",
			secondaryKeys: []string{"error", "category1"},
			indexGetMulti: func(_ context.Context, keys []string) (map[string][]int, error) {
				return nil, errIndex
			},
			expectedErr: errIndex,
		},
		{
			name:          "empty results",
			secondaryKeys: []string{"empty1", "empty2"},
			indexGetMulti: func(_ context.Context, keys []string) (map[string][]int, error) {
				return map[string][]int{}, nil
			},
			expected: map[string][]*loadingcache.Entry[int, string]{},
		},
		{
			name:          "storage error",
			secondaryKeys: []string{"category1", "category2"},
			indexGetMulti: func(_ context.Context, keys []string) (map[string][]int, error) {
				result := map[string][]int{
					"category1": {1, 2},
					"category2": {3, 4},
				}
				return result, nil
			},
			storageGetMulti: func(_ context.Context, keys []int) ([]*loadingcache.CacheEntry[int, string], error) {
				return nil, errStorage
			},
			expectedErr: errStorage,
		},
		{
			name:          "loader error",
			secondaryKeys: []string{"category1", "category2"},
			indexGetMulti: func(_ context.Context, keys []string) (map[string][]int, error) {
				result := map[string][]int{
					"category1": {1, 2},
					"category2": {3, 4},
				}
				return result, nil
			},
			storageGetMulti: func(_ context.Context, keys []int) ([]*loadingcache.CacheEntry[int, string], error) {
				entries := make([]*loadingcache.CacheEntry[int, string], len(keys))
				return entries, nil
			},
			loaderLoadAndStoreMulti: func(_ context.Context, keys []int) ([]*loadingcache.Entry[int, string], error) {
				return nil, errLoader
			},
			expectedErr: errLoader,
		},
		{
			name:          "partial primary key overlap",
			secondaryKeys: []string{"category1", "category2", "category3"},
			indexGetMulti: func(_ context.Context, keys []string) (map[string][]int, error) {
				result := map[string][]int{
					"category1": {1, 2},
					"category2": {2, 3},
					"category3": {3, 4},
				}
				return result, nil
			},
			storageGetMulti: func(_ context.Context, keys []int) ([]*loadingcache.CacheEntry[int, string], error) {
				entries := make([]*loadingcache.CacheEntry[int, string], len(keys))
				for i, key := range keys {
					switch key {
					case 1:
						entries[i] = &loadingcache.CacheEntry[int, string]{Entry: loadingcache.Entry[int, string]{Key: 1, Value: "value1"}}
					case 2:
						entries[i] = &loadingcache.CacheEntry[int, string]{Entry: loadingcache.Entry[int, string]{Key: 2, Value: "value2"}}
					case 3:
						entries[i] = &loadingcache.CacheEntry[int, string]{Entry: loadingcache.Entry[int, string]{Key: 3, Value: "value3"}}
					case 4:
						entries[i] = &loadingcache.CacheEntry[int, string]{Entry: loadingcache.Entry[int, string]{Key: 4, Value: "value4"}}
					}
				}
				return entries, nil
			},
			expected: map[string][]*loadingcache.Entry[int, string]{
				"category1": {
					{Key: 1, Value: "value1"},
					{Key: 2, Value: "value2"},
				},
				"category2": {
					{Key: 2, Value: "value2"},
					{Key: 3, Value: "value3"},
				},
				"category3": {
					{Key: 3, Value: "value3"},
					{Key: 4, Value: "value4"},
				},
			},
		},
		{
			name:          "nil values in results",
			secondaryKeys: []string{"category1", "category2"},
			indexGetMulti: func(_ context.Context, keys []string) (map[string][]int, error) {
				result := map[string][]int{
					"category1": {1},
					"category2": {2},
				}
				return result, nil
			},
			storageGetMulti: func(_ context.Context, keys []int) ([]*loadingcache.CacheEntry[int, string], error) {
				entries := make([]*loadingcache.CacheEntry[int, string], len(keys))
				return entries, nil
			},
			loaderLoadAndStoreMulti: func(_ context.Context, keys []int) ([]*loadingcache.Entry[int, string], error) {
				entries := make([]*loadingcache.Entry[int, string], len(keys))
				// Returning nil entries simulates keys that couldn't be loaded
				return entries, nil
			},
			expected: map[string][]*loadingcache.Entry[int, string]{
				// No entries added since all values were nil
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			idx := &index.FunctionsIndex[string, int]{
				GetMultiFunc: tt.indexGetMulti,
			}

			// Create a proper storage implementation
			s := &storage.FunctionsStorage[int, string]{
				GetMultiFunc: func(ctx context.Context, keys []int) ([]*loadingcache.CacheEntry[int, string], error) {
					if tt.storageGetMulti == nil {
						return make([]*loadingcache.CacheEntry[int, string], len(keys)), nil
					}
					return tt.storageGetMulti(ctx, keys)
				},
				SetFunc: func(_ context.Context, entry *loadingcache.CacheEntry[int, string]) error {
					return nil
				},
				SetMultiFunc: func(_ context.Context, entries []*loadingcache.CacheEntry[int, string]) error {
					return nil
				},
			}

			// Create a proper source implementation
			src := &source.FunctionsSource[int, string]{
				GetMultiFunc: func(ctx context.Context, keys []int) ([]*loadingcache.CacheEntry[int, string], error) {
					if tt.loaderLoadAndStoreMulti == nil {
						return make([]*loadingcache.CacheEntry[int, string], len(keys)), nil
					}

					entries, err := tt.loaderLoadAndStoreMulti(ctx, keys)
					if err != nil {
						return nil, err
					}

					cacheEntries := make([]*loadingcache.CacheEntry[int, string], len(entries))
					for i, entry := range entries {
						if entry != nil {
							cacheEntries[i] = &loadingcache.CacheEntry[int, string]{
								Entry:     *entry,
								ExpiresAt: time.Now().Add(time.Hour),
							}
						}
					}
					return cacheEntries, nil
				},
			}

			cache := loadingcache.NewIndexedLoadingCache(loadingcache.LoadingCache[int, string]{
				Loader:  pureloader.NewPureLoader(s, src),
				Storage: s,
			}, idx)

			result, err := cache.FindBySecondaryKeys(context.Background(), tt.secondaryKeys)

			if tt.expectedErr != nil {
				if err == nil || !errors.Is(err, tt.expectedErr) {
					t.Errorf("expected error %v, got %v", tt.expectedErr, err)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if diff := cmp.Diff(tt.expected, result, cmpopts.SortSlices(func(i, j *loadingcache.Entry[int, string]) bool {
				return i.Key < j.Key
			})); diff != "" {
				t.Errorf("unexpected result (-want +got):\n%s", diff)
			}
		})
	}
}
