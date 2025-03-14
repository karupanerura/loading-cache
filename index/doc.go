// Package index provides utilities and implementations for secondary indexing in the
// loading-cache library. It enables efficient lookups of cached items by secondary keys.
//
// The primary subpackages are:
//
// - omcindex: On-memory implementation with atomic updates
// - intervalupdater: Automatic index refreshing at timed intervals
//
// The index package is designed to integrate with the loading-cache library
// for efficient caching with multiple access patterns. All implementations
// follow consistent interface patterns and handle concurrency appropriately.
package index
