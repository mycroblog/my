// SPDX-License-Identifier: AGPL-3.0-only
// The queries of the session feature (web-client/README.md S1): every feature keeps its `queryOptions` in its
// api/ directory, built from the openapi-react-query client so that keys and types come from the spec.
import type { Api } from "../../../lib/api/client";
import type { components } from "@mycroblog/mykit";

/** The signed-in user's own account, with what only they see of it: their permissions, say. */
export type CredentialAccount = components["schemas"]["CredentialAccount"];

/** The signed-in user's own account (GET /accounts/verify_credentials). */
export function viewerQueryOptions(api: Api) {
  return api.query.queryOptions("get", "/accounts/verify_credentials");
}
