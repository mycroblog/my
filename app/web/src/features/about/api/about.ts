// SPDX-License-Identifier: AGPL-3.0-only
// The server data of the instance's information page (web-client/README.md S1): the instance (its description and rules)
// and the remote domains it moderates, as it lists them publicly (FR-718). The list follows the API's keyset
// pagination as an infinite query whose key ends with "pages" (web-client/10-api-client-and-auth.md §10).
import { infiniteQueryOptions } from "@tanstack/react-query";
import type { Api } from "../../../lib/api/client";
import type { components } from "@mycroblog/mykit";
import { ApiError } from "../../../lib/errors";

type Schemas = components["schemas"];

/** The instance, as GET /instance returns it. */
export type Instance = Schemas["Instance"];

/** A moderated domain as the instance lists it publicly: the domain (obfuscated when the moderators chose so), the kind and the public comment. */
export type PublicDomainAction = Schemas["InstanceDomainAction"];

/** How many moderated domains one page lists. */
const pageSize = 40;

/** The instance: its name, description, rules and the addresses of its documents. */
export function instanceQueryOptions(api: Api) {
  return api.query.queryOptions("get", "/instance");
}

/**
 * The moderated domains in effect, newest first (GET /instance/domain_actions). The instance setting
 * `domain_actions_visibility` decides who may read them: the API answers 404 when nobody may, and 401 to a
 * visitor without a session when only signed-in users may.
 */
export function publicDomainActionsQueryOptions(api: Api) {
  return infiniteQueryOptions({
    queryKey: ["get", "/instance/domain_actions", "pages"] as const,
    queryFn: async ({ pageParam, signal }) => {
      const query = pageParam === undefined ? { limit: pageSize } : { limit: pageSize, cursor: pageParam };
      const { data, error } = await api.fetch.GET("/instance/domain_actions", { params: { query }, signal });
      // The middleware throws an ApiError for every error status; this narrows the type.
      if (data === undefined) throw new ApiError(error);
      return data;
    },
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (page) => page.next_cursor,
  });
}
