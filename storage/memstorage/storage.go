package memstorage

import (
	"context"
	"sort"
	"sync"
	"time"

	loadingcache "github.com/karupanerura/loading-cache"
	"github.com/karupanerura/loading-cache/expiration"
)

type bucket[K loadingcache.KeyConstraint, V loadingcache.ValueConstraint] struct {
	m  map[K]*loadingcache.CacheEntry[K, V]
	mu sync.RWMutex
}

// deleteExpired removes the entries for the given keys if they are still expired.
// It re-checks the expiration under the write lock because an entry may have been
// replaced by a concurrent Set after it was observed as expired under the read lock.
func (b *bucket[K, V]) deleteExpired(policy expiration.ExpirationPolicy, now time.Time, keys []K) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, key := range keys {
		if v, ok := b.m[key]; ok && policy.IsExpired(now, v.ExpiresAt) {
			delete(b.m, key)
		}
	}
}

type distributedStorage[K loadingcache.KeyConstraint, V loadingcache.ValueConstraint] struct {
	buckets []*bucket[K, V]
	options options[K, V]
}

// NewInMemoryStorage creates a new in-memory cache storage.
// Keys are hashed into buckets (DefaultBucketsSize by default), each with its own
// read-write lock, so operations on different buckets do not contend.
func NewInMemoryStorage[K loadingcache.KeyConstraint, V loadingcache.ValueConstraint](opts ...Option[K, V]) loadingcache.CacheStorage[K, V] {
	options := defaultOptions[K, V]()
	for _, opt := range opts {
		opt.apply(&options)
	}
	if options.cloner == nil {
		// Build the default cloner only when no cloner is set (a nil cloner counts as unset),
		// so a custom cloner can be used for value types the default cloner does not support.
		options.cloner = loadingcache.DefaultValueCloner[V]()
	}

	if options.bucketsSize == 1 {
		return &storage[K, V]{
			bucket:  bucket[K, V]{m: map[K]*loadingcache.CacheEntry[K, V]{}},
			options: options,
		}
	}

	buckets := make([]*bucket[K, V], options.bucketsSize)
	for i := range buckets {
		buckets[i] = &bucket[K, V]{m: map[K]*loadingcache.CacheEntry[K, V]{}}
	}

	return &distributedStorage[K, V]{
		buckets: buckets,
		options: options,
	}
}

var _ loadingcache.CacheStorage[uint8, struct{}] = (*distributedStorage[uint8, struct{}])(nil)

// bucketIndex returns the index of the bucket that corresponds to the given key.
func (s *distributedStorage[K, V]) bucketIndex(key K) int {
	index := s.options.hashKey(key) % len(s.buckets)
	if index < 0 {
		index += len(s.buckets)
	}
	return index
}

// resolveBucket returns the bucket that corresponds to the given key.
func (s *distributedStorage[K, V]) resolveBucket(key K) *bucket[K, V] {
	return s.buckets[s.bucketIndex(key)]
}

// resolveBuckets returns the indexes and buckets that correspond to the given keys.
func (s *distributedStorage[K, V]) resolveBuckets(keys []K) (indexes map[K]int, buckets []int) {
	indexes = make(map[K]int, len(keys))
	seen := make(map[int]struct{}, len(keys))
	for _, key := range keys {
		index := s.bucketIndex(key)
		indexes[key] = index
		if _, ok := seen[index]; !ok {
			buckets = append(buckets, index)
			seen[index] = struct{}{}
		}
	}
	return
}

func (s *distributedStorage[K, V]) Get(_ context.Context, key K) (*loadingcache.CacheEntry[K, V], error) {
	bucket := s.resolveBucket(key)
	now := s.options.clock.Now()

	bucket.mu.RLock()
	var entry *loadingcache.CacheEntry[K, V]
	expired := false
	if v, ok := bucket.m[key]; ok {
		if s.options.expirationPolicy.IsExpired(now, v.ExpiresAt) {
			expired = true
		} else {
			entry = cloneCacheEntry(s.options.cloner, v)
		}
	}
	bucket.mu.RUnlock()

	if expired {
		bucket.deleteExpired(s.options.expirationPolicy, now, []K{key})
	}
	return entry, nil
}

func (s *distributedStorage[K, V]) GetMulti(_ context.Context, keys []K) ([]*loadingcache.CacheEntry[K, V], error) {
	indexes, buckets := s.resolveBuckets(keys)
	if len(buckets) != 0 {
		sort.Ints(buckets)
	}
	for _, i := range buckets {
		s.buckets[i].mu.RLock()
	}

	now := s.options.clock.Now()
	result := make([]*loadingcache.CacheEntry[K, V], len(keys))
	var expiredKeys []K
	for i, key := range keys {
		bucket := s.buckets[indexes[key]]
		if v, ok := bucket.m[key]; ok {
			if s.options.expirationPolicy.IsExpired(now, v.ExpiresAt) {
				expiredKeys = append(expiredKeys, key)
			} else {
				result[i] = cloneCacheEntry(s.options.cloner, v)
			}
		}
	}

	for i := len(buckets) - 1; i >= 0; i-- {
		s.buckets[buckets[i]].mu.RUnlock()
	}

	if len(expiredKeys) != 0 {
		expiredKeysByBucket := make(map[int][]K)
		for _, key := range expiredKeys {
			index := indexes[key]
			expiredKeysByBucket[index] = append(expiredKeysByBucket[index], key)
		}
		for index, bucketKeys := range expiredKeysByBucket {
			s.buckets[index].deleteExpired(s.options.expirationPolicy, now, bucketKeys)
		}
	}
	return result, nil
}

