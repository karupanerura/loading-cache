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

// Book represents a book entity
type Book struct {
	ID   int
	Name string
}

func (u *Book) Clone() *Book {
	return &Book{
		ID:   u.ID,
		Name: u.Name,
	}
}

func ExampleRandomizedClock() {
	// Create an in-memory storage
	storage := memstorage.NewInMemoryStorage[int, *Book](
		// With a 50% probability, the cache will expire 30 seconds earlier
		memstorage.WithClock[int, *Book](&loadingcache.RandomizedClock{
			Clock:      loadingcache.SystemClock,
			Duration:   30 * time.Second,
			Percentage: 0.5,
		}),
	)

	// Create a source that simulates loading users from a database
	src := &source.FunctionsSource[int, *Book]{
		GetFunc: func(ctx context.Context, id int) (*loadingcache.CacheEntry[int, *Book], error) {
			// Simulate database lookup
			if id == 1 {
				return &loadingcache.CacheEntry[int, *Book]{
					Entry: loadingcache.Entry[int, *Book]{
						Key:   id,
						Value: &Book{ID: id, Name: "The Great Gatsby"},
					},
					ExpiresAt: time.Now().Add(1 * time.Hour),
				}, nil
			}
			// Return nil for non-existent book (negative cache)
			return &loadingcache.CacheEntry[int, *Book]{
				Entry:         loadingcache.Entry[int, *Book]{Key: id},
				ExpiresAt:     time.Now().Add(5 * time.Minute),
				NegativeCache: true,
			}, nil
		},
	}

	// Create the loading cache
	cache := loadingcache.LoadingCache[int, *Book]{
		Loader:  singleflightloader.NewSingleFlightLoader(storage, src),
		Storage: storage,
	}

	// Get or load a book
	ctx := context.Background()
	entry, err := cache.GetOrLoad(ctx, 1)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	if entry != nil {
		fmt.Println("Found book:", entry.Value.Name)
	} else {
		fmt.Println("Book not found")
	}

	// Try to get a non-existent book
	entry, err = cache.GetOrLoad(ctx, 2)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	if entry != nil {
		fmt.Println("Found book:", entry.Value.Name)
	} else {
		fmt.Println("Book not found")
	}

	// Output:
	// Found book: The Great Gatsby
	// Book not found
}
