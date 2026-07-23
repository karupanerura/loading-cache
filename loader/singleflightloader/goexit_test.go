package singleflightloader_test

import (
	"context"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	loadingcache "github.com/karupanerura/loading-cache"
	"github.com/karupanerura/loading-cache/loader/singleflightloader"
	"github.com/karupanerura/loading-cache/source"
	"github.com/karupanerura/loading-cache/storage"
)

// TestLoadAndStore_GoexitInStorageSet verifies that runtime.Goexit inside
// CacheStorage.Set (e.g. a test storage calling t.Fatal) is propagated to the
// waiters just like runtime.Goexit inside LoadingSource.Get. If the loading
// goroutine dies without notifying the waitlist, the key is wedged forever:
// current waiters block until their context expires and all future
// LoadAndStore calls for the same key join the dead waitlist without ever
// starting a new load.
func TestLoadAndStore_GoexitInStorageSet(t *testing.T) {
	t.Parallel()

	var sourceCalls, setCalls atomic.Int32
	src := &source.FunctionsSource[int, string]{
		GetFunc: func(_ context.Context, key int) (*loadingcache.CacheEntry[int, string], error) {
			sourceCalls.Add(1)
			return &loadingcache.CacheEntry[int, string]{
				Entry:     loadingcache.Entry[int, string]{Key: key, Value: "testValue"},
				ExpiresAt: time.Now().Add(time.Hour),
			}, nil
		},
	}
	st := &storage.FunctionsStorage[int, string]{
		SetFunc: func(_ context.Context, _ *loadingcache.CacheEntry[int, string]) error {
			if setCalls.Add(1) == 1 {
				runtime.Goexit()
			}
			return nil
		},
	}

	options := []singleflightloader.Option[int, string]{
		singleflightloader.WithCloner[int, string](loadingcache.NopValueCloner[string]{}),
	}
	loader := singleflightloader.NewSingleFlightLoader(st, src, options...)

	// The first load dies via runtime.Goexit inside storage.Set. The waiter
	// must observe it as Goexit propagation instead of returning normally.
	firstDone := make(chan struct{})
	var firstReturned bool
	go func() {
		defer close(firstDone)
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_, _ = loader.LoadAndStore(ctx, 1)
		firstReturned = true
	}()
	<-firstDone
	if firstReturned {
		t.Error("LoadAndStore must not return normally when the storage calls runtime.Goexit")
	}

	// The same key must not be wedged: a subsequent call must start a fresh
	// load and succeed.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	entry, err := loader.LoadAndStore(ctx, 1)
	if err != nil {
		t.Fatalf("the key is wedged by the goexit-ed load: %v", err)
	}
	if entry == nil || entry.Value != "testValue" {
		t.Errorf("unexpected entry: %+v", entry)
	}
	if got := sourceCalls.Load(); got != 2 {
		t.Errorf("expected the source to be called twice, but it was called %d times", got)
	}
}

// TestLoadAndStoreMulti_GoexitInStorageSetMulti is the LoadAndStoreMulti
// variant of TestLoadAndStore_GoexitInStorageSet: runtime.Goexit inside
// CacheStorage.SetMulti must be propagated to all waiters and must not wedge
// the keys.
func TestLoadAndStoreMulti_GoexitInStorageSetMulti(t *testing.T) {
	t.Parallel()

	var sourceCalls, setCalls atomic.Int32
	src := &source.FunctionsSource[int, string]{
		GetMultiFunc: func(_ context.Context, keys []int) ([]*loadingcache.CacheEntry[int, string], error) {
			sourceCalls.Add(1)
			entries := make([]*loadingcache.CacheEntry[int, string], len(keys))
			for i, key := range keys {
				entries[i] = &loadingcache.CacheEntry[int, string]{
					Entry:     loadingcache.Entry[int, string]{Key: key, Value: "testValue"},
					ExpiresAt: time.Now().Add(time.Hour),
				}
			}
			return entries, nil
		},
	}
	st := &storage.FunctionsStorage[int, string]{
		SetMultiFunc: func(_ context.Context, _ []*loadingcache.CacheEntry[int, string]) error {
			if setCalls.Add(1) == 1 {
				runtime.Goexit()
			}
			return nil
		},
	}

	options := []singleflightloader.Option[int, string]{
		singleflightloader.WithCloner[int, string](loadingcache.NopValueCloner[string]{}),
	}
	loader := singleflightloader.NewSingleFlightLoader(st, src, options...)

	firstDone := make(chan struct{})
	var firstReturned bool
	go func() {
		defer close(firstDone)
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_, _ = loader.LoadAndStoreMulti(ctx, []int{1, 2})
		firstReturned = true
	}()
	<-firstDone
	if firstReturned {
		t.Error("LoadAndStoreMulti must not return normally when the storage calls runtime.Goexit")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	entries, err := loader.LoadAndStoreMulti(ctx, []int{1, 2})
	if err != nil {
		t.Fatalf("the keys are wedged by the goexit-ed load: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("unexpected entries: %+v", entries)
	}
	for i, entry := range entries {
		if entry == nil || entry.Value != "testValue" {
			t.Errorf("unexpected entry[%d]: %+v", i, entry)
		}
	}
	if got := sourceCalls.Load(); got != 2 {
		t.Errorf("expected the source to be called twice, but it was called %d times", got)
	}
}
