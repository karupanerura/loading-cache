// Package storage provides adapters and error values for loadingcache.CacheStorage implementations.
//
// FunctionsStorage builds a storage from callbacks. SilentErrorStorage wraps a
// storage and passes its errors to a callback instead of returning them, so a
// failed read becomes a cache miss and a failed write is ignored.
//
// The package also defines the error values ErrGet, ErrSet, ErrGetMulti, and ErrSetMulti.
package storage
