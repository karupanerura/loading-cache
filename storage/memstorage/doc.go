// Package memstorage provides an in-memory implementation of the loadingcache.CacheStorage interface.
//
// Keys are hashed into DefaultBucketsSize buckets, each guarded by its own
// read-write lock, so operations on different buckets do not contend. Options
// set the bucket count (WithBucketsSize), the key hash (WithKeyHash), the clock
// (WithClock), the value cloner (WithCloner), and the expiration policy
// (WithExpirationPolicy).
//
// Expired entries are skipped on read and deleted at that point; there is no
// background sweeper.
package memstorage
