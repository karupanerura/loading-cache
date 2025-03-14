// Package singleflightloader provides a cache loader implementation that prevents
// duplicate loading operations for the same key or keys.
//
// This package implements a source loader that uses a single flight mechanism to avoid
// "thundering herd" problems when multiple goroutines request the same key simultaneously.
// When multiple concurrent requests for the same key are made, only one request will be
// sent to the underlying source, and the result will be shared among all requesters.
//
// The SingleFlightLoader can be configured with options:
//   - WithCloner: Allows setting a custom value cloner to use when copying values to multiple requesters
//   - WithBackgroundContextProvider: Sets a custom context provider for background operations
package singleflightloader
