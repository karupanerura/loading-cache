package source_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	loadingcache "github.com/karupanerura/loading-cache"
	"github.com/karupanerura/loading-cache/source"
)

func TestLintSource(t *testing.T) {
	t.Parallel()

	s := &source.LintSource[uint8, string]{
		Source: &source.FunctionsSource[uint8, string]{
			GetFunc: func(_ context.Context, key uint8) (*loadingcache.CacheEntry[uint8, string], error) {
				if key == 1 {
					return &loadingcache.CacheEntry[uint8, string]{Entry: loadingcache.Entry[uint8, string]{Key: key, Value: "value1"}, ExpiresAt: time.Now().Add(1 * time.Hour)}, nil
				}
				return nil, nil
			},
			GetMultiFunc: func(_ context.Context, keys []uint8) ([]*loadingcache.CacheEntry[uint8, string], error) {
				entries := make([]*loadingcache.CacheEntry[uint8, string], len(keys))
				for i, key := range keys {
					if key == 1 {
						entries[i] = &loadingcache.CacheEntry[uint8, string]{Entry: loadingcache.Entry[uint8, string]{Key: key, Value: "value1"}, ExpiresAt: time.Now().Add(1 * time.Hour)}
					} else if key == 2 {
						entries[i] = &loadingcache.CacheEntry[uint8, string]{Entry: loadingcache.Entry[uint8, string]{Key: key, Value: "value2"}, ExpiresAt: time.Now().Add(1 * time.Hour)}
					}
				}
				return entries, nil
			},
		},
	}

	t.Run("Get returns value", func(t *testing.T) {
		t.Parallel()

		entry, err := s.Get(t.Context(), 1)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if entry == nil {
			t.Fatal("expected entry, got nil")
		}

		value := entry.Value
		if value != "value1" {
			t.Errorf("expected value1, got %v", value)
		}
	})

	t.Run("Get not found", func(t *testing.T) {
		t.Parallel()

		entry, err := s.Get(t.Context(), 2)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if entry != nil {
			t.Errorf("expected nil entry, got entry: %+v", entry)
		}
	})

	t.Run("Get panics on mismatch key", func(t *testing.T) {
		t.Parallel()

		s := &source.LintSource[uint8, string]{
			Source: &source.FunctionsSource[uint8, string]{
				GetFunc: func(_ context.Context, key uint8) (*loadingcache.CacheEntry[uint8, string], error) {
					if key == 1 {
						return &loadingcache.CacheEntry[uint8, string]{Entry: loadingcache.Entry[uint8, string]{Key: 2, Value: "value2"}, ExpiresAt: time.Now().Add(time.Hour)}, nil
					}
					return nil, nil
				},
			},
		}

		defer func() {
			if r := recover(); r == nil {
				t.Error("expected panic for mismatch key, but did not panic")
			}
		}()
		s.Get(t.Context(), 1)
	})

	t.Run("Get panics on zero expiration time", func(t *testing.T) {
		t.Parallel()

		s := &source.LintSource[uint8, string]{
			Source: &source.FunctionsSource[uint8, string]{
				GetFunc: func(_ context.Context, key uint8) (*loadingcache.CacheEntry[uint8, string], error) {
					if key == 1 {
						return &loadingcache.CacheEntry[uint8, string]{Entry: loadingcache.Entry[uint8, string]{Key: key, Value: "value1"}}, nil
					}
					return nil, nil
				},
			},
		}

		defer func() {
			if r := recover(); r == nil {
				t.Error("expected panic for zero expiration time, but did not panic")
			}
		}()
		s.Get(t.Context(), 1)
	})

	t.Run("GetMulti returns values", func(t *testing.T) {
		t.Parallel()

		entries, err := s.GetMulti(t.Context(), []uint8{1, 2})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if len(entries) != 2 {
			t.Errorf("expected 2 entries, got %d", len(entries))
		}
		if entries[0].Value != "value1" || entries[1].Value != "value2" {
			t.Errorf("unexpected values: %v, %v", entries[0].Value, entries[1].Value)
		}
	})

	t.Run("GetMulti includes not found", func(t *testing.T) {
		t.Parallel()

		entries, err := s.GetMulti(t.Context(), []uint8{1, 2, 3})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if len(entries) != 3 {
			t.Errorf("expected 3 entries, got %d", len(entries))
		}
		if entries[0].Value != "value1" || entries[1].Value != "value2" || entries[2] != nil {
			t.Errorf("unexpected values: %v, %v, %v", entries[0].Value, entries[1].Value, entries[2])
		}
	})

	t.Run("GetMulti panics on missing keys", func(t *testing.T) {
		t.Parallel()

		s := &source.LintSource[uint8, string]{
			Source: &source.FunctionsSource[uint8, string]{
				GetMultiFunc: func(_ context.Context, keys []uint8) ([]*loadingcache.CacheEntry[uint8, string], error) {
					entries := make([]*loadingcache.CacheEntry[uint8, string], 0, len(keys))
					for _, key := range keys {
						if key == 1 {
							entries = append(entries, &loadingcache.CacheEntry[uint8, string]{Entry: loadingcache.Entry[uint8, string]{Key: key, Value: "value1"}, ExpiresAt: time.Now().Add(time.Hour)})
						} else if key == 2 {
							entries = append(entries, &loadingcache.CacheEntry[uint8, string]{Entry: loadingcache.Entry[uint8, string]{Key: key, Value: "value2"}, ExpiresAt: time.Now().Add(time.Hour)})
						}
					}
					return entries, nil
				},
			},
		}

		defer func() {
			if r := recover(); r == nil {
				t.Errorf("expected panic for missing keys, but did not panic")
			}
		}()
		s.GetMulti(t.Context(), []uint8{0, 1, 3})
	})

	t.Run("GetMulti panics on mismatch key", func(t *testing.T) {
		t.Parallel()

		s := &source.LintSource[uint8, string]{
			Source: &source.FunctionsSource[uint8, string]{
				GetMultiFunc: func(_ context.Context, keys []uint8) ([]*loadingcache.CacheEntry[uint8, string], error) {
					entries := make([]*loadingcache.CacheEntry[uint8, string], len(keys))
					for i, key := range keys {
						if key == 1 {
							entries[i] = &loadingcache.CacheEntry[uint8, string]{Entry: loadingcache.Entry[uint8, string]{Key: 2, Value: "value2"}, ExpiresAt: time.Now().Add(time.Hour)}
						} else if key == 2 {
							entries[i] = &loadingcache.CacheEntry[uint8, string]{Entry: loadingcache.Entry[uint8, string]{Key: 1, Value: "value1"}, ExpiresAt: time.Now().Add(time.Hour)}
						}
					}
					return entries, nil
				},
			},
		}

		defer func() {
			if r := recover(); r == nil {
				t.Errorf("expected panic for mismatch key, but did not panic")
			}
		}()
		s.GetMulti(t.Context(), []uint8{1, 2})
	})

	t.Run("GetMulti panics on zero expiration time", func(t *testing.T) {
		t.Parallel()

		s := &source.LintSource[uint8, string]{
			Source: &source.FunctionsSource[uint8, string]{
				GetMultiFunc: func(_ context.Context, keys []uint8) ([]*loadingcache.CacheEntry[uint8, string], error) {
					entries := make([]*loadingcache.CacheEntry[uint8, string], len(keys))
					for i, key := range keys {
						if key == 1 {
							entries[i] = &loadingcache.CacheEntry[uint8, string]{Entry: loadingcache.Entry[uint8, string]{Key: key, Value: "value1"}}
						} else if key == 2 {
							entries[i] = &loadingcache.CacheEntry[uint8, string]{Entry: loadingcache.Entry[uint8, string]{Key: key, Value: "value2"}}
						}
					}
					return entries, nil
				},
			},
		}

		defer func() {
			if r := recover(); r == nil {
				t.Errorf("expected panic for zero expiration time, but did not panic")
			}
		}()
		s.GetMulti(t.Context(), []uint8{1, 2})
	})

	t.Run("Get returns error from Source", func(t *testing.T) {
		t.Parallel()

		expectedErr := errors.New("source error")
		s := &source.LintSource[uint8, string]{
			Source: &source.FunctionsSource[uint8, string]{
				GetFunc: func(_ context.Context, key uint8) (*loadingcache.CacheEntry[uint8, string], error) {
					return nil, expectedErr
				},
				GetMultiFunc: func(_ context.Context, keys []uint8) ([]*loadingcache.CacheEntry[uint8, string], error) {
					return nil, expectedErr
				},
			},
		}

		_, err := s.Get(t.Context(), 1)
		if err != expectedErr {
			t.Errorf("expected error: %v, got: %v", expectedErr, err)
		}

		_, err = s.GetMulti(t.Context(), []uint8{1, 2})
		if err != expectedErr {
			t.Errorf("expected error: %v, got: %v", expectedErr, err)
		}
	})
}

