package singleflightloader_test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	loadingcache "github.com/karupanerura/loading-cache"
	"github.com/karupanerura/loading-cache/loader/singleflightloader"
	"github.com/karupanerura/loading-cache/source"
	"github.com/karupanerura/loading-cache/storage"
)

func TestLoadAndStore_Parallel(t *testing.T) {
	t.Parallel()

	var callCount uint32
	source := &source.FunctionsSource[int, string]{
		GetFunc: func(_ context.Context, i int) (*loadingcache.CacheEntry[int, string], error) {
			time.Sleep(100 * time.Millisecond)
			atomic.AddUint32(&callCount, 1)
			return &loadingcache.CacheEntry[int, string]{
				Entry: loadingcache.Entry[int, string]{
					Key:   i,
					Value: "testValue",
				},
				ExpiresAt: time.Date(2025, time.January, 1, 1, 30, 30, 0, time.UTC),
			}, nil
		},
	}
	storage := &storage.FunctionsStorage[int, string]{
		SetFunc: func(_ context.Context, entry *loadingcache.CacheEntry[int, string]) error {
			return nil
		},
	}

	options := []singleflightloader.Option[int, string]{
		singleflightloader.WithCloner[int, string](loadingcache.NopValueCloner[string]{}),
		singleflightloader.WithBackgroundContextProvider[int, string](t.Context),
	}
	loader := singleflightloader.NewSingleFlightLoader(storage, source, options...)

	var exec sync.WaitGroup
	var wg sync.WaitGroup
	const numGoroutines = 3
	results := make([]*loadingcache.Entry[int, string], numGoroutines)
	errors := make([]error, numGoroutines)
	exec.Add(1)
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(index int) {
			defer wg.Done()
			exec.Wait()
			results[index], errors[index] = loader.LoadAndStore(t.Context(), 1)
		}(i)
	}

	time.Sleep(300 * time.Millisecond)
	exec.Done()
	wg.Wait()

	for i := 0; i < numGoroutines; i++ {
		if errors[i] != nil {
			t.Errorf("unexpected error: %v", errors[i])
		}
		if results[i].Value != "testValue" {
			t.Errorf("unexpected value: %v (expected: testValue)", results[i])
		}
	}

	if callCount != 1 {
		t.Errorf("expected source to be called once, but it was called %d times", callCount)
	}
}

func TestLoadAndStore_Parallel_EachKey(t *testing.T) {
	t.Parallel()

	var exec sync.WaitGroup

	var callCount uint32
	source := &source.FunctionsSource[int, string]{
		GetFunc: func(_ context.Context, i int) (*loadingcache.CacheEntry[int, string], error) {
			exec.Wait()
			atomic.AddUint32(&callCount, 1)
			return &loadingcache.CacheEntry[int, string]{
				Entry: loadingcache.Entry[int, string]{
					Key:   i,
					Value: "testValue",
				},
				ExpiresAt: time.Date(2025, time.January, 1, 1, 30, 30, 0, time.UTC),
			}, nil
		},
	}
	storage := &storage.FunctionsStorage[int, string]{
		SetFunc: func(_ context.Context, entry *loadingcache.CacheEntry[int, string]) error {
			return nil
		},
	}

	options := []singleflightloader.Option[int, string]{
		singleflightloader.WithCloner[int, string](loadingcache.NopValueCloner[string]{}),
		singleflightloader.WithBackgroundContextProvider[int, string](t.Context),
	}
	loader := singleflightloader.NewSingleFlightLoader(storage, source, options...)

	var wg sync.WaitGroup
	const numGoroutines = 3
	results := make([]*loadingcache.Entry[int, string], numGoroutines)
	errors := make([]error, numGoroutines)
	exec.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			exec.Done()
			results[index], errors[index] = loader.LoadAndStore(t.Context(), index)
		}(i)
	}

	wg.Wait()

	for i := 0; i < numGoroutines; i++ {
		if errors[i] != nil {
			t.Errorf("unexpected error: %v", errors[i])
		}
		if results[i].Value != "testValue" {
			t.Errorf("unexpected value: %v (expected: testValue)", results[i])
		}
	}

	if callCount != numGoroutines {
		t.Errorf("expected source to be called %d times, but it was called %d times", numGoroutines, callCount)
	}
}

