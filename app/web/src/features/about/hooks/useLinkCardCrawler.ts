// SPDX-License-Identifier: AGPL-3.0-only
import { useQuery } from "@tanstack/react-query";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { useLoadFailure } from "../../../lib/loadFailure";
import { shellQueryReader } from "../../../lib/query";
import { type Instance, instanceQueryOptions } from "../api/about";

/** What the page about the crawler knows of this server. It says nothing of the settings before it has read them. */
export type LinkCardCrawlerState =
  | { status: "loading" }
  | { status: "error"; reason: string; retry: () => void; isRetrying: boolean }
  | {
      status: "ready";
      /** The User-Agent of the crawler's requests, as the server builds it. */
      userAgent: string;
      /** The setting link_cards: while false, the crawler fetches nothing. */
      enabled: boolean;
      /** The setting link_cards_ignore_robots_txt: while true, the crawler does not read robots.txt. */
      ignoresRobotsTxt: boolean;
      contact: Instance["contact"];
    };

/**
 * What the crawler does on this server, from GET /instance (which the shell reads on every page and the route's loader
 * has asked for).
 */
export function useLinkCardCrawler(): LinkCardCrawlerState {
  const api = useApi();
  const query = useQuery({ ...instanceQueryOptions(api), ...shellQueryReader });
  const failure = useLoadFailure(query);
  const instance = query.data;
  if (instance !== undefined) {
    return {
      status: "ready",
      userAgent: `my-linkcard/${instance.version} (+https://${instance.domain}/)`,
      enabled: instance.link_cards,
      ignoresRobotsTxt: instance.link_cards_ignore_robots_txt,
      contact: instance.contact,
    };
  }
  if (failure.error !== null) {
    return {
      status: "error",
      reason: errorMessage(failure.error),
      retry: () => void query.refetch(),
      isRetrying: failure.isRetrying,
    };
  }
  return { status: "loading" };
}
