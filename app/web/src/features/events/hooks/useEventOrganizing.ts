// SPDX-License-Identifier: AGPL-3.0-only
// What the organizers of an event do on its page besides changing it (ADR-0161 §4, §9): inviting local users, who then
// see the event whatever its visibility, revoking invitations, and, for the organizer alone, naming up to three
// co-organizers and removing them. People are found by name among the local accounts (GET /accounts/search). Each
// change takes effect at once and is announced where it shows.
import { useNotice } from "@my/ui";
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import { useLoadFailure } from "../../../lib/loadFailure";
import { shellQueryRetry } from "../../../lib/query";
import {
  cacheEvent,
  cacheInvitations,
  coorganizerMutation,
  type Event,
  type EventAccount,
  type EventInvitation,
  eventInvitationsQueryOptions,
  inviteMutation,
  localAccountSearchQueryOptions,
  revokeInvitationMutation,
} from "../api/events";
import { accountName, isolate } from "./names";
import { useEventFailureMessage } from "./useEventFailureMessage";

/** The local accounts found by a search as the user types, without the remote ones. */
export type AccountSearch =
  | { status: "idle" }
  | { status: "loading" }
  | { status: "error"; reason: string }
  | { status: "done"; accounts: readonly EventAccount[] };

/** The search over the local accounts by name; idle while the field is empty. */
function useLocalAccountSearch(text: string): AccountSearch {
  const api = useApi();
  const q = text.trim().replace(/^@/u, "");
  const query = useQuery({ ...localAccountSearchQueryOptions(api, q), enabled: q !== "" });
  if (q === "") return { status: "idle" };
  if (query.data !== undefined) return { status: "done", accounts: query.data };
  if (query.isError) return { status: "error", reason: errorMessage(query.error) };
  return { status: "loading" };
}

type EventInvitations =
  | { status: "loading" }
  | { status: "error"; reason: string; retry: () => void; isRetrying: boolean }
  | {
      status: "ready";
      invitations: readonly EventInvitation[];
      hasMore: boolean;
      loadMore: () => void;
      isLoadingMore: boolean;
    };

/** The invitations of the event, for its organizers, with inviting and revoking. */
export function useEventInvitations(event: Event) {
  const api = useApi();
  const t = useT();
  const notice = useNotice();
  const queryClient = useQueryClient();
  const failed = useEventFailureMessage();
  const [search, setSearch] = useState("");
  const found = useLocalAccountSearch(search);
  const query = useInfiniteQuery({
    ...eventInvitationsQueryOptions(api, event.id),
    retry: shellQueryRetry,
    retryOnMount: false,
  });
  const failure = useLoadFailure(query);
  const invite = useMutation({
    ...inviteMutation(api, event.id),
    onSuccess: () => cacheInvitations(api, queryClient, event.id),
  });
  const revoke = useMutation({
    ...revokeInvitationMutation(api, event.id),
    onSuccess: () => cacheInvitations(api, queryClient, event.id),
  });
  let list: EventInvitations;
  if (query.data !== undefined) {
    list = {
      status: "ready",
      invitations: query.data.pages.flatMap((page) => page.items),
      hasMore: query.hasNextPage,
      loadMore: () => {
        if (!query.isFetching) void query.fetchNextPage();
      },
      isLoadingMore: query.isFetchingNextPage,
    };
  } else if (failure.error !== null) {
    list = {
      status: "error",
      reason: errorMessage(failure.error),
      retry: () => void query.refetch(),
      isRetrying: failure.isRetrying,
    };
  } else {
    list = { status: "loading" };
  }
  const invited = new Set(list.status === "ready" ? list.invitations.map((i) => i.account.id) : []);
  return {
    list,
    search,
    changeSearch: (value: string) => {
      setSearch(value);
      if (invite.isError) invite.reset();
    },
    found,
    /** Whether `account` has an invitation that was not revoked, or organizes the event. */
    isInvited: (account: EventAccount) =>
      invited.has(account.id) ||
      account.id === event.organizer.id ||
      event.coorganizers.some((c) => c.id === account.id),
    invite: (account: EventAccount) => {
      if (invite.isPending) return;
      invite.mutate(account, {
        onSuccess: () => {
          notice.inPlace(t("client:events.invitations.invited", { name: isolate(accountName(account)) }));
        },
      });
    },
    invitingId: invite.isPending ? invite.variables.id : undefined,
    inviteError: invite.isError ? failed(invite.error) : undefined,
    revoke: (account: EventAccount) => {
      if (revoke.isPending) return;
      revoke.mutate(account, {
        onSuccess: () => {
          notice.inPlace(t("client:events.invitations.revoked", { name: isolate(accountName(account)) }));
        },
        onError: (error) => {
          notice.toast({ tone: "danger", message: failed(error) });
        },
      });
    },
    revokingId: revoke.isPending ? revoke.variables.id : undefined,
  };
}

/** Naming co-organizers of the event and removing them, by its organizer. */
export function useCoorganizers(event: Event) {
  const api = useApi();
  const t = useT();
  const notice = useNotice();
  const queryClient = useQueryClient();
  const failed = useEventFailureMessage();
  const [search, setSearch] = useState("");
  const found = useLocalAccountSearch(search);
  const change = useMutation({
    ...coorganizerMutation(api, event.id),
    onSuccess: (saved) => cacheEvent(api, queryClient, saved),
  });
  return {
    search,
    changeSearch: (value: string) => {
      setSearch(value);
      if (change.isError) change.reset();
    },
    found,
    /** Whether `account` organizes or co-organizes the event already. */
    isOrganizer: (account: EventAccount) =>
      account.id === event.organizer.id || event.coorganizers.some((c) => c.id === account.id),
    assign: (account: EventAccount) => {
      if (change.isPending) return;
      change.mutate(
        { account, assign: true },
        {
          onSuccess: () => {
            setSearch("");
            notice.inPlace(t("client:events.coorganizers.assigned", { name: isolate(accountName(account)) }));
          },
        },
      );
    },
    remove: (account: EventAccount) => {
      if (change.isPending) return;
      change.mutate(
        { account, assign: false },
        {
          onSuccess: () => {
            notice.inPlace(t("client:events.coorganizers.removed", { name: isolate(accountName(account)) }));
          },
        },
      );
    },
    pendingId: change.isPending ? change.variables.account.id : undefined,
    error: change.isError ? failed(change.error) : undefined,
  };
}
