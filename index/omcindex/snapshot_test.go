package omcindex_test

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/karupanerura/loading-cache/index"
	"github.com/karupanerura/loading-cache/index/omcindex"
)

// Signal just before waiting, so cancellation and publication can race with
// the transition from observing an uninitialized snapshot to blocking on it.
type observedWaitContext struct {
	context.Context
	waiting chan struct{}
	once    sync.Once
}

func (c *observedWaitContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.waiting) })
	return c.Context.Done()
}

func readIndex(ctx context.Context, idx *omcindex.OnMemoryIndex[int, int], multi bool) ([]int, error) {
	if multi {
		m, err := idx.GetMulti(ctx, []int{1})
		return m[1], err
	}
	return idx.Get(ctx, 1)
}

func TestSnapshotWaitsForExplicitRefresh(t *testing.T) {
	t.Parallel()
	for _, multi := range []bool{false, true} {
		name := "Get"
		if multi {
			name = "GetMulti"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				ctx := t.Context()
				var calls atomic.Int32
				idx := omcindex.NewOnMemoryIndex(index.FunctionIndexSource[int, int](func(context.Context) (map[int][]int, error) {
					calls.Add(1)
					return map[int][]int{1: {2}}, nil
				}))
				var keys []int
				var err error
				returned := false
				go func() { keys, err = readIndex(ctx, idx, multi); returned = true }()
				synctest.Wait()
				if returned {
					t.Fatal("read returned before Refresh")
				}
				if calls.Load() != 0 {
					t.Fatal("read started loading without Refresh")
				}
				if err := idx.Refresh(ctx); err != nil {
					t.Fatal(err)
				}
				synctest.Wait()
				if !returned || err != nil || len(keys) != 1 || keys[0] != 2 {
					t.Fatalf("read after refresh: returned %t, %v, %v", returned, keys, err)
				}
			})
		})
	}
}

func TestSnapshotCancellationDuringPublication(t *testing.T) {
	t.Parallel()
	testCtx, stop := context.WithTimeout(t.Context(), 5*time.Second)
	defer stop()
	for iteration := range 50 {
		idx := omcindex.NewOnMemoryIndex(index.FunctionIndexSource[int, int](func(context.Context) (map[int][]int, error) {
			return map[int][]int{1: {2}}, nil
		}))
		ctx, cancel := context.WithCancel(testCtx)
		waitCtx := &observedWaitContext{Context: ctx, waiting: make(chan struct{})}
		done := make(chan error, 1)
		go func() { _, err := readIndex(waitCtx, idx, iteration%2 == 0); done <- err }()
		select {
		case <-waitCtx.waiting:
		case <-testCtx.Done():
			cancel()
			t.Fatal("reader did not start waiting")
		}
		refreshed := make(chan error, 1)
		go func() { refreshed <- idx.Refresh(testCtx) }()
		cancel()
		select {
		case err := <-done:
			if err != nil && !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
		case <-testCtx.Done():
			t.Fatal("canceled reader did not finish")
		}
		select {
		case err := <-refreshed:
			if err != nil {
				t.Fatal(err)
			}
		case <-testCtx.Done():
			t.Fatal("refresh did not finish")
		}
		keys, err := idx.Get(testCtx, 1)
		if err != nil || len(keys) != 1 || keys[0] != 2 {
			t.Fatalf("read after cancellation and refresh: %v, %v", keys, err)
		}
	}
}

func TestSnapshotWaitingReadersObserveGoexit(t *testing.T) {
	t.Parallel()
	for _, multi := range []bool{false, true} {
		name := "Get"
		if multi {
			name = "GetMulti"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				ctx := t.Context()
				idx := omcindex.NewOnMemoryIndex(index.FunctionIndexSource[int, int](func(context.Context) (map[int][]int, error) {
					runtime.Goexit()
					return nil, nil
				}))
				finished, returned := false, false
				go func() {
					defer func() { finished = true }()
					_, _ = readIndex(ctx, idx, multi)
					returned = true
				}()
				synctest.Wait()
				if finished {
					t.Fatal("reader did not wait for Refresh")
				}
				go func() { _ = idx.Refresh(ctx) }()
				synctest.Wait()
				if !finished || returned {
					t.Fatal("waiting reader did not propagate Goexit")
				}
			})
		})
	}
}

