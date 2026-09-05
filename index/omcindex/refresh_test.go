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
	"github.com/karupanerura/loading-cache/index/intervalupdater"
	"github.com/karupanerura/loading-cache/index/omcindex"
)

type callerKey struct{}

// refreshOutcome records how a Refresh call ended. returned stays false if
// the call exited with runtime.Goexit.
type refreshOutcome struct {
	err      error
	returned bool
}

// goRefresh calls Refresh in a new goroutine and sends the outcome after the goroutine ends,
// including when it exits with runtime.Goexit.
func goRefresh(ctx context.Context, idx *omcindex.OnMemoryIndex[int, int]) <-chan refreshOutcome {
	done := make(chan refreshOutcome, 1)
	go func() {
		var outcome refreshOutcome
		defer func() { done <- outcome }()
		outcome.err = idx.Refresh(ctx)
		outcome.returned = true
	}()
	return done
}

func expectIndex(t *testing.T, idx *omcindex.OnMemoryIndex[int, int], want int, message string) {
	t.Helper()
	for _, multi := range []bool{false, true} {
		keys, err := readIndex(t.Context(), idx, multi)
		if err != nil || len(keys) != 1 || keys[0] != want {
			t.Errorf("%s (multi=%v): %v, %v; want [%d], nil", message, multi, keys, err, want)
		}
	}
}

func TestRefreshCanceledBeforeStart(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		var calls atomic.Int32
		idx := omcindex.NewOnMemoryIndex(index.FunctionIndexSource[int, int](func(context.Context) (map[int][]int, error) {
			calls.Add(1)
			return map[int][]int{1: {2}}, nil
		}))
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if err := idx.Refresh(ctx); !errors.Is(err, context.Canceled) {
			t.Fatalf("Refresh with a canceled context: got %v, want %v", err, context.Canceled)
		}
		if got := calls.Load(); got != 0 {
			t.Errorf("source calls: got %d, want 0", got)
		}

		// The index must stay uninitialized, so a reader keeps waiting.
		readCtx, stopRead := context.WithCancel(t.Context())
		readDone := make(chan error, 1)
		go func() { _, err := idx.Get(readCtx, 1); readDone <- err }()
		synctest.Wait()
		if len(readDone) != 0 {
			t.Fatalf("read returned without a successful refresh: %v", <-readDone)
		}
		stopRead()
		if err := <-readDone; !errors.Is(err, context.Canceled) {
			t.Errorf("canceled read: got %v, want %v", err, context.Canceled)
		}
	})
}

func TestRefreshWaitingCallerCancellation(t *testing.T) {
	t.Parallel()
	for _, cancellation := range []string{"Deadline", "Cancel"} {
		t.Run(cancellation, func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				started := make(chan struct{})
				release := make(chan struct{})
				var calls atomic.Int32
				idx := omcindex.NewOnMemoryIndex(index.FunctionIndexSource[int, int](func(context.Context) (map[int][]int, error) {
					revision := int(calls.Add(1))
					if revision == 1 {
						close(started)
						<-release
					}
					return map[int][]int{1: {revision}}, nil
				}))
				leaderDone := goRefresh(t.Context(), idx)
				<-started

				var ctx context.Context
				var cancel context.CancelFunc
				want := context.Canceled
				if cancellation == "Deadline" {
					ctx, cancel = context.WithTimeout(t.Context(), time.Second)
					want = context.DeadlineExceeded
				} else {
					ctx, cancel = context.WithCancel(t.Context())
				}
				defer cancel()
				waiterDone := goRefresh(ctx, idx)
				synctest.Wait()
				if len(waiterDone) != 0 {
					t.Fatal("waiting Refresh returned before its context was done")
				}
				if cancellation == "Cancel" {
					cancel()
				}
				// The deadline passes while every goroutine is blocked.
				if outcome := <-waiterDone; !outcome.returned || !errors.Is(outcome.err, want) {
					t.Fatalf("waiting Refresh: returned=%v, err=%v; want %v", outcome.returned, outcome.err, want)
				}
				if len(leaderDone) != 0 {
					t.Fatal("the leading Refresh returned before its retrieval completed")
				}

				close(release)
				if outcome := <-leaderDone; !outcome.returned || outcome.err != nil {
					t.Fatalf("leading Refresh: returned=%v, err=%v; want nil", outcome.returned, outcome.err)
				}
				synctest.Wait()
				// The canceled call must not retrieve after the leading one releases the refresh.
				if got := calls.Load(); got != 1 {
					t.Errorf("source calls: got %d, want 1", got)
				}
				expectIndex(t, idx, 1, "read after the leading Refresh")
			})
		})
	}
}