func TestCompactSource(t *testing.T) {
	t.Parallel()

	getFunc := func(_ context.Context, key uint8) (*loadingcache.CacheEntry[uint8, string], error) {
		switch key {
		case 1:
			return &loadingcache.CacheEntry[uint8, string]{Entry: loadingcache.Entry[uint8, string]{Key: key, Value: "value1"}, ExpiresAt: time.Now().Add(time.Hour)}, nil
		case 2:
			return &loadingcache.CacheEntry[uint8, string]{Entry: loadingcache.Entry[uint8, string]{Key: key, Value: "value2"}, ExpiresAt: time.Now().Add(time.Hour)}, nil
		}
		return nil, nil
	}
	getMultiFunc := func(_ context.Context, keys []uint8) ([]*loadingcache.CacheEntry[uint8, string], error) {
		entries := make([]*loadingcache.CacheEntry[uint8, string], 0, len(keys))
		for _, key := range keys {
			if key == 1 {
				entries = append(entries, &loadingcache.CacheEntry[uint8, string]{Entry: loadingcache.Entry[uint8, string]{Key: key, Value: "value1"}, ExpiresAt: time.Now().Add(time.Hour)})
			} else if key == 2 {
				entries = append(entries, &loadingcache.CacheEntry[uint8, string]{Entry: loadingcache.Entry[uint8, string]{Key: key, Value: "value2"}, ExpiresAt: time.Now().Add(time.Hour)})
			}
		}
		return entries, nil
	}
	mockSource := &source.FunctionsSource[uint8, string]{GetFunc: getFunc, GetMultiFunc: getMultiFunc}
	s := &source.CompactSource[uint8, string]{Source: mockSource}

	t.Run("Get returns value", func(t *testing.T) {
		t.Parallel()

		entry, err := s.Get(t.Context(), 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if entry == nil {
			t.Fatal("expected entry, got nil")
		}
		value := entry.Value
		if value != "value1" {
			t.Errorf("expected value1, got %v", value)
		}
	})

	t.Run("GetMulti returns values", func(t *testing.T) {
		t.Parallel()

		entries, err := s.GetMulti(t.Context(), []uint8{1, 2})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if len(entries) != 2 {
			t.Errorf("expected 2 entries, got %d", len(entries))
		}
		if entries[0].Value != "value1" || entries[1].Value != "value2" {
			t.Errorf("unexpected values: %v, %v", entries[0].Value, entries[1].Value)
		}
	})

	t.Run("GetMulti returns nil for missing keys", func(t *testing.T) {
		t.Parallel()

		entries, err := s.GetMulti(t.Context(), []uint8{1, 3})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if len(entries) != 2 {
			t.Errorf("expected 2 entries, got %d", len(entries))
		}
		if entries[0].Value != "value1" || entries[1] != nil {
			t.Errorf("unexpected values: %v, %v", entries[0].Value, entries[1])
		}
	})

	t.Run("GetMulti returns error from Source", func(t *testing.T) {
		t.Parallel()

		expectedErr := errors.New("source error")
		s := &source.CompactSource[uint8, string]{
			Source: &source.FunctionsSource[uint8, string]{
				GetFunc: func(_ context.Context, key uint8) (*loadingcache.CacheEntry[uint8, string], error) {
					return nil, expectedErr
				},
				GetMultiFunc: func(_ context.Context, keys []uint8) ([]*loadingcache.CacheEntry[uint8, string], error) {
					return nil, expectedErr
				},
			},
		}

		_, err := s.Get(t.Context(), 1)
		if err != expectedErr {
			t.Errorf("expected error: %v, got: %v", expectedErr, err)
		}

		_, err = s.GetMulti(t.Context(), []uint8{1, 2})
		if err != expectedErr {
			t.Errorf("expected error: %v, got: %v", expectedErr, err)
		}
	})
}

func TestGetMultiMapFunctionSource(t *testing.T) {
	t.Parallel()

	getMultiMapFunc := func(_ context.Context, keys []uint8) (map[uint8]*loadingcache.CacheEntry[uint8, string], error) {
		entries := make(map[uint8]*loadingcache.CacheEntry[uint8, string], len(keys))
		for _, key := range keys {
			if key == 1 {
				entries[key] = &loadingcache.CacheEntry[uint8, string]{Entry: loadingcache.Entry[uint8, string]{Key: key, Value: "value1"}, ExpiresAt: time.Now().Add(time.Hour)}
			} else if key == 2 {
				entries[key] = &loadingcache.CacheEntry[uint8, string]{Entry: loadingcache.Entry[uint8, string]{Key: key, Value: "value2"}, ExpiresAt: time.Now().Add(time.Hour)}
			}
		}
		return entries, nil
	}
	s := source.GetMultiMapFunctionSource[uint8, string](getMultiMapFunc)

	t.Run("Get returns value", func(t *testing.T) {
		t.Parallel()

		entry, err := s.Get(t.Context(), 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if entry == nil {
			t.Fatal("expected entry, got nil")
		}

		value := entry.Value
		if value != "value1" {
			t.Errorf("expected value1, got %v", value)
		}
	})

	t.Run("Get returns error for missing key", func(t *testing.T) {
		t.Parallel()

		entry, err := s.Get(t.Context(), 3)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if entry != nil {
			t.Errorf("expected nil entry, got entry: %+v", entry)
		}
	})

	t.Run("GetMulti returns values", func(t *testing.T) {
		t.Parallel()

		entries, err := s.GetMulti(t.Context(), []uint8{1, 2})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if len(entries) != 2 {
			t.Errorf("expected 2 entries, got %d", len(entries))
		}
		if entries[0].Value != "value1" || entries[1].Value != "value2" {
			t.Errorf("unexpected values: %v, %v", entries[0].Value, entries[1].Value)
		}
	})

	t.Run("GetMulti returns nil for missing keys", func(t *testing.T) {
		t.Parallel()

		entries, err := s.GetMulti(t.Context(), []uint8{1, 3})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if len(entries) != 2 {
			t.Errorf("expected 2 entries, got %d", len(entries))
		}
		if entries[0].Value != "value1" || entries[1] != nil {
			t.Errorf("unexpected values: %v, %v", entries[0].Value, entries[1])
		}
	})

	t.Run("GetMulti returns error from Source", func(t *testing.T) {
		t.Parallel()

		expectedErr := errors.New("source error")
		s := source.GetMultiMapFunctionSource[uint8, string](func(_ context.Context, keys []uint8) (map[uint8]*loadingcache.CacheEntry[uint8, string], error) {
			return nil, expectedErr
		})

		_, err := s.Get(t.Context(), 1)
		if err != expectedErr {
			t.Errorf("expected error: %v, got: %v", expectedErr, err)
		}

		_, err = s.GetMulti(t.Context(), []uint8{1, 2})
		if err != expectedErr {
			t.Errorf("expected error: %v, got: %v", expectedErr, err)
		}
	})
}

func TestGetMultiFunctionSource(t *testing.T) {
	t.Parallel()

	getMultiFunc := func(_ context.Context, keys []uint8) ([]*loadingcache.CacheEntry[uint8, string], error) {
		entries := make([]*loadingcache.CacheEntry[uint8, string], len(keys))
		for i, key := range keys {
			if key == 1 {
				entries[i] = &loadingcache.CacheEntry[uint8, string]{Entry: loadingcache.Entry[uint8, string]{Key: key, Value: "value1"}, ExpiresAt: time.Now()}
			} else if key == 2 {
				entries[i] = &loadingcache.CacheEntry[uint8, string]{Entry: loadingcache.Entry[uint8, string]{Key: key, Value: "value2"}, ExpiresAt: time.Now()}
			}
		}
		return entries, nil
	}
	s := source.GetMultiFunctionSource[uint8, string](getMultiFunc)

	t.Run("Get returns value", func(t *testing.T) {
		t.Parallel()

		entry, err := s.Get(t.Context(), 1)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if entry == nil {
			t.Fatal("expected value, got nil")
		}

		value := entry.Value
		if value != "value1" {
			t.Errorf("expected value1, got %v", value)
		}
	})

	t.Run("Get returns error for missing key", func(t *testing.T) {
		t.Parallel()

		entry, err := s.Get(t.Context(), 3)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if entry != nil {
			t.Errorf("expected nil entry, got entry: %+v", entry)
		}
	})

	t.Run("GetMulti returns values", func(t *testing.T) {
		t.Parallel()

		entries, err := s.GetMulti(t.Context(), []uint8{1, 2})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if len(entries) != 2 {
			t.Errorf("expected 2 entries, got %d", len(entries))
		}
		if entries[0].Value != "value1" || entries[1].Value != "value2" {
			t.Errorf("unexpected values: %v, %v", entries[0].Value, entries[1].Value)
		}
	})

	t.Run("GetMulti returns error from Source", func(t *testing.T) {
		t.Parallel()

		expectedErr := errors.New("source error")
		s := source.GetMultiFunctionSource[uint8, string](func(_ context.Context, keys []uint8) ([]*loadingcache.CacheEntry[uint8, string], error) {
			return nil, expectedErr
		})

		_, err := s.Get(t.Context(), 1)
		if err != expectedErr {
			t.Errorf("expected error: %v, got: %v", expectedErr, err)
		}

		_, err = s.GetMulti(t.Context(), []uint8{1, 2})
		if err != expectedErr {
			t.Errorf("expected error: %v, got: %v", expectedErr, err)
		}
	})
}

func TestGetMultiFunctionSourceGetRejectsInvalidResults(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		count int
	}{{"Missing", 0}, {"Extra", 2}, {"WrongKey", 1}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := source.GetMultiFunctionSource[uint8, string](func(context.Context, []uint8) ([]*loadingcache.CacheEntry[uint8, string], error) {
				entries := make([]*loadingcache.CacheEntry[uint8, string], tc.count)
				if tc.count == 1 {
					entries[0] = &loadingcache.CacheEntry[uint8, string]{Entry: loadingcache.Entry[uint8, string]{Key: 2, Value: "wrong"}, ExpiresAt: time.Now().Add(time.Hour)}
				}
				return entries, nil
			})
			entry, err := s.Get(t.Context(), 1)
			if !errors.Is(err, loadingcache.ErrInvalidSourceResult) || entry != nil {
				t.Fatalf("Get: got %v, %v; want nil entry and ErrInvalidSourceResult", entry, err)
			}
		})
	}
}

