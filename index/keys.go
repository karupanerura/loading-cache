package index

import (
	"iter"

	loadingcache "github.com/karupanerura/loading-cache"
)

// MaybeKey is a struct that contains a key and a flag that indicates whether the key is present.
type MaybeKey[K loadingcache.KeyConstraint] struct {
	// Key is the key.
	// If Empty is true, Key must be zero value.
	Key K

	// Empty is true if the key is not present.
	Empty bool
}

func (k *MaybeKey[K]) Iter() iter.Seq[K] {
	return iter.Seq[K](func(yield func(K) bool) {
		if !k.Empty && !yield(k.Key) {
			return
		}
	})
}

// Keys is a struct with two secondary keys used as a key for OrIndex and AndIndex.
//
// The secondary keys are stored in the Left and Right fields. (general case)
// If both keys are present, the composite index queries both side indexes.
// For example:
// - AndIndex.GetMulti: [{Left: 1, Right: 2}, {Left: 3, Right: 4}] => (left = 1 AND right = 2) OR (left = 3 AND right = 4)
// - OrIndex.GetMulti: [{Left: 1, Right: 2}, {Left: 3, Right: 4}] => (left = 1 OR right = 2 OR left = 3 OR right = 4)
//
// If one key is missing, the composite index only queries the non-empty side index.
// For example:
// - AndIndex.GetMulti: [{Left: 1, Right: 2}, {Left: 3, Right: None}] => (left = 1 AND right = 2) OR (left = 3)
// - OrIndex.GetMulti: [{Left: 1, Right: 2}, {Left: None, Right: 3}] => (left = 1 OR right = 2 OR right = 3)
//
// If both keys are missing, the composite index returns an empty result.
type Keys[LeftSecondaryKey loadingcache.KeyConstraint, RightSecondaryKey loadingcache.KeyConstraint] struct {
	Left  MaybeKey[LeftSecondaryKey]
	Right MaybeKey[RightSecondaryKey]
}

// NewKeys returns a new Keys instance with the given secondary keys.
func NewKeys[LeftSecondaryKey loadingcache.KeyConstraint, RightSecondaryKey loadingcache.KeyConstraint](left LeftSecondaryKey, right RightSecondaryKey) Keys[LeftSecondaryKey, RightSecondaryKey] {
	return Keys[LeftSecondaryKey, RightSecondaryKey]{
		Left:  MaybeKey[LeftSecondaryKey]{Key: left},
		Right: MaybeKey[RightSecondaryKey]{Key: right},
	}
}

// ZipKeys returns a slice of Keys instances by packing the given secondary keys.
// If the length of the left or right slice is less than the other, the empty key is used.
// The length of the returned slice is equal to the maximum length of the left and right slices.
// The left and right slices must not be nil.
func ZipKeys[LeftSecondaryKey loadingcache.KeyConstraint, RightSecondaryKey loadingcache.KeyConstraint](left []LeftSecondaryKey, right []RightSecondaryKey) []Keys[LeftSecondaryKey, RightSecondaryKey] {
	keys := make([]Keys[LeftSecondaryKey, RightSecondaryKey], max(len(left), len(right)))
	for i := 0; i != len(keys); i++ {
		if i < len(left) {
			keys[i].Left.Key = left[i]
		} else {
			keys[i].Left.Empty = true
		}

		if i < len(right) {
			keys[i].Right.Key = right[i]
		} else {
			keys[i].Right.Empty = true
		}
	}
	return keys
}

// LeftKey returns a new Keys instance with the given left secondary key only.
func LeftKey[LeftSecondaryKey loadingcache.KeyConstraint, RightSecondaryKey loadingcache.KeyConstraint](left LeftSecondaryKey) Keys[LeftSecondaryKey, RightSecondaryKey] {
	return Keys[LeftSecondaryKey, RightSecondaryKey]{
		Left:  MaybeKey[LeftSecondaryKey]{Key: left},
		Right: MaybeKey[RightSecondaryKey]{Empty: true},
	}
}

// RightKey returns a new Keys instance with the given right secondary key only.
func RightKey[LeftSecondaryKey loadingcache.KeyConstraint, RightSecondaryKey loadingcache.KeyConstraint](right RightSecondaryKey) Keys[LeftSecondaryKey, RightSecondaryKey] {
	return Keys[LeftSecondaryKey, RightSecondaryKey]{
		Left:  MaybeKey[LeftSecondaryKey]{Empty: true},
		Right: MaybeKey[RightSecondaryKey]{Key: right},
	}
}

// LeftKeys returns a slice of Keys instances with the given left secondary keys only.
func LeftKeys[LeftSecondaryKey loadingcache.KeyConstraint, RightSecondaryKey loadingcache.KeyConstraint](left []LeftSecondaryKey) []Keys[LeftSecondaryKey, RightSecondaryKey] {
	keys := make([]Keys[LeftSecondaryKey, RightSecondaryKey], len(left))
	for i, l := range left {
		keys[i].Left.Key = l
		keys[i].Right.Empty = true
	}
	return keys
}

// RightKeys returns a slice of Keys instances with the given right secondary keys only.
func RightKeys[LeftSecondaryKey loadingcache.KeyConstraint, RightSecondaryKey loadingcache.KeyConstraint](right []RightSecondaryKey) []Keys[LeftSecondaryKey, RightSecondaryKey] {
	keys := make([]Keys[LeftSecondaryKey, RightSecondaryKey], len(right))
	for i, r := range right {
		keys[i].Left.Empty = true
		keys[i].Right.Key = r
	}
	return keys
}
