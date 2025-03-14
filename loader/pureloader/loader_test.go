package pureloader_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	loadingcache "github.com/karupanerura/loading-cache"
	"github.com/karupanerura/loading-cache/loader/pureloader"
	"github.com/karupanerura/loading-cache/source"
	"github.com/karupanerura/loading-cache/storage"
)

func TestLoadAndStore(t *testing.T) {
	sourceErr := errors.New("source error")
	tests := []struct {
		name       string
		source     *source.FunctionsSource[int, string]
		key        int
		wantEntry  *loadingcache.Entry[int, string]
		wantErr    error
		wantStored []*loadingcache.CacheEntry[int, string]
	}{
		{
			name: "successful load and store",
			source: &source.FunctionsSource[int, string]{
				GetFunc: func(_ context.Context, i int) (*loadingcache.CacheEntry[int, string], error) {
					if i == 1 {
						return &loadingcache.CacheEntry[int, string]{
							Entry: loadingcache.Entry[int, string]{
								Key:   i,
								Value: "testValue",
							},
							ExpiresAt: time.Date(2025, time.January, 1, 1, 30, 30, 0, time.UTC),
						}, nil
					}
					return nil, nil
				},
			},
			key: 1,
			wantEntry: &loadingcache.Entry[int, string]{
				Key:   1,
				Value: "testValue",
			},
			wantErr: nil,
			wantStored: []*loadingcache.CacheEntry[int, string]{
				{
					Entry: loadingcache.Entry[int, string]{
						Key:   1,
						Value: "testValue",
					},
					ExpiresAt: time.Date(2025, time.January, 1, 1, 30, 30, 0, time.UTC),
				},
			},
		},
		{
			name: "error from source",
			source: &source.FunctionsSource[int, string]{
				GetFunc: func(_ context.Context, i int) (*loadingcache.CacheEntry[int, string], error) {
					return nil, sourceErr
				},
			},
			key:        1,
			wantEntry:  nil,
			wantErr:    sourceErr,
			wantStored: nil,
		},
		{
			name: "key not found",
			source: &source.FunctionsSource[int, string]{
				GetFunc: func(_ context.Context, i int) (*loadingcache.CacheEntry[int, string], error) {
					return nil, nil
				},
			},
			key:        1,
			wantEntry:  nil,
			wantErr:    nil,
			wantStored: nil,
		},
		{
			name: "negative cache",
			source: &source.FunctionsSource[int, string]{
				GetFunc: func(_ context.Context, i int) (*loadingcache.CacheEntry[int, string], error) {
					return &loadingcache.CacheEntry[int, string]{
						Entry: loadingcache.Entry[int, string]{
							Key:   i,
							Value: "",
						},
						ExpiresAt:     time.Date(2025, time.January, 1, 1, 30, 30, 0, time.UTC),
						NegativeCache: true,
					}, nil
				},
			},
			key:       1,
			wantEntry: nil,
			wantErr:   nil,
			wantStored: []*loadingcache.CacheEntry[int, string]{
				{
					Entry: loadingcache.Entry[int, string]{
						Key:   1,
						Value: "",
					},
					ExpiresAt:     time.Date(2025, time.January, 1, 1, 30, 30, 0, time.UTC),
					NegativeCache: true,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stored []*loadingcache.CacheEntry[int, string]
			storage := &storage.FunctionsStorage[int, string]{
				SetFunc: func(_ context.Context, entry *loadingcache.CacheEntry[int, string]) error {
					stored = append(stored, entry)
					return nil
				},
			}

			loader := pureloader.NewPureLoader(storage, tt.source)
			gotEntry, gotErr := loader.LoadAndStore(t.Context(), tt.key)
			if tt.wantErr == nil && gotErr != nil {
				t.Fatal(gotErr)
			} else if tt.wantErr != nil && !errors.Is(gotErr, tt.wantErr) {
				t.Errorf("unexpected error: %v (expected: %v)", gotErr, tt.wantErr)
			}

			if diff := cmp.Diff(tt.wantEntry, gotEntry); diff != "" {
				t.Errorf("unexpected entry (-want +got):\n%s", diff)
			}

			if diff := cmp.Diff(tt.wantStored, stored); diff != "" {
				t.Errorf("unexpected stored entries (-want +got):\n%s", diff)
			}
		})
	}
}

func TestLoadAndStore_SetError(t *testing.T) {
	cacheErr := errors.New("cache error")
	source := &source.FunctionsSource[int, string]{
		GetFunc: func(_ context.Context, i int) (*loadingcache.CacheEntry[int, string], error) {
			if i == 1 {
				return &loadingcache.CacheEntry[int, string]{
					Entry: loadingcache.Entry[int, string]{
						Key:   i,
						Value: "testValue",
					},
					ExpiresAt: time.Date(2025, time.January, 1, 1, 30, 30, 0, time.UTC),
				}, nil
			}
			return nil, nil
		},
	}
	storage := &storage.FunctionsStorage[int, string]{
		SetFunc: func(_ context.Context, entry *loadingcache.CacheEntry[int, string]) error {
			return cacheErr
		},
	}

	loader := pureloader.NewPureLoader(storage, source)
	gotEntry, gotErr := loader.LoadAndStore(t.Context(), 1)
	if !errors.Is(gotErr, cacheErr) {
		t.Errorf("unexpected error: %v (expected: %v)", gotErr, cacheErr)
	}

	if gotEntry != nil {
		t.Errorf("unexpected entry: %v (expected: nil)", gotEntry)
	}
}

func TestLoadAndStoreMulti(t *testing.T) {
	sourceErr := errors.New("source error")
	tests := []struct {
		name        string
		source      *source.FunctionsSource[int, string]
		keys        []int
		wantEntries []*loadingcache.Entry[int, string]
		wantErr     error
		wantStored  []*loadingcache.CacheEntry[int, string]
	}{
		{
			name: "successful load and store multi",
			source: &source.FunctionsSource[int, string]{
				GetMultiFunc: func(_ context.Context, keys []int) ([]*loadingcache.CacheEntry[int, string], error) {
					entries := make([]*loadingcache.CacheEntry[int, string], len(keys))
					for i, key := range keys {
						if key == 1 {
							entries[i] = &loadingcache.CacheEntry[int, string]{
								Entry: loadingcache.Entry[int, string]{
									Key:   key,
									Value: "testValue1",
								},
								ExpiresAt: time.Date(2025, time.January, 1, 1, 30, 30, 0, time.UTC),
							}
						} else if key == 2 {
							entries[i] = &loadingcache.CacheEntry[int, string]{
								Entry: loadingcache.Entry[int, string]{
									Key:   key,
									Value: "testValue2",
								},
								ExpiresAt: time.Date(2025, time.January, 1, 2, 30, 30, 0, time.UTC),
							}
						}
					}
					return entries, nil
				},
			},
			keys: []int{1, 2},
			wantEntries: []*loadingcache.Entry[int, string]{
				{
					Key:   1,
					Value: "testValue1",
				},
				{
					Key:   2,
					Value: "testValue2",
				},
			},
			wantErr: nil,
			wantStored: []*loadingcache.CacheEntry[int, string]{
				{
					Entry: loadingcache.Entry[int, string]{
						Key:   1,
						Value: "testValue1",
					},
					ExpiresAt: time.Date(2025, time.January, 1, 1, 30, 30, 0, time.UTC),
				},
				{
					Entry: loadingcache.Entry[int, string]{
						Key:   2,
						Value: "testValue2",
					},
					ExpiresAt: time.Date(2025, time.January, 1, 2, 30, 30, 0, time.UTC),
				},
			},
		},
		{
			name: "partially successful load and store multi",
			source: &source.FunctionsSource[int, string]{
				GetMultiFunc: func(_ context.Context, keys []int) ([]*loadingcache.CacheEntry[int, string], error) {
					entries := make([]*loadingcache.CacheEntry[int, string], len(keys))
					for i, key := range keys {
						if key == 1 {
							entries[i] = &loadingcache.CacheEntry[int, string]{
								Entry: loadingcache.Entry[int, string]{
									Key:   key,
									Value: "testValue1",
								},
								ExpiresAt: time.Date(2025, time.January, 1, 1, 30, 30, 0, time.UTC),
							}
						}
					}
					return entries, nil
				},
			},
			keys: []int{1, 2},
			wantEntries: []*loadingcache.Entry[int, string]{
				{
					Key:   1,
					Value: "testValue1",
				},
				nil,
			},
			wantErr: nil,
			wantStored: []*loadingcache.CacheEntry[int, string]{
				{
					Entry: loadingcache.Entry[int, string]{
						Key:   1,
						Value: "testValue1",
					},
					ExpiresAt: time.Date(2025, time.January, 1, 1, 30, 30, 0, time.UTC),
				},
				nil,
			},
		},
		{
			name: "error from source",
			source: &source.FunctionsSource[int, string]{
				GetMultiFunc: func(_ context.Context, keys []int) ([]*loadingcache.CacheEntry[int, string], error) {
					return nil, sourceErr
				},
			},
			keys:        []int{1, 2},
			wantEntries: nil,
			wantErr:     sourceErr,
			wantStored:  nil,
		},
		{
			name: "with negative cache",
			source: &source.FunctionsSource[int, string]{
				GetMultiFunc: func(_ context.Context, keys []int) ([]*loadingcache.CacheEntry[int, string], error) {
					entries := make([]*loadingcache.CacheEntry[int, string], len(keys))
					for i, key := range keys {
						if key == 1 {
							entries[i] = &loadingcache.CacheEntry[int, string]{
								Entry: loadingcache.Entry[int, string]{
									Key:   key,
									Value: "testValue1",
								},
								ExpiresAt: time.Date(2025, time.January, 1, 1, 30, 30, 0, time.UTC),
							}
						} else if key == 2 {
							entries[i] = &loadingcache.CacheEntry[int, string]{
								Entry: loadingcache.Entry[int, string]{
									Key:   key,
									Value: "",
								},
								ExpiresAt:     time.Date(2025, time.January, 1, 2, 30, 30, 0, time.UTC),
								NegativeCache: true,
							}
						}
					}
					return entries, nil
				},
			},
			keys: []int{1, 2},
			wantEntries: []*loadingcache.Entry[int, string]{
				{
					Key:   1,
					Value: "testValue1",
				},
				nil,
			},
			wantErr: nil,
			wantStored: []*loadingcache.CacheEntry[int, string]{
				{
					Entry: loadingcache.Entry[int, string]{
						Key:   1,
						Value: "testValue1",
					},
					ExpiresAt: time.Date(2025, time.January, 1, 1, 30, 30, 0, time.UTC),
				},
				{
					Entry: loadingcache.Entry[int, string]{
						Key:   2,
						Value: "",
					},
					ExpiresAt:     time.Date(2025, time.January, 1, 2, 30, 30, 0, time.UTC),
					NegativeCache: true,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stored []*loadingcache.CacheEntry[int, string]
			storage := &storage.FunctionsStorage[int, string]{
				SetMultiFunc: func(_ context.Context, entries []*loadingcache.CacheEntry[int, string]) error {
					stored = entries
					return nil
				},
			}

			loader := pureloader.NewPureLoader(storage, tt.source)
			gotEntries, gotErr := loader.LoadAndStoreMulti(t.Context(), tt.keys)
			if tt.wantErr == nil && gotErr != nil {
				t.Fatal(gotErr)
			} else if tt.wantErr != nil && !errors.Is(gotErr, tt.wantErr) {
				t.Errorf("unexpected error: %v (expected: %v)", gotErr, tt.wantErr)
			}

			if diff := cmp.Diff(tt.wantEntries, gotEntries); diff != "" {
				t.Errorf("unexpected entries (-want +got):\n%s", diff)
			}

			if diff := cmp.Diff(tt.wantStored, stored); diff != "" {
				t.Errorf("unexpected stored entries (-want +got):\n%s", diff)
			}
		})
	}
}

func TestLoadAndStoreMulti_SetMultiError(t *testing.T) {
	cacheErr := errors.New("cache error")
	source := &source.FunctionsSource[int, string]{
		GetMultiFunc: func(_ context.Context, keys []int) ([]*loadingcache.CacheEntry[int, string], error) {
			entries := make([]*loadingcache.CacheEntry[int, string], len(keys))
			for i, key := range keys {
				if key == 1 {
					entries[i] = &loadingcache.CacheEntry[int, string]{
						Entry: loadingcache.Entry[int, string]{
							Key:   key,
							Value: "testValue1",
						},
						ExpiresAt: time.Date(2025, time.January, 1, 1, 30, 30, 0, time.UTC),
					}
				} else if key == 2 {
					entries[i] = &loadingcache.CacheEntry[int, string]{
						Entry: loadingcache.Entry[int, string]{
							Key:   key,
							Value: "testValue2",
						},
						ExpiresAt: time.Date(2025, time.January, 1, 2, 30, 30, 0, time.UTC),
					}
				}
			}
			return entries, nil
		},
	}
	storage := &storage.FunctionsStorage[int, string]{
		SetMultiFunc: func(_ context.Context, entries []*loadingcache.CacheEntry[int, string]) error {
			return cacheErr
		},
	}

	loader := pureloader.NewPureLoader(storage, source)
	gotValues, gotErr := loader.LoadAndStoreMulti(t.Context(), []int{1, 2})
	if !errors.Is(gotErr, cacheErr) {
		t.Errorf("unexpected error: %v (expected: %v)", gotErr, cacheErr)
	}

	if gotValues != nil {
		t.Errorf("unexpected values: %v (expected: nil)", gotValues)
	}
}
