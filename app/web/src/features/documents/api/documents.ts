// SPDX-License-Identifier: AGPL-3.0-only
// The server data of the documents feature (web-client/README.md S1): the instance, whose `terms_url` and
// `privacy_policy_url` lead to the pages of its terms of service and privacy policy, and the documents themselves
// (GET /instance/documents/{kind}), which the pages at `/about/terms`, `/about/privacy` and `/about/commerce` show.
import type { Api } from "../../../lib/api/client";
import type { components } from "@mycroblog/mykit";

/** The instance, for the links to its documents. */
export function instanceQueryOptions(api: Api) {
  return api.query.queryOptions("get", "/instance");
}

/** The kind of a document, as the API names it. */
export type DocumentKind = components["schemas"]["InstanceDocumentKind"];

/** The published document of `kind`; a 404 when the instance has not published it. */
export function documentQueryOptions(api: Api, kind: DocumentKind) {
  return api.query.queryOptions("get", "/instance/documents/{kind}", { params: { path: { kind } } });
}
