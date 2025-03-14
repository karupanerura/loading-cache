package intervalupdater_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/karupanerura/loading-cache/index/intervalupdater"
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
