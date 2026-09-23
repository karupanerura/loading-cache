// Package singleflightloader provides a cache loader implementation that prevents
// duplicate loading operations for the same key or keys.
//
// This package implements a source loader that uses a single flight mechanism to avoid
// "thundering herd" problems when multiple goroutines request the same key simultaneously.
// When multiple concurrent requests for the same key are made, only one request will be
// sent to the underlying source, and the result will be shared among all requesters.
//
// Values are kept separate in two layers. The storage clones entries it stores
// and returns, which separates the cache from the callers. The loader clones the
// values it hands to callers, which separates callers from each other:
//   - When a load asks the source for a single key, the last waiter for the key
//     receives the value returned by the source, and every other waiter receives
//     a copy made with the cloner.
//   - When a load asks the source for several keys at once, each non-nil result
//     that is not a negative-cache entry is copied with the cloner for every
//     waiter, even if the key has only one waiter. The source may share mutable
//     values across keys, and the copies keep the returned values separate.
//
// The number of keys asked of the source is the number of keys the load starts
// after joining in-flight loads, not the number of keys in the caller's input.
// How deep the copy goes depends on the cloner.
//
// LoadAndStoreMulti returns when all input positions have results or when it
// receives the first error, without waiting for the other loads, which continue
// in the background.
//
// The SingleFlightLoader can be configured with options:
//   - WithCloner: Allows setting a custom value cloner to use when copying values returned to requesters
//   - WithBackgroundContextProvider: Sets a custom context provider for background operations
package singleflightloader