// cancelOnDoneContext cancels itself when Done is first called, so the
// cancellation becomes ready in the same select that tries to acquire the refresh,
// regardless of how many times Err is called before it.
type cancelOnDoneContext struct {
	context.Context
	cancel context.CancelFunc
	once   sync.Once
}

func (c *cancelOnDoneContext) Done() <-chan struct{} {
	c.once.Do(c.cancel)
	return c.Context.Done()
}

func TestRefreshCanceledWhileAcquiring(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		var calls atomic.Int32
		idx := omcindex.NewOnMemoryIndex(index.FunctionIndexSource[int, int](func(context.Context) (map[int][]int, error) {
			return map[int][]int{1: {int(calls.Add(1))}}, nil
		}))

		// The refresh is free and the context is canceled when select waits on it,
		// so select may either acquire the refresh or observe the cancellation.
		// Repeat so that both choices are exercised.
		for range 64 {
			ctx, cancel := context.WithCancel(t.Context())
			err := idx.Refresh(&cancelOnDoneContext{Context: ctx, cancel: cancel})
			cancel()
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("Refresh canceled while acquiring: got %v, want %v", err, context.Canceled)
			}
			if got := calls.Load(); got != 0 {
				t.Fatalf("source calls after a canceled acquisition: got %d, want 0", got)
			}
		}

		// Every canceled call must have released the refresh; synctest reports a deadlock otherwise.
		if err := idx.Refresh(t.Context()); err != nil {
			t.Fatal(err)
		}
		expectIndex(t, idx, 1, "read after a live Refresh")
	})
}

func TestRefreshUsesEachContext(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		type call struct {
			caller   string
			deadline time.Time
		}
		started := make(chan struct{})
		var mu sync.Mutex
		var calls []call
		idx := omcindex.NewOnMemoryIndex(index.FunctionIndexSource[int, int](func(ctx context.Context) (map[int][]int, error) {
			caller, _ := ctx.Value(callerKey{}).(string)
			deadline, _ := ctx.Deadline()
			mu.Lock()
			calls = append(calls, call{caller: caller, deadline: deadline})
			mu.Unlock()
			if caller == "first" {
				close(started)
				<-ctx.Done()
				return nil, ctx.Err()
			}
			return map[int][]int{1: {2}}, nil
		}))

		firstDeadline := time.Now().Add(time.Hour)
		firstCtx, cancelFirst := context.WithDeadline(context.WithValue(t.Context(), callerKey{}, "first"), firstDeadline)
		defer cancelFirst()
		secondDeadline := time.Now().Add(2 * time.Hour)
		secondCtx, cancelSecond := context.WithDeadline(context.WithValue(t.Context(), callerKey{}, "second"), secondDeadline)
		defer cancelSecond()

		firstDone := goRefresh(firstCtx, idx)
		<-started
		secondDone := goRefresh(secondCtx, idx)
		synctest.Wait()

		// Canceling the first call must not affect the second call waiting for it.
		cancelFirst()
		if outcome := <-firstDone; !outcome.returned || !errors.Is(outcome.err, context.Canceled) {
			t.Fatalf("first Refresh: returned=%v, err=%v; want %v", outcome.returned, outcome.err, context.Canceled)
		}
		if outcome := <-secondDone; !outcome.returned || outcome.err != nil {
			t.Fatalf("second Refresh: returned=%v, err=%v; want nil", outcome.returned, outcome.err)
		}

		mu.Lock()
		defer mu.Unlock()
		want := []call{{caller: "first", deadline: firstDeadline}, {caller: "second", deadline: secondDeadline}}
		if len(calls) != len(want) {
			t.Fatalf("source calls: got %v, want %v", calls, want)
		}
		for n := range want {
			if calls[n].caller != want[n].caller || !calls[n].deadline.Equal(want[n].deadline) {
				t.Errorf("source call %d: got %+v, want %+v", n, calls[n], want[n])
			}
		}
		expectIndex(t, idx, 2, "read after the second Refresh")
	})
}

