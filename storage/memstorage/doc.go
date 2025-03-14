// Package memstorage provides an in-memory implementation of the loadingcache.CacheStorage interface.
//
// The in-memory storage can be distributed across multiple buckets for improved performance and
// concurrency. It supports various configuration options like custom key hashing, bucket sizing,
// clock implementation, and value cloning strategies.
//
// The storage handles cache entry expiration and negative caching automatically.
package memstorage
