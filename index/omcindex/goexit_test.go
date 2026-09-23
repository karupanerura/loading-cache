package omcindex_test

import (
	"context"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/karupanerura/loading-cache/index"
	"github.com/karupanerura/loading-cache/index/omcindex"
)

// TestOnMemoryIndex_GoexitAllowsLaterRefresh verifies that readers propagate
// a refresh's Goexit and that a subsequent Refresh can publish usable data.
func TestOnMemoryIndex_GoexitAllowsLaterRefresh(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

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
	go func() {
		defer close(refreshDone)
		_ = idx.Refresh(ctx)
		t.Error("Refresh must not return normally when the source calls runtime.Goexit")
	}()
	waitOrFatal(t, ctx, refreshDone, "refresh did not finish")

	// A getter observes the goexit state and propagates runtime.Goexit.
	getDone := make(chan struct{})
	go func() {
		defer close(getDone)
		_, _ = idx.Get(ctx, 1)
		t.Error("Get must not return normally after the refresh called runtime.Goexit")
	}()
	waitOrFatal(t, ctx, getDone, "reader did not propagate Goexit")

	// A subsequent refresh must still be able to publish a new snapshot.
	refreshDone2 := make(chan error, 1)
	go func() {
		refreshDone2 <- idx.Refresh(ctx)
	}()
	select {
	case err := <-refreshDone2:
		if err != nil {
			t.Fatalf("unexpected refresh error: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("Refresh did not finish after the reader propagated Goexit")
	}

	// The index must serve data afterwards.
	pks, err := idx.Get(ctx, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pks) != 2 || pks[0] != 2 || pks[1] != 3 {
		t.Errorf("unexpected primary keys: %v", pks)
	}
}
