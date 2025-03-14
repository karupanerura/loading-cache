package iterutil

import (
	"iter"
)

// OmitKey returns a new iterator that omits the keys from the input iterator.
func OmitKey[K, V any](seq iter.Seq2[K, V]) iter.Seq[V] {
	return iter.Seq[V](func(yield func(V) bool) {
		for _, v := range seq {
			if !yield(v) {
				return
			}
		}
	})
}

// Concat returns a new iterator that concatenates the input iterators.
func Concat[V any](iters ...iter.Seq[V]) iter.Seq[V] {
	return iter.Seq[V](func(yield func(V) bool) {
		for _, seq := range iters {
			for v := range seq {
				if !yield(v) {
					return
				}
			}
		}
	})
}

// Intersection returns a new iterator that yields the intersection of the input iterators.
// The intersection is the set of values that are present in all input iterators.
func Intersection[V comparable](iters ...iter.Seq[V]) iter.Seq[V] {
	return iter.Seq[V](func(yield func(V) bool) {
		seen := map[V]int{}
		for _, seq := range iters {
			for v := range seq {
				seen[v]++
				if seen[v] == len(iters) && !yield(v) {
					return
				}
			}
		}
	})
}

// Uniq returns a new iterator that yields the unique values from the input iterator.
// The order of the output is the same as the input.
func Uniq[V comparable](seq iter.Seq[V]) iter.Seq[V] {
	seen := map[V]struct{}{}
	return iter.Seq[V](func(yield func(V) bool) {
		for v := range seq {
			if _, ok := seen[v]; !ok {
				seen[v] = struct{}{}
				if !yield(v) {
					return
				}
			}
		}
	})
}

// Map returns a new iterator that applies the function to each value from the input iterator.
// The output iterator yields the results of the function calls.
func Map[V, R any](seq iter.Seq[V], f func(V) R) iter.Seq[R] {
	return iter.Seq[R](func(yield func(R) bool) {
		for v := range seq {
			if !yield(f(v)) {
				return
			}
		}
	})
}
