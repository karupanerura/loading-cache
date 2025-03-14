package index_test

import (
	"context"
	"fmt"
	"slices"
	"time"

	loadingcache "github.com/karupanerura/loading-cache"
	"github.com/karupanerura/loading-cache/index"
	"github.com/karupanerura/loading-cache/index/omcindex"
	"github.com/karupanerura/loading-cache/loader/singleflightloader"
	"github.com/karupanerura/loading-cache/source"
	"github.com/karupanerura/loading-cache/storage/memstorage"
)

// Product represents a simplified product entity
type Product struct {
	ID       int
	Name     string
	Category string
	InStock  bool
}

func (p Product) Clone() Product {
	return p
}

func ExampleOrIndex() {
	// Create an index for products by category
	categoryIndex := omcindex.NewOnMemoryIndex(index.FunctionIndexSource[string, int](func(ctx context.Context) (map[string][]int, error) {
		categoryToIDs := map[string][]int{
			"Electronics": {1, 2, 3},
			"Books":       {4, 5},
			"Clothing":    {6, 7},
		}
		return categoryToIDs, nil
	}))
	if err := categoryIndex.Refresh(context.Background()); err != nil {
		panic("failed to initialize index: " + err.Error())
	}

	// Create an index for products by availability
	stockIndex := omcindex.NewOnMemoryIndex(index.FunctionIndexSource[bool, int](func(ctx context.Context) (map[bool][]int, error) {
		return map[bool][]int{
			true:  {1, 3, 4, 6}, // Products in stock
			false: {2, 5, 7},    // Products out of stock
		}, nil
	}))
	if err := stockIndex.Refresh(context.Background()); err != nil {
		panic("failed to initialize index: " + err.Error())
	}

	// Create a composite OR index
	orIndex := &index.OrIndex[string, bool, int]{
		Left:  categoryIndex,
		Right: stockIndex,
	}

	// Prepare other resources
	storage := memstorage.NewInMemoryStorage[int, Product]()
	src := source.GetMultiFunctionSource[int, Product](func(_ context.Context, keys []int) ([]*loadingcache.CacheEntry[int, Product], error) {
		// Simulate database lookup
		products := map[int]Product{
			1: {ID: 1, Name: "Laptop", Category: "Electronics", InStock: true},
			2: {ID: 2, Name: "Smartphone", Category: "Electronics", InStock: false},
			3: {ID: 3, Name: "Headphones", Category: "Electronics", InStock: true},
			4: {ID: 4, Name: "Book", Category: "Books", InStock: true},
			5: {ID: 5, Name: "T-shirt", Category: "Clothing", InStock: false},
			6: {ID: 6, Name: "Jacket", Category: "Clothing", InStock: true},
			7: {ID: 7, Name: "Jeans", Category: "Clothing", InStock: false},
		}
		results := make([]*loadingcache.CacheEntry[int, Product], len(keys))
		for i, key := range keys {
			if product, ok := products[key]; ok {
				results[i] = &loadingcache.CacheEntry[int, Product]{
					Entry:     loadingcache.Entry[int, Product]{Key: key, Value: product},
					ExpiresAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
				}
			}
		}
		return results, nil
	})
	cache := loadingcache.NewIndexedLoadingCache(loadingcache.LoadingCache[int, Product]{
		Loader:  singleflightloader.NewSingleFlightLoader(storage, src),
		Storage: storage,
	}, orIndex)

	// Find products that are either in the "Electronics" category OR in stock
	ctx := context.Background()
	entries, err := cache.FindBySecondaryKey(ctx, index.Keys[string, bool]{
		Left:  "Electronics", // Category
		Right: true,          // In stock
	})

	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	// Sort IDs for consistent output
	slices.SortFunc(entries, func(a, b *loadingcache.Entry[int, Product]) int {
		return a.Key - b.Key
	})

	fmt.Println("Products that are either Electronics OR in stock:")
	for _, entry := range entries {
		fmt.Println("Product ID:", entry.Key)
	}

	// Output:
	// Products that are either Electronics OR in stock:
	// Product ID: 1
	// Product ID: 2
	// Product ID: 3
	// Product ID: 4
	// Product ID: 6
}

func ExampleAndIndex() {
	// Create an index for products by category
	categoryIndex := omcindex.NewOnMemoryIndex(index.FunctionIndexSource[string, int](func(ctx context.Context) (map[string][]int, error) {
		categoryToIDs := map[string][]int{
			"Electronics": {1, 2, 3},
			"Books":       {4, 5},
			"Clothing":    {6, 7},
		}
		return categoryToIDs, nil
	}))
	if err := categoryIndex.Refresh(context.Background()); err != nil {
		panic("failed to initialize index: " + err.Error())
	}

	// Create an index for products by availability
	stockIndex := omcindex.NewOnMemoryIndex(index.FunctionIndexSource[bool, int](func(ctx context.Context) (map[bool][]int, error) {
		return map[bool][]int{
			true:  {1, 3, 4, 6}, // Products in stock
			false: {2, 5, 7},    // Products out of stock
		}, nil
	}))
	if err := stockIndex.Refresh(context.Background()); err != nil {
		panic("failed to initialize index: " + err.Error())
	}

	// Create a composite AND index
	andIndex := &index.AndIndex[string, bool, int]{
		Left:  categoryIndex,
		Right: stockIndex,
	}

	// Prepare other resources
	storage := memstorage.NewInMemoryStorage[int, Product]()
	src := source.GetMultiFunctionSource[int, Product](func(_ context.Context, keys []int) ([]*loadingcache.CacheEntry[int, Product], error) {
		// Simulate database lookup
		products := map[int]Product{
			1: {ID: 1, Name: "Laptop", Category: "Electronics", InStock: true},
			2: {ID: 2, Name: "Smartphone", Category: "Electronics", InStock: false},
			3: {ID: 3, Name: "Headphones", Category: "Electronics", InStock: true},
			4: {ID: 4, Name: "Book", Category: "Books", InStock: true},
			5: {ID: 5, Name: "T-shirt", Category: "Clothing", InStock: false},
			6: {ID: 6, Name: "Jacket", Category: "Clothing", InStock: true},
			7: {ID: 7, Name: "Jeans", Category: "Clothing", InStock: false},
		}
		results := make([]*loadingcache.CacheEntry[int, Product], len(keys))
		for i, key := range keys {
			if product, ok := products[key]; ok {
				results[i] = &loadingcache.CacheEntry[int, Product]{
					Entry:     loadingcache.Entry[int, Product]{Key: key, Value: product},
					ExpiresAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
				}
			}
		}
		return results, nil
	})
	cache := loadingcache.NewIndexedLoadingCache(loadingcache.LoadingCache[int, Product]{
		Loader:  singleflightloader.NewSingleFlightLoader(storage, src),
		Storage: storage,
	}, andIndex)

	// Find products that are either in the "Electronics" category OR in stock
	ctx := context.Background()
	entries, err := cache.FindBySecondaryKey(ctx, index.Keys[string, bool]{
		Left:  "Electronics", // Category
		Right: true,          // In stock
	})

	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	// Sort IDs for consistent output
	slices.SortFunc(entries, func(a, b *loadingcache.Entry[int, Product]) int {
		return a.Key - b.Key
	})

	fmt.Println("Products that are both Electronics AND in stock:")
	for _, entry := range entries {
		fmt.Println("Product ID:", entry.Key)
	}

	// Output:
	// Products that are both Electronics AND in stock:
	// Product ID: 1
	// Product ID: 3
}