func TestSnapshotSuccessfulRefreshClearsGoexit(t *testing.T) {
	t.Parallel()
	for _, multi := range []bool{false, true} {
		name := "Get"
		if multi {
			name = "GetMulti"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			var calls atomic.Int32
			idx := omcindex.NewOnMemoryIndex(index.FunctionIndexSource[int, int](func(context.Context) (map[int][]int, error) {
				if calls.Add(1) == 1 {
					runtime.Goexit()
				}
				return nil, nil
			}))
			refreshed := make(chan struct{})
			go func() { defer close(refreshed); _ = idx.Refresh(ctx) }()
			waitOrFatal(t, ctx, refreshed, "refresh did not finish")
			// A call after the failed refresh must start a new acquisition.
			if err := idx.Refresh(ctx); err != nil {
				t.Fatal(err)
			}
			done := make(chan struct{})
			var keys []int
			var err error
			returned := false
			go func() {
				defer close(done)
				keys, err = readIndex(ctx, idx, multi)
				returned = true
			}()
			waitOrFatal(t, ctx, done, "reader did not observe the empty refresh")
			if !returned || err != nil || len(keys) != 0 {
				t.Fatalf("read after empty refresh: returned=%v, keys=%v, err=%v", returned, keys, err)
			}
		})
	}
}

func TestSnapshotConcurrentRefreshesRetrieveInTurn(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		var mu sync.Mutex
		var revision, inFlight, maxInFlight int
		revisionOf := map[int]int{} // the revision retrieved by each caller
		idx := omcindex.NewOnMemoryIndex(index.FunctionIndexSource[int, int](func(ctx context.Context) (map[int][]int, error) {
			mu.Lock()
			revision++
			rev := revision
			inFlight++
			maxInFlight = max(maxInFlight, inFlight)
			if caller, ok := ctx.Value(callerKey{}).(int); ok {
				revisionOf[caller] = rev
			}
			mu.Unlock()
			// Another retrieval running at the same time would be counted while this one sleeps.
			time.Sleep(time.Second)
			mu.Lock()
			inFlight--
			mu.Unlock()
			return map[int][]int{1: {rev}}, nil
		}))
		if err := idx.Refresh(t.Context()); err != nil {
			t.Fatal(err)
		}

		const callers = 4
		type result struct {
			caller int
			err    error
			read   int
		}
		done := make(chan result, callers)
		for caller := range callers {
			go func() {
				r := result{caller: caller, read: -1}
				r.err = idx.Refresh(context.WithValue(t.Context(), callerKey{}, caller))
				if r.err == nil {
					if keys, err := idx.Get(t.Context(), 1); err == nil && len(keys) == 1 {
						r.read = keys[0]
					}
				}
				done <- r
			}()
		}
		synctest.Wait()
		// One call retrieves while the others wait for it.
		mu.Lock()
		if revision != 2 {
			t.Errorf("source calls while the calls wait: got %d, want 2 including initialization", revision)
		}
		mu.Unlock()
		if len(done) != 0 {
			t.Error("Refresh returned before its retrieval completed")
		}

		for range callers {
			r := <-done
			if r.err != nil {
				t.Errorf("caller %d: %v", r.caller, r.err)
				continue
			}
			// A successful call returns after publishing its own result. A later
			// call may already have published newer data, but never older data.
			mu.Lock()
			own := revisionOf[r.caller]
			mu.Unlock()
			if r.read < own {
				t.Errorf("caller %d read revision %d after Refresh, want at least its own revision %d", r.caller, r.read, own)
			}
		}

		mu.Lock()
		defer mu.Unlock()
		if revision != callers+1 {
			t.Errorf("source calls: got %d, want %d including initialization", revision, callers+1)
		}
		if len(revisionOf) != callers {
			t.Errorf("callers that retrieved with their own context: got %v, want all %d", revisionOf, callers)
		}
		if maxInFlight != 1 {
			t.Errorf("maximum concurrent retrievals: got %d, want 1", maxInFlight)
		}
		expectIndex(t, idx, callers+1, "read after all Refresh calls")
	})
}

func TestSnapshotRefreshAfterSourceChange(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		started := make(chan struct{})
		release := make(chan struct{})
		var mu sync.Mutex
		data, calls := 1, 0
		var idx *omcindex.OnMemoryIndex[int, int]
		idx = omcindex.NewOnMemoryIndex(index.FunctionIndexSource[int, int](func(ctx context.Context) (map[int][]int, error) {
			mu.Lock()
			value := data
			calls++
			call := calls
			mu.Unlock()
			switch call {
			case 1:
				close(started)
				<-release
			case 2:
				// The second retrieval starts after the first result is published.
				if keys, err := idx.Get(ctx, 1); err != nil || len(keys) != 1 || keys[0] != 1 {
					t.Errorf("read at the start of the second retrieval: %v, %v; want [1], nil", keys, err)
				}
			}
			return map[int][]int{1: {value}}, nil
		}))

		firstDone := goRefresh(t.Context(), idx)
		<-started
		// The source changes after the first retrieval has read it.
		mu.Lock()
		data = 2
		mu.Unlock()
		secondDone := goRefresh(t.Context(), idx)
		synctest.Wait()
		mu.Lock()
		if calls != 1 {
			t.Errorf("source calls during the first retrieval: got %d, want 1", calls)
		}
		mu.Unlock()

		close(release)
		for _, done := range []<-chan refreshOutcome{firstDone, secondDone} {
			if outcome := <-done; !outcome.returned || outcome.err != nil {
				t.Fatalf("Refresh: returned=%v, err=%v; want nil", outcome.returned, outcome.err)
			}
		}
		// The second call must not be satisfied by the retrieval that started before it.
		expectIndex(t, idx, 2, "read after both Refresh calls")
		mu.Lock()
		defer mu.Unlock()
		if calls != 2 {
			t.Errorf("source calls: got %d, want 2", calls)
		}
	})
}

