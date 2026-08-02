package omcindex

import (
	"context"

	loadingcache "github.com/karupanerura/loading-cache"
)

// Option is the interface for the options of the OnMemoryIndex.
type Option[SecondaryKey loadingcache.KeyConstraint, PrimaryKey loadingcache.KeyConstraint] interface {
	apply(*OnMemoryIndex[SecondaryKey, PrimaryKey])
}

type optionFunc[SecondaryKey loadingcache.KeyConstraint, PrimaryKey loadingcache.KeyConstraint] func(*OnMemoryIndex[SecondaryKey, PrimaryKey])

func (f optionFunc[SecondaryKey, PrimaryKey]) apply(i *OnMemoryIndex[SecondaryKey, PrimaryKey]) {
	f(i)
}

// WithBackgroundContextProvider sets the context provider used for the lazy
// initial load triggered by Get/GetMulti. The provider must return a new
// context for each call. The default context provider is context.Background.
//
// The initial load keeps running until the source returns, even after every
// waiter has given up (canceled its own context). A hanging source can
// therefore only be bounded by returning a context with a deadline from this
// provider.
func WithBackgroundContextProvider[SecondaryKey loadingcache.KeyConstraint, PrimaryKey loadingcache.KeyConstraint](provider func() context.Context) Option[SecondaryKey, PrimaryKey] {
	return optionFunc[SecondaryKey, PrimaryKey](func(i *OnMemoryIndex[SecondaryKey, PrimaryKey]) {
		i.context = provider
	})
}
