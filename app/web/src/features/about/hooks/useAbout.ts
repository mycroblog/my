// SPDX-License-Identifier: AGPL-3.0-only
import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { useApi } from "../../../lib/auth/SessionProvider";
import { ApiError, errorMessage } from "../../../lib/errors";
import { shellQueryReader, shellQueryRetry } from "../../../lib/query";
import {
  type Instance,
  instanceQueryOptions,
  type PublicDomainAction,
  publicDomainActionsQueryOptions,
} from "../api/about";

export type About = {
  /** The one-paragraph summary, as plain text; "" when there is none or the instance has not loaded. */
  summary: string;
  /** The paragraphs of the full description (plain text, split at blank lines); empty when there is none. */
  description: readonly string[];
  rules: Instance["rules"];
  /** The image that represents the instance; undefined when there is none. */
  thumbnail: Instance["thumbnail"];
  /** How people reach the operators; undefined when the instance has no contact. */
  contact: Instance["contact"];
  /**
   * What the instance states to search engines and AI crawlers (FR-725, ADR-0057); undefined while the instance
   * has not loaded.
   */
  crawlers: { searchIndexing: Instance["search_indexing"]; aiUsage: Instance["ai_usage"] } | undefined;
  /**
   * How much of its running costs the instance publishes (FR-903, ADR-0067 §8); undefined while the instance has not
   * loaded.
   */
  finance: Instance["finance_visibility"] | undefined;
  /**
   * How much of its income the instance publishes (FR-917, ADR-0073 §7); undefined while the instance has not loaded.
   */
  income: Instance["income_visibility"] | undefined;
  /**
   * Where users get the source code the server runs (AGPL-3.0 section 13, ADR-0011): an https URL or a mailto: URI;
   * undefined while the instance has not loaded.
   */
  sourceUrl: string | undefined;
  /**
   * The server's page of the commerce disclosures of its supporter plans (ADR-0072 §13, `/about/commerce`);
   * undefined when the instance publishes none or has not loaded.
   */
  commerceDisclosureUrl: string | undefined;
};

/** Splits a plain-text description into paragraphs at blank lines, without empty ones. */
function paragraphs(text: string | undefined): string[] {
  if (text === undefined) return [];
  return text
    .split(/\n\s*\n/u)
    .map((paragraph) => paragraph.trim())
    .filter((paragraph) => paragraph !== "");
}

/**
 * What the information page says about the instance, from GET /instance (which the shell reads on every page and
 * the route's loader has asked for). While it loads, and when it cannot, the page has only its heading, the
 * moderated domains and the links the shell knows.
 */
export function useAbout(): About {
  const api = useApi();
  const { data: instance } = useQuery({ ...instanceQueryOptions(api), ...shellQueryReader });
  return {
    summary: instance?.short_description?.trim() ?? "",
    description: paragraphs(instance?.description),
    rules: instance?.rules ?? [],
    thumbnail: instance?.thumbnail,
    contact: instance?.contact,
    crawlers:
      instance === undefined ? undefined : { searchIndexing: instance.search_indexing, aiUsage: instance.ai_usage },
    finance: instance?.finance_visibility,
    income: instance?.income_visibility,
    sourceUrl: instance?.source_url,
    commerceDisclosureUrl: instance?.commerce_disclosure_url,
  };
}

/** A moderated domain and the key of its row. */
type ModeratedDomain = { key: string; action: PublicDomainAction };

/**
 * Keys the actions for their rows. The list has no IDs: an action is its domain's digest and its kind, and the
 * rare second action of the same kind on one domain (a renewal before the first expires) is told apart by its
 * place among them, which later pages never change.
 */
function keyed(actions: readonly PublicDomainAction[]): ModeratedDomain[] {
  const seen = new Map<string, number>();
  return actions.map((action) => {
    const base = `${action.digest}:${action.kind}`;
    const n = seen.get(base) ?? 0;
    seen.set(base, n + 1);
    return { key: n === 0 ? base : `${base}:${String(n)}`, action };
  });
}

/**
 * The moderated domains as the page shows them. `hidden` is the instance setting that lists them to nobody (404);
 * `signed-in-only` the setting that lists them to signed-in users only, to a visitor without a session (401).
 */
export type ModeratedDomains =
  | { status: "hidden" }
  | { status: "signed-in-only" }
  | { status: "loading" }
  | { status: "error"; reason: string; retry: () => void; isRetrying: boolean }
  | {
      status: "ready";
      /** The actions, each with the key of its row. */
      items: readonly ModeratedDomain[];
      hasMore: boolean;
      loadMore: () => void;
      isLoadingMore: boolean;
      /** Why the next page failed to load; undefined when it did not. */
      moreError: string | undefined;
    };

/**
 * The remote domains the instance moderates, newest first (FR-718), a page at a time. A 4xx is the instance's
 * setting, not a failure to try again, so it is not retried; the loader has already asked once, and mounting does
 * not ask again for an answer it already has.
 */
export function useModeratedDomains(): ModeratedDomains {
  const api = useApi();
  const query = useInfiniteQuery({
    ...publicDomainActionsQueryOptions(api),
    retry: shellQueryRetry,
    retryOnMount: false,
  });
  if (query.data !== undefined) {
    return {
      status: "ready",
      items: keyed(query.data.pages.flatMap((page) => page.items)),
      hasMore: query.hasNextPage,
      loadMore: () => void query.fetchNextPage(),
      isLoadingMore: query.isFetchingNextPage,
      moreError: query.isFetchNextPageError ? errorMessage(query.error) : undefined,
    };
  }
  if (query.error !== null) {
    if (query.error instanceof ApiError && query.error.status === 404) return { status: "hidden" };
    if (query.error instanceof ApiError && query.error.status === 401) return { status: "signed-in-only" };
    return {
      status: "error",
      reason: errorMessage(query.error),
      retry: () => void query.refetch(),
      isRetrying: query.isFetching,
    };
  }
  return { status: "loading" };
}
