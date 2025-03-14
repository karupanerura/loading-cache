package source_test

import (
	"context"
	"fmt"
	"time"

	loadingcache "github.com/karupanerura/loading-cache"
	"github.com/karupanerura/loading-cache/source"
)

type User struct {
	ID   int
	Name string
	Age  int
}

func ExampleFunctionsSource() {
	// Create a source using functions
	src := &source.FunctionsSource[int, User]{
		GetFunc: func(ctx context.Context, id int) (*loadingcache.CacheEntry[int, User], error) {
			// Simulate database lookup
			users := map[int]User{
				1: {ID: 1, Name: "Alice", Age: 30},
				2: {ID: 2, Name: "Bob", Age: 25},
			}

			if user, ok := users[id]; ok {
				return &loadingcache.CacheEntry[int, User]{
					Entry: loadingcache.Entry[int, User]{
						Key:   id,
						Value: user,
					},
					ExpiresAt: time.Now().Add(1 * time.Hour),
				}, nil
			}

			// Return negative cache for non-existent users
			return &loadingcache.CacheEntry[int, User]{
				Entry:         loadingcache.Entry[int, User]{Key: id},
				ExpiresAt:     time.Now().Add(5 * time.Minute),
				NegativeCache: true,
			}, nil
		},
		GetMultiFunc: func(ctx context.Context, ids []int) ([]*loadingcache.CacheEntry[int, User], error) {
			// Simulate batch database lookup
			users := map[int]User{
				1: {ID: 1, Name: "Alice", Age: 30},
				2: {ID: 2, Name: "Bob", Age: 25},
			}

			entries := make([]*loadingcache.CacheEntry[int, User], len(ids))
			for i, id := range ids {
				if user, ok := users[id]; ok {
					entries[i] = &loadingcache.CacheEntry[int, User]{
						Entry: loadingcache.Entry[int, User]{
							Key:   id,
							Value: user,
						},
						ExpiresAt: time.Now().Add(1 * time.Hour),
					}
				} else {
					// Negative cache for non-existent users
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

	// Use the source to get a user
	ctx := context.Background()
	cacheEntry, err := src.Get(ctx, 1)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	if cacheEntry != nil && !cacheEntry.NegativeCache {
		fmt.Printf("Found user: %s (age %d)\n", cacheEntry.Value.Name, cacheEntry.Value.Age)
	} else {
		fmt.Println("User not found")
	}

	// Output:
	// Found user: Alice (age 30)
}

func ExampleGetMultiMapFunctionSource() {
	// Create a source using a map-returning function
	src := source.GetMultiMapFunctionSource[int, User](func(ctx context.Context, ids []int) (map[int]*loadingcache.CacheEntry[int, User], error) {
		// Simulate database lookup
		users := map[int]User{
			1: {ID: 1, Name: "Alice", Age: 30},
			2: {ID: 2, Name: "Bob", Age: 25},
		}

		result := make(map[int]*loadingcache.CacheEntry[int, User])
		for _, id := range ids {
			if user, ok := users[id]; ok {
				result[id] = &loadingcache.CacheEntry[int, User]{
					Entry: loadingcache.Entry[int, User]{
						Key:   id,
						Value: user,
					},
					ExpiresAt: time.Now().Add(1 * time.Hour),
				}
			}
		}
		return result, nil
	})

	// Get multiple users
	ctx := context.Background()
	entries, err := src.GetMulti(ctx, []int{1, 2, 3})
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	// Print results
	for i, entry := range entries {
		id := i + 1
		if entry != nil && !entry.NegativeCache {
			fmt.Printf("User %d: %s (age %d)\n", id, entry.Value.Name, entry.Value.Age)
		} else {
			fmt.Printf("User %d: not found\n", id)
		}
	}

	// Output:
	// User 1: Alice (age 30)
	// User 2: Bob (age 25)
	// User 3: not found
}

func ExampleLintSource() {
	// Create a base source
	baseSource := &source.FunctionsSource[int, User]{
		GetFunc: func(ctx context.Context, id int) (*loadingcache.CacheEntry[int, User], error) {
			// This is a valid implementation
			if id == 1 {
				return &loadingcache.CacheEntry[int, User]{
					Entry: loadingcache.Entry[int, User]{
						Key:   id,
						Value: User{ID: id, Name: "Alice", Age: 30},
					},
					ExpiresAt: time.Now().Add(1 * time.Hour),
				}, nil
			}
			return nil, nil
		},
		GetMultiFunc: func(ctx context.Context, ids []int) ([]*loadingcache.CacheEntry[int, User], error) {
			// This is a valid implementation
			entries := make([]*loadingcache.CacheEntry[int, User], len(ids))
			for i, id := range ids {
				if id == 1 {
					entries[i] = &loadingcache.CacheEntry[int, User]{
						Entry: loadingcache.Entry[int, User]{
							Key:   id,
							Value: User{ID: id, Name: "Alice", Age: 30},
						},
						ExpiresAt: time.Now().Add(1 * time.Hour),
					}
				}
			}
			return entries, nil
		},
	}

	// Wrap with a lint source to validate behavior
	lintSource := &source.LintSource[int, User]{
		Source: baseSource,
	}

	// Use the source
	ctx := context.Background()
	cacheEntry, err := lintSource.Get(ctx, 1)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	if cacheEntry != nil {
		fmt.Printf("Found user: %s (age %d)\n", cacheEntry.Value.Name, cacheEntry.Value.Age)
	} else {
		fmt.Println("User not found")
	}

	// Output:
	// Found user: Alice (age 30)
}
