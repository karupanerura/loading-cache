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
// The default value cloner is loadingcache.DefaultValueCloner.
// The loader uses it to separate the values it returns to different waiters;
// the storage clones values separately to protect cached data.
// The cloner must be safe for concurrent use by different loads.
func WithCloner[K loadingcache.KeyConstraint, V loadingcache.ValueConstraint](cloner loadingcache.ValueCloner[V]) Option[K, V] {
	return optionFunc[K, V](func(l *SingleFlightLoader[K, V]) {
		l.cloner = cloner
	})
}

// WithBackgroundContextProvider sets the context provider to the loader.
// The default context provider is context.Background.
// The provider must return a non-nil context; it need not return a new context for each call.
// A load continues with this context after callers cancel their waits.
// The loader never cancels the returned context; manage its lifetime outside the loader if needed.
// The provider is called outside the loader mutex, in the loading goroutine.
// It must be safe for concurrent calls from different loads.
func WithBackgroundContextProvider[K loadingcache.KeyConstraint, V loadingcache.ValueConstraint](provider func() context.Context) Option[K, V] {
	return optionFunc[K, V](func(l *SingleFlightLoader[K, V]) {
		l.context = provider
	})
}
