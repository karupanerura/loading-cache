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

// TestOnMemoryIndex_GoexitReleasesLock verifies that a getter that propagates
// runtime.Goexit (after the refresh goroutine called runtime.Goexit) releases
// the read lock before exiting. If the lock leaks, a subsequent Refresh
// deadlocks forever.
func TestOnMemoryIndex_GoexitReleasesLock(t *testing.T) {
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
	go func() {
		defer close(refreshDone)
		_ = idx.Refresh(context.Background())
		t.Error("Refresh must not return normally when the source calls runtime.Goexit")
	}()
	<-refreshDone

	// A getter observes the goexit state and propagates runtime.Goexit.
	getDone := make(chan struct{})
	go func() {
		defer close(getDone)
		_, _ = idx.Get(context.Background(), 1)
		t.Error("Get must not return normally after the refresh called runtime.Goexit")
	}()
	<-getDone

	// A subsequent refresh must not deadlock on a leaked read lock.
	refreshDone2 := make(chan error, 1)
	go func() {
		refreshDone2 <- idx.Refresh(context.Background())
	}()
	select {
	case err := <-refreshDone2:
		if err != nil {
			t.Fatalf("unexpected refresh error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Refresh deadlocked: the goexit path in Get leaked the read lock")
	}

	// The index must serve data afterwards.
	pks, err := idx.Get(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pks) != 2 || pks[0] != 2 || pks[1] != 3 {
		t.Errorf("unexpected primary keys: %v", pks)
	}
}
