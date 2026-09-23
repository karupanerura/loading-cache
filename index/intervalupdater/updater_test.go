package intervalupdater_test

import (
	"context"
	"errors"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/karupanerura/loading-cache/index/intervalupdater"
	"github.com/karupanerura/loading-cache/index/omcindex"
)

type mockRefreshIndex func(context.Context) error

func (f mockRefreshIndex) Refresh(ctx context.Context) error {
	return f(ctx)
}

func TestLaunchBackgroundUpdater(t *testing.T) {
	t.Parallel()

	var callCount uint32
	idx := mockRefreshIndex(func(context.Context) error {
		atomic.AddUint32(&callCount, 1)
		return nil
	})

	var bgErrs []error
	var mu sync.Mutex
	updater := intervalupdater.NewIntervalIndexUpdater(idx, 200*time.Millisecond, func(err error) {
		mu.Lock()
		defer mu.Unlock()
		bgErrs = append(bgErrs, err)
	})
	updater.LaunchBackgroundUpdater(t.Context())

	time.Sleep(100 * time.Millisecond)
	if atomic.LoadUint32(&callCount) != 1 {
		t.Errorf("expect to refreshed at first time")
	}

	time.Sleep(200 * time.Millisecond)
	if atomic.LoadUint32(&callCount) != 2 {
		t.Errorf("expect to refreshed at second time")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(bgErrs) != 0 {
		t.Errorf("should no background errors, but got: %+v", bgErrs)
	}
}

func TestLaunchBackgroundUpdater_Error(t *testing.T) {
	t.Parallel()

	refreshErr := errors.New("refresh error")
	idx := mockRefreshIndex(func(context.Context) error {
		return refreshErr
	})

	var bgErrs []error
	var mu sync.Mutex
	updater := intervalupdater.NewIntervalIndexUpdater(idx, 200*time.Millisecond, func(err error) {
		mu.Lock()
		defer mu.Unlock()
		bgErrs = append(bgErrs, err)
	})
	updater.LaunchBackgroundUpdater(t.Context())

	time.Sleep(100 * time.Millisecond)
	func() {
		mu.Lock()
		defer mu.Unlock()
		if df := cmp.Diff([]error{refreshErr}, bgErrs, cmp.Comparer(func(x, y error) bool {
			return errors.Is(x, y) || errors.Is(y, x)
		})); df != "" {
			t.Errorf("unexpected background errors: %+v", bgErrs)
		}
	}()

	time.Sleep(200 * time.Millisecond)
	func() {
		mu.Lock()
		defer mu.Unlock()
		if df := cmp.Diff([]error{refreshErr, refreshErr}, bgErrs, cmp.Comparer(func(x, y error) bool {
			return errors.Is(x, y) || errors.Is(y, x)
		})); df != "" {
			t.Errorf("unexpected background errors: %+v", bgErrs)
		}
	}()
}

func TestLaunchBackgroundUpdater_CanceledBeforeStart(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		var calls atomic.Int32
		idx := mockRefreshIndex(func(ctx context.Context) error {
			calls.Add(1)
			return ctx.Err()
		})
		var errCalls atomic.Int32
		updater := intervalupdater.NewIntervalIndexUpdater(idx, time.Minute, func(error) {
			errCalls.Add(1)
		})

		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		updater.LaunchBackgroundUpdater(ctx)
		synctest.Wait()

		// synctest.Test also fails if the updater goroutine does not exit.
		if got := calls.Load(); got != 0 {
			t.Errorf("Refresh calls: got %d, want 0", got)
		}
		if got := errCalls.Load(); got != 0 {
			t.Errorf("error callback calls: got %d, want 0", got)
		}
	})
}

func TestLaunchBackgroundUpdater_CanceledWhileRefreshing(t *testing.T) {
	t.Parallel()
	const interval = time.Minute

	// When the in-flight Refresh returns, both the next tick and the cancellation are ready,
	// and select picks either of them. Repeat so that both choices are exercised.
	for range 32 {
		synctest.Test(t, func(t *testing.T) {
			periodicStarted := make(chan struct{})
			release := make(chan struct{})
			var calls atomic.Int32
			idx := mockRefreshIndex(func(ctx context.Context) error {
				switch calls.Add(1) {
				case 1:
					// The initial Refresh runs before the ticker is created.
					return nil
				case 2:
					close(periodicStarted)
					<-release
					return ctx.Err()
				default:
					return nil
				}
			})
			var mu sync.Mutex
			var bgErrs []error
			updater := intervalupdater.NewIntervalIndexUpdater(idx, interval, func(err error) {
				mu.Lock()
				defer mu.Unlock()
				bgErrs = append(bgErrs, err)
			})

			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			updater.LaunchBackgroundUpdater(ctx)

			// Wait for the first periodic Refresh, then let the next tick fire while it is running.
			<-periodicStarted
			time.Sleep(interval)
			synctest.Wait()
			cancel()
			close(release)
			synctest.Wait()

			// synctest.Test also fails if the updater goroutine does not exit.
			if got := calls.Load(); got != 2 {
				t.Errorf("Refresh calls: got %d, want 2", got)
			}
			mu.Lock()
			defer mu.Unlock()
			if len(bgErrs) != 1 || !errors.Is(bgErrs[0], context.Canceled) {
				t.Errorf("background errors: got %v, want the error returned by the in-flight Refresh", bgErrs)
			}
		})
	}
}