// compactTestSource has a Get that follows the LoadingSource contract and a
// GetMulti that omits missing keys and reverses the order, and counts calls to each.
type compactTestSource struct {
	getCalls, getMultiCalls int
}

func (s *compactTestSource) lookup(key int) *loadingcache.CacheEntry[int, int] {
	if key == 9 {
		return nil
	}
	return &loadingcache.CacheEntry[int, int]{Entry: loadingcache.Entry[int, int]{Key: key, Value: key * 10}, ExpiresAt: time.Now().Add(time.Hour)}
}

func (s *compactTestSource) Get(_ context.Context, key int) (*loadingcache.CacheEntry[int, int], error) {
	s.getCalls++
	return s.lookup(key), nil
}

func (s *compactTestSource) GetMulti(_ context.Context, keys []int) ([]*loadingcache.CacheEntry[int, int], error) {
	s.getMultiCalls++
	var entries []*loadingcache.CacheEntry[int, int]
	for _, key := range slices.Backward(keys) {
		if entry := s.lookup(key); entry != nil {
			entries = append(entries, entry)
		}
	}
	return entries, nil
}

func TestCompactSourceGetDelegatesToSource(t *testing.T) {
	t.Parallel()
	src := &compactTestSource{}
	s := &source.CompactSource[int, int]{Source: src}

	for _, key := range []int{1, 9} {
		entry, err := s.Get(t.Context(), key)
		if err != nil {
			t.Fatalf("Get(%d): %v", key, err)
		}
		if key == 9 && entry != nil {
			t.Errorf("Get(%d): got %+v, want nil for a missing key", key, entry)
		}
		if key == 1 && (entry == nil || entry.Key != 1 || entry.Value != 10) {
			t.Errorf("Get(%d): got %+v, want the entry for the key", key, entry)
		}
	}
	if src.getCalls != 2 || src.getMultiCalls != 0 {
		t.Errorf("source calls after Get: Get=%d, GetMulti=%d; want Get=2, GetMulti=0", src.getCalls, src.getMultiCalls)
	}

	entries, err := s.GetMulti(t.Context(), []int{9, 1, 2})
	if err != nil || len(entries) != 3 {
		t.Fatalf("GetMulti: %v, %v", entries, err)
	}
	if entries[0] != nil || entries[1] == nil || entries[1].Key != 1 || entries[2] == nil || entries[2].Key != 2 {
		t.Errorf("GetMulti: got %+v, want [nil, entry 1, entry 2]", entries)
	}
	if src.getCalls != 2 || src.getMultiCalls != 1 {
		t.Errorf("source calls after GetMulti: Get=%d, GetMulti=%d; want Get=2, GetMulti=1", src.getCalls, src.getMultiCalls)
	}
}

