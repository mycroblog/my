// SPDX-License-Identifier: AGPL-3.0-only

// Package acmetls is throwaway spike code for issue #2823 (serving HTTPS with ACME). It lives
// only on the branch spike/2823-acme-tls and is never merged (ADR-0000).
//
// It holds:
//   - BlobCache: an autocert.Cache over blob.Store whose values are sealed with kernel.Sealer,
//     the storage idea of the issue;
//   - DBStore: the alternative, ACME state in tables modeled after ADR-0020 (a resource for
//     each certificate and account, events for order attempts and failures), with the keys in
//     sealed columns that `my kek rewrap` can list;
//   - Lease: a lease over kernel.KeyValueStore.CompareAndSwap, so that one process orders at a time;
//   - Manager: a small certificate manager over github.com/mholt/acmez/v3 (ARI, profiles,
//     "replaces"), with its challenges shared through the KeyValueStore.
//
// The Pebble tests run when MY_SPIKE_PEBBLE_DIR names a Pebble directory URL; see pebble_test.go.
package acmetls
