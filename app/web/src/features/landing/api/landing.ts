// SPDX-License-Identifier: AGPL-3.0-only
// The server data of the landing page (web-client/README.md S1): the instance (its description, rules and how people sign
// up), its counts and the people who run it, and a few of the newest posts of the local timeline as a sample of the server.
import type { Api } from "../../../lib/api/client";
import type { components } from "@mycroblog/mykit";

/** The instance, as GET /instance returns it. */
export type Instance = components["schemas"]["Instance"];

/** A post of the sample of the local timeline. */
export type RecentPost = components["schemas"]["Status"];

/** The counts of the instance, as GET /instance/overview returns them. */
export type InstanceOverview = components["schemas"]["InstanceOverview"];

/** How many recent posts the landing page shows. */
const recentPostsLimit = 3;

/** The instance: its name, description, rules, registration mode and the public timelines it serves. */
export function instanceQueryOptions(api: Api) {
  return api.query.queryOptions("get", "/instance");
}

/** How many members, how many public posts lately, and since when; only while the server shows them. */
export function overviewQueryOptions(api: Api) {
  return api.query.queryOptions("get", "/instance/overview");
}

/**
 * The newest posts of the local timeline, one page of `recentPostsLimit`. It is a plain query, apart from the local
 * timeline's infinite query (whose key ends with "pages").
 */
export function recentPostsQueryOptions(api: Api) {
  return api.query.queryOptions("get", "/timelines/local", { params: { query: { limit: recentPostsLimit } } });
}
