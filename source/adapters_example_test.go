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

// userTable is a source whose Get follows the LoadingSource contract,
// but whose GetMulti omits missing keys.
type userTable map[int]User

func (t userTable) Get(_ context.Context, id int) (*loadingcache.CacheEntry[int, User], error) {
	user, ok := t[id]
	if !ok {
		return nil, nil
	}
	return &loadingcache.CacheEntry[int, User]{
		Entry:     loadingcache.Entry[int, User]{Key: id, Value: user},
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}, nil
}

func (t userTable) GetMulti(ctx context.Context, ids []int) ([]*loadingcache.CacheEntry[int, User], error) {
	var entries []*loadingcache.CacheEntry[int, User]
	for _, id := range ids {
		entry, err := t.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		if entry != nil {
			entries = append(entries, entry)
		}
	}
	return entries, nil
}

func ExampleCompactSource() {
	src := userTable{
		1: {ID: 1, Name: "Alice", Age: 30},
		2: {ID: 2, Name: "Bob", Age: 25},
	}

	// CompactSource normalizes src.GetMulti, which omits missing keys.
	// Get calls src.Get, which already returns nil for a missing key.
	adapted := &source.CompactSource[int, User]{Source: src}

	ctx := context.Background()
	for _, id := range []int{1, 3} {
		entry, err := adapted.Get(ctx, id)
		if err != nil {
			fmt.Println("Error:", err)
			return
		}
		if entry != nil {
			fmt.Printf("Get %d: %s\n", id, entry.Value.Name)
		} else {
			fmt.Printf("Get %d: not found\n", id)
		}
	}

	ids := []int{3, 2, 1}
	entries, err := adapted.GetMulti(ctx, ids)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	for i, entry := range entries {
		if entry != nil {
			fmt.Printf("GetMulti %d: %s\n", ids[i], entry.Value.Name)
		} else {
			fmt.Printf("GetMulti %d: not found\n", ids[i])
		}
	}

	// Output:
	// Get 1: Alice
	// Get 3: not found
	// GetMulti 3: not found
	// GetMulti 2: Bob
	// GetMulti 1: Alice
}

// This example adapts a batch function that omits missing keys. The batch
// function is not a LoadingSource on its own, so CompactSource normalizes it
// and its normalized GetMulti becomes the GetMultiFunc of a FunctionsSource,
// from which Get is derived. Do not call the Get of the CompactSource here:
// it would call the unnormalized function directly.
func ExampleCompactSource_batchFunction() {
	users := map[int]User{
		1: {ID: 1, Name: "Alice", Age: 30},
		2: {ID: 2, Name: "Bob", Age: 25},
	}
	const deletedID = 4 // known not to exist; cached as a negative entry

	// findUsers returns only the entries it finds, in any order.
	findUsers := func(_ context.Context, ids []int) ([]*loadingcache.CacheEntry[int, User], error) {
		var entries []*loadingcache.CacheEntry[int, User]
		for _, id := range ids {
			if user, ok := users[id]; ok {
				entries = append(entries, &loadingcache.CacheEntry[int, User]{
					Entry:     loadingcache.Entry[int, User]{Key: id, Value: user},
					ExpiresAt: time.Now().Add(1 * time.Hour),
				})
			} else if id == deletedID {
				entries = append(entries, &loadingcache.CacheEntry[int, User]{
					Entry:         loadingcache.Entry[int, User]{Key: id},
					ExpiresAt:     time.Now().Add(5 * time.Minute),
					NegativeCache: true,
				})
			}
		}
		return entries, nil
	}

	compact := &source.CompactSource[int, User]{Source: source.GetMultiFunctionSource[int, User](findUsers)}
	src := &source.FunctionsSource[int, User]{GetMultiFunc: compact.GetMulti}

	describe := func(entry *loadingcache.CacheEntry[int, User]) string {
		switch {
		case entry == nil:
			return "not found"
		case entry.NegativeCache:
			return "negative cache"
		default:
			return entry.Value.Name
		}
	}

	ctx := context.Background()
	ids := []int{1, 3, 4}
	for _, id := range ids {
		entry, err := src.Get(ctx, id)
		if err != nil {
			fmt.Println("Error:", err)
			return
		}
		fmt.Printf("Get %d: %s\n", id, describe(entry))
	}
	entries, err := src.GetMulti(ctx, ids)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	for i, entry := range entries {
		fmt.Printf("GetMulti %d: %s\n", ids[i], describe(entry))
	}

	// Output:
	// Get 1: Alice
	// Get 3: not found
	// Get 4: negative cache
	// GetMulti 1: Alice
	// GetMulti 3: not found
	// GetMulti 4: negative cache
}
