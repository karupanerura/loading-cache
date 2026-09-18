package memstorage_test

import (
	"maps"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
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

func TestClonerForTypeWithoutCloneMethod(t *testing.T) {
	t.Parallel()

	for name, bucketsSize := range map[string]int{"SingleBucket": 1, "MultipleBucket": 8} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			// newStorage returns a storage for a map value type, which the default cloner does not support,
			// and a counter of the calls to the given cloner.
			newStorage := func() (loadingcache.CacheStorage[uint8, map[string]int], *atomic.Int64) {
				var calls atomic.Int64
				cloner := loadingcache.ValueClonerFunc[map[string]int](func(v map[string]int) map[string]int {
					calls.Add(1)
					return maps.Clone(v)
				})
				return memstorage.NewInMemoryStorage(
					memstorage.WithBucketsSize[uint8, map[string]int](bucketsSize),
					memstorage.WithCloner[uint8](cloner),
				), &calls
			}
			newEntry := func(key uint8, value map[string]int) *loadingcache.CacheEntry[uint8, map[string]int] {
				return &loadingcache.CacheEntry[uint8, map[string]int]{
					Entry:     loadingcache.Entry[uint8, map[string]int]{Key: key, Value: value},
					ExpiresAt: time.Now().Add(time.Hour),
				}
			}

			t.Run("Single", func(t *testing.T) {
				t.Parallel()

				storage, calls := newStorage()
				input := map[string]int{"a": 1}
				if err := storage.Set(t.Context(), newEntry(1, input)); err != nil {
					t.Fatal(err)
				}
				input["a"] = 2

				got, err := storage.Get(t.Context(), 1)
				if err != nil {
					t.Fatal(err)
				}
				if df := cmp.Diff(map[string]int{"a": 1}, got.Value); df != "" {
					t.Errorf("stored value changed by mutating the input (-want +got):\n%s", df)
				}
				got.Value["a"] = 3

				got, err = storage.Get(t.Context(), 1)
				if err != nil {
					t.Fatal(err)
				}
				if df := cmp.Diff(map[string]int{"a": 1}, got.Value); df != "" {
					t.Errorf("stored value changed by mutating a returned value (-want +got):\n%s", df)
				}
				if n := calls.Load(); n != 3 {
					t.Errorf("expected the given cloner to be called 3 times, but got %d", n)
				}
			})

			t.Run("Multi", func(t *testing.T) {
				t.Parallel()

				storage, calls := newStorage()
				inputs := []map[string]int{{"a": 1}, {"b": 1}}
				if err := storage.SetMulti(t.Context(), []*loadingcache.CacheEntry[uint8, map[string]int]{
					newEntry(1, inputs[0]),
					newEntry(2, inputs[1]),
				}); err != nil {
					t.Fatal(err)
				}
				inputs[0]["a"] = 2
				inputs[1]["b"] = 2

				want := []map[string]int{{"a": 1}, {"b": 1}}
				got, err := storage.GetMulti(t.Context(), []uint8{1, 2})
				if err != nil {
					t.Fatal(err)
				}
				if df := cmp.Diff(want, entryValues(got)); df != "" {
					t.Errorf("stored values changed by mutating the inputs (-want +got):\n%s", df)
				}
				got[0].Value["a"] = 3
				got[1].Value["b"] = 3

				got, err = storage.GetMulti(t.Context(), []uint8{1, 2})
				if err != nil {
					t.Fatal(err)
				}
				if df := cmp.Diff(want, entryValues(got)); df != "" {
					t.Errorf("stored values changed by mutating returned values (-want +got):\n%s", df)
				}
				if n := calls.Load(); n != 6 {
					t.Errorf("expected the given cloner to be called 6 times, but got %d", n)
				}
			})
		})
	}

	t.Run("PanicWithoutCloner", func(t *testing.T) {
		t.Parallel()

		defer func() {
			if r := recover(); r == nil {
				t.Errorf("expected panic for a value type unsupported by the default cloner, but did not panic")
			}
		}()
		memstorage.NewInMemoryStorage[uint8, map[string]int]()
	})
}

func entryValues[K comparable, V any](entries []*loadingcache.CacheEntry[K, V]) []V {
	values := make([]V, len(entries))
	for i, entry := range entries {
		if entry != nil {
			values[i] = entry.Value
		}
	}
	return values
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

func TestConcurrentExpiration(t *testing.T) {
	t.Parallel()
	t.Run("SingleBucket", func(t *testing.T) {
		t.Parallel()

		storagetest.TestConcurrentExpiration(t, func(clock loadingcache.Clock) (loadingcache.CacheStorage[uint8, int8], func()) {
			return memstorage.NewInMemoryStorage(memstorage.WithBucketsSize[uint8, int8](1), memstorage.WithClock[uint8, int8](clock)), func() {}
		})
	})
	t.Run("MultipleBucket", func(t *testing.T) {
		t.Parallel()

		storagetest.TestConcurrentExpiration(t, func(clock loadingcache.Clock) (loadingcache.CacheStorage[uint8, int8], func()) {
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
