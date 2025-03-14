# loading-cache

A flexible, type-safe, and concurrent cache library for Go with generic support.

[![Go Reference](https://pkg.go.dev/badge/github.com/karupanerura/loading-cache.svg)](https://pkg.go.dev/github.com/karupanerura/loading-cache)

## Features

- **Type-safe**: Using Go's generics for strong type safety
- **Concurrent**: Thread-safe operations with lock-free reads when possible
- **Flexible**: Pluggable storage backends and loading mechanisms
- **Efficient**: Optimized for high concurrency with low memory overhead
- **Secondary indexes**: Support for lookup by secondary keys
- **Batched operations**: Efficient multi-key get/set operations
- **Single flight**: Prevents duplicate loading of the same key (thundering herd)
- **Negative caching**: Efficiently handles missing keys

## Installation

```shell
go get github.com/karupanerura/loading-cache
```

Requires Go 1.18+ for generics support.

## Quick Start

```go
package main

import (
	"context"
	"fmt"
	"time"

	loadingcache "github.com/karupanerura/loading-cache"
	"github.com/karupanerura/loading-cache/loader/singleflightloader"
	"github.com/karupanerura/loading-cache/source"
	"github.com/karupanerura/loading-cache/storage/memstorage"
)

// Define your custom type
type User struct {
	ID    int
	Name  string
	Email string
}

// Implement Clone method for proper copying
func (u *User) Clone() User {
	return &User{
		ID:    u.ID,
		Name:  u.Name,
		Email: u.Email,
	}
}

func main() {
	// Create storage backend
	storage := memstorage.NewInMemoryStorage[int, *User]()

	// Create a data source
	src := &source.FunctionsSource[int, *User]{
		GetFunc: func(ctx context.Context, id int) (*loadingcache.CacheEntry[int, *User], error) {
			// Simulate database lookup
			if id == 1 {
				return &loadingcache.CacheEntry[int, *User]{
					Entry: loadingcache.Entry[int, *User]{
						Key:   id,
						Value: User{ID: id, Name: "Alice", Email: "alice@example.com"},
					},
					ExpiresAt: time.Now().Add(1 * time.Hour),
				}, nil
			}
			// Return negative cache for non-existent users
			return &loadingcache.CacheEntry[int, *User]{
				Entry:         loadingcache.Entry[int, *User]{Key: id},
				ExpiresAt:     time.Now().Add(5 * time.Minute),
				NegativeCache: true,
			}, nil
		},
	}

	// Create a loader with single flight capability
	loader := singleflightloader.NewSingleFlightLoader(storage, src)

	// Create the loading cache
	cache := loadingcache.LoadingCache[int, *User]{
		Loader:  loader,
		Storage: storage,
	}

	// Get or load a user
	ctx := context.Background()
	entry, err := cache.GetOrLoad(ctx, 1)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	if entry != nil {
		fmt.Printf("Found user: %s (%s)\n", entry.Value.Name, entry.Value.Email)
	} else {
		fmt.Println("User not found")
	}
}
```

## Core Components

The library consists of several key components:

### Cache Storage

Storage backends for cache data. The library provides:

- **memstorage**: In-memory implementation with concurrent access support

```go
// Create in-memory storage with default settings
storage := memstorage.NewInMemoryStorage[int, *User]()

// Or with custom options
storage := memstorage.NewInMemoryStorage[int, *User](
    memstorage.WithBucketsSize[int, User](512),
    memstorage.WithCloner[int, User](customCloner),
)
```

### Loaders

Loaders retrieve data from external sources and store it in the cache:

- **singleflightloader**: Prevents thundering herd problem by coalescing concurrent requests

```go
loader := singleflightloader.NewSingleFlightLoader(
    storage,
    source,
)
```

### Sources

Sources define how to fetch data from external systems:

```go
src := &source.FunctionsSource[int, *User]{
    GetFunc: func(ctx context.Context, id int) (*loadingcache.CacheEntry[int, *User], error) {
        // Implementation for single key lookup
    },
    GetMultiFunc: func(ctx context.Context, ids []int) ([]*loadingcache.CacheEntry[int, *User], error) {
        // Implementation for multi-key lookup
    },
}
```

### Secondary Indexes

For looking up entries by fields other than primary keys:

```go
// Create an index
categoryIndex := omcindex.NewOnMemoryIndex(index.FunctionIndexSource[string, int](
    func(ctx context.Context) (map[string][]int, error) {
        // Implementation for get all index entries
    },
))

// Create indexed cache
indexedCache := loadingcache.NewIndexedLoadingCache(
    loadingcache.LoadingCache[int, *User]{
        Loader:  loader,
        Storage: storage,
    },
    categoryIndex,
)

// Find by secondary key
admins, err := indexedCache.FindBySecondaryKey(ctx, "admin")
```

### Composite Indexes

Support for combining indexes with logical operations:

```go
// AND index
andIndex := &index.AndIndex[string, bool, int]{
    Left:  categoryIndex,
    Right: activeIndex,
}

// OR index
orIndex := &index.OrIndex[string, bool, int]{
    Left:  categoryIndex, 
    Right: premiumIndex,
}
```

## Advanced Usage

### Negative Caching

```go
// Return a negative cache entry for non-existent keys
return &loadingcache.CacheEntry[int, User]{
    Entry:         loadingcache.Entry[int, User]{Key: id},
    ExpiresAt:     time.Now().Add(5 * time.Minute),
    NegativeCache: true,
}, nil
```

### Background Index Updates

```go
updater := intervalupdater.NewIntervalIndexUpdater(
    index,
    5*time.Minute,
    func(err error) {
        log.Printf("index refresh error: %v", err)
    },
)
updater.LaunchBackgroundUpdater(ctx)
```

## Best Practices

1. **Implement Clone methods** for complex types to ensure proper value copying
2. **Use appropriate bucket sizes** for your expected workload
3. **Set reasonable TTLs** for cache entries based on data volatility
4. **Handle errors gracefully**, especially in loaders and sources
5. **Consider NopValueCloner** for immutable types to avoid unnecessary copying

## License

MIT License
