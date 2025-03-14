package memstorage_test

import (
	"testing"

	"github.com/karupanerura/loading-cache/storage/memstorage"
)

func TestWithBucketsSize(t *testing.T) {
	t.Parallel()

	t.Run("panic on negative buckets", func(t *testing.T) {
		t.Parallel()

		defer func() {
			if r := recover(); r == nil {
				t.Errorf("expected panic for negative buckets, but did not panic")
			}
		}()
		memstorage.WithBucketsSize[uint8, uint8](-1)
	})

	t.Run("panic on zero buckets", func(t *testing.T) {
		t.Parallel()

		defer func() {
			if r := recover(); r == nil {
				t.Errorf("expected panic for zero buckets, but did not panic")
			}
		}()
		memstorage.WithBucketsSize[uint8, uint8](0)
	})
}
