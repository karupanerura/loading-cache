package singleflightloader_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	loadingcache "github.com/karupanerura/loading-cache"
	"github.com/karupanerura/loading-cache/loader/singleflightloader"
	"github.com/karupanerura/loading-cache/source"
	"github.com/karupanerura/loading-cache/storage"
)

func TestLoadAndStore(t *testing.T) {
	t.Parallel()

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
			name: "not found from source",
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
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var stored []*loadingcache.CacheEntry[int, string]
			storage := &storage.FunctionsStorage[int, string]{
				SetFunc: func(_ context.Context, entry *loadingcache.CacheEntry[int, string]) error {
					stored = append(stored, entry)
					return nil
				},
			}

			options := []singleflightloader.Option[int, string]{
				singleflightloader.WithCloner[int, string](loadingcache.NopValueCloner[string]{}),
				singleflightloader.WithBackgroundContextProvider[int, string](t.Context),
			}
			loader := singleflightloader.NewSingleFlightLoader(storage, tt.source, options...)
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
	t.Parallel()

	cacheErr := errors.New("cache error")
	src := &source.FunctionsSource[int, string]{
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
	store := &storage.FunctionsStorage[int, string]{
		SetFunc: func(_ context.Context, entry *loadingcache.CacheEntry[int, string]) error {
			return cacheErr
		},
	}

	options := []singleflightloader.Option[int, string]{
		singleflightloader.WithCloner[int, string](loadingcache.NopValueCloner[string]{}),
		singleflightloader.WithBackgroundContextProvider[int, string](t.Context),
	}
	loader := singleflightloader.NewSingleFlightLoader(store, src, options...)
	gotEntry, gotErr := loader.LoadAndStore(t.Context(), 1)
	if !errors.Is(gotErr, cacheErr) {
		t.Errorf("unexpected error: %v (expected: %v)", gotErr, cacheErr)
	}

	if gotEntry != nil {
		t.Errorf("unexpected entry: %v (expected: nil)", gotEntry)
	}
}

func TestLoadAndStore_ContextCancel(t *testing.T) {
	t.Parallel()

	src := &source.FunctionsSource[int, string]{
		GetFunc: func(_ context.Context, i int) (*loadingcache.CacheEntry[int, string], error) {
			time.Sleep(1 * time.Second)
			return nil, errors.New("storage err")
		},
	}
	store := &storage.FunctionsStorage[int, string]{
		SetFunc: func(_ context.Context, entry *loadingcache.CacheEntry[int, string]) error {
			return nil
		},
	}

	options := []singleflightloader.Option[int, string]{
		singleflightloader.WithCloner[int, string](loadingcache.NopValueCloner[string]{}),
		singleflightloader.WithBackgroundContextProvider[int, string](t.Context),
	}
	loader := singleflightloader.NewSingleFlightLoader(store, src, options...)

	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	_, err := loader.LoadAndStore(ctx, 1)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("unexpected error: %v (expected: context deadline exceeded)", err)
	}
}

func TestLoadAndStoreMulti(t *testing.T) {
	t.Parallel()

	sourceErr := errors.New("source error")
	tests := []struct {
		name       string
		source     *source.FunctionsSource[int, string]
		keys       []int
		wantValues []*loadingcache.Entry[int, string]
		wantErr    error
		wantStored []*loadingcache.CacheEntry[int, string]
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
			wantValues: []*loadingcache.Entry[int, string]{
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
			wantValues: []*loadingcache.Entry[int, string]{
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
			},
		},
		{
			name: "error from source",
			source: &source.FunctionsSource[int, string]{
				GetMultiFunc: func(_ context.Context, keys []int) ([]*loadingcache.CacheEntry[int, string], error) {
					return nil, sourceErr
				},
			},
			keys:       []int{1, 2},
			wantValues: nil,
			wantErr:    sourceErr,
			wantStored: nil,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var stored []*loadingcache.CacheEntry[int, string]
			storage := &storage.FunctionsStorage[int, string]{
				SetMultiFunc: func(_ context.Context, entries []*loadingcache.CacheEntry[int, string]) error {
					for _, e := range entries {
						if e != nil {
							stored = append(stored, e)
						}
					}
					return nil
				},
			}

			options := []singleflightloader.Option[int, string]{
				singleflightloader.WithCloner[int, string](loadingcache.NopValueCloner[string]{}),
				singleflightloader.WithBackgroundContextProvider[int, string](t.Context),
			}
			loader := singleflightloader.NewSingleFlightLoader(storage, tt.source, options...)
			gotValues, gotErr := loader.LoadAndStoreMulti(t.Context(), tt.keys)
			if tt.wantErr == nil && gotErr != nil {
				t.Fatal(gotErr)
			} else if tt.wantErr != nil && !errors.Is(gotErr, tt.wantErr) {
				t.Errorf("unexpected error: %v (expected: %v)", gotErr, tt.wantErr)
			}

			if diff := cmp.Diff(tt.wantValues, gotValues); diff != "" {
				t.Errorf("unexpected values (-want +got):\n%s", diff)
			}

			if diff := cmp.Diff(tt.wantStored, stored); diff != "" {
				t.Errorf("unexpected stored entries (-want +got):\n%s", diff)
			}
		})
	}
}

func TestLoadAndStoreMulti_SetMultiError(t *testing.T) {
	t.Parallel()

	cacheErr := errors.New("cache error")
	src := &source.FunctionsSource[int, string]{
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
	store := &storage.FunctionsStorage[int, string]{
		SetMultiFunc: func(_ context.Context, entries []*loadingcache.CacheEntry[int, string]) error {
			return cacheErr
		},
	}

	options := []singleflightloader.Option[int, string]{
		singleflightloader.WithCloner[int, string](loadingcache.NopValueCloner[string]{}),
		singleflightloader.WithBackgroundContextProvider[int, string](t.Context),
	}
	loader := singleflightloader.NewSingleFlightLoader(store, src, options...)
	gotValues, gotErr := loader.LoadAndStoreMulti(t.Context(), []int{1, 2})
	if !errors.Is(gotErr, cacheErr) {
		t.Errorf("unexpected error: %v (expected: %v)", gotErr, cacheErr)
	}

	if gotValues != nil {
		t.Errorf("unexpected values: %v (expected: nil)", gotValues)
	}
}

func TestLoadAndStoreMulti_ContextCancel(t *testing.T) {
	t.Parallel()

	src := &source.FunctionsSource[int, string]{
		GetMultiFunc: func(ctx context.Context, keys []int) ([]*loadingcache.CacheEntry[int, string], error) {
			time.Sleep(1 * time.Second)
			return nil, errors.New("storage err")
		},
	}
	store := &storage.FunctionsStorage[int, string]{
		SetFunc: func(_ context.Context, entry *loadingcache.CacheEntry[int, string]) error {
			return nil
		},
	}

	options := []singleflightloader.Option[int, string]{
		singleflightloader.WithCloner[int, string](loadingcache.NopValueCloner[string]{}),
		singleflightloader.WithBackgroundContextProvider[int, string](t.Context),
	}
	loader := singleflightloader.NewSingleFlightLoader(store, src, options...)

	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	_, err := loader.LoadAndStoreMulti(ctx, []int{1, 2, 3})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("unexpected error: %v (expected: context deadline exceeded)", err)
	}
}
