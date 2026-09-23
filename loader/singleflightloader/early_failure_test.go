package singleflightloader_test

import (
	"context"
	"errors"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	loadingcache "github.com/karupanerura/loading-cache"
	"github.com/karupanerura/loading-cache/loader/singleflightloader"
	"github.com/karupanerura/loading-cache/source"
	"github.com/karupanerura/loading-cache/storage/memstorage"
)

// TestLoadAndStoreMulti_ReturnsOnFirstReceivedError verifies that a batch call
// returns the error of one key without waiting for a slow load of another key,
// regardless of the input order, and that the slow load still completes.
func TestLoadAndStoreMulti_ReturnsOnFirstReceivedError(t *testing.T) {
	t.Parallel()
	for _, keys := range [][]int{{1, 2}, {2, 1}} {
		t.Run("", func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				errFail := errors.New("key 2 failed")
				release := make(chan struct{})
				finishSlowLoad := sync.OnceFunc(func() { close(release) })
				defer finishSlowLoad()
				var failures atomic.Int32
				src := &source.FunctionsSource[int, int]{
					GetFunc: func(_ context.Context, key int) (*loadingcache.CacheEntry[int, int], error) {
						if key == 1 {
							<-release
						}
						return &loadingcache.CacheEntry[int, int]{Entry: loadingcache.Entry[int, int]{Key: key, Value: key * 10}, ExpiresAt: time.Now().Add(time.Hour)}, nil
					},
					GetMultiFunc: func(_ context.Context, keys []int) ([]*loadingcache.CacheEntry[int, int], error) {
						if slices.Contains(keys, 2) && failures.Add(1) == 1 {
							return nil, errFail
						}
						entries := make([]*loadingcache.CacheEntry[int, int], len(keys))
						for i, key := range keys {
							entries[i] = &loadingcache.CacheEntry[int, int]{Entry: loadingcache.Entry[int, int]{Key: key, Value: key * 10}, ExpiresAt: time.Now().Add(time.Hour)}
						}
						return entries, nil
					},
				}
				st := memstorage.NewInMemoryStorage[int, int]()
				l := singleflightloader.NewSingleFlightLoader(st, src)
				ctx := t.Context()

				// Stall a single-key load of key 1.
				var singleEntry *loadingcache.Entry[int, int]
				var singleErr error
				singleDone := make(chan struct{})
				go func() {
					defer close(singleDone)
					singleEntry, singleErr = l.LoadAndStore(ctx, 1)
				}()
				synctest.Wait()

				// The batch joins the stalled load of key 1 and fails on key 2.
				var batchErr error
				batchDone := make(chan struct{})
				go func() {
					defer close(batchDone)
					_, batchErr = l.LoadAndStoreMulti(ctx, keys)
				}()
				synctest.Wait()
				select {
				case <-batchDone:
				default:
					t.Fatal("LoadAndStoreMulti waited for the stalled load of key 1")
				}
				if !errors.Is(batchErr, errFail) {
					t.Errorf("LoadAndStoreMulti: got %v, want %v", batchErr, errFail)
				}

				// The shared load continues after the batch returned.
				finishSlowLoad()
				<-singleDone
				if singleErr != nil || singleEntry == nil || singleEntry.Value != 10 {
					t.Errorf("LoadAndStore: got %+v, %v; want value 10", singleEntry, singleErr)
				}
				if cached, err := st.Get(ctx, 1); err != nil || cached == nil || cached.Value != 10 {
					t.Errorf("stored entry for key 1: got %+v, %v", cached, err)
				}

				// A later call retries the failed key and returns results in input order.
				entries, err := l.LoadAndStoreMulti(ctx, keys)
				if err != nil {
					t.Fatalf("retry: %v", err)
				}
				for i, entry := range entries {
					if entry == nil || entry.Key != keys[i] || entry.Value != keys[i]*10 {
						t.Errorf("retry result %d: got %+v, want key %d", i, entry, keys[i])
					}
				}
			})
		})
	}
}

// TestLoadAndStoreMulti_WaitsForAllPositionsOnSuccess verifies that a
// successful batch returns only after results for all input positions arrive,
// including positions served by a load started by another caller.
func TestLoadAndStoreMulti_WaitsForAllPositionsOnSuccess(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		release := make(chan struct{})
		finishSlowLoad := sync.OnceFunc(func() { close(release) })
		defer finishSlowLoad()
		src := &source.FunctionsSource[int, int]{
			GetFunc: func(_ context.Context, key int) (*loadingcache.CacheEntry[int, int], error) {
				<-release
				return &loadingcache.CacheEntry[int, int]{Entry: loadingcache.Entry[int, int]{Key: key, Value: key * 10}, ExpiresAt: time.Now().Add(time.Hour)}, nil
			},
			GetMultiFunc: func(_ context.Context, keys []int) ([]*loadingcache.CacheEntry[int, int], error) {
				entries := make([]*loadingcache.CacheEntry[int, int], len(keys))
				for i, key := range keys {
					entries[i] = &loadingcache.CacheEntry[int, int]{Entry: loadingcache.Entry[int, int]{Key: key, Value: key * 10}, ExpiresAt: time.Now().Add(time.Hour)}
				}
				return entries, nil
			},
		}
		l := singleflightloader.NewSingleFlightLoader(memstorage.NewInMemoryStorage[int, int](), src)
		ctx := t.Context()

		go l.LoadAndStore(ctx, 1)
		synctest.Wait()

		var entries []*loadingcache.Entry[int, int]
		var err error
		done := make(chan struct{})
		go func() {
			defer close(done)
			entries, err = l.LoadAndStoreMulti(ctx, []int{2, 1, 2})
		}()
		synctest.Wait()
		select {
		case <-done:
			t.Fatal("LoadAndStoreMulti returned before the load of key 1 finished")
		default:
		}

		finishSlowLoad()
		<-done
		if err != nil {
			t.Fatal(err)
		}
		for i, want := range []int{2, 1, 2} {
			if entries[i] == nil || entries[i].Key != want || entries[i].Value != want*10 {
				t.Errorf("result %d: got %+v, want key %d", i, entries[i], want)
			}
		}
		if entries[0] == entries[2] {
			t.Error("duplicate positions share the same entry")
		}
	})
}
