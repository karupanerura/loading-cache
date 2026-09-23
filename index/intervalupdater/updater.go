package intervalupdater

import (
	"context"
	"fmt"
	"time"

	loadingcache "github.com/karupanerura/loading-cache"
)

// IntervalIndexUpdater is a background updater that refreshes the index at a fixed interval.
// It schedules periodic refresh operations on any index that implements the loadingcache.RefreshIndex interface.
// This ensures that cached indexes remain up-to-date without manual intervention.
type IntervalIndexUpdater struct {
	index             loadingcache.RefreshIndex
	interval          time.Duration
	onBackgroundError func(error)
}

// NewIntervalIndexUpdater creates a new IntervalIndexUpdater.
// onBackgroundError is called with each error returned by a background Refresh.
// It must not be nil; NewIntervalIndexUpdater panics otherwise.
// interval must be positive; NewIntervalIndexUpdater panics otherwise.
func NewIntervalIndexUpdater(index loadingcache.RefreshIndex, interval time.Duration, onBackgroundError func(error)) *IntervalIndexUpdater {
	if interval <= 0 {
		panic(fmt.Sprintf("intervalupdater: non-positive interval %v for NewIntervalIndexUpdater", interval))
	}
	if onBackgroundError == nil {
		panic("intervalupdater: nil onBackgroundError for NewIntervalIndexUpdater")
	}
	return &IntervalIndexUpdater{
		index:             index,
		interval:          interval,
		onBackgroundError: onBackgroundError,
	}
}

// LaunchBackgroundUpdater starts the background updater in a new goroutine.
// Unless the context is already canceled, the updater refreshes the index once
// immediately and then at every interval until the context is canceled.
// As with time.Ticker, ticks the updater cannot receive in time may be dropped,
// so a Refresh slower than the interval does not queue one Refresh per missed tick,
// although the next Refresh may start right after it returns.
// The updater stops after the running Refresh returns. It does not start a Refresh
// after it observes the cancellation, but a Refresh may still start if the context
// is canceled just after that check.
// Errors returned by a Refresh, including those caused by the cancellation,
// are passed to onBackgroundError.
// If a Refresh calls runtime.Goexit, the updater goroutine exits without calling
// onBackgroundError, and no further refreshes run.
//
// The updater waits for each Refresh to return. With an index that serializes
// refreshes, such as omcindex.OnMemoryIndex, background refreshes take turns with
// manual Refresh calls. While another Refresh is retrieving, the updater's Refresh
// waits for it, and canceling ctx ends that wait with ctx's error, which is passed
// to onBackgroundError. A retrieval that has started stops only if the index's
// source honors the context: if the source ignores it and never returns, that
// Refresh never finishes, its error is never reported, and the updater cannot
// refresh until it returns. See the index's Refresh documentation for details.
func (u *IntervalIndexUpdater) LaunchBackgroundUpdater(ctx context.Context) {
	go u.poll(ctx)
}

// poll polls the index at the fixed interval.
func (u *IntervalIndexUpdater) poll(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}
	if err := u.index.Refresh(ctx); err != nil {
		u.onBackgroundError(err)
	}

	ticker := time.NewTicker(u.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case <-ticker.C:
			// select picks either case when a tick and the cancellation are both ready.
			if ctx.Err() != nil {
				return
			}
			if err := u.index.Refresh(ctx); err != nil {
				u.onBackgroundError(err)
			}
		}
	}
}
