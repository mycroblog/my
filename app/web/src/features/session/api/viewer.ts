// SPDX-License-Identifier: AGPL-3.0-only
// The queries of the session feature (web-client/README.md S1): every feature keeps its `queryOptions` in its
// api/ directory, built from the openapi-react-query client so that keys and types come from the spec.
import type { Api } from "../../../lib/api/client";
import type { components } from "@mycroblog/mykit";

/** An account, as the client API returns it. */
export type Account = components["schemas"]["Account"];

/** The signed-in user's own account (GET /accounts/verify_credentials). */
export function viewerQueryOptions(api: Api) {
  return api.query.queryOptions("get", "/accounts/verify_credentials");
}
