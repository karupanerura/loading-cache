package index

import (
	"context"
	"slices"

	loadingcache "github.com/karupanerura/loading-cache"
	"github.com/karupanerura/loading-cache/internal/iterutil"
)

// Keys is a struct that contains two secondary keys.
// It is used as a key for the OrIndex and AndIndex.
type Keys[LeftSecondaryKey loadingcache.KeyConstraint, RightSecondaryKey loadingcache.KeyConstraint] struct {
	Left  LeftSecondaryKey
	Right RightSecondaryKey
}

// OrIndex is an index that performs a logical OR operation on two indexes.
// It returns the union of the primary keys that are associated with the given secondary keys.
type OrIndex[LeftSecondaryKey loadingcache.KeyConstraint, RightSecondaryKey loadingcache.KeyConstraint, PrimaryKey loadingcache.KeyConstraint] struct {
	Left  loadingcache.Index[LeftSecondaryKey, PrimaryKey]
	Right loadingcache.Index[RightSecondaryKey, PrimaryKey]
}

var _ loadingcache.Index[Keys[uint8, uint8], uint8] = (*OrIndex[uint8, uint8, uint8])(nil)

// Get retrieves primary keys by secondary keys.
// It returns the union of the primary keys that are associated with the given secondary keys.
func (i *OrIndex[LeftSecondaryKey, RightSecondaryKey, PrimaryKey]) Get(ctx context.Context, key Keys[LeftSecondaryKey, RightSecondaryKey]) ([]PrimaryKey, error) {
	leftPks, err := i.Left.Get(ctx, key.Left)
	if err != nil {
		return nil, err
	}

	rightPks, err := i.Right.Get(ctx, key.Right)
	if err != nil {
		return nil, err
	}

	pks := slices.Collect(iterutil.Uniq(iterutil.Concat(slices.Values(rightPks), slices.Values(leftPks))))
	return pks, nil
}

// GetMulti retrieves primary keys by multiple secondary keys.
// It returns the union of the primary keys that are associated with the given secondary keys.
func (i *OrIndex[LeftSecondaryKey, RightSecondaryKey, PrimaryKey]) GetMulti(ctx context.Context, keys []Keys[LeftSecondaryKey, RightSecondaryKey]) (map[Keys[LeftSecondaryKey, RightSecondaryKey]][]PrimaryKey, error) {
	leftSks := slices.Collect(iterutil.Map(iterutil.OmitKey(slices.All(keys)), func(key Keys[LeftSecondaryKey, RightSecondaryKey]) LeftSecondaryKey {
		return key.Left
	}))
	rightSks := slices.Collect(iterutil.Map(iterutil.OmitKey(slices.All(keys)), func(key Keys[LeftSecondaryKey, RightSecondaryKey]) RightSecondaryKey {
		return key.Right
	}))

	leftPks, err := i.Left.GetMulti(ctx, leftSks)
	if err != nil {
		return nil, err
	}

	rightPks, err := i.Right.GetMulti(ctx, rightSks)
	if err != nil {
		return nil, err
	}

	result := make(map[Keys[LeftSecondaryKey, RightSecondaryKey]][]PrimaryKey)
	for _, key := range keys {
		result[key] = slices.Collect(
			iterutil.Uniq(
				iterutil.Concat(
					iterutil.OmitKey(slices.All(leftPks[key.Left])),
					iterutil.OmitKey(slices.All(rightPks[key.Right])),
				),
			),
		)
	}
	return result, nil
}

// AndIndex is an index that performs a logical AND operation on two indexes.
// It returns the intersection of the primary keys that are associated with the given secondary keys.
type AndIndex[LeftSecondaryKey loadingcache.KeyConstraint, RightSecondaryKey loadingcache.KeyConstraint, PrimaryKey loadingcache.KeyConstraint] struct {
	Left  loadingcache.Index[LeftSecondaryKey, PrimaryKey]
	Right loadingcache.Index[RightSecondaryKey, PrimaryKey]
}

var _ loadingcache.Index[Keys[uint8, uint8], uint8] = (*AndIndex[uint8, uint8, uint8])(nil)

// Get retrieves primary keys by secondary keys.
// It returns the intersection of the primary keys that are associated with the given secondary keys.
func (i *AndIndex[LeftSecondaryKey, RightSecondaryKey, PrimaryKey]) Get(ctx context.Context, key Keys[LeftSecondaryKey, RightSecondaryKey]) ([]PrimaryKey, error) {
	leftPks, err := i.Left.Get(ctx, key.Left)
	if err != nil {
		return nil, err
	}

	rightPks, err := i.Right.Get(ctx, key.Right)
	if err != nil {
		return nil, err
	}

	pks := slices.Collect(iterutil.Intersection(slices.Values(rightPks), slices.Values(leftPks)))
	return pks, nil
}

// GetMulti retrieves primary keys by multiple secondary keys.
// It returns the intersection of the primary keys that are associated with the given secondary keys.
func (i *AndIndex[LeftSecondaryKey, RightSecondaryKey, PrimaryKey]) GetMulti(ctx context.Context, keys []Keys[LeftSecondaryKey, RightSecondaryKey]) (map[Keys[LeftSecondaryKey, RightSecondaryKey]][]PrimaryKey, error) {
	leftSks := slices.Collect(iterutil.Map(iterutil.OmitKey(slices.All(keys)), func(key Keys[LeftSecondaryKey, RightSecondaryKey]) LeftSecondaryKey {
		return key.Left
	}))
	rightSks := slices.Collect(iterutil.Map(iterutil.OmitKey(slices.All(keys)), func(key Keys[LeftSecondaryKey, RightSecondaryKey]) RightSecondaryKey {
		return key.Right
	}))

	leftPks, err := i.Left.GetMulti(ctx, leftSks)
	if err != nil {
		return nil, err
	}

	rightPks, err := i.Right.GetMulti(ctx, rightSks)
	if err != nil {
		return nil, err
	}

	result := make(map[Keys[LeftSecondaryKey, RightSecondaryKey]][]PrimaryKey)
	for _, key := range keys {
		result[key] = slices.Collect(
			iterutil.Intersection(
				iterutil.OmitKey(slices.All(leftPks[key.Left])),
				iterutil.OmitKey(slices.All(rightPks[key.Right])),
			),
		)
	}
	return result, nil
}
