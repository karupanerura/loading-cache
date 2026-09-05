package intervalupdater

import (
	"context"
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
// It must not be nil.
func NewIntervalIndexUpdater(index loadingcache.RefreshIndex, interval time.Duration, onBackgroundError func(error)) *IntervalIndexUpdater {
	return &IntervalIndexUpdater{
		index:             index,
		interval:          interval,
		onBackgroundError: onBackgroundError,
	}
}

// LaunchBackgroundUpdater starts the background updater in a new goroutine.
// The updater refreshes the index once immediately and then at every interval
// until the context is canceled.
func (u *IntervalIndexUpdater) LaunchBackgroundUpdater(ctx context.Context) {
	go u.poll(ctx)
}

// poll polls the index at the fixed interval.
func (u *IntervalIndexUpdater) poll(ctx context.Context) {
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
			if err := u.index.Refresh(ctx); err != nil {
				u.onBackgroundError(err)
			}
		}
	}
}
