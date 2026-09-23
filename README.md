# loading-cache

A generic loading cache for Go. On a cache miss, a loader fetches the value from a source and stores it in the cache storage. Entries can also be looked up by secondary keys.

[![Go Reference](https://pkg.go.dev/badge/github.com/karupanerura/loading-cache.svg)](https://pkg.go.dev/github.com/karupanerura/loading-cache)

## Features

- **Generic**: Keys and values are type parameters, so call sites need no `any` conversions
- **Concurrent**: Safe for concurrent use. The in-memory storage shards keys into buckets, each with its own read-write lock, and the in-memory index serves reads from an atomically published snapshot
- **Pluggable**: Storage, loader, source, and index are interfaces
- **Secondary indexes**: Lookup by secondary keys, with AND/OR composition of indexes
- **Batched operations**: Multi-key get and load in a single call
- **Single flight**: Concurrent loads of the same key are coalesced into one source call
- **Negative caching**: Missing keys are cached so that repeated lookups do not reach the source
- **Expiration policies**: Fixed expiration, no expiration, or randomized early expiration to spread refreshes

## Installation

```shell
go get github.com/karupanerura/loading-cache
```

Requires Go 1.26.0 or later.

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

// The value type to cache
type User struct {
	ID    int
	Name  string
	Email string
}

// Clone is required by the default value cloner for pointer and struct types (see Best Practices)
func (u *User) Clone() *User {
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
						Value: &User{ID: id, Name: "Alice", Email: "alice@example.com"},
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

A `LoadingCache` combines a storage and a loader. On a miss, the loader fetches the entry from a source and writes it to the storage.

### Cache Storage

Storage keeps cache entries and checks their expiration. The library provides:

- **memstorage**: In-memory storage. Keys are hashed into buckets, each with its own read-write lock.

The `storage` package also provides `FunctionsStorage`, which builds a storage from callbacks, and `SilentErrorStorage`, which passes storage errors to a callback and returns a cache miss instead.

```go
// Create in-memory storage with default settings
storage := memstorage.NewInMemoryStorage[int, *User]()

// Or with custom options
storage := memstorage.NewInMemoryStorage[int, *User](
    memstorage.WithBucketsSize[int, *User](512),
    memstorage.WithCloner[int, *User](customCloner),
)
```

### Loaders

Loaders fetch entries from a source and store them in the storage:

- **singleflightloader**: Coalesces concurrent loads of the same key into one source call
- **pureloader**: Calls the source once per call without coalescing. Intended for sequential use and tests.

The storage clones the entries it stores and returns, which keeps callers from mutating cached data. `SingleFlightLoader` also clones the values it hands to callers, which keeps callers from sharing values with each other: when a load asks the source for one key, the last waiter receives the source's value and the other waiters receive copies; when a load asks the source for several keys at once, every returned value is copied because the source may share mutable data across keys. `PureLoader` does not clone, so repeated keys in one call may share a value.

```go
loader := singleflightloader.NewSingleFlightLoader(
    storage,
    src,
)
```

### Sources

Sources fetch entries from external systems. `FunctionsSource` needs at least one of the two callbacks; the missing one is derived from the other. If both are set, they must agree on values and negative-cache results.

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

For a source whose batch lookup omits missing keys or returns entries out of order, wrap it in `source.CompactSource`. `CompactSource` normalizes only `GetMulti` and passes `Get` to the wrapped source, whose `Get` must follow the `LoadingSource` contract on its own. For a source that has only a batch lookup omitting missing keys, use `source.GetMultiMapFunctionSource`, or normalize the function with `CompactSource` and use the result as the `GetMultiFunc` of a `FunctionsSource`, which derives `Get` from it:

```go
compact := &source.CompactSource[int, *User]{Source: source.GetMultiFunctionSource[int, *User](findUsers)}
src := &source.FunctionsSource[int, *User]{GetMultiFunc: compact.GetMulti}
```

Do not call `compact.Get` in this setup: it calls `findUsers` directly without normalization.

A storage's `GetMulti` must return one element per input position, with nil for a missing key. `LoadingCache` returns an error for a result of any other length instead of loading the keys.

## Advanced Usage

### Negative Caching

A negative-cache entry records that the key does not exist in the source, so later lookups return "not found" without calling the source until the entry expires. Set `Key` to the requested key and leave `Value` as the zero value:

```go
// Return a negative cache entry for non-existent keys
return &loadingcache.CacheEntry[int, *User]{
    Entry:         loadingcache.Entry[int, *User]{Key: id},
    ExpiresAt:     time.Now().Add(5 * time.Minute),
    NegativeCache: true,
}, nil
```

### Secondary Indexes

An index maps a secondary key to the primary keys of matching entries. Reads block until the first successful `Refresh`, so refresh the index before serving. `Refresh` calls run one at a time, and each call retrieves the whole mapping from the source with its own context, so N concurrent calls retrieve it up to N times. A call waiting for another refresh returns when its context is done:

```go
// Create an index
categoryIndex := omcindex.NewOnMemoryIndex(index.FunctionIndexSource[string, int](
    func(ctx context.Context) (map[string][]int, error) {
        // Return every secondary key with its primary keys
    },
))

// Initialize the index before reading it.
if err := categoryIndex.Refresh(ctx); err != nil {
    log.Fatal(err)
}

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

`AndIndex` and `OrIndex` combine two indexes. They are queried with `index.Keys`, which holds an optional key for each side:

```go
// AND index
andIndex := &index.AndIndex[string, bool, int]{
    Left:  categoryIndex,
    Right: activeIndex,
}
activeAdmins, err := andIndex.Get(ctx, index.NewKeys("admin", true))

// OR index
orIndex := &index.OrIndex[string, bool, int]{
    Left:  categoryIndex,
    Right: premiumIndex,
}
adminsOrPremium, err := orIndex.Get(ctx, index.NewKeys("admin", true))

// Query one side only
admins, err := orIndex.Get(ctx, index.LeftKey[string, bool]("admin"))
```

### Background Index Updates

Unless the context is already canceled, `LaunchBackgroundUpdater` refreshes the index once immediately and then at every interval until the context is canceled. A refresh slower than the interval does not queue one refresh per missed tick, although the next refresh may start right after it returns. The updater checks the context before each refresh, but a refresh can still start if the context is canceled just after that check. Refresh errors go to the callback, including errors from a refresh that was running when the context was canceled. If a refresh calls `runtime.Goexit`, the updater stops without calling the callback. The interval must be positive; `NewIntervalIndexUpdater` panics otherwise.

The updater waits for each refresh to return. With `OnMemoryIndex`, background refreshes also wait for manual `Refresh` calls, and canceling the updater's context ends that wait with the context error, which goes to the callback. A retrieval that has started stops only if the source honors its context; a source that ignores the context and never returns blocks further refreshes, and its error never reaches the callback:

```go
updater := intervalupdater.NewIntervalIndexUpdater(
    categoryIndex,
    5*time.Minute,
    func(err error) {
        log.Printf("index refresh error: %v", err)
    },
)
updater.LaunchBackgroundUpdater(ctx)
```

## Best Practices

1. **Use value types the default cloner supports, or set a custom cloner.** The storage, the single-flight loader, and the indexed cache clone values so that callers cannot mutate cached data. The default cloner calls the value type's `Clone() V` or `DeepCopy() V` method (preferring `Clone`), passes primitive types through, and panics at construction time for any other type. An interface type is supported when it declares one of these methods itself; a nil interface value is returned as is. Interface types such as `any` and `error` are not supported even if the stored values have these methods. Each of them builds its own default cloner unless a non-nil cloner is given with its option: `memstorage.WithCloner`, `singleflightloader.WithCloner`, and `loadingcache.WithValueCloner`. For a type without these methods, such as a map, set a cloner on each of them that you use. For values that are never mutated after caching, `Clone` may return the receiver, or pass `loadingcache.NopValueCloner` to these options to skip copying.
2. **Size the buckets to your write concurrency.** `memstorage` uses `DefaultBucketsSize` buckets, each with its own lock. Raise it with `WithBucketsSize` when many goroutines write at the same time.
3. **Spread expirations.** Every entry needs `ExpiresAt`. When many entries expire at once, `expiration.EarlyExpirationPolicy` (set with `memstorage.WithExpirationPolicy`) expires some of them early at random so that refreshes do not all happen together.
4. **Decide how storage errors surface.** Loaders and the cache return storage errors to the caller. To keep serving from the source instead, wrap the storage in `storage.SilentErrorStorage`, which passes errors to a callback and returns a cache miss.

## Changes in the next release

These changes are not yet released. Several of them are breaking.

- The minimum Go version in `go.mod` is now 1.26.0. CI runs on Go 1.26.x and 1.27.x.
- `OnMemoryIndex.Refresh` treats a successful nil map as an initialized empty index. Errors preserve the previous data.
- `OnMemoryIndex.Refresh` calls run one at a time. Each call retrieves the whole mapping with its own context after the call is made, so N concurrent calls retrieve it up to N times. A call waiting for another refresh returns when its context is done, and an older retrieval no longer overwrites a newer snapshot. If the source calls `runtime.Goexit`, the goroutine running that `Refresh` exits without affecting other `Refresh` calls, and readers of an uninitialized index propagate the Goexit until a later `Refresh` succeeds.
- The no-op `OnMemoryIndex.Goexit()` method has been removed. Initialize and update the index with `Refresh`.
- Calls require a non-nil context; use `context.Background()` or `context.TODO()` as appropriate.
- An already-canceled context returns its error before any other work. `LoadingCache` checks before accessing storage, even for cache hits or empty batches. `IndexedLoadingCache` checks before querying the index, even for empty inputs or missing matches. `OnMemoryIndex` reads, `OnMemoryIndex.Refresh`, and loader calls check as well.
- `SingleFlightLoader` does not register loads for already-canceled calls. A load that is already registered continues after a caller cancels its wait.
- `SingleFlightLoader`'s context provider and cloner may run concurrently for different loads and must be safe for concurrent use.
- `memstorage.NewInMemoryStorage` builds the default cloner only when no cloner is set after the options are applied (a nil cloner counts as unset), so a custom cloner can be used for value types without `Clone` or `DeepCopy`.
- Loaders reject incorrect result counts and keys before storing them. Use `errors.Is(err, loadingcache.ErrInvalidSourceResult)` to identify these errors. Negative-cache entries must also set `Entry.Key` to the requested key.
- `CompactSource.GetMulti` deduplicates input keys, then expands results to the original input order; repeated keys share the same entry. Nil entries are ignored and missing keys produce nil slots. Unrequested or duplicate result keys are rejected with an error wrapping `ErrInvalidSourceResult`.
- `FunctionsSource` can derive either method from the other when only one callback is set. When both are set, their value and negative-cache semantics must agree.
- `NewIntervalIndexUpdater` panics for a zero or negative interval. Previously the updater goroutine panicked after the first refresh, crashing the process; now the constructor fails even if the updater is never launched.
- `LoadingCache.GetOrLoadMulti` returns an error when the storage's `GetMulti` returns a number of entries other than the number of keys. Previously a short result silently skipped loading some keys and a long result panicked.
- `DefaultValueCloner` chooses the method from the static value type. Interface types that declare `Clone() V` or `DeepCopy() V` are now supported; other interface types, such as `any` and `error`, panic with a descriptive message instead of a reflection panic.

## License

MIT License
