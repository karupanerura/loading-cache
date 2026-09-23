package index

import (
	"context"

	loadingcache "github.com/karupanerura/loading-cache"
)

// FunctionsIndex is a loadingcache.Index that delegates Get and GetMulti to functions.
// Both functions must be set.
type FunctionsIndex[SecondaryKey loadingcache.KeyConstraint, PrimaryKey loadingcache.KeyConstraint] struct {
	GetFunc      func(context.Context, SecondaryKey) ([]PrimaryKey, error)
	GetMultiFunc func(context.Context, []SecondaryKey) (map[SecondaryKey][]PrimaryKey, error)
}

var _ loadingcache.Index[uint8, uint8] = (*FunctionsIndex[uint8, uint8])(nil)

// Get calls GetFunc.
func (f *FunctionsIndex[SecondaryKey, PrimaryKey]) Get(ctx context.Context, key SecondaryKey) ([]PrimaryKey, error) {
	return f.GetFunc(ctx, key)
}

// GetMulti calls GetMultiFunc.
func (f *FunctionsIndex[SecondaryKey, PrimaryKey]) GetMulti(ctx context.Context, keys []SecondaryKey) (map[SecondaryKey][]PrimaryKey, error) {
	return f.GetMultiFunc(ctx, keys)
}

// FunctionIndexSource is a loadingcache.IndexSource that delegates GetAll to a function.
type FunctionIndexSource[SecondaryKey loadingcache.KeyConstraint, PrimaryKey loadingcache.KeyConstraint] func(context.Context) (map[SecondaryKey][]PrimaryKey, error)

var _ loadingcache.IndexSource[uint8, uint8] = (*FunctionIndexSource[uint8, uint8])(nil)

// GetAll calls the function.
func (f FunctionIndexSource[SecondaryKey, PrimaryKey]) GetAll(ctx context.Context) (map[SecondaryKey][]PrimaryKey, error) {
	return f(ctx)
}
