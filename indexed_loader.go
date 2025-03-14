package loadingcache

import "context"

// IndexedLoadingCache is a LoadingCache with an index.
type IndexedLoadingCache[PrimaryKey KeyConstraint, SecondaryKey KeyConstraint, Value ValueConstraint] struct {
	LoadingCache[PrimaryKey, Value]
	index  Index[SecondaryKey, PrimaryKey]
	cloner ValueCloner[Value]
}

// NewIndexedLoadingCache creates a new IndexedLoadingCache.
func NewIndexedLoadingCache[PrimaryKey KeyConstraint, SecondaryKey KeyConstraint, Value ValueConstraint](cache LoadingCache[PrimaryKey, Value], index Index[SecondaryKey, PrimaryKey], opts ...IndexedLoadingCacheOption[PrimaryKey, SecondaryKey, Value]) *IndexedLoadingCache[PrimaryKey, SecondaryKey, Value] {
	c := &IndexedLoadingCache[PrimaryKey, SecondaryKey, Value]{
		LoadingCache: cache,
		index:        index,
	}
	for _, opt := range opts {
		opt.apply(c)
	}
	if c.cloner == nil {
		c.cloner = DefaultValueCloner[Value]()
	}
	return c
}

type IndexedLoadingCacheOption[PrimaryKey KeyConstraint, SecondaryKey KeyConstraint, Value ValueConstraint] interface {
	apply(*IndexedLoadingCache[PrimaryKey, SecondaryKey, Value])
}

type indexedLoadingCacheOptionFunc[PrimaryKey KeyConstraint, SecondaryKey KeyConstraint, Value ValueConstraint] func(*IndexedLoadingCache[PrimaryKey, SecondaryKey, Value])

func (f indexedLoadingCacheOptionFunc[PrimaryKey, SecondaryKey, Value]) apply(c *IndexedLoadingCache[PrimaryKey, SecondaryKey, Value]) {
	f(c)
}

// WithValueCloner sets the value cloner to the cache.
func WithValueCloner[PrimaryKey KeyConstraint, SecondaryKey KeyConstraint, Value ValueConstraint](cloner ValueCloner[Value]) IndexedLoadingCacheOption[PrimaryKey, SecondaryKey, Value] {
	return indexedLoadingCacheOptionFunc[PrimaryKey, SecondaryKey, Value](func(c *IndexedLoadingCache[PrimaryKey, SecondaryKey, Value]) {
		c.cloner = cloner
	})
}

// FindBySecondaryKey retrieves entries by secondary key.
func (c *IndexedLoadingCache[PrimaryKey, SecondaryKey, Value]) FindBySecondaryKey(ctx context.Context, sk SecondaryKey) ([]*Entry[PrimaryKey, Value], error) {
	pks, err := c.index.Get(ctx, sk)
	if err != nil {
		return nil, err
	}
	if len(pks) == 0 {
		return nil, nil
	}
	return c.GetOrLoadMulti(ctx, pks)
}

// FindBySecondaryKeys retrieves entries by secondary keys.
func (c *IndexedLoadingCache[PrimaryKey, SecondaryKey, Value]) FindBySecondaryKeys(ctx context.Context, sks []SecondaryKey) (map[SecondaryKey][]*Entry[PrimaryKey, Value], error) {
	m, err := c.index.GetMulti(ctx, sks)
	if err != nil {
		return nil, err
	}
	if len(m) == 0 {
		return map[SecondaryKey][]*Entry[PrimaryKey, Value]{}, nil
	}

	var keys []PrimaryKey
	rm := map[PrimaryKey][]SecondaryKey{}
	for sk, pks := range m {
		for _, pk := range pks {
			if _, ok := rm[pk]; !ok {
				keys = append(keys, pk)
			}
			rm[pk] = append(rm[pk], sk)
		}
	}

	entries, err := c.GetOrLoadMulti(ctx, keys)
	if err != nil {
		return nil, err
	}

	result := make(map[SecondaryKey][]*Entry[PrimaryKey, Value], len(rm))
	for _, entry := range entries {
		if entry == nil {
			continue
		}

		for i, sk := range rm[entry.Key] {
			if i == 0 {
				// note: we clone the value only if it is not the first receiver
				// to avoid unnecessary cloning when there are multiple receivers.
				result[sk] = append(result[sk], entry)
			} else {
				e := *entry
				e.Value = c.cloner.CloneValue(e.Value)
				result[sk] = append(result[sk], &e)
			}
		}
	}
	return result, nil
}
