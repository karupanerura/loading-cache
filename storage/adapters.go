package storage

import (
	"context"

	loadingcache "github.com/karupanerura/loading-cache"
)

var _ loadingcache.CacheStorage[uint8, struct{}] = (*SilentErrorStorage[uint8, struct{}])(nil)

// SilentErrorStorage is a decorator for a loadingcache.CacheStorage that silently handles
// errors during operations. Instead of propagating the error, it calls the provided OnError function.
type SilentErrorStorage[K loadingcache.KeyConstraint, V loadingcache.ValueConstraint] struct {
	// Storage is the underlying storage that this decorator wraps.
	Storage loadingcache.CacheStorage[K, V]

	// OnError is a function that is called when an error occurs during an operation.
	// The error is passed to the function as an argument.
	OnError func(error)
}

// Get retrieves the value associated with the given key from the underlying storage.
// If an error occurs during the retrieval process and an OnError handler is set, the error
// will be passed to the OnError handler. If an error occurs, the method returns nil entry and nil error.
func (s *SilentErrorStorage[K, V]) Get(ctx context.Context, key K) (*loadingcache.CacheEntry[K, V], error) {
	value, err := s.Storage.Get(ctx, key)
	if err != nil {
		if s.OnError != nil {
			s.OnError(err)
		}
		return nil, nil
	}
	return value, nil
}

// GetMulti retrieves multiple entries from the underlying storage.
// If an error occurs during the retrieval process and an OnError handler is set, the error
// will be passed to the OnError handler. The method itself always returns the nil entries and nil error.
func (s *SilentErrorStorage[K, V]) GetMulti(ctx context.Context, keys []K) ([]*loadingcache.CacheEntry[K, V], error) {
	entries, err := s.Storage.GetMulti(ctx, keys)
	if err != nil {
		if s.OnError != nil {
			s.OnError(err)
		}
		return make([]*loadingcache.CacheEntry[K, V], len(keys)), nil
	}
	return entries, nil
}

// Set stores the given key-value pair in the underlying storage with an expiration time.
// If an error occurs during the storage operation and an OnError handler is set, the error
// will be passed to the OnError handler. The method itself always returns nil.
func (s *SilentErrorStorage[K, V]) Set(ctx context.Context, entry *loadingcache.CacheEntry[K, V]) error {
	if err := s.Storage.Set(ctx, entry); err != nil && s.OnError != nil {
		s.OnError(err)
	}
	return nil
}

// SetMulti stores multiple cache entries in the underlying storage.
// If an error occurs during the storage operation and an error handler is defined,
// the error handler will be invoked with the error. The method itself always returns nil.
func (s *SilentErrorStorage[K, V]) SetMulti(ctx context.Context, entries []*loadingcache.CacheEntry[K, V]) error {
	if err := s.Storage.SetMulti(ctx, entries); err != nil && s.OnError != nil {
		s.OnError(err)
	}
	return nil
}

var _ loadingcache.CacheStorage[uint8, struct{}] = (*FunctionsStorage[uint8, struct{}])(nil)

// FunctionsStorage is a loadingcache.CacheStorage implementation that uses functions to perform the storage operations.
type FunctionsStorage[K loadingcache.KeyConstraint, V loadingcache.ValueConstraint] struct {
	// SetFunc stores a value with the given key and expiration time.
	// If the key already exists, it should overwrite the existing value.
	SetFunc func(context.Context, *loadingcache.CacheEntry[K, V]) error

	// SetMultiFunc stores multiple values.
	SetMultiFunc func(context.Context, []*loadingcache.CacheEntry[K, V]) error

	// GetFunc retrieves a value by its key.
	// It returns the value wrapped in a CacheEntry, along with its expiration time and an error, if any.
	// If the key is not found or expired, it should return nil as the CacheEntry.
	// If the key is cached as a negative cache, it should return a CacheEntry with NegativeCache set to true.
	GetFunc func(context.Context, K) (*loadingcache.CacheEntry[K, V], error)

	// GetMultiFunc retrieves multiple values by keys.
	// The order of the returned values matches the order of the input keys.
	// If a key is not found or expired, it returns nil for that key.
	// If a key is cached as a negative cache, it should return a CacheEntry with NegativeCache set to true.
	GetMultiFunc func(context.Context, []K) ([]*loadingcache.CacheEntry[K, V], error)
}

// Set calls the SetFunc function to store the given key-value pair.
func (s *FunctionsStorage[K, V]) Set(ctx context.Context, entry *loadingcache.CacheEntry[K, V]) error {
	return s.SetFunc(ctx, entry)
}

// SetMulti calls the SetMultiFunc function to store multiple entries.
func (s *FunctionsStorage[K, V]) SetMulti(ctx context.Context, entries []*loadingcache.CacheEntry[K, V]) error {
	return s.SetMultiFunc(ctx, entries)
}

// Get calls the GetFunc function to retrieve the value associated with the given key.
func (s *FunctionsStorage[K, V]) Get(ctx context.Context, key K) (*loadingcache.CacheEntry[K, V], error) {
	return s.GetFunc(ctx, key)
}

// GetMulti calls the GetMultiFunc function to retrieve multiple entries.
func (s *FunctionsStorage[K, V]) GetMulti(ctx context.Context, keys []K) ([]*loadingcache.CacheEntry[K, V], error) {
	return s.GetMultiFunc(ctx, keys)
}
