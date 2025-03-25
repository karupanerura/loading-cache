package index

import (
	"context"
	"iter"
	"slices"

	loadingcache "github.com/karupanerura/loading-cache"
	"github.com/karupanerura/loading-cache/internal/iterutil"
)

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
	var leftPks []PrimaryKey
	if !key.Left.Empty {
		var err error
		leftPks, err = i.Left.Get(ctx, key.Left.Key)
		if err != nil {
			return nil, err
		}
	}

	var rightPks []PrimaryKey
	if !key.Right.Empty {
		var err error
		rightPks, err = i.Right.Get(ctx, key.Right.Key)
		if err != nil {
			return nil, err
		}
	}

	total := len(leftPks) + len(rightPks)
	switch total {
	case 0:
		return nil, nil
	case len(leftPks):
		return leftPks, nil
	case len(rightPks):
		return rightPks, nil
	default:
		pks := slices.Collect(iterutil.Union(slices.Values(rightPks), slices.Values(leftPks)))
		return pks, nil
	}
}

// GetMulti retrieves primary keys by multiple secondary keys.
// It returns the union of the primary keys that are associated with the given secondary keys.
func (i *OrIndex[LeftSecondaryKey, RightSecondaryKey, PrimaryKey]) GetMulti(ctx context.Context, keys []Keys[LeftSecondaryKey, RightSecondaryKey]) (map[Keys[LeftSecondaryKey, RightSecondaryKey]][]PrimaryKey, error) {
	leftSks := slices.Collect(iterutil.Uniq(iterutil.FlatMap(slices.Values(keys), func(key Keys[LeftSecondaryKey, RightSecondaryKey]) iter.Seq[LeftSecondaryKey] {
		return key.Left.Iter()
	})))
	rightSks := slices.Collect(iterutil.Uniq(iterutil.FlatMap(slices.Values(keys), func(key Keys[LeftSecondaryKey, RightSecondaryKey]) iter.Seq[RightSecondaryKey] {
		return key.Right.Iter()
	})))

	var leftPks map[LeftSecondaryKey][]PrimaryKey
	if len(leftSks) != 0 {
		var err error
		leftPks, err = i.Left.GetMulti(ctx, leftSks)
		if err != nil {
			return nil, err
		}
	}

	var rightPks map[RightSecondaryKey][]PrimaryKey
	if len(rightSks) != 0 {
		var err error
		rightPks, err = i.Right.GetMulti(ctx, rightSks)
		if err != nil {
			return nil, err
		}
	}

	result := make(map[Keys[LeftSecondaryKey, RightSecondaryKey]][]PrimaryKey, len(keys))
	for _, key := range keys {
		var left []PrimaryKey
		if !key.Left.Empty {
			if pks, ok := leftPks[key.Left.Key]; ok {
				left = pks
			}
		}

		var right []PrimaryKey
		if !key.Right.Empty {
			if pks, ok := rightPks[key.Right.Key]; ok {
				right = pks
			}
		}

		total := len(left) + len(right)
		switch total {
		case 0:
			continue
		case len(left):
			result[key] = left
		case len(right):
			result[key] = right
		default:
			result[key] = slices.Collect(iterutil.Union(slices.Values(left), slices.Values(right)))
		}
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
	var leftPks []PrimaryKey
	if !key.Left.Empty {
		var err error
		leftPks, err = i.Left.Get(ctx, key.Left.Key)
		if err != nil {
			return nil, err
		}
	}

	var rightPks []PrimaryKey
	if !key.Right.Empty {
		var err error
		rightPks, err = i.Right.Get(ctx, key.Right.Key)
		if err != nil {
			return nil, err
		}
	}

	total := len(leftPks) + len(rightPks)
	switch total {
	case 0:
		return nil, nil
	case len(leftPks):
		if key.Right.Empty {
			return leftPks, nil
		}
		return nil, nil
	case len(rightPks):
		if key.Left.Empty {
			return rightPks, nil
		}
		return nil, nil
	default:
		pks := slices.Collect(iterutil.Intersection(slices.Values(leftPks), slices.Values(rightPks)))
		return pks, nil
	}
}

// GetMulti retrieves primary keys by multiple secondary keys.
// It returns the intersection of the primary keys that are associated with the given secondary keys.
func (i *AndIndex[LeftSecondaryKey, RightSecondaryKey, PrimaryKey]) GetMulti(ctx context.Context, keys []Keys[LeftSecondaryKey, RightSecondaryKey]) (map[Keys[LeftSecondaryKey, RightSecondaryKey]][]PrimaryKey, error) {
	leftSks := slices.Collect(iterutil.Uniq(iterutil.FlatMap(slices.Values(keys), func(key Keys[LeftSecondaryKey, RightSecondaryKey]) iter.Seq[LeftSecondaryKey] {
		return key.Left.Iter()
	})))
	rightSks := slices.Collect(iterutil.Uniq(iterutil.FlatMap(slices.Values(keys), func(key Keys[LeftSecondaryKey, RightSecondaryKey]) iter.Seq[RightSecondaryKey] {
		return key.Right.Iter()
	})))

	var leftPks map[LeftSecondaryKey][]PrimaryKey
	if len(leftSks) != 0 {
		var err error
		leftPks, err = i.Left.GetMulti(ctx, leftSks)
		if err != nil {
			return nil, err
		}
	}

	var rightPks map[RightSecondaryKey][]PrimaryKey
	if len(rightSks) != 0 {
		var err error
		rightPks, err = i.Right.GetMulti(ctx, rightSks)
		if err != nil {
			return nil, err
		}
	}

	result := make(map[Keys[LeftSecondaryKey, RightSecondaryKey]][]PrimaryKey)
	for _, key := range keys {
		var left []PrimaryKey
		if !key.Left.Empty {
			if pks, ok := leftPks[key.Left.Key]; ok {
				left = pks
			}
		}

		var right []PrimaryKey
		if !key.Right.Empty {
			if pks, ok := rightPks[key.Right.Key]; ok {
				right = pks
			}
		}

		total := len(left) + len(right)
		switch total {
		case 0:
			continue
		case len(left):
			if key.Right.Empty {
				result[key] = left
			}
		case len(right):
			if key.Left.Empty {
				result[key] = right
			}
		default:
			result[key] = slices.Collect(iterutil.Intersection(slices.Values(left), slices.Values(right)))
		}
	}
	return result, nil
}
