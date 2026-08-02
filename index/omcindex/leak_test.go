package omcindex_test

import (
	"context"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/karupanerura/loading-cache/index"
	"github.com/karupanerura/loading-cache/index/omcindex"
)

// TestGet_CanceledWaitersLeaveNoGoroutines verifies that getters canceled
// while the initial load is running leave no goroutines behind: waiting is a
// plain select on the shared flight, so a canceled getter just returns. Only
// the single shared load goroutine may remain until the source returns, no
// matter how many getters were canceled.
//
// (The previous condition-variable-based implementation parked two goroutines
// per canceled getter until the next broadcast — potentially forever when no
// refresh ever happened. This test pins the stronger guarantee of the
// redesign.)
//
// This test asserts goroutine counts, so it must not run in parallel with
// other tests.
func TestGet_CanceledWaitersLeaveNoGoroutines(t *testing.T) {
	const numCanceledGetters = 16

	release := make(chan struct{})
	idx := omcindex.NewOnMemoryIndex[uint8, uint8](index.FunctionIndexSource[uint8, uint8](func(context.Context) (map[uint8][]uint8, error) {
		<-release
		return map[uint8][]uint8{1: {2, 3}}, nil
	}))

	baseline := runtime.NumGoroutine()

	// Each getter joins the shared initial load and gives up when its timeout
	// fires.
	var wg sync.WaitGroup
	for i := 0; i < numCanceledGetters; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
			defer cancel()
			if _, err := idx.Get(ctx, 1); err != context.DeadlineExceeded {
				t.Errorf("expected context.DeadlineExceeded while initializing, got %v", err)
			}
		}()
	}
	wg.Wait()

	// All canceled getters have returned. Give their goroutines a moment to
	// fully terminate, then only the single shared load goroutine may remain.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && runtime.NumGoroutine() > baseline+1 {
		time.Sleep(time.Millisecond)
	}
	if now := runtime.NumGoroutine(); now > baseline+1 {
		t.Errorf("canceled getters left goroutines behind (only the shared load may remain): baseline=%d now=%d", baseline, now)
	}

	// Let the load finish: everything must settle back to the baseline and
	// the index must serve the loaded data.
	close(release)

	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && runtime.NumGoroutine() > baseline {
		time.Sleep(10 * time.Millisecond)
	}
	if now := runtime.NumGoroutine(); now > baseline {
		t.Errorf("the initial load goroutine did not finish: baseline=%d now=%d", baseline, now)
	}

	pks, err := idx.Get(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error after the initial load finished: %v", err)
	}
	if len(pks) != 2 || pks[0] != 2 || pks[1] != 3 {
		t.Errorf("unexpected primary keys: %v", pks)
	}
}
