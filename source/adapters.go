package source

import (
	"context"
	"fmt"

	loadingcache "github.com/karupanerura/loading-cache"
	"github.com/karupanerura/loading-cache/internal/sourceutil"
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

	if err := sourceutil.ValidateEntry(key, entry); err != nil {
		panic(err)
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
	if err := sourceutil.ValidateEntries(keys, entries); err != nil {
		panic(err)
	}
	for i := range keys {
		// nil entry means not found, so ignore it
		if entries[i] == nil {
			continue
		}

		if entries[i].ExpiresAt.IsZero() {
			panic("missing expiration time")
		}
	}
	return entries, nil
}

// FunctionsSource is a loading source that uses functions to load the values.
// At least one function must be set; an omitted function is derived from the other.
// If both are set, they must agree on values and negative-cache behavior for each key.
// Each function must itself meet the result contract described on its field.
// FunctionsSource adapts the functions and does not detect every contract violation when used directly;
// use LintSource to diagnose a source. Loaders validate result counts and keys before storing them.
type FunctionsSource[K loadingcache.KeyConstraint, V loadingcache.ValueConstraint] struct {
	// GetFunc is a function that loads a value by key.
	// It returns the value, the expiration time, and an error if any.
	// If the key is not found, it should return nil as *CacheEntry.
	// A non-nil entry, including a negative-cache entry, must have Key equal to the requested key.
	GetFunc func(context.Context, K) (*loadingcache.CacheEntry[K, V], error)

	// GetMultiFunc is a function that loads multiple values by keys.
	// It returns a slice of CacheEntry and an error if any.
	// On success, it must return exactly one entry per input key, in the same order.
	// If a key is not found, its result entry must be nil.
	// Each non-nil entry, including a negative-cache entry, must have Key equal to the input key at that position.
	GetMultiFunc func(context.Context, []K) ([]*loadingcache.CacheEntry[K, V], error)
}

var _ loadingcache.LoadingSource[uint8, struct{}] = (*FunctionsSource[uint8, struct{}])(nil)

// Get calls GetFunc, or derives a single result from GetMultiFunc if GetFunc is nil.
func (s *FunctionsSource[K, V]) Get(ctx context.Context, key K) (*loadingcache.CacheEntry[K, V], error) {
	if s.GetFunc != nil {
		return s.GetFunc(ctx, key)
	}
	if s.GetMultiFunc == nil {
		panic("source.FunctionsSource: either GetFunc or GetMultiFunc must be set")
	}
	return GetMultiFunctionSource[K, V](s.GetMultiFunc).Get(ctx, key)
}

// GetMulti calls GetMultiFunc, or calls GetFunc sequentially for each key if GetMultiFunc is nil.
// The fallback stops at the first error and returns no partial results.
func (s *FunctionsSource[K, V]) GetMulti(ctx context.Context, keys []K) ([]*loadingcache.CacheEntry[K, V], error) {
	if s.GetMultiFunc != nil {
		return s.GetMultiFunc(ctx, keys)
	}
	if s.GetFunc == nil {
		panic("source.FunctionsSource: either GetFunc or GetMultiFunc must be set")
	}
	entries := make([]*loadingcache.CacheEntry[K, V], len(keys))
	for i, key := range keys {
		entry, err := s.GetFunc(ctx, key)
		if err != nil {
			return nil, err
		}
		entries[i] = entry
	}
	return entries, nil
}

// GetMultiFunctionSource is a loading source that uses a function to load multiple entries from the source.
type GetMultiFunctionSource[K loadingcache.KeyConstraint, V loadingcache.ValueConstraint] func(context.Context, []K) ([]*loadingcache.CacheEntry[K, V], error)

var _ loadingcache.LoadingSource[uint8, struct{}] = (GetMultiFunctionSource[uint8, struct{}])(nil)

// Get calls the GetMultiFunctionSource function to load the value associated with the given key.
// It returns an error wrapping [loadingcache.ErrInvalidSourceResult] if the function does not return exactly
// one result for the requested key.
func (s GetMultiFunctionSource[K, V]) Get(ctx context.Context, key K) (*loadingcache.CacheEntry[K, V], error) {
	entries, err := s(ctx, []K{key})
	if err != nil {
		return nil, err
	}
	if err := sourceutil.ValidateEntries([]K{key}, entries); err != nil {
		return nil, err
	}
	return entries[0], nil
}

// GetMulti calls the GetMultiFunctionSource function to load multiple entries from the source.
func (s GetMultiFunctionSource[K, V]) GetMulti(ctx context.Context, keys []K) ([]*loadingcache.CacheEntry[K, V], error) {
	return s(ctx, keys)
}

// GetMultiMapFunctionSource is a loading source that uses a function to load multiple entries from the source.
// Each non-nil entry, including a negative-cache entry, must have Key equal to its map key.
// The map key alone is not sufficient; loaders reject mismatches with an error wrapping
// [loadingcache.ErrInvalidSourceResult].
// The function must itself meet this contract. The adapter selects the entries for the requested keys,
// with nil for missing keys, and does not return map entries for unrequested keys.
// It does not detect every contract violation when used directly.
// Repeated input keys refer to the same returned entry; this adapter does not clone values.
// Because the map is indexed by key before the loader sees the results, duplicate
// entries for a key are not detectable, unlike with CompactSource.
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

// CompactSource adapts a source whose GetMulti may omit missing keys, include
// nil entries, or return entries in any order. Only GetMulti is normalized;
// Get calls the underlying Get, which must follow the LoadingSource contract on
// its own and agree with the normalized GetMulti on values and negative-cache
// results. For a source that has only a batch function omitting missing keys,
// use GetMultiMapFunctionSource, or wrap the function in GetMultiFunctionSource
// and CompactSource and pass the CompactSource's GetMulti as the GetMultiFunc of
// a FunctionsSource, which derives Get from the normalized results.
// A source that already follows LoadingSource's positional contract can be used directly.
// Input keys are deduplicated before loading, preserving their first occurrence order.
// The source must return at most one non-nil entry per requested key; duplicate
// or unrequested result keys return an error wrapping [loadingcache.ErrInvalidSourceResult].
// Results are expanded to the original input order, with nil for missing keys.
// Repeated input keys refer to the same returned entry; this adapter does not clone values.
//
// CompactSource checks the results before normalization, where duplicates and
// unrequested keys are still visible. Loaders check the normalized results: one
// entry per input position, each with the key at that position. An entry whose
// Key is left unset has the zero key, so it is rejected unless the zero key was
// requested, in which case it cannot be told apart from a valid entry for that key.
type CompactSource[K loadingcache.KeyConstraint, V loadingcache.ValueConstraint] struct {
	Source loadingcache.LoadingSource[K, V]
}

var _ loadingcache.LoadingSource[uint8, struct{}] = (*CompactSource[uint8, struct{}])(nil)

// Get retrieves the value associated with the given key from the underlying source.
func (s *CompactSource[K, V]) Get(ctx context.Context, key K) (*loadingcache.CacheEntry[K, V], error) {
	return s.Source.Get(ctx, key)
}

// GetMulti retrieves multiple entries from the source.
// It returns exactly one entry per input key, in the same order, with nil for missing keys.
func (s *CompactSource[K, V]) GetMulti(ctx context.Context, keys []K) ([]*loadingcache.CacheEntry[K, V], error) {
	byKey := make(map[K]*loadingcache.CacheEntry[K, V], len(keys))
	uniqueKeys := make([]K, 0, len(keys))
	for _, key := range keys {
		if _, exists := byKey[key]; !exists {
			byKey[key] = nil
			uniqueKeys = append(uniqueKeys, key)
		}
	}
	entries, err := s.Source.GetMulti(ctx, uniqueKeys)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		previous, requested := byKey[entry.Key]
		if !requested {
			return nil, fmt.Errorf("%w: unrequested key %v", loadingcache.ErrInvalidSourceResult, entry.Key)
		}
		if previous != nil {
			return nil, fmt.Errorf("%w: duplicate result for key %v", loadingcache.ErrInvalidSourceResult, entry.Key)
		}
		byKey[entry.Key] = entry
	}

	result := make([]*loadingcache.CacheEntry[K, V], len(keys))
	for i, key := range keys {
		result[i] = byKey[key]
	}
	return result, nil
}
