// Package intervalupdater refreshes an index in the background at a fixed interval.
//
// IntervalIndexUpdater refreshes any loadingcache.RefreshIndex once when
// launched with a context that is not yet canceled, and then at every interval
// until the context is canceled. It checks the context before each Refresh, so
// it does not start one after it observes the cancellation. Refresh errors are
// passed to a callback and do not stop the updater. If a Refresh calls
// runtime.Goexit, the updater stops without calling the callback.
//
// The updater waits for each Refresh to return, so a Refresh whose source ignores
// the context and never returns stops further refreshes. With an index that
// serializes refreshes, such as omcindex.OnMemoryIndex, background refreshes also
// wait for manual Refresh calls, and canceling the updater's context ends that wait.
package intervalupdater
