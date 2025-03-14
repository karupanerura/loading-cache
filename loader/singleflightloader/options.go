package singleflightloader

import (
	"context"

	loadingcache "github.com/karupanerura/loading-cache"
)

// Option is the interface for the options of the SingleFlightLoader.
type Option[K loadingcache.KeyConstraint, V loadingcache.ValueConstraint] interface {
	apply(*SingleFlightLoader[K, V])
}

type optionFunc[K loadingcache.KeyConstraint, V loadingcache.ValueConstraint] func(*SingleFlightLoader[K, V])

func (f optionFunc[K, V]) apply(l *SingleFlightLoader[K, V]) {
	f(l)
}

// WithCloner sets the value cloner to the loader.
// The default value cloner is loadingcache.NopValueCloner.
func WithCloner[K loadingcache.KeyConstraint, V loadingcache.ValueConstraint](cloner loadingcache.ValueCloner[V]) Option[K, V] {
	return optionFunc[K, V](func(l *SingleFlightLoader[K, V]) {
		l.cloner = cloner
	})
}

// WithBackgroundContextProvider sets the context provider to the loader.
// The provider must return a new context for each call.
// The default context provider is context.Background.
func WithBackgroundContextProvider[K loadingcache.KeyConstraint, V loadingcache.ValueConstraint](provider func() context.Context) Option[K, V] {
	return optionFunc[K, V](func(l *SingleFlightLoader[K, V]) {
		l.context = provider
	})
}
