package source_test

import (
	"context"
	"errors"
	"testing"
	"time"

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
		if key == 1 {
			return &loadingcache.CacheEntry[uint8, string]{Entry: loadingcache.Entry[uint8, string]{Key: key, Value: "value1"}, ExpiresAt: time.Now().Add(time.Hour)}, nil
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
