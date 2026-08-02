package singleflightloader_test

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	loadingcache "github.com/karupanerura/loading-cache"
	"github.com/karupanerura/loading-cache/loader/singleflightloader"
	"github.com/karupanerura/loading-cache/source"
	"github.com/karupanerura/loading-cache/storage"
)

// TestLoadAndStoreMulti_SourceLengthContractViolation verifies that a source
// returning a slice with a length different from the input keys is reported
// as an error to every waiter instead of panicking as an index-out-of-range
// on the background loading goroutine (which would crash the whole process
// and leave the waiters unnotified), and that the keys are not wedged
// afterwards.
func TestLoadAndStoreMulti_SourceLengthContractViolation(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	src := &source.FunctionsSource[int, string]{
		GetMultiFunc: func(_ context.Context, keys []int) ([]*loadingcache.CacheEntry[int, string], error) {
			if calls.Add(1) == 1 {
				// violate the length contract on the first load
				return nil, nil
			}
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
		SetMultiFunc: func(context.Context, []*loadingcache.CacheEntry[int, string]) error { return nil },
	}
	loader := singleflightloader.NewSingleFlightLoader(st, src, singleflightloader.WithCloner[int, string](loadingcache.NopValueCloner[string]{}))

	_, err := loader.LoadAndStoreMulti(t.Context(), []int{1, 2})
	if err == nil {
		t.Fatal("expected an error for a source violating the length contract, got nil")
	}
	if !strings.Contains(err.Error(), "must return exactly one entry per key") {
		t.Fatalf("unexpected error: %v", err)
	}

	// The keys must not be wedged by the failed load: a subsequent call must
	// start a fresh load and succeed.
	entries, err := loader.LoadAndStoreMulti(t.Context(), []int{1, 2})
	if err != nil {
		t.Fatalf("the keys are wedged by the failed load: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("unexpected entries: %+v", entries)
	}
	for i, entry := range entries {
		if entry == nil || entry.Value != "testValue" {
			t.Errorf("unexpected entry[%d]: %+v", i, entry)
		}
	}
}
