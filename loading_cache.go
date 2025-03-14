package loadingcache

import (
	"context"
)

// LoadingCache is a cache that loads values from an external source.
type LoadingCache[K KeyConstraint, V ValueConstraint] struct {
	Loader  SourceLoader[K, V]
	Storage CacheStorage[K, V]
}

// GetOrLoad retrieves the value associated with the given key from the cache.
// If the value is not found in the cache, it loads the value from the external source.
// If an error occurs during the loading process, the method returns the zero value of V and the error.
func (c *LoadingCache[K, V]) GetOrLoad(ctx context.Context, key K) (*Entry[K, V], error) {
	if cacheEntry, err := c.Storage.Get(ctx, key); err != nil {
		return nil, err
	} else if cacheEntry != nil {
		if cacheEntry.NegativeCache {
			return nil, nil
		}
		return &cacheEntry.Entry, nil
	}

	entry, err := c.Loader.LoadAndStore(ctx, key)
	return entry, err
}

// GetOrLoadMulti retrieves multiple values from the cache.
// If a value is not found in the cache, it loads the value from the external source.
// If an error occurs during the loading process, the method returns the zero value of V and the error.
func (cl *LoadingCache[K, V]) GetOrLoadMulti(ctx context.Context, keys []K) ([]*Entry[K, V], error) {
	cacheEntries, err := cl.Storage.GetMulti(ctx, keys)
	if err != nil {
		return nil, err
	}

	entries := make([]*Entry[K, V], len(keys))
	indexes := make([]int, 0, len(keys))
	for i, entry := range cacheEntries {
		if entry == nil {
			indexes = append(indexes, i)
		} else if !entry.NegativeCache {
			entries[i] = &entry.Entry
		}
	}
	if len(indexes) == 0 {
		return entries, nil
	}

	missing := make([]K, len(indexes))
	for i, j := range indexes {
		missing[i] = keys[j]
	}
	loaded, err := cl.Loader.LoadAndStoreMulti(ctx, missing)
	if err != nil {
		return nil, err
	}

	for i, j := range indexes {
		entries[j] = loaded[i]
	}
	return entries, nil
}
