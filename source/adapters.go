package source

import (
	"context"

	loadingcache "github.com/karupanerura/loading-cache"
)

// LintSource is a loading source that is used for linting purposes.
// It uses a source to load the values.
type LintSource[K loadingcache.KeyConstraint, V loadingcache.ValueConstraint] struct {
	Source loadingcache.LoadingSource[K, V]
}

var _ loadingcache.LoadingSource[uint8, struct{}] = (*LintSource[uint8, struct{}])(nil)

// Get retrieves the value associated with the given key from the source.
// It validates the behavior of the source implementation, ensuring it properly follows the LoadingSource contract.
// In particular, it checks that Get returns the result for the given key with a valid expiration time.
func (s *LintSource[K, V]) Get(ctx context.Context, key K) (*loadingcache.CacheEntry[K, V], error) {
	entry, err := s.Source.Get(ctx, key)
	if err != nil {
		return nil, err
	}

	// nil entry means not found, so ignore it
	if entry == nil {
		return nil, nil
	}

	if entry.Key != key {
		panic("key mismatch")
	}
	if entry.ExpiresAt.IsZero() {
		panic("missing expiration time")
	}
	return entry, nil
}

// GetMulti retrieves multiple entries from the source.
// It validates the behavior of the source implementation, ensuring it properly follows the LoadingSource contract.
// In particular, it checks that GetMulti returns results for all keys in the correct order.
func (s *LintSource[K, V]) GetMulti(ctx context.Context, keys []K) ([]*loadingcache.CacheEntry[K, V], error) {
	entries, err := s.Source.GetMulti(ctx, keys)
	if err != nil {
		return nil, err
	}
	if len(entries) != len(keys) {
		panic("must return results for all keys in the same order as the keys")
	}
	for i := range keys {
		// nil entry means not found, so ignore it
		if entries[i] == nil {
			continue
		}

		if entries[i].Key != keys[i] {
			panic("key order mismatch")
		} else if entries[i].ExpiresAt.IsZero() {
			panic("missing expiration time")
		}
	}
	return entries, nil
}

// FunctionsSource is a loading source that uses functions to load the values.
type FunctionsSource[K loadingcache.KeyConstraint, V loadingcache.ValueConstraint] struct {
	// GetFunc is a function that loads a value by key.
	// It returns the value, the expiration time, and an error if any.
	// If the key is not found, it should return nil as *CacheEntry.
	GetFunc      func(context.Context, K) (*loadingcache.CacheEntry[K, V], error)

	// GetMultiFunc is a function that loads multiple values by keys.
	// It returns a slice of CacheEntry and an error if any.
	// Must return results for all keys in the same order as the input keys.
	// If a key is not found, it should return nil as *CacheEntry.
	GetMultiFunc func(context.Context, []K) ([]*loadingcache.CacheEntry[K, V], error)
}

var _ loadingcache.LoadingSource[uint8, struct{}] = (*FunctionsSource[uint8, struct{}])(nil)

// Get calls the GetFunc function to load the value associated with the given key.
func (s *FunctionsSource[K, V]) Get(ctx context.Context, key K) (*loadingcache.CacheEntry[K, V], error) {
	return s.GetFunc(ctx, key)
}

// GetMulti calls the GetMultiFunc function to load multiple entries from the source.
func (s *FunctionsSource[K, V]) GetMulti(ctx context.Context, keys []K) ([]*loadingcache.CacheEntry[K, V], error) {
	return s.GetMultiFunc(ctx, keys)
}

// GetMultiFunctionSource is a loading source that uses a function to load multiple entries from the source.
type GetMultiFunctionSource[K loadingcache.KeyConstraint, V loadingcache.ValueConstraint] func(context.Context, []K) ([]*loadingcache.CacheEntry[K, V], error)

var _ loadingcache.LoadingSource[uint8, struct{}] = (GetMultiFunctionSource[uint8, struct{}])(nil)

// Get calls the GetMultiFunctionSource function to load the value associated with the given key.
func (s GetMultiFunctionSource[K, V]) Get(ctx context.Context, key K) (*loadingcache.CacheEntry[K, V], error) {
	entries, err := s(ctx, []K{key})
	if err != nil {
		return nil, err
	}
	return entries[0], nil
}

// GetMulti calls the GetMultiFunctionSource function to load multiple entries from the source.
func (s GetMultiFunctionSource[K, V]) GetMulti(ctx context.Context, keys []K) ([]*loadingcache.CacheEntry[K, V], error) {
	return s(ctx, keys)
}

// GetMultiMapFunctionSource is a loading source that uses a function to load multiple entries from the source.
type GetMultiMapFunctionSource[K loadingcache.KeyConstraint, V loadingcache.ValueConstraint] func(context.Context, []K) (map[K]*loadingcache.CacheEntry[K, V], error)

var _ loadingcache.LoadingSource[uint8, struct{}] = (GetMultiMapFunctionSource[uint8, struct{}])(nil)

// Get calls the GetMultiMapFunctionSource function to load the value associated with the given key.
func (s GetMultiMapFunctionSource[K, V]) Get(ctx context.Context, key K) (*loadingcache.CacheEntry[K, V], error) {
	entries, err := s(ctx, []K{key})
	if err != nil {
		return nil, err
	}
	return entries[key], nil
}

// GetMulti calls the GetMultiMapFunctionSource function to load multiple entries from the source.
func (s GetMultiMapFunctionSource[K, V]) GetMulti(ctx context.Context, keys []K) ([]*loadingcache.CacheEntry[K, V], error) {
	entries, err := s(ctx, keys)
	if err != nil {
		return nil, err
	}

	results := make([]*loadingcache.CacheEntry[K, V], len(keys))
	for i, key := range keys {
		results[i] = entries[key]
	}
	return results, nil
}

// CompactSource is a loading source that uses a source to load the values.
// It ensures the results for missing keys are nil in the result of GetMulti.
type CompactSource[K loadingcache.KeyConstraint, V loadingcache.ValueConstraint] struct {
	Source loadingcache.LoadingSource[K, V]
}

var _ loadingcache.LoadingSource[uint8, struct{}] = (*CompactSource[uint8, struct{}])(nil)

// Get retrieves the value associated with the given key from the source.
func (s *CompactSource[K, V]) Get(ctx context.Context, key K) (*loadingcache.CacheEntry[K, V], error) {
	return s.Source.Get(ctx, key)
}

// GetMulti retrieves multiple entries from the source.
// It invokes the GetMulti method of the source and directly returns the results if all keys are found.
//
// If the source omits missing keys from the returned entries, this method ensures the result includes nil entries
// for the missing keys, maintaining the same order as the input keys.
func (s *CompactSource[K, V]) GetMulti(ctx context.Context, keys []K) ([]*loadingcache.CacheEntry[K, V], error) {
	entries, err := s.Source.GetMulti(ctx, keys)
	if err != nil {
		return nil, err
	}
	if len(entries) == len(keys) {
		return entries, nil
	}

	m := make(map[K]*loadingcache.CacheEntry[K, V], len(entries))
	for _, entry := range entries {
		m[entry.Key] = entry
	}

	entries = make([]*loadingcache.CacheEntry[K, V], len(keys))
	for i, key := range keys {
		entries[i] = m[key]
	}
	return entries, nil
}
