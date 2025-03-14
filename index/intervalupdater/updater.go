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
// The IntervalIndexUpdater includes a callback mechanism for handling errors that occur during
// background refresh operations. When creating an updater, you must provide an error handler function as a parameter.
func NewIntervalIndexUpdater(index loadingcache.RefreshIndex, interval time.Duration, onBackgroundError func(error)) *IntervalIndexUpdater {
	return &IntervalIndexUpdater{
		index:             index,
		interval:          interval,
		onBackgroundError: onBackgroundError,
	}
}

// LaunchBackgroundUpdater starts the background updater.
// The background updater can be stopped by canceling the context passed to LaunchBackgroundUpdater.
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
