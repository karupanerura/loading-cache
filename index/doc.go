// Package index provides secondary-index building blocks for the loading-cache
// library. An index maps a secondary key to the primary keys of the cached
// entries that match it.
//
// This package contains:
//   - AndIndex and OrIndex, which combine two indexes and are queried with Keys
//   - FunctionsIndex and FunctionIndexSource, which adapt functions to the
//     loadingcache.Index and loadingcache.IndexSource interfaces
//
// Implementations live in the subpackages:
//   - omcindex: an in-memory index that serves reads from an atomically published snapshot
//   - intervalupdater: refreshes an index in the background at a fixed interval
package index
