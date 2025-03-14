package memstorage_test

import (
	loadingcache "github.com/karupanerura/loading-cache"
	"github.com/karupanerura/loading-cache/storage/memstorage"
)

type MyValue struct {
	Number uint8
}

func ExampleNewInMemoryStorage() {
	// Create a simple in-memory storage
	storage := memstorage.NewInMemoryStorage[string, MyValue]()

	_ = storage
}

func ExampleNewInMemoryStorage_opts() {
	// Create a storage with custom options
	storage := memstorage.NewInMemoryStorage[string, MyValue](
		memstorage.WithBucketsSize[string, MyValue](512),
		memstorage.WithKeyHash[string, MyValue](func(key string) int {
			return len(key)
		}),
		memstorage.WithCloner[string, MyValue](loadingcache.NopValueCloner[MyValue]{}),
	)

	_ = storage
}
