package pureloader_test

import (
	"context"
	"strings"
	"testing"

	loadingcache "github.com/karupanerura/loading-cache"
	"github.com/karupanerura/loading-cache/loader/pureloader"
	"github.com/karupanerura/loading-cache/source"
	"github.com/karupanerura/loading-cache/storage"
)

// TestLoadAndStoreMulti_SourceLengthContractViolation verifies that a source
// returning a slice with a length different from the input keys is reported
// as an error instead of surfacing far from its cause as an
// index-out-of-range in LoadingCache.GetOrLoadMulti.
func TestLoadAndStoreMulti_SourceLengthContractViolation(t *testing.T) {
	t.Parallel()

	src := &source.FunctionsSource[int, string]{
		GetMultiFunc: func(_ context.Context, keys []int) ([]*loadingcache.CacheEntry[int, string], error) {
			// violate the length contract
			return nil, nil
		},
	}
	st := &storage.FunctionsStorage[int, string]{
		SetMultiFunc: func(context.Context, []*loadingcache.CacheEntry[int, string]) error { return nil },
	}
	loader := pureloader.NewPureLoader[int, string](st, src)

	_, err := loader.LoadAndStoreMulti(t.Context(), []int{1, 2})
	if err == nil {
		t.Fatal("expected an error for a source violating the length contract, got nil")
	}
	if !strings.Contains(err.Error(), "must return exactly one entry per key") {
		t.Fatalf("unexpected error: %v", err)
	}
}