func TestLaunchBackgroundUpdater_RefreshGoexit(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		var calls atomic.Int32
		idx := mockRefreshIndex(func(context.Context) error {
			calls.Add(1)
			runtime.Goexit()
			return nil
		})
		var errCalls atomic.Int32
		updater := intervalupdater.NewIntervalIndexUpdater(idx, time.Minute, func(error) {
			errCalls.Add(1)
		})

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		updater.LaunchBackgroundUpdater(ctx)
		time.Sleep(10 * time.Minute)
		synctest.Wait()

		// Goexit ends the updater goroutine: no callback and no further refreshes.
		if got := calls.Load(); got != 1 {
			t.Errorf("Refresh calls: got %d, want 1", got)
		}
		if got := errCalls.Load(); got != 0 {
			t.Errorf("error callback calls: got %d, want 0", got)
		}
	})
}

func TestNewIntervalIndexUpdater_NonPositiveInterval(t *testing.T) {
	t.Parallel()
	for _, interval := range []time.Duration{0, -time.Second} {
		t.Run(interval.String(), func(t *testing.T) {
			t.Parallel()
			var refreshCalls, errCalls atomic.Int32
			idx := mockRefreshIndex(func(context.Context) error {
				refreshCalls.Add(1)
				return nil
			})

			func() {
				defer func() {
					if recover() == nil {
						t.Errorf("NewIntervalIndexUpdater(%v): expected panic", interval)
					}
				}()
				intervalupdater.NewIntervalIndexUpdater(idx, interval, func(error) {
					errCalls.Add(1)
				})
			}()

			if got := refreshCalls.Load(); got != 0 {
				t.Errorf("Refresh calls: got %d, want 0", got)
			}
			if got := errCalls.Load(); got != 0 {
				t.Errorf("error callback calls: got %d, want 0", got)
			}
		})
	}
}

func TestNewIntervalIndexUpdater_NilOnBackgroundError(t *testing.T) {
	t.Parallel()
	idx := mockRefreshIndex(func(context.Context) error { return nil })
	defer func() {
		if recover() == nil {
			t.Error("NewIntervalIndexUpdater(nil onBackgroundError): expected panic")
		}
	}()
	intervalupdater.NewIntervalIndexUpdater(idx, time.Minute, nil)
}

// stallingIndexSource returns {1: [call number]} for each GetAll call, except
// that the call number stallAt ignores its context and waits for release.
type stallingIndexSource struct {
	calls   atomic.Int32
	stallAt int32
	release chan struct{}
}

func (s *stallingIndexSource) GetAll(context.Context) (map[int][]int, error) {
	n := s.calls.Add(1)
	if n == s.stallAt {
		<-s.release
	}
	return map[int][]int{1: {int(n)}}, nil
}

// TestLaunchBackgroundUpdater_WaitsForStalledManualRefresh verifies how the
// updater behaves while a manual Refresh of an OnMemoryIndex is stalled in a
// source that ignores its context.
func TestLaunchBackgroundUpdater_WaitsForStalledManualRefresh(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"CancelUpdater", "ReleaseSource"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				src := &stallingIndexSource{stallAt: 2, release: make(chan struct{})}
				releaseSource := sync.OnceFunc(func() { close(src.release) })
				defer releaseSource()
				idx := omcindex.NewOnMemoryIndex[int, int](src)
				if err := idx.Refresh(t.Context()); err != nil {
					t.Fatalf("initial Refresh: %v", err)
				}

				// Stall a manual Refresh in the source, then cancel its context.
				manualCtx, cancelManual := context.WithCancel(t.Context())
				manualDone := make(chan struct{})
				go func() {
					defer close(manualDone)
					_ = idx.Refresh(manualCtx)
				}()
				synctest.Wait()
				cancelManual()
				synctest.Wait()
				select {
				case <-manualDone:
					t.Fatal("manual Refresh returned although the source ignores its context")
				default:
				}

				var mu sync.Mutex
				var bgErrs []error
				updater := intervalupdater.NewIntervalIndexUpdater(idx, time.Minute, func(err error) {
					mu.Lock()
					defer mu.Unlock()
					bgErrs = append(bgErrs, err)
				})
				updaterCtx, cancelUpdater := context.WithCancel(t.Context())
				defer cancelUpdater()
				updater.LaunchBackgroundUpdater(updaterCtx)
				synctest.Wait()

				// The updater waits for the manual Refresh and does not retrieve.
				if got := src.calls.Load(); got != 2 {
					t.Fatalf("GetAll calls while the manual Refresh is stalled: got %d, want 2", got)
				}
				// Readers still see the previous snapshot.
				if pks, err := idx.Get(t.Context(), 1); err != nil || !slices.Equal(pks, []int{1}) {
					t.Errorf("Get while stalled: got %v, %v; want [1]", pks, err)
				}

				switch scenario {
				case "CancelUpdater":
					cancelUpdater()
					synctest.Wait()
					mu.Lock()
					if len(bgErrs) != 1 || !errors.Is(bgErrs[0], context.Canceled) {
						t.Errorf("background errors: got %v, want [context.Canceled]", bgErrs)
					}
					mu.Unlock()
					releaseSource()
					<-manualDone
					synctest.Wait()
					if got := src.calls.Load(); got != 2 {
						t.Errorf("GetAll calls after the updater was canceled: got %d, want 2", got)
					}

				case "ReleaseSource":
					releaseSource()
					<-manualDone
					synctest.Wait()
					// The waiting updater resumes and retrieves on its own.
					if got := src.calls.Load(); got != 3 {
						t.Errorf("GetAll calls after release: got %d, want 3", got)
					}
					if pks, err := idx.Get(t.Context(), 1); err != nil || !slices.Equal(pks, []int{3}) {
						t.Errorf("Get after the updater refreshed: got %v, %v; want [3]", pks, err)
					}
					cancelUpdater()
					synctest.Wait()
					mu.Lock()
					if len(bgErrs) != 0 {
						t.Errorf("background errors: got %v, want none", bgErrs)
					}
					mu.Unlock()
				}
			})
		})
	}
}
