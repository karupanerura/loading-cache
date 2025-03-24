package omcindex_test

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/karupanerura/loading-cache/index"
	"github.com/karupanerura/loading-cache/index/intervalupdater"
	"github.com/karupanerura/loading-cache/index/omcindex"
)

// ExampleOnMemoryIndex demonstrates the basic usage of the OnMemoryIndex
func Example_basic() {
	// Create a simple index source that maps userIDs (string) to productIDs (int)
	// This simulates a data source that might retrieve user-product relationships
	// from a database or external service
	source := index.FunctionIndexSource[string, int](
		func(ctx context.Context) (map[string][]int, error) {
			// In a real application, this would fetch data from a database or service
			return map[string][]int{
				"user1": {101, 102, 103},
				"user2": {201, 202},
				"user3": {301},
			}, nil
		},
	)

	// Create a new OnMemoryIndex with our source
	index := omcindex.NewOnMemoryIndex[string, int](source)

	// Initialize the index
	ctx := context.Background()
	if err := index.Refresh(ctx); err != nil {
		log.Fatalf("Failed to initialize index: %v", err)
	}

	// Get products for a single user
	products, err := index.Get(ctx, "user1")
	if err != nil {
		log.Fatalf("Failed to get products: %v", err)
	}
	fmt.Printf("Products for user1: %v\n", products)

	// Get products for multiple users
	userProducts, err := index.GetMulti(ctx, []string{"user1", "user2", "user4"})
	if err != nil {
		log.Fatalf("Failed to get multiple products: %v", err)
	}

	// Print results - note that user4 won't be in the results as it doesn't exist
	for _, user := range []string{"user1", "user2", "user4"} {
		prods := userProducts[user]
		fmt.Printf("Products for %s: %v\n", user, prods)
	}

	// Output:
	// Products for user1: [101 102 103]
	// Products for user1: [101 102 103]
	// Products for user2: [201 202]
	// Products for user4: []
}

// Example_intervalUpdater demonstrates using OnMemoryIndex with IntervalIndexUpdater
// for background refreshing of the index
func Example_intervalUpdater() {
	// Create a source with simulated changing data
	// In a real application, this would be data from a database or service
	// that changes over time
	dataVersion := 0
	source := index.FunctionIndexSource[string, int](
		func(ctx context.Context) (map[string][]int, error) {
			dataVersion++
			fmt.Printf("Fetching data version %d\n", dataVersion)

			// Return different data each time to simulate updates
			return map[string][]int{
				"user1": {100 + dataVersion, 200 + dataVersion},
				"user2": {300 + dataVersion},
			}, nil
		},
	)

	// Create and initialize the index
	index := omcindex.NewOnMemoryIndex[string, int](source)
	ctx := context.Background()

	// First manual refresh
	if err := index.Refresh(ctx); err != nil {
		log.Fatalf("Failed to initialize index: %v", err)
	}

	// Query the index
	products, _ := index.Get(ctx, "user1")
	fmt.Printf("Initial products for user1: %v\n", products)

	// Create an interval updater that refreshes every 100ms
	errorHandler := func(err error) {
		log.Printf("Background refresh error: %v", err)
	}
	updater := intervalupdater.NewIntervalIndexUpdater(index, 100*time.Millisecond, errorHandler)

	// Start the background updater with a context that we'll cancel shortly
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	updater.LaunchBackgroundUpdater(ctx)

	// Sleep to allow the background updater to run a couple of times
	time.Sleep(250 * time.Millisecond)

	// Query the index again to see updated data
	products, _ = index.Get(context.Background(), "user1")
	fmt.Printf("Updated products for user1: %v\n", products)

	// We don't use Output: here because the exact timing may vary slightly
	// in different environments

	// Example output:
	// Fetching data version 1
	// Initial products for user1: [101 201]
	// Fetching data version 2
	// Fetching data version 3
	// Updated products for user1: [103 203]
}