func TestCompactSourceNormalizesResults(t *testing.T) {
	t.Parallel()
	one := &loadingcache.CacheEntry[int, int]{Entry: loadingcache.Entry[int, int]{Key: 1, Value: 42}, ExpiresAt: time.Now().Add(time.Hour)}
	two := &loadingcache.CacheEntry[int, int]{Entry: loadingcache.Entry[int, int]{Key: 2, Value: 43}, ExpiresAt: time.Now().Add(time.Hour)}
	for _, tc := range []struct {
		name    string
		entries []*loadingcache.CacheEntry[int, int]
	}{
		{"ShortWithNil", []*loadingcache.CacheEntry[int, int]{nil, two, one}},
		{"LongWithNil", []*loadingcache.CacheEntry[int, int]{nil, one, nil, two, nil}},
		{"SameLengthReordered", []*loadingcache.CacheEntry[int, int]{two, nil, one, nil}},
		{"PaddedDuplicateRequest", []*loadingcache.CacheEntry[int, int]{one, two, nil, nil}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := &source.CompactSource[int, int]{Source: source.GetMultiFunctionSource[int, int](func(context.Context, []int) ([]*loadingcache.CacheEntry[int, int], error) {
				return tc.entries, nil
			})}
			entries, err := s.GetMulti(t.Context(), []int{1, 2, 1, 3})
			if err != nil || len(entries) != 4 {
				t.Fatalf("GetMulti: %v, %v", entries, err)
			}
			for i, want := range []*loadingcache.CacheEntry[int, int]{one, two, one, nil} {
				if entries[i] != want {
					t.Errorf("entry %d: got %+v, want %+v", i, entries[i], want)
				}
			}
		})
	}
}