func (s *distributedStorage[K, V]) Set(_ context.Context, entry *loadingcache.CacheEntry[K, V]) error {
	bucket := s.resolveBucket(entry.Key)
	bucket.mu.Lock()
	defer bucket.mu.Unlock()

	bucket.m[entry.Key] = cloneCacheEntry(s.options.cloner, entry)
	return nil
}

func (s *distributedStorage[K, V]) SetMulti(_ context.Context, entries []*loadingcache.CacheEntry[K, V]) error {
	keys := make([]K, 0, len(entries))
	for _, entry := range entries {
		if entry != nil {
			keys = append(keys, entry.Key)
		}
	}

	indexes, buckets := s.resolveBuckets(keys)
	if len(buckets) != 0 {
		sort.Ints(buckets)
	}
	for _, index := range buckets {
		bucket := s.buckets[index]
		bucket.mu.Lock()
		defer bucket.mu.Unlock()
	}

	for _, e := range entries {
		if e != nil {
			bucket := s.buckets[indexes[e.Key]]
			bucket.m[e.Key] = cloneCacheEntry(s.options.cloner, e)
		}
	}
	return nil
}

type storage[K loadingcache.KeyConstraint, V loadingcache.ValueConstraint] struct {
	bucket[K, V]
	options options[K, V]
}

var _ loadingcache.CacheStorage[uint8, struct{}] = (*storage[uint8, struct{}])(nil)

func (s *storage[K, V]) Get(_ context.Context, key K) (*loadingcache.CacheEntry[K, V], error) {
	now := s.options.clock.Now()

	s.mu.RLock()
	var entry *loadingcache.CacheEntry[K, V]
	expired := false
	if v, ok := s.m[key]; ok {
		if s.options.expirationPolicy.IsExpired(now, v.ExpiresAt) {
			expired = true
		} else {
			entry = cloneCacheEntry(s.options.cloner, v)
		}
	}
	s.mu.RUnlock()

	if expired {
		s.bucket.deleteExpired(s.options.expirationPolicy, now, []K{key})
	}
	return entry, nil
}

func (s *storage[K, V]) GetMulti(_ context.Context, keys []K) ([]*loadingcache.CacheEntry[K, V], error) {
	now := s.options.clock.Now()

	s.mu.RLock()
	result := make([]*loadingcache.CacheEntry[K, V], len(keys))
	var expiredKeys []K
	for i, key := range keys {
		if v, ok := s.m[key]; ok {
			if s.options.expirationPolicy.IsExpired(now, v.ExpiresAt) {
				expiredKeys = append(expiredKeys, key)
			} else {
				result[i] = cloneCacheEntry(s.options.cloner, v)
			}
		}
	}
	s.mu.RUnlock()

	if len(expiredKeys) != 0 {
		s.bucket.deleteExpired(s.options.expirationPolicy, now, expiredKeys)
	}
	return result, nil
}

func (s *storage[K, V]) Set(_ context.Context, entry *loadingcache.CacheEntry[K, V]) error {
	s.bucket.mu.Lock()
	defer s.bucket.mu.Unlock()

	s.bucket.m[entry.Key] = cloneCacheEntry(s.options.cloner, entry)
	return nil
}

func (s *storage[K, V]) SetMulti(_ context.Context, entries []*loadingcache.CacheEntry[K, V]) error {
	s.bucket.mu.Lock()
	defer s.bucket.mu.Unlock()

	for _, e := range entries {
		if e != nil {
			s.bucket.m[e.Key] = cloneCacheEntry(s.options.cloner, e)
		}
	}
	return nil
}

func cloneCacheEntry[K loadingcache.KeyConstraint, V loadingcache.ValueConstraint](cloner loadingcache.ValueCloner[V], v *loadingcache.CacheEntry[K, V]) *loadingcache.CacheEntry[K, V] {
	if v.NegativeCache {
		return &loadingcache.CacheEntry[K, V]{
			Entry:         loadingcache.Entry[K, V]{Key: v.Key},
			ExpiresAt:     v.ExpiresAt,
			NegativeCache: true,
		}
	}
	return &loadingcache.CacheEntry[K, V]{
		Entry: loadingcache.Entry[K, V]{
			Key:   v.Key,
			Value: cloner.CloneValue(v.Value),
		},
		ExpiresAt: v.ExpiresAt,
	}
}
