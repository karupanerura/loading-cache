// Package sourceutil validates the shape and keys of loading source results.
package sourceutil

import (
	"fmt"

	loadingcache "github.com/karupanerura/loading-cache"
)

// ValidateEntry returns an error wrapping loadingcache.ErrInvalidSourceResult
// if a non-nil entry's Key differs from key, including for negative-cache entries.
func ValidateEntry[K loadingcache.KeyConstraint, V loadingcache.ValueConstraint](key K, entry *loadingcache.CacheEntry[K, V]) error {
	if entry != nil && entry.Key != key {
		return fmt.Errorf("%w: returned key %v, expected %v", loadingcache.ErrInvalidSourceResult, entry.Key, key)
	}
	return nil
}

// ValidateEntries returns an error wrapping loadingcache.ErrInvalidSourceResult
// unless entries has exactly one slot per key, each nil or keyed by the input
// key at the same position.
func ValidateEntries[K loadingcache.KeyConstraint, V loadingcache.ValueConstraint](keys []K, entries []*loadingcache.CacheEntry[K, V]) error {
	if len(entries) != len(keys) {
		return fmt.Errorf("%w: returned %d entries for %d keys; expected exactly one entry per key", loadingcache.ErrInvalidSourceResult, len(entries), len(keys))
	}
	for i, key := range keys {
		if err := ValidateEntry(key, entries[i]); err != nil {
			return fmt.Errorf("entry %d: %w", i, err)
		}
	}
	return nil
}
