// Package source provides adapters for implementing the loadingcache.LoadingSource interface.
//
// This package contains:
//   - FunctionsSource: builds a source from a Get function, a GetMulti function, or both
//   - GetMultiFunctionSource and GetMultiMapFunctionSource: build a source from a single batch function
//   - CompactSource: adapts a source whose GetMulti omits missing keys or returns entries out of order
//   - LintSource: wraps a source and validates that it follows the LoadingSource contract
//
// CompactSource normalizes only GetMulti; Get calls the underlying Get, which
// must meet the LoadingSource contract on its own. For a source that has only a
// batch function omitting missing keys, use GetMultiMapFunctionSource, or pass the
// GetMulti of a CompactSource wrapping the function as the GetMultiFunc of a
// FunctionsSource, as shown in the CompactSource batch function example.
package source
