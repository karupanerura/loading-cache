package keyhash

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"hash"
	"hash/fnv"
	"math"
	"sync"

	"github.com/goccy/go-reflect"

	loadingcache "github.com/karupanerura/loading-cache"
)

const (
	// intSize is the size of an int in bytes.
	intSize = 32 << (^uint(0) >> 63)
)

var (
	// defaultKeyHashMapMutex is a mutex for the defaultKeyHashMap.
	defaultKeyHashMapMutex = sync.RWMutex{}

	// defaultKeyHashMap is a map that stores hash functions for different types.
	defaultKeyHashMap = map[string]func(any) int{}
)

// GetOrCreateKeyHash returns a hash function for the given key type.
// It uses a map to cache the hash functions for different types.
func GetOrCreateKeyHash[K loadingcache.KeyConstraint]() func(any) int {
	var zero K
	return getOrCreateKeyHashAny(zero)
}

// getOrCreateKeyHashAny retrieves or creates a hash function for the given type.
// It uses a map to cache the hash functions for different types.
func getOrCreateKeyHashAny(t any) func(any) int {
	name := reflect.TypeOf(t).String()

	defaultKeyHashMapMutex.RLock()
	if f, ok := defaultKeyHashMap[name]; ok {
		defaultKeyHashMapMutex.RUnlock()
		return f
	}

	defaultKeyHashMapMutex.RUnlock()
	defaultKeyHashMapMutex.Lock()
	defer defaultKeyHashMapMutex.Unlock()
	if f, ok := defaultKeyHashMap[name]; ok {
		return f
	}

	f := createKeyHashAny(t)
	defaultKeyHashMap[name] = f
	return f
}

// createKeyHashAny creates a hash function for the given type.
// It uses FNV-1a hash algorithm and supports various primitive types.
func createKeyHashAny(t any) func(any) int {
	hash := hash64
	if intSize == 32 {
		hash = hash32
	}

	switch t.(type) {
	case int:
		if intSize == 32 {
			return func(v any) int {
				var b [4]byte
				binary.BigEndian.PutUint32(b[:], uint32(v.(int)))
				return hash32(b[:])
			}
		}
		return func(v any) int {
			var b [8]byte
			binary.BigEndian.PutUint64(b[:], uint64(v.(int)))
			return hash64(b[:])
		}
	case int8:
		return func(v any) int {
			var b [1]byte
			b[0] = uint8(v.(int8))
			return hash(b[:])
		}
	case int16:
		return func(v any) int {
			var b [2]byte
			binary.BigEndian.PutUint16(b[:], uint16(v.(int16)))
			return hash(b[:])
		}
	case int32:
		return func(v any) int {
			var b [4]byte
			binary.BigEndian.PutUint32(b[:], uint32(v.(int32)))
			return hash(b[:])
		}
	case int64:
		return func(v any) int {
			var b [8]byte
			binary.BigEndian.PutUint64(b[:], uint64(v.(int64)))
			return hash(b[:])
		}
	case uint:
		if intSize == 32 {
			return func(v any) int {
				var b [4]byte
				binary.BigEndian.PutUint32(b[:], uint32(v.(uint)))
				return hash32(b[:])
			}
		}
		return func(v any) int {
			var b [8]byte
			binary.BigEndian.PutUint64(b[:], uint64(v.(uint)))
			return hash64(b[:])
		}
	case uint8:
		return func(v any) int {
			var b [1]byte
			b[0] = v.(uint8)
			return hash(b[:])
		}
	case uint16:
		return func(v any) int {
			var b [2]byte
			binary.BigEndian.PutUint16(b[:], v.(uint16))
			return hash(b[:])
		}
	case uint32:
		return func(v any) int {
			var b [4]byte
			binary.BigEndian.PutUint32(b[:], v.(uint32))
			return hash(b[:])
		}
	case uint64:
		return func(v any) int {
			var b [8]byte
			binary.BigEndian.PutUint64(b[:], v.(uint64))
			return hash(b[:])
		}
	case uintptr:
		panic("uintptr cannot be hash key")
	case float32:
		return func(v any) int {
			var b [4]byte
			binary.BigEndian.PutUint32(b[:], math.Float32bits(v.(float32)))
			return hash(b[:])
		}
	case float64:
		return func(v any) int {
			var b [8]byte
			binary.BigEndian.PutUint64(b[:], math.Float64bits(v.(float64)))
			return hash(b[:])
		}
	case string:
		return func(v any) int {
			s := v.(string)

			b := bytesBufferPool.Get()
			defer bytesBufferPool.Put(b)

			_, _ = b.WriteString(s)
			return hash(b.Bytes())
		}
	default:
		panic(fmt.Sprintf("unknown type: %T", t))
	}
}

var hash32BufferPool = &resettablePool[hash.Hash32]{
	pool: sync.Pool{
		New: func() any {
			return fnv.New32a()
		},
	},
}

// hash64BufferPool is a pool for 64-bit FNV-1a hash objects.
var hash64BufferPool = &resettablePool[hash.Hash64]{
	pool: sync.Pool{
		New: func() any {
			return fnv.New64a()
		},
	},
}

// bytesBufferPool is a pool for bytes.Buffer objects.
var bytesBufferPool = &resettablePool[*bytes.Buffer]{
	pool: sync.Pool{
		New: func() any {
			return bytes.NewBuffer(make([]byte, 4096))
		},
	},
}

// resetter is an interface that defines a Reset method.
// Types that implement this interface can be used with resettablePool.
type resetter interface {
	Reset()
}

// resettablePool is a generic pool for objects that implement the resetter interface.
// It uses a sync.Pool to manage the objects and ensures that they are reset before being reused.
type resettablePool[H resetter] struct {
	pool sync.Pool
}

// Put adds an object to the pool after resetting it.
func (p *resettablePool[H]) Put(h H) {
	h.Reset()
	p.pool.Put(h)
}

// Get retrieves an object from the pool.
func (p *resettablePool[H]) Get() H {
	return p.pool.Get().(H)
}

// hash32 computes a 32-bit FNV-1a hash of the given byte slice.
func hash32(b []byte) int {
	h := hash32BufferPool.Get()
	defer hash32BufferPool.Put(h)
	_, _ = h.Write(b[:])
	return int(h.Sum32())
}

// hash64 computes a 64-bit FNV-1a hash of the given byte slice.
func hash64(b []byte) int {
	h := hash64BufferPool.Get()
	defer hash64BufferPool.Put(h)
	_, _ = h.Write(b[:])
	return int(h.Sum64())
}