func TestSnapshotNilRefreshIsEmpty(t *testing.T) {
	t.Parallel()
	for _, initialized := range []bool{false, true} {
		name := "FirstRefresh"
		if initialized {
			name = "AfterData"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			var calls atomic.Int32
			idx := omcindex.NewOnMemoryIndex(index.FunctionIndexSource[int, int](func(context.Context) (map[int][]int, error) {
				if calls.Add(1) == 1 && initialized {
					return map[int][]int{1: {2}}, nil
				}
				return nil, nil
			}))
			if initialized {
				if err := idx.Refresh(ctx); err != nil {
					t.Fatal(err)
				}
			}
			if err := idx.Refresh(ctx); err != nil {
				t.Fatal(err)
			}
			for _, multi := range []bool{false, true} {
				keys, err := readIndex(ctx, idx, multi)
				if err != nil || len(keys) != 0 {
					t.Fatalf("read after nil refresh (multi=%v): %v, %v", multi, keys, err)
				}
			}
		})
	}
}

func TestSnapshotFailedRefreshPreservesData(t *testing.T) {
	t.Parallel()
	for _, failure := range []string{"Error", "Panic", "Goexit"} {
		for _, empty := range []bool{false, true} {
			name := failure + "/Data"
			if empty {
				name = failure + "/Empty"
			}
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
				defer cancel()
				var calls atomic.Int32
				failureErr := errors.New("refresh failed")
				idx := omcindex.NewOnMemoryIndex(index.FunctionIndexSource[int, int](func(context.Context) (map[int][]int, error) {
					if calls.Add(1) == 1 {
						if empty {
							return nil, nil
						}
						return map[int][]int{1: {2}}, nil
					}
					switch failure {
					case "Goexit":
						runtime.Goexit()
					case "Panic":
						panic(failureErr)
					}
					return nil, failureErr
				}))
				if err := idx.Refresh(ctx); err != nil {
					t.Fatal(err)
				}
				refreshed := make(chan struct{})
				var refreshErr error
				returned := false
				go func() {
					defer close(refreshed)
					refreshErr = idx.Refresh(ctx)
					returned = true
				}()
				waitOrFatal(t, ctx, refreshed, "failed refresh did not finish")
				if failure == "Goexit" {
					if returned {
						t.Fatal("refresh did not propagate Goexit")
					}
				} else if !errors.Is(refreshErr, failureErr) {
					t.Fatalf("refresh: got %v, want %v", refreshErr, failureErr)
				}
				for _, multi := range []bool{false, true} {
					keys, err := readIndex(ctx, idx, multi)
					if err != nil || (empty && len(keys) != 0) || (!empty && (len(keys) != 1 || keys[0] != 2)) {
						t.Fatalf("read after failed refresh (multi=%v): %v, %v", multi, keys, err)
					}
				}
			})
		}
	}
}

func TestSnapshotCanceledRead(t *testing.T) {
	t.Parallel()
	for _, state := range []string{"Uninitialized", "Empty", "Data", "Goexit"} {
		t.Run(state, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			idx := omcindex.NewOnMemoryIndex(index.FunctionIndexSource[int, int](func(context.Context) (map[int][]int, error) {
				switch state {
				case "Goexit":
					runtime.Goexit()
				case "Data":
					return map[int][]int{1: {2}}, nil
				}
				return nil, nil
			}))
			if state != "Uninitialized" {
				refreshed := make(chan struct{})
				go func() { defer close(refreshed); _ = idx.Refresh(ctx) }()
				waitOrFatal(t, ctx, refreshed, "refresh did not finish")
			}
			canceled, stop := context.WithCancel(ctx)
			stop()
			for _, multi := range []bool{false, true} {
				done := make(chan struct{})
				var err error
				returned := false
				go func() {
					defer close(done)
					_, err = readIndex(canceled, idx, multi)
					returned = true
				}()
				waitOrFatal(t, ctx, done, "canceled reader did not finish")
				if !returned || !errors.Is(err, context.Canceled) {
					t.Fatalf("canceled read (multi=%v): returned=%v, err=%v", multi, returned, err)
				}
			}
		})
	}
}

func waitOrFatal(t *testing.T, ctx context.Context, ch <-chan struct{}, message string) {
	t.Helper()
	select {
	case <-ch:
	case <-ctx.Done():
		t.Fatal(message)
	}
}
