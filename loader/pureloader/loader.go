package pureloader

import (
	"context"

	loadingcache "github.com/karupanerura/loading-cache"
)

// PureLoader is a simple SourceLoader for sequential tasks. Useful for testing.
// It gets values from a source and caches them.
type PureLoader[K loadingcache.KeyConstraint, V loadingcache.ValueConstraint] struct {
	source  loadingcache.LoadingSource[K, V]
	storage loadingcache.CacheStorage[K, V]
}

var _ loadingcache.SourceLoader[uint8, struct{}] = (*PureLoader[uint8, struct{}])(nil)

// NewPureLoader creates a new PureLoader with the given storage and source.
func NewPureLoader[K loadingcache.KeyConstraint, V loadingcache.ValueConstraint](storage loadingcache.CacheStorage[K, V], source loadingcache.LoadingSource[K, V]) *PureLoader[K, V] {
	return &PureLoader[K, V]{
		storage: storage,
		source:  source,
	}
}

// LoadAndStore retrieves a value associated with the given key from the source,
// stores it in the storage with an expiration time, and returns the value.
// If an error occurs during retrieval or storage, it returns the zero value of V and the error.
func (p *PureLoader[K, V]) LoadAndStore(ctx context.Context, key K) (*loadingcache.Entry[K, V], error) {
	cacheEntry, err := p.source.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	if cacheEntry == nil {
		return nil, nil
	}

	if err := p.storage.Set(ctx, cacheEntry); err != nil {
		return nil, err
	}
	if cacheEntry.NegativeCache {
		return nil, nil
	}
	return &cacheEntry.Entry, nil
}

// LoadAndStoreMulti loads multiple entries from the source using the provided keys,
// stores them in the cache, and returns the loaded entries. If an error occurs during
// the loading or storing process, it returns the error.
func (p *PureLoader[K, V]) LoadAndStoreMulti(ctx context.Context, keys []K) ([]*loadingcache.Entry[K, V], error) {
	cacheEntries, err := p.source.GetMulti(ctx, keys)
	if err != nil {
		return nil, err
	}

	if err := p.storage.SetMulti(ctx, cacheEntries); err != nil {
		return nil, err
	}

	entries := make([]*loadingcache.Entry[K, V], len(cacheEntries))
	for i, e := range cacheEntries {
		if e != nil && !e.NegativeCache {
			entries[i] = &e.Entry
		}
	}
	return entries, nil
}
