package keyhash_test

import (
	"hash/fnv"
	"reflect"
	"runtime"
	"sync"
	"testing"

	"github.com/karupanerura/loading-cache/internal/keyhash"
)

const (
	intSize = 32 << (^uint(0) >> 63)
)

func TestGetOrCreateKeyHash(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name     string
		hashFunc func(any) int
		value    any
		want     uint64
	}

	var tests []testCase
	if intSize == 32 {
		tests = []testCase{
			{"int", keyhash.GetOrCreateKeyHash[int](), int(-42), 0xba15cf26},
			{"int8", keyhash.GetOrCreateKeyHash[int8](), int8(-42), 0x530b44e9},
			{"int16", keyhash.GetOrCreateKeyHash[int16](), int16(-42), 0xb81e9548},
			{"int32", keyhash.GetOrCreateKeyHash[int32](), int32(-42), 0xba15cf26},
			{"int64", keyhash.GetOrCreateKeyHash[int64](), int64(-42), 0x83ae2e92},
			{"uint", keyhash.GetOrCreateKeyHash[uint](), uint(42), 0x3195cc27},
			{"uint8", keyhash.GetOrCreateKeyHash[uint8](), uint8(42), 0x2f0c9f3d},
			{"uint16", keyhash.GetOrCreateKeyHash[uint16](), uint16(42), 0x2776ba6f},
			{"uint32", keyhash.GetOrCreateKeyHash[uint32](), uint32(42), 0x3195cc27},
			{"uint64", keyhash.GetOrCreateKeyHash[uint64](), uint64(42), 0x81e14877},
			{"float32", keyhash.GetOrCreateKeyHash[float32](), float32(42.0), 0xb4eab2af},
			{"float64", keyhash.GetOrCreateKeyHash[float64](), float64(42.0), 0x2887997e},
			{"string", keyhash.GetOrCreateKeyHash[string](), "test", 0xafd071e5},
		}
	} else {
		tests = []testCase{
			{"int", keyhash.GetOrCreateKeyHash[int](), int(-42), 0x8cf5318bfca3af52},
			{"int8", keyhash.GetOrCreateKeyHash[int8](), int8(-42), 0xaf648b4c860315e9},
			{"int16", keyhash.GetOrCreateKeyHash[int16](), int16(-42), 0xa99f007b6f689a8},
			{"int32", keyhash.GetOrCreateKeyHash[int32](), int32(-42), 0x994f4d653e29f3a6},
			{"int64", keyhash.GetOrCreateKeyHash[int64](), int64(-42), 0x8cf5318bfca3af52},
			{"uint", keyhash.GetOrCreateKeyHash[uint](), uint(42), 0xa8c7de32281a0d97},
			{"uint8", keyhash.GetOrCreateKeyHash[uint8](), uint8(42), 0xaf63a74c8601927d},
			{"uint16", keyhash.GetOrCreateKeyHash[uint16](), uint16(42), 0x8329e07b4eb954f},
			{"uint32", keyhash.GetOrCreateKeyHash[uint32](), uint32(42), 0x4d255c7f9dcde7c7},
			{"uint64", keyhash.GetOrCreateKeyHash[uint64](), uint64(42), 0xa8c7de32281a0d97},
			{"float32", keyhash.GetOrCreateKeyHash[float32](), float32(42.0), 0xe64108a69be87c0f},
			{"float64", keyhash.GetOrCreateKeyHash[float64](), float64(42.0), 0xe17c3355bfbe5a7e},
			{"string", keyhash.GetOrCreateKeyHash[string](), "test", 0xf9e6e6ef197c2b25},
		}
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.hashFunc(tt.value)
			if uint64(got) != tt.want {
				t.Errorf("expected %x, got %x", tt.want, uint64(got))
			}
		})
	}
}

func TestStringKeyHashConsistency(t *testing.T) {
	hashFunc := keyhash.GetOrCreateKeyHash[string]()

	const key = "consistency-check"
	var want int
	if intSize == 32 {
		h := fnv.New32a()
		_, _ = h.Write([]byte(key))
		want = int(h.Sum32())
	} else {
		h := fnv.New64a()
		_, _ = h.Write([]byte(key))
		want = int(h.Sum64())
	}

	// Force sync.Pool to drop pooled buffers so that the next hashing
	// allocates a fresh buffer. A fresh buffer must produce the same hash
	// as a reused one.
	runtime.GC()
	runtime.GC()

	if got := hashFunc(key); got != want {
		t.Errorf("hash with a fresh pooled buffer = %#x, want %#x", got, want)
	}

	// Concurrent hashing allocates additional pool buffers; all of them must agree.
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 1000 {
				if got := hashFunc(key); got != want {
					t.Errorf("hash = %#x, want %#x", got, want)
					return
				}
			}
		}()
	}
	wg.Wait()
}

func TestGetOrCreateKeyHash_ReturnsSameFunctionForSameType(t *testing.T) {
	t.Parallel()

	hashFunc1 := keyhash.GetOrCreateKeyHash[int]()
	hashFunc2 := keyhash.GetOrCreateKeyHash[int]()
	hashFunc3 := keyhash.GetOrCreateKeyHash[int64]()

	if reflect.ValueOf(hashFunc1).Pointer() != reflect.ValueOf(hashFunc2).Pointer() {
		t.Errorf("expected the same function for the same type, but got different functions")
	}
	if reflect.ValueOf(hashFunc1).Pointer() == reflect.ValueOf(hashFunc3).Pointer() {
		t.Errorf("expected different functions for different types, but got the same function")
	}
}
