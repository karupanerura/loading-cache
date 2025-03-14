package singleflightloader_test

import (
	"context"
	"fmt"
	"sync"
	"time"

	loadingcache "github.com/karupanerura/loading-cache"
	"github.com/karupanerura/loading-cache/loader/singleflightloader"
	"github.com/karupanerura/loading-cache/source"
	"github.com/karupanerura/loading-cache/storage/memstorage"
)

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

func ExampleNewSingleFlightLoader() {
	// Create a counter to track how many times the source is called
	var callCount int
	var mu sync.Mutex

	// Create an in-memory storage
	storage := memstorage.NewInMemoryStorage[int, User]()

	// Create a source that simulates slow database access
	src := &source.FunctionsSource[int, User]{
		GetFunc: func(ctx context.Context, id int) (*loadingcache.CacheEntry[int, User], error) {
			// Simulate database lookup with delay
			time.Sleep(50 * time.Millisecond)

			// Count the number of calls
			mu.Lock()
			callCount++
			mu.Unlock()

			// Return user data
			if id == 1 {
				return &loadingcache.CacheEntry[int, User]{
					Entry: loadingcache.Entry[int, User]{
						Key:   id,
						Value: User{ID: id, Name: "Alice"},
					},
					ExpiresAt: time.Now().Add(1 * time.Hour),
				}, nil
			}
			return nil, nil
		},
	}

	// Create a single flight loader with options
	loader := singleflightloader.NewSingleFlightLoader(
		storage,
		src,
		singleflightloader.WithCloner[int, User](loadingcache.NopValueCloner[User]{}),
		singleflightloader.WithBackgroundContextProvider[int, User](context.Background),
	)

	// Create a wait group to coordinate multiple goroutines
	var wg sync.WaitGroup
	wg.Add(3)

	// Function to make a request
	makeRequest := func() {
		defer wg.Done()
		ctx := context.Background()
		entry, err := loader.LoadAndStore(ctx, 1)
		if err != nil {
			fmt.Println("Error:", err)
			return
		}

		if entry != nil {
			fmt.Printf("Goroutine found user: %s\n", entry.Value.Name)
		} else {
			fmt.Println("Goroutine: User not found")
		}
	}

	// Launch three concurrent requests for the same user
	go makeRequest()
	go makeRequest()
	go makeRequest()

	// Wait for all goroutines to complete
	wg.Wait()

	// Print how many times the source was called
	fmt.Printf("Source was called %d time(s)\n", callCount)

	// Because we can't guarantee the order of goroutine execution,
	// we don't include this in the Output block for testing
}

func ExampleSingleFlightLoader_LoadAndStoreMulti() {
	// Create a counter to track how many times the source is called
	var callCount int
	var mu sync.Mutex

	// Create an in-memory storage
	storage := memstorage.NewInMemoryStorage[int, User]()

	// Create a source that simulates slow database access
	src := &source.FunctionsSource[int, User]{
		GetMultiFunc: func(ctx context.Context, ids []int) ([]*loadingcache.CacheEntry[int, User], error) {
			// Simulate database lookup with delay
			time.Sleep(50 * time.Millisecond)

			// Count the number of calls
			mu.Lock()
			callCount++
			count := callCount // capture for consistent output
			mu.Unlock()

			fmt.Printf("Source call #%d for %d keys\n", count, len(ids))

			// Return user data
			entries := make([]*loadingcache.CacheEntry[int, User], len(ids))
			for i, id := range ids {
				if id == 1 || id == 2 {
					entries[i] = &loadingcache.CacheEntry[int, User]{
						Entry: loadingcache.Entry[int, User]{
							Key:   id,
							Value: User{ID: id, Name: fmt.Sprintf("User%d", id)},
						},
						ExpiresAt: time.Now().Add(1 * time.Hour),
					}
				}
			}
			return entries, nil
		},
	}

	// Create a single flight loader
	loader := singleflightloader.NewSingleFlightLoader(storage, src)

	// Simultaneously load multiple users
	ctx := context.Background()
	entries, err := loader.LoadAndStoreMulti(ctx, []int{1, 2, 3})
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	// Print the results
	for i, entry := range entries {
		id := i + 1
		if entry != nil {
			fmt.Printf("Found user %d: %s\n", id, entry.Value.Name)
		} else {
			fmt.Printf("User %d not found\n", id)
		}
	}

	// Output:
	// Source call #1 for 3 keys
	// Found user 1: User1
	// Found user 2: User2
	// User 3 not found
}
