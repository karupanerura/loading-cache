// Package loadingcache provides a generic loading cache for Go.
//
// LoadingCache combines a CacheStorage and a SourceLoader. On a miss, the
// loader fetches the entry from a LoadingSource and writes it to the storage.
// IndexedLoadingCache adds lookups by secondary key through an Index.
//
// This package defines LoadingCache and IndexedLoadingCache, the interfaces
// they are built from, the entry types, and supporting types such as the value
// cloners and Clock. Implementations of the interfaces live in the subpackages:
// storage, storage/memstorage, loader/singleflightloader, loader/pureloader,
// source, index, index/omcindex, index/intervalupdater, and expiration.
// storage/storagetest provides tests that CacheStorage implementations can run.
package loadingcache
