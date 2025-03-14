package loadingcache_test

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

// Product represents a product entity
type Product struct {
	ID       int
	Name     string
	Category string
	Price    float64
}

func (p Product) Clone() Product {
	return p
}

func ExampleIndexedLoadingCache_FindBySecondaryKey() {
	// Create an in-memory storage for products
	storage := memstorage.NewInMemoryStorage[int, Product]()

	// Create a source for products
	src := &source.FunctionsSource[int, Product]{
		GetMultiFunc: func(ctx context.Context, ids []int) ([]*loadingcache.CacheEntry[int, Product], error) {
			// Simulate database lookup
			products := map[int]Product{
				1: {ID: 1, Name: "Laptop", Category: "Electronics", Price: 999.99},
				2: {ID: 2, Name: "Smartphone", Category: "Electronics", Price: 599.99},
				3: {ID: 3, Name: "Headphones", Category: "Electronics", Price: 99.99},
				4: {ID: 4, Name: "Book", Category: "Books", Price: 19.99},
				5: {ID: 5, Name: "T-shirt", Category: "Clothing", Price: 29.99},
			}

			entries := make([]*loadingcache.CacheEntry[int, Product], len(ids))
			for i, id := range ids {
				if product, ok := products[id]; ok {
					entries[i] = &loadingcache.CacheEntry[int, Product]{
						Entry: loadingcache.Entry[int, Product]{
							Key:   id,
							Value: product,
						},
						ExpiresAt: time.Now().Add(1 * time.Hour),
					}
				}
			}
			return entries, nil
		},
	}

	// Create a category index
	categoryIndex := omcindex.NewOnMemoryIndex(index.FunctionIndexSource[string, int](func(ctx context.Context) (map[string][]int, error) {
		// Simulate database lookup
		return map[string][]int{
			"Electronics": {1, 2, 3},
			"Books":       {4},
			"Clothing":    {5},
		}, nil
	}))
	if err := categoryIndex.Refresh(context.Background()); err != nil {
		panic("failed to initialize index: " + err.Error())
	}

	// Create the indexed loading cache
	cache := loadingcache.NewIndexedLoadingCache(loadingcache.LoadingCache[int, Product]{
		Loader:  singleflightloader.NewSingleFlightLoader(storage, src),
		Storage: storage,
	}, categoryIndex)

	// Find products by category
	ctx := context.Background()
	products, err := cache.FindBySecondaryKey(ctx, "Electronics")
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	// Sort products by ID for consistent output
	slices.SortFunc(products, func(i, j *loadingcache.Entry[int, Product]) int {
		return i.Key - j.Key
	})

	fmt.Println("Electronics products:")
	for _, product := range products {
		fmt.Printf("- %s: $%.2f\n", product.Value.Name, product.Value.Price)
	}

	// Output:
	// Electronics products:
	// - Laptop: $999.99
	// - Smartphone: $599.99
	// - Headphones: $99.99
}

func ExampleIndexedLoadingCache_FindBySecondaryKeys() {
	// Create an in-memory storage for products
	storage := memstorage.NewInMemoryStorage[int, Product]()

	// Create a source for products
	src := &source.FunctionsSource[int, Product]{
		GetMultiFunc: func(ctx context.Context, ids []int) ([]*loadingcache.CacheEntry[int, Product], error) {
			// Simulate database lookup
			products := map[int]Product{
				1: {ID: 1, Name: "Laptop", Category: "Electronics", Price: 999.99},
				2: {ID: 2, Name: "Smartphone", Category: "Electronics", Price: 599.99},
				3: {ID: 3, Name: "Headphones", Category: "Electronics", Price: 99.99},
				4: {ID: 4, Name: "Book", Category: "Books", Price: 19.99},
				5: {ID: 5, Name: "T-shirt", Category: "Clothing", Price: 29.99},
			}

			entries := make([]*loadingcache.CacheEntry[int, Product], len(ids))
			for i, id := range ids {
				if product, ok := products[id]; ok {
					entries[i] = &loadingcache.CacheEntry[int, Product]{
						Entry: loadingcache.Entry[int, Product]{
							Key:   id,
							Value: product,
						},
						ExpiresAt: time.Now().Add(1 * time.Hour),
					}
				}
			}
			return entries, nil
		},
	}

	// Create a category index
	categoryIndex := omcindex.NewOnMemoryIndex(index.FunctionIndexSource[string, int](func(ctx context.Context) (map[string][]int, error) {
		// Simulate database lookup
		return map[string][]int{
			"Electronics": {1, 2, 3},
			"Books":       {4},
			"Clothing":    {5},
		}, nil
	}))
	if err := categoryIndex.Refresh(context.Background()); err != nil {
		panic("failed to initialize index: " + err.Error())
	}

	// Create the indexed loading cache
	cache := loadingcache.NewIndexedLoadingCache(loadingcache.LoadingCache[int, Product]{
		Loader:  singleflightloader.NewSingleFlightLoader(storage, src),
		Storage: storage,
	}, categoryIndex)

	// Find products by multiple categories
	ctx := context.Background()
	categoryProducts, err := cache.FindBySecondaryKeys(ctx, []string{"Books", "Clothing"})
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	// Process the results
	categories := []string{"Books", "Clothing"} // Using a slice to ensure consistent order
	for _, category := range categories {
		products := categoryProducts[category]
		fmt.Printf("%s products:\n", category)
		if len(products) == 0 {
			fmt.Println("- None")
			continue
		}

		// Sort products by ID for consistent output
		slices.SortFunc(products, func(i, j *loadingcache.Entry[int, Product]) int {
			return i.Key - j.Key
		})

		for _, product := range products {
			fmt.Printf("- %s: $%.2f\n", product.Value.Name, product.Value.Price)
		}
	}

	// Output:
	// Books products:
	// - Book: $19.99
	// Clothing products:
	// - T-shirt: $29.99
}
