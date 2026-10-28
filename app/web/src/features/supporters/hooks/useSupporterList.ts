// SPDX-License-Identifier: AGPL-3.0-only
// The supporter list (ADR-0071 §4): the members who chose to be listed, by plan, a page at a time; the user's own
// choice to be on it; and the plans and the list that the information page links to.
import { useAnnouncer } from "@my/ui";
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useApi } from "../../../lib/auth/SessionProvider";
import { ApiError, errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import { isTauriApp } from "../../../lib/platform";
import { shellQueryReader, shellQueryRetry } from "../../../lib/query";
import {
  cacheListing,
  instanceQueryOptions,
  type SupporterListEntry,
  type SupporterListPlan,
  supporterListingQueryOptions,
  supporterListQueryOptions,
  type SupporterPlan,
  supporterPlansQueryOptions,
  updateListingMutation,
} from "../api/supporters";

/** A plan of the list with its members on the pages read so far, in the list's order. */
export type SupporterGroup = { plan: SupporterListPlan; members: readonly SupporterListEntry[] };

/**
 * The supporter list as the page shows it. `hidden` is the instance setting that publishes it to nobody (404);
 * `signed-in-only` the setting that publishes it to signed-in users only, to a visitor without a session (401).
 */
export type SupporterList =
  | { status: "hidden" }
  | { status: "signed-in-only" }
  | { status: "loading" }
  | { status: "error"; reason: string; retry: () => void; isRetrying: boolean }
  | {
      status: "ready";
      /** The plans that have members on the pages read so far, in the plans' order. */
      groups: readonly SupporterGroup[];
      hasMore: boolean;
      loadMore: () => void;
      isLoadingMore: boolean;
      moreError: string | undefined;
    };

/**
 * The members who chose to be listed, by plan (GET /instance/supporters). A 4xx is the instance's setting, not a
 * failure to try again, so it is not retried; the loader has already asked once.
 */
export function useSupporterList(): SupporterList {
  const api = useApi();
  const query = useInfiniteQuery({ ...supporterListQueryOptions(api), retry: shellQueryRetry, retryOnMount: false });
  if (query.data !== undefined) {
    const plans = query.data.pages[0]?.plans ?? [];
    const members = query.data.pages.flatMap((page) => page.items);
    return {
      status: "ready",
      groups: plans
        .map((plan) => ({ plan, members: members.filter((m) => m.plan_id === plan.id) }))
        .filter((g) => g.members.length > 0),
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

/** What the information page says of the supporter program: its plans (outside the Tauri app) and whether it links to the list. */
export type AboutSupporters = {
  /** The plans that take subscriptions; undefined when the page shows none (disabled, the app, not loaded). */
  plans: readonly SupporterPlan[] | undefined;
  /** Whether the instance publishes the supporter list to someone. */
  listPublished: boolean;
};

export function useAboutSupporters(): AboutSupporters {
  const api = useApi();
  const inApp = isTauriApp();
  const instance = useQuery({ ...instanceQueryOptions(api), ...shellQueryReader });
  const enabled = instance.data?.supporters_enabled === true;
  const plans = useQuery({
    ...supporterPlansQueryOptions(api),
    retry: shellQueryRetry,
    retryOnMount: false,
    enabled: enabled && !inApp,
  });
  return {
    plans: enabled && !inApp ? plans.data?.items : undefined,
    listPublished: instance.data !== undefined && instance.data.supporter_list_visibility !== "off",
  };
}

/** The user's choice to be on the supporter list, as its switch shows it; undefined when the section is not shown. */
export type SupporterListing = {
  listed: boolean;
  onChange: (listed: boolean) => void;
  /** Why the last change was not saved, localized; undefined when it was. */
  error: string | undefined;
};

/**
 * The user's choice to be on the supporter list (GET / PUT /preferences/supporter_listing), saved as soon as it
 * changes. Shown only to a member of a plan (the API answers 404 to the others) while the instance publishes the list.
 */
export function useSupporterListing(): SupporterListing | undefined {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const announce = useAnnouncer();
  const instance = useQuery({ ...instanceQueryOptions(api), ...shellQueryReader });
  const published = instance.data !== undefined && instance.data.supporter_list_visibility !== "off";
  const listing = useQuery({
    ...supporterListingQueryOptions(api),
    retry: shellQueryRetry,
    retryOnMount: false,
    enabled: published,
  });
  const mutation = useMutation({
    ...updateListingMutation(api),
    onSuccess: async (saved) => {
      await cacheListing(api, queryClient, saved);
      announce(t(saved.listed ? "client:supporter.listing.listed" : "client:supporter.listing.unlisted"));
    },
  });
  if (!published || listing.data === undefined) return undefined;
  return {
    listed: mutation.isPending ? mutation.variables : listing.data.listed,
    onChange: (listed) => {
      mutation.mutate(listed);
    },
    error: mutation.isError ? errorMessage(mutation.error) : undefined,
  };
}
