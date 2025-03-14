package memstorage

import (
	"context"
	"sort"
	"sync"

	loadingcache "github.com/karupanerura/loading-cache"
)

type bucket[K loadingcache.KeyConstraint, V loadingcache.ValueConstraint] struct {
	m  map[K]*loadingcache.CacheEntry[K, V]
	mu sync.RWMutex
}

type distributedStorage[K loadingcache.KeyConstraint, V loadingcache.ValueConstraint] struct {
	buckets []*bucket[K, V]
	options options[K, V]
}

// NewInMemoryStorage creates a new in-memory cache storage.
// The storage can be distributed across multiple buckets for improved performance and scalability.
// The storage uses a hash function to distribute the keys across the buckets.
func NewInMemoryStorage[K loadingcache.KeyConstraint, V loadingcache.ValueConstraint](opts ...Option[K, V]) loadingcache.CacheStorage[K, V] {
	options := defaultOptions[K, V]()
	for _, opt := range opts {
		opt.apply(&options)
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

// resolveBucket returns the bucket that corresponds to the given key.
func (s *distributedStorage[K, V]) resolveBucket(key K) *bucket[K, V] {
	index := s.options.hashKey(key) % len(s.buckets)
	if index < 0 {
		index *= -1
	}
	return s.buckets[index]
}

// resolveBuckets returns the indexes and buckets that correspond to the given keys.
func (s *distributedStorage[K, V]) resolveBuckets(keys []K) (indexes map[K]int, buckets []int) {
	indexes = make(map[K]int, len(keys))
	seen := make(map[int]struct{}, len(keys))
	for _, key := range keys {
		index := s.options.hashKey(key) % len(s.buckets)
		if index < 0 {
			index *= -1
		}
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
	bucket.mu.RLock()
	defer bucket.mu.RUnlock()

	if v, ok := bucket.m[key]; ok && s.options.clock.Now().Before(v.ExpiresAt) {
		return cloneCacheEntry(s.options.cloner, v), nil
	}
	return nil, nil
}

func (s *distributedStorage[K, V]) GetMulti(_ context.Context, keys []K) ([]*loadingcache.CacheEntry[K, V], error) {
	indexes, buckets := s.resolveBuckets(keys)
	if len(buckets) != 0 {
		sort.Ints(buckets)
	}
	for _, i := range buckets {
		bucket := s.buckets[i]
		bucket.mu.RLock()
		defer bucket.mu.RUnlock()
	}

	now := s.options.clock.Now()
	result := make([]*loadingcache.CacheEntry[K, V], len(keys))
	for i, key := range keys {
		bucket := s.buckets[indexes[key]]
		if v, ok := bucket.m[key]; ok && now.Before(v.ExpiresAt) {
			result[i] = cloneCacheEntry(s.options.cloner, v)
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
	s.mu.RLock()
	defer s.mu.RUnlock()

	if v, ok := s.m[key]; ok && s.options.clock.Now().Before(v.ExpiresAt) {
		return cloneCacheEntry(s.options.cloner, v), nil
	}
	return nil, nil
}

func (s *storage[K, V]) GetMulti(_ context.Context, keys []K) ([]*loadingcache.CacheEntry[K, V], error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	now := s.options.clock.Now()
	result := make([]*loadingcache.CacheEntry[K, V], len(keys))
	for i, key := range keys {
		if v, ok := s.bucket.m[key]; ok && now.Before(v.ExpiresAt) {
			result[i] = cloneCacheEntry(s.options.cloner, v)
		}
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