func TestCompactSourceDeduplicatesKeys(t *testing.T) {
	t.Parallel()
	for _, key := range []int{0, 1} {
		for _, negative := range []bool{false, true} {
			t.Run(fmt.Sprintf("Key%d/Negative%t", key, negative), func(t *testing.T) {
				t.Parallel()
				one := &loadingcache.CacheEntry[int, int]{Entry: loadingcache.Entry[int, int]{Key: key, Value: 42}, NegativeCache: negative, ExpiresAt: time.Now().Add(time.Hour)}
				keys := []int{key, 2, key, 3}
				s := &source.CompactSource[int, int]{Source: source.GetMultiFunctionSource[int, int](func(_ context.Context, requested []int) ([]*loadingcache.CacheEntry[int, int], error) {
					if diff := cmp.Diff([]int{key, 2, 3}, requested); diff != "" {
						t.Errorf("source request (-want +got):\n%s", diff)
					}
					return []*loadingcache.CacheEntry[int, int]{nil, one, nil}, nil
				})}
				entries, err := s.GetMulti(t.Context(), keys)
				if err != nil || len(entries) != 4 {
					t.Fatalf("GetMulti: %v, %v", entries, err)
				}
				if entries[0] != one || entries[2] != one || entries[1] != nil || entries[3] != nil {
					t.Fatalf("unexpected expansion: %v", entries)
				}
				if diff := cmp.Diff([]int{key, 2, key, 3}, keys); diff != "" {
					t.Errorf("input keys were modified (-want +got):\n%s", diff)
				}
			})
		}
	}
}

