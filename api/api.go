// SPDX-License-Identifier: AGPL-3.0-only

// Package api embeds the OpenAPI document of the client API (openapi.yaml), the bundle of the split
// spec in api/spec/ from which the server interface and the client types are generated (ADR-0016,
// ADR-0016).
package api

import _ "embed"

// Spec is the OpenAPI 3.1 document of the client API, in YAML: the bundle that `scripts/task.sh api:generate` writes.
//
//go:embed openapi.yaml
var Spec []byte
