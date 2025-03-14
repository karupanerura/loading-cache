package intervalupdater_test

import (
	"context"
	"log"
	"time"

	loadingcache "github.com/karupanerura/loading-cache"
	"github.com/karupanerura/loading-cache/index/intervalupdater"
)

type nopRefreshIndex struct{}

func (nopRefreshIndex) Refresh(context.Context) error { return nil }

var index loadingcache.RefreshIndex = nopRefreshIndex{}

func Example() {
	updater := intervalupdater.NewIntervalIndexUpdater(index, 10*time.Minute, func(err error) {
		log.Printf("background updater error: %v", err)
	})
	updater.LaunchBackgroundUpdater(context.Background())
}
