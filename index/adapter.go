package index

import (
	"context"

	loadingcache "github.com/karupanerura/loading-cache"
)

type FunctionsIndex[SecondaryKey loadingcache.KeyConstraint, PrimaryKey loadingcache.KeyConstraint] struct {
	GetFunc      func(context.Context, SecondaryKey) ([]PrimaryKey, error)
	GetMultiFunc func(context.Context, []SecondaryKey) (map[SecondaryKey][]PrimaryKey, error)
}

var _ loadingcache.Index[uint8, uint8] = (*FunctionsIndex[uint8, uint8])(nil)

func (f *FunctionsIndex[SecondaryKey, PrimaryKey]) Get(ctx context.Context, key SecondaryKey) ([]PrimaryKey, error) {
	return f.GetFunc(ctx, key)
}

func (f *FunctionsIndex[SecondaryKey, PrimaryKey]) GetMulti(ctx context.Context, keys []SecondaryKey) (map[SecondaryKey][]PrimaryKey, error) {
	return f.GetMultiFunc(ctx, keys)
}

type FunctionIndexSource[SecondaryKey loadingcache.KeyConstraint, PrimaryKey loadingcache.KeyConstraint] func(context.Context) (map[SecondaryKey][]PrimaryKey, error)

var _ loadingcache.IndexSource[uint8, uint8] = (*FunctionIndexSource[uint8, uint8])(nil)

func (f FunctionIndexSource[SecondaryKey, PrimaryKey]) GetAll(ctx context.Context) (map[SecondaryKey][]PrimaryKey, error) {
	return f(ctx)
}