func TestLoadAndStoreMulti_Parallel(t *testing.T) {
	t.Parallel()

	var callCount uint32
	source := &source.FunctionsSource[int, string]{
		GetMultiFunc: func(_ context.Context, keys []int) ([]*loadingcache.CacheEntry[int, string], error) {
			time.Sleep(100 * time.Millisecond)
			atomic.AddUint32(&callCount, 1)
			entries := make([]*loadingcache.CacheEntry[int, string], len(keys))
			for i, key := range keys {
				entries[i] = &loadingcache.CacheEntry[int, string]{
					Entry: loadingcache.Entry[int, string]{
						Key:   key,
						Value: fmt.Sprintf("value%d", key),
					},
					ExpiresAt: time.Date(2025, time.January, 1, 1, 30, 30, 0, time.UTC),
				}
			}
			return entries, nil
		},
	}
	storage := &storage.FunctionsStorage[int, string]{
		SetMultiFunc: func(_ context.Context, entries []*loadingcache.CacheEntry[int, string]) error {
			return nil
		},
	}

	options := []singleflightloader.Option[int, string]{
		singleflightloader.WithCloner[int, string](loadingcache.NopValueCloner[string]{}),
		singleflightloader.WithBackgroundContextProvider[int, string](t.Context),
	}
	loader := singleflightloader.NewSingleFlightLoader(storage, source, options...)

	var exec sync.WaitGroup
	var wg sync.WaitGroup
	const numGoroutines = 3
	const numKeys = 4
	results := make([][]*loadingcache.Entry[int, string], numGoroutines)
	errors := make([]error, numGoroutines)

	exec.Add(1)
	wg.Add(numGoroutines)

	// All goroutines will request the same set of keys
	requestKeys := []int{1, 2, 3, 4}

	for i := 0; i < numGoroutines; i++ {
		go func(index int) {
			defer wg.Done()
			exec.Wait()
			results[index], errors[index] = loader.LoadAndStoreMulti(t.Context(), requestKeys)
		}(i)
	}

	// Give enough time for all goroutines to reach the exec.Wait()
	time.Sleep(50 * time.Millisecond)
	exec.Done()
	wg.Wait()

	// Check for errors
	for i := 0; i < numGoroutines; i++ {
		if errors[i] != nil {
			t.Errorf("unexpected error in goroutine %d: %v", i, errors[i])
		}
	}

	// Verify all goroutines received the expected values
	for i := 0; i < numGoroutines; i++ {
		if len(results[i]) != numKeys {
			t.Errorf("expected %d results in goroutine %d, got %d", numKeys, i, len(results[i]))
			continue
		}

		for j, key := range requestKeys {
			expected := fmt.Sprintf("value%d", key)
			if results[i][j].Value != expected {
				t.Errorf("goroutine %d, key %d: expected value %q, got %q",
					i, key, expected, results[i][j].Value)
			}
		}
	}

	if callCount != 1 {
		t.Errorf("expected source to be called once, but it was called %d times", callCount)
	}
}

func TestLoadAndStoreMulti_Parallel_EachKeys(t *testing.T) {
	t.Parallel()

	var exec sync.WaitGroup
	var callCount uint32
	var keysCount uint32
	source := &source.FunctionsSource[int, string]{
		GetMultiFunc: func(_ context.Context, keys []int) ([]*loadingcache.CacheEntry[int, string], error) {
			exec.Wait()
			time.Sleep(100 * time.Millisecond)
			atomic.AddUint32(&callCount, 1)
			atomic.AddUint32(&keysCount, uint32(len(keys)))
			entries := make([]*loadingcache.CacheEntry[int, string], len(keys))
			for i, key := range keys {
				entries[i] = &loadingcache.CacheEntry[int, string]{
					Entry: loadingcache.Entry[int, string]{
						Key:   key,
						Value: fmt.Sprintf("value%d", key),
					},
					ExpiresAt: time.Date(2025, time.January, 1, 1, 30, 30, 0, time.UTC),
				}
			}
			return entries, nil
		},
	}

	storage := &storage.FunctionsStorage[int, string]{
		SetMultiFunc: func(_ context.Context, entries []*loadingcache.CacheEntry[int, string]) error {
			return nil
		},
	}

	options := []singleflightloader.Option[int, string]{
		singleflightloader.WithCloner[int, string](loadingcache.NopValueCloner[string]{}),
		singleflightloader.WithBackgroundContextProvider[int, string](t.Context),
	}
	loader := singleflightloader.NewSingleFlightLoader(storage, source, options...)

	var wg sync.WaitGroup
	const numGoroutines = 3

	// Each goroutine will request a different set of keys
	keySets := [][]int{
		{1, 2, 3, 4},
		{4, 5, 6, 7},
		{7, 8, 9, 10},
	}

	results := make([][]*loadingcache.Entry[int, string], numGoroutines)
	errors := make([]error, numGoroutines)

	exec.Add(1)
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(index int) {
			defer wg.Done()
			exec.Wait()
			results[index], errors[index] = loader.LoadAndStoreMulti(t.Context(), keySets[index])
		}(i)
	}

	// Allow a small delay for all goroutines to reach the exec.Wait()
	time.Sleep(500 * time.Millisecond)
	exec.Done()
	wg.Wait()

	// Check for errors
	for i := 0; i < numGoroutines; i++ {
		if errors[i] != nil {
			t.Errorf("unexpected error in goroutine %d: %v", i, errors[i])
		}
	}

	// Verify that all goroutines received the expected values
	for i := 0; i < numGoroutines; i++ {
		keys := keySets[i]

		if len(results[i]) != len(keys) {
			t.Errorf("expected %d results in goroutine %d, got %d", len(keys), i, len(results[i]))
			continue
		}

		for j, key := range keys {
			expected := fmt.Sprintf("value%d", key)
			if results[i][j].Value != expected {
				t.Errorf("goroutine %d, key %d: expected value %q, got %q",
					i, key, expected, results[i][j].Value)
			}
		}
	}

	// Since each goroutine requested different keys, the source should be called
	// once for each unique key (total number of keys across all requests)
	if keysCount != 10 {
		t.Errorf("expected source to be called for %d keys, but it was called for %d keys", 10, keysCount)
	}
	if callCount != 3 {
		t.Errorf("expected source to be called only 3 times, but it was called %d times", callCount)
	}
}