func TestCompactSourceRejectsAmbiguousResults(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name         string
		keys         []int
		returnedKeys []int
	}{
		{"UnrequestedZeroKey", []int{1, 2}, []int{0, 0}},
		{"UnrequestedKey", []int{1, 2}, []int{1, 3}},
		{"ConflictingZeroKeys", []int{0, 1}, []int{0, 0}},
		{"DuplicateSourceKey", []int{1}, []int{1, 1}},
		{"DuplicateSourceKeyForRepeatedRequest", []int{1, 1}, []int{1, 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := &source.CompactSource[int, int]{Source: source.GetMultiFunctionSource[int, int](func(context.Context, []int) ([]*loadingcache.CacheEntry[int, int], error) {
				entries := make([]*loadingcache.CacheEntry[int, int], len(tc.returnedKeys))
				for i, key := range tc.returnedKeys {
					entries[i] = &loadingcache.CacheEntry[int, int]{Entry: loadingcache.Entry[int, int]{Key: key, Value: i}, ExpiresAt: time.Now().Add(time.Hour)}
				}
				return entries, nil
			})}
			entries, err := s.GetMulti(t.Context(), tc.keys)
			if !errors.Is(err, loadingcache.ErrInvalidSourceResult) || entries != nil {
				t.Fatalf("GetMulti: %v, %v; want nil results and ErrInvalidSourceResult", entries, err)
			}
		})
	}
}

