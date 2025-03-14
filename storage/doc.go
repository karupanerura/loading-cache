// Package storage provides cache storage adapters and utilities for the loading-cache library.
//
// This package contains adapters such as SilentErrorStorage, which wraps any CacheStorage
// implementation to silently handle errors, and FunctionsStorage, which allows building
// custom storage implementations using function callbacks.
//
// This package also defines common error types for storage operations:
// ErrGet, ErrSet, ErrGetMulti, and ErrSetMulti.
package storage
