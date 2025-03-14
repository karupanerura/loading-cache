package storage_test

import (
	"context"
	"errors"
	"testing"
	"time"

	loadingcache "github.com/karupanerura/loading-cache"
	"github.com/karupanerura/loading-cache/storage"
)

func TestSilentErrorStorage_Get(t *testing.T) {
	t.Parallel()

	expectedError := errors.New("get error")
	mockStorage := &storage.FunctionsStorage[uint8, struct{}]{
		GetFunc: func(_ context.Context, key uint8) (*loadingcache.CacheEntry[uint8, struct{}], error) {
			return nil, expectedError
		},
	}

	var capturedError error
	silentStorage := &storage.SilentErrorStorage[uint8, struct{}]{
		Storage: mockStorage,
		OnError: func(err error) {
			capturedError = err
		},
	}

	key := uint8(1)
	entry, err := silentStorage.Get(t.Context(), key)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if entry != nil {
		t.Fatalf("expected nil entry, got %v", entry)
	}
	if capturedError == nil || !errors.Is(capturedError, expectedError) {
		t.Fatalf("expected captured error 'get error', got %v", capturedError)
	}
}

func TestSilentErrorStorage_Get_WithoutError(t *testing.T) {
	t.Parallel()

	expiresAt := time.Now().Add(time.Hour)
	expectedEntry := &loadingcache.CacheEntry[uint8, struct{}]{
		Entry: loadingcache.Entry[uint8, struct{}]{
			Key:   1,
			Value: struct{}{},
		},
		ExpiresAt: expiresAt,
	}

	mockStorage := &storage.FunctionsStorage[uint8, struct{}]{
		GetFunc: func(_ context.Context, key uint8) (*loadingcache.CacheEntry[uint8, struct{}], error) {
			return expectedEntry, nil
		},
	}

	var capturedError error
	silentStorage := &storage.SilentErrorStorage[uint8, struct{}]{
		Storage: mockStorage,
		OnError: func(err error) {
			capturedError = err
		},
	}

	key := uint8(1)
	entry, err := silentStorage.Get(t.Context(), key)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if entry == nil {
		t.Fatalf("expected entry, got nil")
	}
	if capturedError != nil {
		t.Fatalf("expected no captured error, got %v", capturedError)
	}
}

func TestSilentErrorStorage_GetMulti(t *testing.T) {
	t.Parallel()

	expectedError := errors.New("get multi error")
	mockStorage := &storage.FunctionsStorage[uint8, struct{}]{
		GetMultiFunc: func(_ context.Context, keys []uint8) ([]*loadingcache.CacheEntry[uint8, struct{}], error) {
			return nil, expectedError
		},
	}

	var capturedError error
	silentStorage := &storage.SilentErrorStorage[uint8, struct{}]{
		Storage: mockStorage,
		OnError: func(err error) {
			capturedError = err
		},
	}

	keys := []uint8{1, 2, 3}
	result, err := silentStorage.GetMulti(t.Context(), keys)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(result) != len(keys) {
		t.Fatalf("expected %d entries, got %d", len(keys), len(result))
	}
	for i, _ := range keys {
		if result[i] != nil {
			t.Fatalf("expected nil entry at index %d, got %v", i, result[i])
		}
	}
	if capturedError == nil || !errors.Is(capturedError, expectedError) {
		t.Fatalf("expected captured error 'get multi error', got %v", capturedError)
	}
}

