package loadingcache

import "errors"

// ErrInvalidSourceResult indicates that a source returned an unexpected number
// of entries or an entry whose key does not match the requested key.
// Source adapters also use it to reject ambiguous results, such as duplicate
// entries for a deduplicated request or entries for unrequested keys.
// Loaders reject such results before writing them to storage.
var ErrInvalidSourceResult = errors.New("loadingcache: invalid source result")
