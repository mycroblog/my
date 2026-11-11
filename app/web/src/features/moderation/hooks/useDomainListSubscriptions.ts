// SPDX-License-Identifier: AGPL-3.0-only
// The subscriptions to the domain blocklists that others publish (ADR-0159 §4, §5, manage_domains): the list, the form
// of a new one, and the page of one: its drafts accepted or rejected one by one or together (a suspension among them
// only after its confirmation, NFR-MOD-7), fetching it now and reading it again until the fetch has run, its settings,
// and deleting it. What a change did is kept for the StatusMessage of its section; a failure is said there too.
import { useMutation, useQueryClient, useSuspenseInfiniteQuery, useSuspenseQuery } from "@tanstack/react-query";
import { type SyntheticEvent, useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import {
  cacheSavedSubscription,
  createDomainListSubscriptionMutation,
  decideDomainListDraftsMutation,
  deleteDomainListSubscriptionMutation,
  type DomainListDraft,
  domainListDraftsQueryOptions,
  domainListFetchesQueryOptions,
  type DomainListFormat,
  type DomainListSubscription,
  type DomainListSubscriptionChanges,
  domainListSubscriptionQueryOptions,
  domainListSubscriptionsQueryOptions,
  type DraftDecision,
  fetchDomainListMutation,
  forgetDeletedSubscription,
  refreshDecidedDrafts,
  refreshDomainListSubscriptions,
  updateDomainListSubscriptionMutation,
} from "../api/domainLists";
import type { ApplyDraft } from "../components/domainListParts";
import { type PagedPages, toPagedList } from "./pagedList";

/** The subscriptions, newest first. */
export function useDomainListSubscriptionList(): PagedPages<DomainListSubscription> {
  const api = useApi();
  return toPagedList(useSuspenseInfiniteQuery(domainListSubscriptionsQueryOptions(api)));
}

/** How a subscription applies its list, from what the server holds. */
function applyDraftOf(subscription: DomainListSubscription): ApplyDraft {
  return {
    mode: subscription.mode,
    suspendAs: subscription.suspend_as,
    silenceAs: subscription.silence_as,
    applyRejectMedia: subscription.apply_reject_media,
    applyRejectReports: subscription.apply_reject_reports,
    publicCommentFromList: subscription.public_comment_from_list,
  };
}

/** The fields of the API of how a subscription applies its list. */
function applyRequest(draft: ApplyDraft): Required<Omit<DomainListSubscriptionChanges, "title">> {
  return {
    mode: draft.mode,
    suspend_as: draft.suspendAs,
    silence_as: draft.silenceAs,
    apply_reject_media: draft.applyRejectMedia,
    apply_reject_reports: draft.applyRejectReports,
    public_comment_from_list: draft.publicCommentFromList,
  };
}

/** The form of a new subscription, as it is filled in. */
export type NewSubscriptionDraft = ApplyDraft & { title: string; url: string; format: DomainListFormat };

/**
 * The form of a new subscription, starting with the defaults of the API. A success calls `onCreated` with the
 * subscription once the list has it, to open its page (the page's heading is the result, G24).
 */
export function useNewDomainListSubscription(
  initial: NewSubscriptionDraft,
  onCreated: (subscription: DomainListSubscription) => void,
) {
  const api = useApi();
  const queryClient = useQueryClient();
  const [draft, setDraft] = useState(initial);
  const create = useMutation({
    ...createDomainListSubscriptionMutation(api),
    onSuccess: async (subscription) => {
      await refreshDomainListSubscriptions(queryClient);
      onCreated(subscription);
    },
  });
  return {
    draft,
    change: (change: Partial<NewSubscriptionDraft>) => {
      setDraft((previous) => ({ ...previous, ...change }));
    },
    submit: (event: SyntheticEvent<HTMLFormElement>) => {
      event.preventDefault();
      if (create.isPending) return;
      create.mutate({ title: draft.title.trim(), url: draft.url.trim(), format: draft.format, ...applyRequest(draft) });
    },
    isPending: create.isPending,
    /** Why subscribing failed, localized; the form keeps what was typed. */
    error: create.isError ? errorMessage(create.error) : undefined,
  };
}

/** The fetch asked for by hand that the page waits for: when, and the last fetch before it. */
type Watch = { since: number; lastFetchId: string | undefined };

/** How long the page reads a subscription again after "Fetch now", and how often. */
const watchFor = 60_000;
const watchEvery = 3000;

/**
 * One subscription with its fetches, and fetching it now. After "Fetch now" the subscription and its fetches are read
 * again every 3 seconds for a minute, until its last fetch is another one; `status` then says how it went.
 */
export function useDomainListSubscription(id: string) {
  const api = useApi();
  const t = useT();
  const [watch, setWatch] = useState<Watch | null>(null);
  const [status, setStatus] = useState<string | undefined>(undefined);
  const refetchInterval = () => (watch !== null && Date.now() - watch.since < watchFor ? watchEvery : false);
  const subscription = useSuspenseQuery({ ...domainListSubscriptionQueryOptions(api, id), refetchInterval }).data;
  const fetches = toPagedList(useSuspenseInfiniteQuery({ ...domainListFetchesQueryOptions(api, id), refetchInterval }));
  const lastFetch = subscription.last_fetch;
  // The fetch has run once the last fetch is another: what it did is kept from this render on (react.dev, "Storing
  // information from previous renders").
  if (watch !== null && lastFetch !== undefined && lastFetch.id !== watch.lastFetchId) {
    setWatch(null);
    setStatus(
      lastFetch.outcome === "failed"
        ? t("client:admin.domain_lists.fetch.failed")
        : t("client:admin.domain_lists.fetch.done", { count: lastFetch.entry_count }),
    );
  }
  const fetchNow = useMutation({
    ...fetchDomainListMutation(api, id),
    onMutate: () => {
      setStatus(undefined);
    },
    onSuccess: () => {
      setWatch({ since: Date.now(), lastFetchId: lastFetch?.id });
      setStatus(t("client:admin.domain_lists.fetch.queued"));
    },
  });
  return {
    subscription,
    fetches,
    fetchNow: () => {
      if (!fetchNow.isPending) fetchNow.mutate();
    },
    isFetching: fetchNow.isPending,
    /** Why the fetch could not be queued, localized. */
    fetchError: fetchNow.isError ? errorMessage(fetchNow.error) : undefined,
    /** What the last fetch asked for by hand did, for the section's StatusMessage. */
    fetchStatus: status,
  };
}

/** The drafts of a subscription. */
export function useDomainListDrafts(id: string): DomainListDraft[] {
  const api = useApi();
  return useSuspenseQuery(domainListDraftsQueryOptions(api, id)).data.items;
}

/** Whether accepting a draft issues a suspension, which is confirmed first (NFR-MOD-7). */
function suspends(draft: DomainListDraft): boolean {
  return draft.change === "issue" && draft.kind === "suspend";
}

/**
 * Accepting and rejecting drafts by key. `onDecided(keys)` is called once the server has done it, just before the
 * drafts are fetched again without them (for focus). Accepting drafts that suspend a domain asks first: `confirmation`
 * holds them, and how many domains they suspend, until it is confirmed or cancelled.
 */
export function useDecideDrafts(id: string, onDecided: (keys: readonly string[]) => void) {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const [status, setStatus] = useState<string | undefined>(undefined);
  const [confirming, setConfirming] = useState<readonly DomainListDraft[] | null>(null);
  const decide = useMutation({
    ...decideDomainListDraftsMutation(api, id),
    onMutate: () => {
      setStatus(undefined);
    },
    onSuccess: async (result, { decision, keys }) => {
      // The confirmation closes before the drafts are fetched again: while it is open, it keeps focus inside itself,
      // so the move to the heading or the next row, once the decided drafts are gone, would not stay.
      setConfirming(null);
      onDecided(keys);
      await refreshDecidedDrafts(queryClient, decision, result.done);
      const count = result.done.length;
      const missing = result.missing.length;
      setStatus(
        decision === "accept"
          ? missing === 0
            ? t("client:admin.domain_lists.drafts.accepted", { count })
            : t("client:admin.domain_lists.drafts.accepted_missing", { count, missing })
          : missing === 0
            ? t("client:admin.domain_lists.drafts.rejected", { count })
            : t("client:admin.domain_lists.drafts.rejected_missing", { count, missing }),
      );
    },
  });
  const run = (decision: DraftDecision, drafts: readonly DomainListDraft[]) => {
    decide.mutate({ decision, keys: drafts.map((draft) => draft.key) });
  };
  return {
    /** Accepts or rejects `drafts`; accepting a suspension asks first. */
    decide: (decision: DraftDecision, drafts: readonly DomainListDraft[]) => {
      if (decide.isPending || drafts.length === 0) return;
      decide.reset();
      if (decision === "accept" && drafts.some(suspends)) setConfirming(drafts);
      else run(decision, drafts);
    },
    /** The keys being decided. */
    pendingKeys: decide.isPending ? decide.variables.keys : [],
    isPending: decide.isPending,
    /** The confirmation of accepting suspensions: how many domains they suspend, or null while it is closed. */
    confirmation:
      confirming === null
        ? null
        : {
            suspensions: confirming.filter(suspends).length,
            confirm: () => {
              run("accept", confirming);
            },
            cancel: () => {
              setConfirming(null);
              decide.reset();
            },
          },
    /** Why the last decision failed, localized: in the confirmation while it is open, else above the drafts. */
    error: decide.isError ? errorMessage(decide.error) : undefined,
    /** What the last decision did, for the section's StatusMessage. */
    status,
  };
}

/**
 * The settings of a subscription: its title and how it applies its list. Only what changed is sent; the server queues a
 * fetch that applies them, and the page shows the subscription from the answer.
 */
export function useSubscriptionSettings(subscription: DomainListSubscription) {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const saved = { title: subscription.title, ...applyDraftOf(subscription) };
  const [drafts, setDrafts] = useState<Partial<typeof saved>>({});
  const [status, setStatus] = useState<string | undefined>(undefined);
  const draft = { ...saved, ...drafts };
  const save = useMutation({
    ...updateDomainListSubscriptionMutation(api, subscription.id),
    onMutate: () => {
      setStatus(undefined);
    },
    onSuccess: async (next) => {
      await cacheSavedSubscription(queryClient, api, next);
      setDrafts({});
      setStatus(t("client:admin.domain_lists.settings.saved"));
    },
  });
  const changes = (): DomainListSubscriptionChanges => {
    const all = { title: draft.title.trim(), ...applyRequest(draft) };
    const before = { title: saved.title, ...applyRequest(saved) };
    return Object.fromEntries(
      Object.entries(all).filter(([key, value]) => before[key as keyof typeof before] !== value),
    );
  };
  return {
    draft,
    change: (change: Partial<typeof saved>) => {
      setDrafts((previous) => ({ ...previous, ...change }));
    },
    submit: (event: SyntheticEvent<HTMLFormElement>) => {
      event.preventDefault();
      if (save.isPending) return;
      const body = changes();
      if (Object.keys(body).length === 0) {
        setStatus(t("client:admin.domain_lists.settings.unchanged"));
        return;
      }
      save.mutate(body);
    },
    isSaving: save.isPending,
    /** Why saving failed, localized; the form keeps the drafts. */
    error: save.isError ? errorMessage(save.error) : undefined,
    status,
  };
}

/**
 * Deleting a subscription, lifting its actions when asked. `onDeleted` is called once it is deleted, before the lists
 * are fetched again, so that the page can leave for the list. Resolves with whether it succeeded.
 */
export function useDeleteSubscription(id: string, onDeleted: (liftedActions: boolean) => void) {
  const api = useApi();
  const queryClient = useQueryClient();
  const remove = useMutation(deleteDomainListSubscriptionMutation(api, id));
  return {
    remove: async (liftActions: boolean): Promise<boolean> => {
      try {
        await remove.mutateAsync(liftActions);
      } catch {
        return false;
      }
      onDeleted(liftActions);
      await forgetDeletedSubscription(queryClient, id, liftActions);
      return true;
    },
    isPending: remove.isPending,
    error: remove.isError ? errorMessage(remove.error) : undefined,
  };
}
