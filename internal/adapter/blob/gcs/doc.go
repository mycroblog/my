// SPDX-License-Identifier: AGPL-3.0-only

// Package gcs implements the BlobStore port (internal/app/blob) on the JSON API of Google Cloud
// Storage, with Application Default Credentials: the service account of Cloud Run or of the
// instance, or the file of GOOGLE_APPLICATION_CREDENTIALS, so that no static key is kept. It
// passes testkit.BlobStoreContract.
//
// SignedURL signs V4 URLs with the private key of the credentials when they hold one, and
// otherwise with the signBlob method of the IAM Service Account Credentials API as the service
// account itself, which needs roles/iam.serviceAccountTokenCreator on that service account.
//
// The client reaches only the Google APIs, so it uses its own HTTP client rather than the
// SSRF-guarded kernel.HTTPClient, whose guard is for the destinations that others choose.
package gcs
