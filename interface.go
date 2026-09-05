package loadingcache

import (
	"context"
	"time"
)

// KeyConstraint is an interface for key constraints.
type KeyConstraint interface {
	comparable
}

// ValueConstraint is an interface for value constraints.
type ValueConstraint interface {
	any
}

// Entry is a key-value pair.
type Entry[K KeyConstraint, V ValueConstraint] struct {
	// Key is the key of the entry.
	Key K

	// Value is the value associated with the key.
	Value V
}

// CacheEntry is a key-value pair with an expiration time.
type CacheEntry[K KeyConstraint, V ValueConstraint] struct {
	Entry[K, V]

	// ExpiresAt is the expiration time of the entry.
	// This field is required for all entries.
	ExpiresAt time.Time

	// NegativeCache indicates whether the entry is a negative cache.
	// This field is optional and indicates if the entry is a negative cache.
	// A negative cache entry means that the key does not exist in the source.
	// It is used to prevent repeated lookups for non-existent keys.
	// If NegativeCache is true, the Value field must be the zero value of V.
	// Key must still be set to the requested key for a negative-cache entry.
	NegativeCache bool
}

// CacheStorage is an interface for a cache storage backend.
// Implementations must be thread-safe.
type CacheStorage[K KeyConstraint, V ValueConstraint] interface {
	// Set stores a value with the given key and expiration time.
	// If the key already exists, it should overwrite the existing value.
	// It must clone the input entry before storing it.
	Set(context.Context, *CacheEntry[K, V]) error

	// SetMulti stores multiple values.
	// Input entries may be nil; implementations must skip them.
	// It must clone the input entries before storing them.
	SetMulti(context.Context, []*CacheEntry[K, V]) error

	// Get retrieves a value by its key.
	// It returns the value wrapped in a CacheEntry, along with its expiration time and an error, if any.
	// If the key is not found or expired, it should return nil as the CacheEntry.
	// If the key is cached as a negative cache, it should return a CacheEntry with NegativeCache set to true.
	// It must clone the returned entry before returning it.
	Get(context.Context, K) (*CacheEntry[K, V], error)

	// GetMulti retrieves multiple values by keys.
	// The order of the returned values matches the order of the input keys.
	// If a key is not found or expired, it returns nil for that key.
	// If a key is cached as a negative cache, it should return a CacheEntry with NegativeCache set to true.
	// It must clone the returned entries before returning them.
	GetMulti(context.Context, []K) ([]*CacheEntry[K, V], error)
}

// LoadingSource is an interface for loading data from an external source.
//
// The returned entries are owned by the caller: implementations must not
// retain and mutate them (including the values inside) after returning,
// otherwise the mutation would race with cache internals reading them.
// Get and GetMulti must agree on values and negative-cache behavior for each key.
type LoadingSource[K KeyConstraint, V ValueConstraint] interface {
	// Get retrieves a value by its key.
	// It returns the value wrapped in a CacheEntry, along with its expiration time and an error, if any.
	// If the key is not found, it should return nil as the CacheEntry.
	// A non-nil entry, including a negative-cache entry, must have Key equal to the requested key.
	Get(context.Context, K) (*CacheEntry[K, V], error)

	// GetMulti retrieves multiple values by their keys.
	// It returns the results as a slice of CacheEntry pointers and an error, if any.
	// On success, it must return exactly one entry per input position, including
	// repeated keys. Positions with the same key may refer to the same entry.
	// The results must maintain the same order as the input keys.
	// If a key is not found, it should return nil for that key in the result slice.
	// Each non-nil entry, including a negative-cache entry, must have Key equal to the input key at that position.
	GetMulti(context.Context, []K) ([]*CacheEntry[K, V], error)
}

// SourceLoader is an interface for loading data from an external source and storing it in the cache storage.
// Implementations must be thread-safe.
type SourceLoader[K KeyConstraint, V ValueConstraint] interface {
	// LoadAndStore loads a value by key from the external source and stores it in the cache storage.
	// An already-canceled context must return its error without starting a load.
	// Invalid result keys are rejected with an error wrapping ErrInvalidSourceResult.
	LoadAndStore(context.Context, K) (*Entry[K, V], error)

	// LoadAndStoreMulti loads multiple values by keys from the external source and stores them in the cache storage.
	// An already-canceled context must return its error without starting any loads.
	// The result must contain one entry per input position, including repeated keys, in the same order.
	// Whether entries for repeated keys share values depends on the implementation.
	// Missing or negatively cached keys are represented by nil entries.
	// Invalid result lengths or keys are rejected with an error wrapping ErrInvalidSourceResult.
	LoadAndStoreMulti(context.Context, []K) ([]*Entry[K, V], error)
}

// Index is an interface for indexing data.
// Implementations must be thread-safe.
type Index[SecondaryKey KeyConstraint, PrimaryKey KeyConstraint] interface {
	// Get retrieves primary keys by secondary key.
	// The primary keys are unique.
	Get(context.Context, SecondaryKey) ([]PrimaryKey, error)

	// GetMulti retrieves primary keys by multiple secondary keys.
	// The primary keys are unique per secondary key.
	GetMulti(context.Context, []SecondaryKey) (map[SecondaryKey][]PrimaryKey, error)
}

type CompositeIndex[LeftSecondaryKey KeyConstraint, RightSecondaryKey KeyConstraint, PrimaryKey KeyConstraint] interface {
}

// RefreshIndex is an interface for refreshing an index.
// Implementations must be thread-safe.
type RefreshIndex interface {
	// Refresh refreshes the index entries.
	// It should update the index entries based on the current state of the data source.
	Refresh(context.Context) error
}

// IndexSource is an interface for indexing data sources.
type IndexSource[SecondaryKey KeyConstraint, PrimaryKey KeyConstraint] interface {
	// GetAll retrieves all secondary keys and their corresponding primary keys.
	// A nil map with a nil error represents an empty index.
	// The returned map (including its slices) is owned by the caller:
	// implementations must return nil or a fresh map and must not retain and mutate
	// it after returning, otherwise the mutation would race with index reads.
	GetAll(context.Context) (map[SecondaryKey][]PrimaryKey, error)
}
