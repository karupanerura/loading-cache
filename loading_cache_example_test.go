package loadingcache_test

import (
	"context"
	"fmt"
	"time"

	loadingcache "github.com/karupanerura/loading-cache"
	"github.com/karupanerura/loading-cache/loader/singleflightloader"
	"github.com/karupanerura/loading-cache/source"
	"github.com/karupanerura/loading-cache/storage/memstorage"
)

// User represents a user entity
type User struct {
	ID   int
	Name string
}

func (u User) Clone() User {
	return User{
		ID:   u.ID,
		Name: u.Name,
	}
}

func ExampleLoadingCache_GetOrLoad() {
	// Create an in-memory storage
	storage := memstorage.NewInMemoryStorage[int, User]()

	// Create a source that simulates loading users from a database
	src := &source.FunctionsSource[int, User]{
		GetFunc: func(ctx context.Context, id int) (*loadingcache.CacheEntry[int, User], error) {
			// Simulate database lookup
			if id == 1 {
				return &loadingcache.CacheEntry[int, User]{
					Entry: loadingcache.Entry[int, User]{
						Key:   id,
						Value: User{ID: id, Name: "Alice"},
					},
					ExpiresAt: time.Now().Add(1 * time.Hour),
				}, nil
			}
			// Return nil for non-existent user (negative cache)
			return &loadingcache.CacheEntry[int, User]{
				Entry:         loadingcache.Entry[int, User]{Key: id},
				ExpiresAt:     time.Now().Add(5 * time.Minute),
				NegativeCache: true,
			}, nil
		},
	}

	// Create the loading cache
	cache := loadingcache.LoadingCache[int, User]{
		Loader:  singleflightloader.NewSingleFlightLoader(storage, src),
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
		fmt.Println("Found user:", entry.Value.Name)
	} else {
		fmt.Println("User not found")
	}

	// Try to get a non-existent user
	entry, err = cache.GetOrLoad(ctx, 2)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	if entry != nil {
		fmt.Println("Found user:", entry.Value.Name)
	} else {
		fmt.Println("User not found")
	}

	// Output:
	// Found user: Alice
	// User not found
}

func ExampleLoadingCache_GetOrLoadMulti() {
	// Create an in-memory storage
	storage := memstorage.NewInMemoryStorage[int, User]()

	// Create a source that simulates loading users from a database
	src := &source.FunctionsSource[int, User]{
		GetMultiFunc: func(ctx context.Context, ids []int) ([]*loadingcache.CacheEntry[int, User], error) {
			entries := make([]*loadingcache.CacheEntry[int, User], len(ids))
			for i, id := range ids {
				switch id {
				case 1:
					entries[i] = &loadingcache.CacheEntry[int, User]{
						Entry: loadingcache.Entry[int, User]{
							Key:   id,
							Value: User{ID: id, Name: "Alice"},
						},
						ExpiresAt: time.Now().Add(1 * time.Hour),
					}
				case 2:
					entries[i] = &loadingcache.CacheEntry[int, User]{
						Entry: loadingcache.Entry[int, User]{
							Key:   id,
							Value: User{ID: id, Name: "Bob"},
						},
						ExpiresAt: time.Now().Add(1 * time.Hour),
					}
				default:
					// For non-existent users, use negative cache
					entries[i] = &loadingcache.CacheEntry[int, User]{
						Entry:         loadingcache.Entry[int, User]{Key: id},
						ExpiresAt:     time.Now().Add(5 * time.Minute),
						NegativeCache: true,
					}
				}
			}
			return entries, nil
		},
	}

	// Create the loading cache
	cache := loadingcache.LoadingCache[int, User]{
		Loader:  singleflightloader.NewSingleFlightLoader(storage, src),
		Storage: storage,
	}

	// Get or load multiple users
	ctx := context.Background()
	entries, err := cache.GetOrLoadMulti(ctx, []int{1, 2, 3})
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	// Print the results
	for i, entry := range entries {
		if entry != nil {
			fmt.Printf("User %d: %s\n", i+1, entry.Value.Name)
		} else {
			fmt.Printf("User %d: not found\n", i+1)
		}
	}

	// Output:
	// User 1: Alice
	// User 2: Bob
	// User 3: not found
}