func TestSilentErrorStorage_GetMulti_WithoutError(t *testing.T) {
	t.Parallel()

	expiresAt := time.Now().Add(time.Hour)
	expectedEntries := []*loadingcache.CacheEntry[uint8, struct{}]{
		{
			Entry: loadingcache.Entry[uint8, struct{}]{
				Key:   1,
				Value: struct{}{},
			},
			ExpiresAt: expiresAt,
		},
		{
			Entry: loadingcache.Entry[uint8, struct{}]{
				Key:   2,
				Value: struct{}{},
			},
			ExpiresAt: expiresAt,
		},
		{
			Entry: loadingcache.Entry[uint8, struct{}]{
				Key:   3,
				Value: struct{}{},
			},
			ExpiresAt: expiresAt,
		},
	}

	mockStorage := &storage.FunctionsStorage[uint8, struct{}]{
		GetMultiFunc: func(_ context.Context, keys []uint8) ([]*loadingcache.CacheEntry[uint8, struct{}], error) {
			return expectedEntries, nil
		},
	}

	var capturedError error
	silentStorage := &storage.SilentErrorStorage[uint8, struct{}]{
		Storage: mockStorage,
		OnError: func(err error) {
			capturedError = err
		},
	}

	keys := []uint8{1, 2, 3}
	result, err := silentStorage.GetMulti(t.Context(), keys)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(result) != len(keys) {
		t.Fatalf("expected %d entries, got %d", len(keys), len(result))
	}
	for i, key := range keys {
		if result[i] == nil || result[i].Key != key {
			t.Fatalf("expected entry with key %v at index %d, got %v", key, i, result[i])
		}
	}
	if capturedError != nil {
		t.Fatalf("expected no captured error, got %v", capturedError)
	}
}

func TestSilentErrorStorage_Set(t *testing.T) {
	t.Parallel()

	expectedError := errors.New("set error")
	mockStorage := &storage.FunctionsStorage[uint8, struct{}]{
		SetFunc: func(_ context.Context, entry *loadingcache.CacheEntry[uint8, struct{}]) error {
			if entry.Key == 1 {
				return expectedError
			}
			return nil
		},
	}

	var capturedError error
	silentStorage := &storage.SilentErrorStorage[uint8, struct{}]{
		Storage: mockStorage,
		OnError: func(err error) {
			capturedError = err
		},
	}

	expiresAt := time.Now().Add(time.Hour)
	entry := &loadingcache.CacheEntry[uint8, struct{}]{
		Entry: loadingcache.Entry[uint8, struct{}]{
			Key:   1,
			Value: struct{}{},
		},
		ExpiresAt: expiresAt,
	}
	err := silentStorage.Set(t.Context(), entry)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if capturedError == nil || !errors.Is(capturedError, expectedError) {
		t.Fatalf("expected captured error 'set error', got %v", capturedError)
	}
}

func TestSilentErrorStorage_SetMulti(t *testing.T) {
	t.Parallel()

	expectedError := errors.New("set multi error")
	mockStorage := &storage.FunctionsStorage[uint8, struct{}]{
		SetMultiFunc: func(_ context.Context, entries []*loadingcache.CacheEntry[uint8, struct{}]) error {
			for _, entry := range entries {
				if entry.Key == 1 {
					return expectedError
				}
			}
			return nil
		},
	}

	var capturedError error
	silentStorage := &storage.SilentErrorStorage[uint8, struct{}]{
		Storage: mockStorage,
		OnError: func(err error) {
			capturedError = err
		},
	}

	entries := []*loadingcache.CacheEntry[uint8, struct{}]{
		{Entry: loadingcache.Entry[uint8, struct{}]{Key: 1, Value: struct{}{}}, ExpiresAt: time.Now().Add(time.Hour)},
		{Entry: loadingcache.Entry[uint8, struct{}]{Key: 2, Value: struct{}{}}, ExpiresAt: time.Now().Add(time.Hour)},
		{Entry: loadingcache.Entry[uint8, struct{}]{Key: 3, Value: struct{}{}}, ExpiresAt: time.Now().Add(time.Hour)},
	}
	err := silentStorage.SetMulti(t.Context(), entries)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if capturedError == nil || !errors.Is(capturedError, expectedError) {
		t.Fatalf("expected captured error 'set multi error', got %v", capturedError)
	}
}
