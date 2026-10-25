// SPDX-License-Identifier: MIT

// Package mykit is the Go client of the client API of my, an ActivityPub microblog server (ADR-0093).
//
// The types and the client of every operation are generated from the server's OpenAPI spec (api.gen.go). New
// wraps that client with what every call needs: the access token from a TokenSource, renewed once after a 401;
// Accept-Language and User-Agent; an Idempotency-Key for the operations that take one; retries of 429 and 503
// that follow Retry-After; and errors: an answer with a status of 400 or more is a *Problem (RFC 9457), a
// request without an answer a *NetworkError. All iterates a paginated collection, UploadMedia uploads a file
// from an io.Reader, and NewCodeVerifier, CodeChallenge, and OAuthConfig.Exchange do the PKCE part of the
// authorization code flow. API.Stream reads the event stream of GET /streaming and connects again whenever it
// ends; the server replays nothing, so read the timelines and the notifications again on every reconnection.
//
// The library is MIT-licensed, apart from the AGPL-3.0 server.
package mykit