func TestFunctionsSourceFallbacks(t *testing.T) {
	t.Parallel()
	for _, batchOnly := range []bool{false, true} {
		name := "GetOnly"
		if batchOnly {
			name = "GetMultiOnly"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			get := func(_ context.Context, key int) (*loadingcache.CacheEntry[int, int], error) {
				return &loadingcache.CacheEntry[int, int]{Entry: loadingcache.Entry[int, int]{Key: key}, NegativeCache: true, ExpiresAt: time.Now().Add(time.Hour)}, nil
			}
			s := &source.FunctionsSource[int, int]{GetFunc: get}
			if batchOnly {
				s.GetFunc = nil
				s.GetMultiFunc = func(ctx context.Context, keys []int) ([]*loadingcache.CacheEntry[int, int], error) {
					entries := make([]*loadingcache.CacheEntry[int, int], len(keys))
					for i, key := range keys {
						entries[i], _ = get(ctx, key)
					}
					return entries, nil
				}
			}
			for _, wrapped := range []loadingcache.LoadingSource[int, int]{s, &source.CompactSource[int, int]{Source: s}} {
				entry, err := wrapped.Get(t.Context(), 1)
				if err != nil || entry == nil || entry.Key != 1 || !entry.NegativeCache {
					t.Fatalf("Get: %v, %v; want negative cache entry", entry, err)
				}
				entries, err := wrapped.GetMulti(t.Context(), []int{1, 2, 1})
				if err != nil || len(entries) != 3 {
					t.Fatalf("GetMulti: %v, %v", entries, err)
				}
				for i, key := range []int{1, 2, 1} {
					if entries[i] == nil || entries[i].Key != key || !entries[i].NegativeCache {
						t.Fatalf("entry %d: %+v", i, entries[i])
					}
				}
			}
		})
	}
}

func TestFunctionsSourceFallbackStopsOnError(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("source failed")
	var calls []int
	s := &source.FunctionsSource[int, int]{GetFunc: func(_ context.Context, key int) (*loadingcache.CacheEntry[int, int], error) {
		calls = append(calls, key)
		if key == 2 {
			return nil, wantErr
		}
		return &loadingcache.CacheEntry[int, int]{Entry: loadingcache.Entry[int, int]{Key: key}, ExpiresAt: time.Now().Add(time.Hour)}, nil
	}}
	entries, err := s.GetMulti(t.Context(), []int{1, 2, 3})
	if !errors.Is(err, wantErr) || entries != nil || len(calls) != 2 || calls[0] != 1 || calls[1] != 2 {
		t.Fatalf("GetMulti: entries=%v, err=%v, calls=%v; want no partial results and no calls after key 2", entries, err, calls)
	}
	s = &source.FunctionsSource[int, int]{GetMultiFunc: func(context.Context, []int) ([]*loadingcache.CacheEntry[int, int], error) {
		return nil, wantErr
	}}
	if entry, err := s.Get(t.Context(), 1); !errors.Is(err, wantErr) || entry != nil {
		t.Fatalf("Get: %v, %v; want nil entry and source error", entry, err)
	}
}
