package memstorage_test

import (
	"strconv"
	"testing"

	loadingcache "github.com/karupanerura/loading-cache"
	"github.com/karupanerura/loading-cache/storage/memstorage"
	"github.com/karupanerura/loading-cache/storage/storagetest"
)

func BenchmarkSet(b *testing.B) {
	b.Run("SingleBucket", func(b *testing.B) {
		storage := memstorage.NewInMemoryStorage(memstorage.WithBucketsSize[uint8, int8](1))
		keys := make([]uint8, 1024)
		for i := range keys {
			keys[i] = uint8(i % 256)
		}
		storagetest.BenchmarkSet(b, storage, keys)
	})
	b.Run("MultipleBucket", func(b *testing.B) {
		storage := memstorage.NewInMemoryStorage(memstorage.WithKeyHash[uint8, int8](func(u uint8) int {
			return int(u)
		}))
		keys := make([]uint8, 1024)
		for i := range keys {
			keys[i] = uint8(i % 256)
		}
		storagetest.BenchmarkSet(b, storage, keys)
	})
}

func TestConsistency(t *testing.T) {
	t.Parallel()
	for i := range 7 {
		i := i
		t.Run(strconv.Itoa(i+1), func(t *testing.T) {
			t.Parallel()

			storagetest.TestConsistency(t, func() (loadingcache.CacheStorage[uint8, int8], func()) {
				return memstorage.NewInMemoryStorage(memstorage.WithBucketsSize[uint8, int8](i + 1)), func() {}
			})
		})
	}
}

func TestKeyHash(t *testing.T) {
	t.Parallel()
	for i := range 7 {
		i := i
		t.Run(strconv.Itoa(i+1), func(t *testing.T) {
			t.Parallel()

			storagetest.TestConsistency(t, func() (loadingcache.CacheStorage[uint8, int8], func()) {
				bucketSize := i + 1
				return memstorage.NewInMemoryStorage(memstorage.WithBucketsSize[uint8, int8](bucketSize), memstorage.WithKeyHash[uint8, int8](func(key uint8) int {
					return int(key) % bucketSize
				})), func() {}
			})
		})
	}
}

func TestCloneStruct(t *testing.T) {
	t.Parallel()
	t.Run("SingleBucket", func(t *testing.T) {
		t.Parallel()

		storagetest.TestCloneStruct(t, func() (loadingcache.CacheStorage[uint8, *storagetest.TestClonerStruct], func()) {
			return memstorage.NewInMemoryStorage(memstorage.WithBucketsSize[uint8, *storagetest.TestClonerStruct](1)), func() {}
		})
	})
	t.Run("MultipleBucket", func(t *testing.T) {
		t.Parallel()

		storagetest.TestCloneStruct(t, func() (loadingcache.CacheStorage[uint8, *storagetest.TestClonerStruct], func()) {
			return memstorage.NewInMemoryStorage(memstorage.WithBucketsSize[uint8, *storagetest.TestClonerStruct](8)), func() {}
		})
	})
}

func TestDeepCopyStruct(t *testing.T) {
	t.Parallel()
	t.Run("SingleBucket", func(t *testing.T) {
		t.Parallel()

		storagetest.TestDeepCopyStruct(t, func() (loadingcache.CacheStorage[uint8, *storagetest.TestDeepCopyerStruct], func()) {
			return memstorage.NewInMemoryStorage(memstorage.WithBucketsSize[uint8, *storagetest.TestDeepCopyerStruct](1)), func() {}
		})
	})
	t.Run("MultipleBucket", func(t *testing.T) {
		t.Parallel()

		storagetest.TestDeepCopyStruct(t, func() (loadingcache.CacheStorage[uint8, *storagetest.TestDeepCopyerStruct], func()) {
			return memstorage.NewInMemoryStorage(memstorage.WithBucketsSize[uint8, *storagetest.TestDeepCopyerStruct](8)), func() {}
		})
	})
}

func TestCloner(t *testing.T) {
	t.Parallel()
	t.Run("SingleBucket", func(t *testing.T) {
		t.Parallel()

		storagetest.TestDeepCopyStruct(t, func() (loadingcache.CacheStorage[uint8, *storagetest.TestDeepCopyerStruct], func()) {
			return memstorage.NewInMemoryStorage(
				memstorage.WithBucketsSize[uint8, *storagetest.TestDeepCopyerStruct](1),
				memstorage.WithCloner[uint8](loadingcache.ValueClonerFunc[*storagetest.TestDeepCopyerStruct](func(v *storagetest.TestDeepCopyerStruct) *storagetest.TestDeepCopyerStruct {
					return v.DeepCopy()
				})),
			), func() {}
		})
	})
	t.Run("MultipleBucket", func(t *testing.T) {
		t.Parallel()

		storagetest.TestDeepCopyStruct(t, func() (loadingcache.CacheStorage[uint8, *storagetest.TestDeepCopyerStruct], func()) {
			return memstorage.NewInMemoryStorage(
				memstorage.WithBucketsSize[uint8, *storagetest.TestDeepCopyerStruct](8),
				memstorage.WithCloner[uint8](loadingcache.ValueClonerFunc[*storagetest.TestDeepCopyerStruct](func(v *storagetest.TestDeepCopyerStruct) *storagetest.TestDeepCopyerStruct {
					return v.DeepCopy()
				})),
			), func() {}
		})
	})
}

func TestExpiration(t *testing.T) {
	t.Parallel()
	t.Run("SingleBucket", func(t *testing.T) {
		t.Parallel()

		storagetest.TestExpiration(t, func(clock loadingcache.Clock) (loadingcache.CacheStorage[uint8, int8], func()) {
			return memstorage.NewInMemoryStorage(memstorage.WithBucketsSize[uint8, int8](1), memstorage.WithClock[uint8, int8](clock)), func() {}
		})
	})
	t.Run("MultipleBucket", func(t *testing.T) {
		t.Parallel()

		storagetest.TestExpiration(t, func(clock loadingcache.Clock) (loadingcache.CacheStorage[uint8, int8], func()) {
			return memstorage.NewInMemoryStorage(memstorage.WithBucketsSize[uint8, int8](8), memstorage.WithClock[uint8, int8](clock)), func() {}
		})
	})
}

func TestNegativeCache(t *testing.T) {
	t.Parallel()
	t.Run("SingleBucket", func(t *testing.T) {
		t.Parallel()

		storagetest.TestNegativeCache(t, func(clock loadingcache.Clock) (loadingcache.CacheStorage[uint8, int8], func()) {
			return memstorage.NewInMemoryStorage(memstorage.WithBucketsSize[uint8, int8](1), memstorage.WithClock[uint8, int8](clock)), func() {}
		})
	})
	t.Run("MultipleBucket", func(t *testing.T) {
		t.Parallel()

		storagetest.TestNegativeCache(t, func(clock loadingcache.Clock) (loadingcache.CacheStorage[uint8, int8], func()) {
			return memstorage.NewInMemoryStorage(memstorage.WithBucketsSize[uint8, int8](8), memstorage.WithClock[uint8, int8](clock)), func() {}
		})
	})
}
