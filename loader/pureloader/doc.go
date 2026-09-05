// Package pureloader provides a minimal implementation of the
// loadingcache.SourceLoader interface.
//
// PureLoader calls the source once per call and stores the result. It does not
// coalesce concurrent loads of the same key and does not clone returned values:
// the caller receives the entries returned by the source, so positions for
// repeated keys may share a value. The storage clones what it stores, so the
// returned values are still separate from the cache. Use singleflightloader for
// coalescing and cloning. It is intended for sequential use and tests.
package pureloader
