package memstorage

import (
	loadingcache "github.com/karupanerura/loading-cache"
	"github.com/karupanerura/loading-cache/internal/keyhash"
)

// DefaultBucketsSize is the default number of buckets in the cache.
var DefaultBucketsSize = 256

// Option is the interface for the options of the in-memory cache storage.
type Option[K loadingcache.KeyConstraint, V loadingcache.ValueConstraint] interface {
	apply(*options[K, V])
}

type optionFunc[K loadingcache.KeyConstraint, V loadingcache.ValueConstraint] func(*options[K, V])

func (f optionFunc[K, V]) apply(o *options[K, V]) {
	f(o)
}

// WithKeyHash sets the key hash function to the storage.
func WithKeyHash[K loadingcache.KeyConstraint, V loadingcache.ValueConstraint](f func(K) int) Option[K, V] {
	return optionFunc[K, V](func(o *options[K, V]) {
		o.hashKey = func(key any) int {
			return f(key.(K))
		}
	})
}

// WithBucketsSize sets the number of buckets in the cache.
// The number of buckets must be a natural number.
func WithBucketsSize[K loadingcache.KeyConstraint, V loadingcache.ValueConstraint](bucketsSize int) Option[K, V] {
	if bucketsSize <= 0 {
		panic("bucketSize must be natural number")
	}
	return optionFunc[K, V](func(o *options[K, V]) {
		o.bucketsSize = bucketsSize
	})
}

// WithClock sets the clock to the storage.
func WithClock[K loadingcache.KeyConstraint, V loadingcache.ValueConstraint](clock loadingcache.Clock) Option[K, V] {
	return optionFunc[K, V](func(o *options[K, V]) {
		o.clock = clock
	})
}

// WithCloner sets the value cloner to the storage.
func WithCloner[K loadingcache.KeyConstraint, V loadingcache.ValueConstraint](cloner loadingcache.ValueCloner[V]) Option[K, V] {
	return optionFunc[K, V](func(o *options[K, V]) {
		o.cloner = cloner
	})
}

type options[K loadingcache.KeyConstraint, V loadingcache.ValueConstraint] struct {
	hashKey     func(any) int
	bucketsSize int
	clock       loadingcache.Clock
	cloner      loadingcache.ValueCloner[V]
}

func defaultOptions[K loadingcache.KeyConstraint, V loadingcache.ValueConstraint]() options[K, V] {
	return options[K, V]{
		hashKey:     keyhash.GetOrCreateKeyHash[K](),
		bucketsSize: DefaultBucketsSize,
		clock:       loadingcache.SystemClock,
		cloner:      loadingcache.DefaultValueCloner[V](),
	}
}
