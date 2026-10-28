// SPDX-License-Identifier: AGPL-3.0-only

// Package s3 implements the BlobStore port (internal/app/blob) on S3-compatible object storage:
// Amazon S3, Cloudflare R2, the XML API of Google Cloud Storage, MinIO, and the like. It passes
// testkit.BlobStoreContract.
//
// The AWS SDK reaches only the endpoint that the operator configured, so it uses the SDK's own
// HTTP client rather than the SSRF-guarded kernel.HTTPClient, whose guard is for the
// destinations that others choose.
package s3
