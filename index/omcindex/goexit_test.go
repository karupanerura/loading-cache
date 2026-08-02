package omcindex_test

import (
	"context"
	"runtime"
	"sync/atomic"
	"testing"

	"github.com/karupanerura/loading-cache/index"
	"github.com/karupanerura/loading-cache/index/omcindex"
)

// TestOnMemoryIndex_GoexitInLazyInitialLoad verifies that runtime.Goexit
// inside the source (e.g. a test source calling t.Fatal) during the lazy
// initial load is propagated to the getters waiting for that load — the same
// symmetry rule as golang.org/x/sync/singleflight — and that the index is not
// wedged: a subsequent Get starts a fresh load and succeeds.
func TestOnMemoryIndex_GoexitInLazyInitialLoad(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	source := index.FunctionIndexSource[uint8, uint8](func(ctx context.Context) (map[uint8][]uint8, error) {
		if calls.Add(1) == 1 {
			runtime.Goexit()
		}
		return map[uint8][]uint8{1: {2, 3}}, nil
	})
	idx := omcindex.NewOnMemoryIndex[uint8, uint8](source)

	// The first Get triggers the initial load, which dies via runtime.Goexit.
	// The waiting getter must observe it as Goexit propagation instead of
	// returning normally.
	getDone := make(chan struct{})
	var getReturned bool
	go func() {
		defer close(getDone)
		_, _ = idx.Get(context.Background(), 1)
		getReturned = true
	}()
	<-getDone
	if getReturned {
		t.Error("Get must not return normally when the initial load calls runtime.Goexit")
	}

	// The index must not be wedged: a subsequent Get starts a fresh load and
	// succeeds.
	pks, err := idx.Get(context.Background(), 1)
	if err != nil {
		t.Fatalf("the index is wedged by the goexit-ed load: %v", err)
	}
	if len(pks) != 2 || pks[0] != 2 || pks[1] != 3 {
		t.Errorf("unexpected primary keys: %v", pks)
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("expected the source to be called twice, but it was called %d times", got)
	}
}

// TestOnMemoryIndex_GoexitInRefreshIsLocal verifies that runtime.Goexit
// inside the source during an explicit Refresh terminates only the Refresh
// caller's goroutine: getters are not involved in that load, so they are not
// affected and a later Get initializes the index with a fresh load.
func TestOnMemoryIndex_GoexitInRefreshIsLocal(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	source := index.FunctionIndexSource[uint8, uint8](func(ctx context.Context) (map[uint8][]uint8, error) {
		if calls.Add(1) == 1 {
			runtime.Goexit()
		}
		return map[uint8][]uint8{1: {2, 3}}, nil
	})
	idx := omcindex.NewOnMemoryIndex[uint8, uint8](source)

	// The first refresh terminates via runtime.Goexit inside the source.
	refreshDone := make(chan struct{})
	var refreshReturned bool
	go func() {
		defer close(refreshDone)
		_ = idx.Refresh(context.Background())
		refreshReturned = true
	}()
	<-refreshDone
	if refreshReturned {
		t.Error("Refresh must not return normally when the source calls runtime.Goexit")
	}

	// The goexit-ed refresh is local to its caller: a Get afterwards triggers
	// its own initial load and succeeds.
	pks, err := idx.Get(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pks) != 2 || pks[0] != 2 || pks[1] != 3 {
		t.Errorf("unexpected primary keys: %v", pks)
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("expected the source to be called twice, but it was called %d times", got)
	}
}
