// Package expiration provides policies that decide when a cache entry is expired.
//
// This package defines the ExpirationPolicy interface and three implementations:
// GeneralExpirationPolicy expires an entry at its expiration time,
// NeverExpirationPolicy never expires an entry, and EarlyExpirationPolicy
// expires some entries early at random to spread refreshes. memstorage applies
// the policy set with memstorage.WithExpirationPolicy; the default is
// GeneralExpirationPolicy.
package expiration