func TestRefreshFailureDoesNotAffectWaitingRefresh(t *testing.T) {
	t.Parallel()
	for _, failure := range []string{"Error", "Panic", "Goexit"} {
		t.Run(failure, func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				failureStarted := make(chan struct{})
				releaseFailure := make(chan struct{})
				retryStarted := make(chan struct{})
				releaseRetry := make(chan struct{})
				failureErr := errors.New("refresh failed")
				var calls atomic.Int32
				idx := omcindex.NewOnMemoryIndex(index.FunctionIndexSource[int, int](func(context.Context) (map[int][]int, error) {
					revision := int(calls.Add(1))
					switch revision {
					case 2:
						close(failureStarted)
						<-releaseFailure
						switch failure {
						case "Panic":
							panic(failureErr)
						case "Goexit":
							runtime.Goexit()
						}
						return nil, failureErr
					case 3:
						close(retryStarted)
						<-releaseRetry
					}
					return map[int][]int{1: {revision}}, nil
				}))
				if err := idx.Refresh(t.Context()); err != nil {
					t.Fatal(err)
				}

				failedDone := goRefresh(t.Context(), idx)
				<-failureStarted
				retryDone := goRefresh(t.Context(), idx)
				synctest.Wait()
				if got := calls.Load(); got != 2 {
					t.Fatalf("source calls while the failing Refresh runs: got %d, want 2 including initialization", got)
				}

				close(releaseFailure)
				outcome := <-failedDone
				if failure == "Goexit" {
					if outcome.returned {
						t.Error("the failing Refresh did not propagate Goexit")
					}
				} else if !outcome.returned || !errors.Is(outcome.err, failureErr) {
					t.Errorf("failing Refresh: returned=%v, err=%v; want %v", outcome.returned, outcome.err, failureErr)
				}

				// The waiting call retrieves on its own. Until it publishes,
				// readers keep the snapshot from before the failure.
				<-retryStarted
				expectIndex(t, idx, 1, "read after the failure")
				close(releaseRetry)
				if outcome := <-retryDone; !outcome.returned || outcome.err != nil {
					t.Fatalf("waiting Refresh after the failure: returned=%v, err=%v; want nil", outcome.returned, outcome.err)
				}
				expectIndex(t, idx, 3, "read after the waiting Refresh")
			})
		})
	}
}

func TestRefreshWithIntervalUpdater(t *testing.T) {
	t.Parallel()
	const interval = time.Minute
	for _, manual := range []string{"Canceled", "Goexit"} {
		t.Run(manual, func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				manualStarted := make(chan struct{})
				releaseManual := make(chan struct{})
				var mu sync.Mutex
				var callers []string
				idx := omcindex.NewOnMemoryIndex(index.FunctionIndexSource[int, int](func(ctx context.Context) (map[int][]int, error) {
					caller, _ := ctx.Value(callerKey{}).(string)
					mu.Lock()
					callers = append(callers, caller)
					revision := len(callers)
					mu.Unlock()
					if caller == "manual" {
						close(manualStarted)
						if manual == "Goexit" {
							<-releaseManual
							runtime.Goexit()
						}
						<-ctx.Done()
						return nil, ctx.Err()
					}
					return map[int][]int{1: {revision}}, nil
				}))
				var bgErrs []error
				updater := intervalupdater.NewIntervalIndexUpdater(idx, interval, func(err error) {
					mu.Lock()
					defer mu.Unlock()
					bgErrs = append(bgErrs, err)
				})
				expectCallers := func(want []string, message string) {
					t.Helper()
					mu.Lock()
					defer mu.Unlock()
					if len(callers) != len(want) {
						t.Fatalf("%s: source callers %v, want %v", message, callers, want)
					}
					for n := range want {
						if callers[n] != want[n] {
							t.Fatalf("%s: source callers %v, want %v", message, callers, want)
						}
					}
				}

				manualCtx, cancelManual := context.WithCancel(context.WithValue(t.Context(), callerKey{}, "manual"))
				defer cancelManual()
				manualDone := goRefresh(manualCtx, idx)
				<-manualStarted

				updaterCtx, stopUpdater := context.WithCancel(context.WithValue(t.Context(), callerKey{}, "updater"))
				defer stopUpdater()
				updater.LaunchBackgroundUpdater(updaterCtx)
				synctest.Wait()
				// The updater's initial Refresh waits for the manual one.
				expectCallers([]string{"manual"}, "while the manual Refresh runs")

				if manual == "Goexit" {
					close(releaseManual)
				} else {
					cancelManual()
				}
				outcome := <-manualDone
				if manual == "Goexit" {
					if outcome.returned {
						t.Error("the manual Refresh did not propagate Goexit")
					}
				} else if !outcome.returned || !errors.Is(outcome.err, context.Canceled) {
					t.Errorf("manual Refresh: returned=%v, err=%v; want %v", outcome.returned, outcome.err, context.Canceled)
				}

				// The updater's initial Refresh retrieves with its own context.
				synctest.Wait()
				expectCallers([]string{"manual", "updater"}, "after the updater's initial Refresh")
				expectIndex(t, idx, 2, "read after the updater's initial Refresh")

				// The next tick refreshes again.
				time.Sleep(interval)
				synctest.Wait()
				expectCallers([]string{"manual", "updater", "updater"}, "after the next tick")
				expectIndex(t, idx, 3, "read after the next tick")

				stopUpdater()
				synctest.Wait()
				mu.Lock()
				defer mu.Unlock()
				if len(bgErrs) != 0 {
					t.Errorf("background errors: %v", bgErrs)
				}
			})
		})
	}
}
