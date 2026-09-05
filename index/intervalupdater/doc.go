// Package intervalupdater refreshes an index in the background at a fixed interval.
//
// IntervalIndexUpdater refreshes any loadingcache.RefreshIndex once when
// launched and then at every interval until the context is canceled. Refresh
// errors are passed to a callback and do not stop the updater.
package intervalupdater
