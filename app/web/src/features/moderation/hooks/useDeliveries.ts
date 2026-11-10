// SPDX-License-Identifier: AGPL-3.0-only
// The deliveries to a remote server on its page (#2748): the list under the filter the moderator ran, the form of
// that filter, and revealing the masked fields of one delivery. A failure to load the list stays in its region; a
// failed reveal is a toast and keeps the button (web-client/README.md E2, web-client/16-look-states-and-wording.md G16).
import { useToast } from "@my/ui";
import { useInfiniteQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { type SyntheticEvent, useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import { useLoadFailure } from "../../../lib/loadFailure";
import { shellQueryRetry } from "../../../lib/query";
import { refreshAuditLog } from "../api/staff";
import {
  type Delivery,
  type DeliveryFilter,
  type DeliveryReveal,
  deliveriesQueryOptions,
  noDeliveryFilter,
  revealDeliveryMutation,
  UnknownLocalAccountError,
} from "../api/deliveries";
import { type PagedPages, toPagedList } from "./pagedList";

/** The deliveries of one filter: loading, failed (with a retry), or the list so far. */
export type Deliveries =
  | { status: "loading" }
  | { status: "error"; reason: string; retry: () => void; isRetrying: boolean }
  | { status: "ready"; list: PagedPages<Delivery>; moreError: string | undefined };

/** The deliveries to `domain` under `filter`, read when the region mounts or the filter changes. */
export function useDeliveries(domain: string, filter: DeliveryFilter): Deliveries {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const query = useInfiniteQuery({
    ...deliveriesQueryOptions(api, queryClient, domain, filter),
    retry: shellQueryRetry,
  });
  const failure = useLoadFailure(query);
  if (query.data === undefined) {
    if (failure.error === null) return { status: "loading" };
    return {
      status: "error",
      reason:
        failure.error instanceof UnknownLocalAccountError
          ? t("client:admin.instance.deliveries.filter.unknown_account", { username: failure.error.username })
          : errorMessage(failure.error),
      retry: () => {
        void query.refetch();
      },
      isRetrying: failure.isRetrying,
    };
  }
  return {
    status: "ready",
    list: toPagedList({
      data: query.data,
      hasNextPage: query.hasNextPage,
      isFetchingNextPage: query.isFetchingNextPage,
      isFetching: query.isFetching,
      fetchNextPage: query.fetchNextPage,
    }),
    moreError: query.isFetchNextPageError ? errorMessage(query.error) : undefined,
  };
}

/** The filter form: what is typed, and the filter that runs (none until the moderator submits one). */
export type DeliveryFilterForm = {
  draft: DeliveryFilter;
  change: (next: Partial<DeliveryFilter>) => void;
  /** Runs the typed filter, once the browser's constraint validation passed. */
  submit: (event: SyntheticEvent<HTMLFormElement>) => void;
  /** Goes back to every delivery and empties the form. */
  clear: () => void;
  /** The filter that runs. */
  applied: DeliveryFilter;
  isFiltered: boolean;
};

/** The username of a local account as typed: without spaces and a leading `@`. */
function normalizedUsername(value: string): string {
  return value.trim().replace(/^@/u, "");
}

/**
 * The filter of the deliveries. It lives in the page, not in its address: a filter by account is recorded in the
 * audit log each time it runs, so it runs only when the moderator submits it, never from a link or a reload.
 */
export function useDeliveryFilterForm(): DeliveryFilterForm {
  const [draft, setDraft] = useState<DeliveryFilter>(noDeliveryFilter);
  const [applied, setApplied] = useState<DeliveryFilter>(noDeliveryFilter);
  return {
    draft,
    change: (next) => {
      setDraft({ ...draft, ...next });
    },
    submit: (event) => {
      event.preventDefault();
      setApplied({ username: normalizedUsername(draft.username), activityUri: draft.activityUri.trim() });
    },
    clear: () => {
      setDraft(noDeliveryFilter);
      setApplied(noDeliveryFilter);
    },
    applied,
    isFiltered: applied.username !== "" || applied.activityUri !== "",
  };
}

/** The masked fields of one delivery: hidden until `reveal()`, then what the server answered. */
export type RevealDelivery = {
  reveal: () => void;
  isPending: boolean;
  data: DeliveryReveal | undefined;
};

/**
 * The sender, activity URI and inbox of delivery `id` to `domain`. Nothing is asked of the server until `reveal()`,
 * the moderator's own action, and the answer is kept by this mutation only: it is not cached, so the delivery shows
 * them again only when asked again (and recorded again). They are shown, and `onRevealed()` runs, once the audit log,
 * which records the call, is marked stale (refreshAuditLog).
 */
export function useRevealDelivery(domain: string, id: string, onRevealed: () => void): RevealDelivery {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const toast = useToast();
  const mutation = useMutation({
    ...revealDeliveryMutation(api),
    onSuccess: async () => {
      await refreshAuditLog(queryClient);
      onRevealed();
    },
    onError: (error) => {
      toast.show({
        message: t("client:admin.instance.deliveries.reveal.failed", { reason: errorMessage(error) }),
        tone: "danger",
      });
    },
  });
  return {
    reveal: () => {
      if (!mutation.isPending) mutation.mutate({ domain, id });
    },
    isPending: mutation.isPending,
    data: mutation.data,
  };
}
