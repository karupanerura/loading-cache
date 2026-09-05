// Package omcindex provides an in-memory implementation of the
// loadingcache.Index interface. It maps secondary keys to primary keys.
//
// Refresh loads the whole mapping from a loadingcache.IndexSource and publishes
// it as an immutable snapshot. Reads load the current snapshot atomically
// without taking a lock and return copies of its slices.
// Refresh calls run one at a time. Each call retrieves the whole mapping with
// its own context after the call is made, so N concurrent calls retrieve it up
// to N times. A call waiting for another refresh returns when its context is
// done, and an older retrieval never overwrites a newer snapshot.
//
// # Basic usage
//
//	// Create an index source
//	source := index.FunctionIndexSource[string, int](
//	    func(ctx context.Context) (map[string][]int, error) {
//	        return map[string][]int{
//	            "user1": {101, 102, 103},
//	            "user2": {201, 202},
//	        }, nil
//	    },
//	)
//
//	// Create and initialize index
//	idx := omcindex.NewOnMemoryIndex[string, int](source)
//	err := idx.Refresh(ctx)
//	if err != nil {
//	    return err
//	}
//
//	// Look up by secondary key
//	primaryKeys, err := idx.Get(ctx, "user1")
//	// primaryKeys contains [101, 102, 103]
//
//	// Look up multiple keys
//	results, err := idx.GetMulti(ctx, []string{"user1", "user2"})
//	// results maps each key to its values
//
// # Background refreshing
//
// Use intervalupdater for periodic refreshes:
//
//	updater := intervalupdater.NewIntervalIndexUpdater(
//	    idx,                  // The index
//	    5*time.Minute,        // Refresh interval
//	    func(err error) {     // Error handler
//	        log.Printf("refresh error: %v", err)
//	    },
//	)
//	updater.LaunchBackgroundUpdater(ctx)
//
// # Behavior
//
//   - Reads block until the first successful Refresh, including one that returns an empty result
//   - Get and GetMulti return an already-canceled context's error, even after initialization
//   - A failed Refresh keeps the previous snapshot
//   - Refresh returns an already-canceled context's error without calling the source
//   - If the source calls runtime.Goexit, the goroutine running that Refresh exits without
//     affecting other Refresh calls; readers of an uninitialized index propagate the Goexit
//     until a Refresh succeeds
//   - Returned slices are copies, so callers cannot mutate the index
//
// The implementation suits read-heavy workloads with infrequent refreshes:
// a refresh rebuilds the whole mapping, while a read only loads a pointer and
// copies the matching slice.
package omcindex
