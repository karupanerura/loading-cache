// Package omcindex provides an in-memory implementation of the index interface
// for the loading-cache library. It maps secondary keys to primary keys and
// supports concurrent reads with atomic updates.
//
// Basic Usage:
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
// Background Refreshing:
//
// Use with intervalupdater for automatic updates:
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
// OnMemoryIndex Features:
//
// - Thread-safe for concurrent reads
// - Atomic index updates via Refresh()
// - First reads block until index is initialized
// - All operations respect context cancellation
// - Copies returned data to prevent mutation
//
// The implementation is optimized for read-heavy workloads where updates
// are infrequent. When initialized with many keys or large value slices,
// memory usage scales proportionally.
package omcindex
