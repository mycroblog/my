// SPDX-License-Identifier: AGPL-3.0-only
import { useQueryClient } from "@tanstack/react-query";
import { createContext, use, useEffect, useState, useSyncExternalStore } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { registerReloadBlocker } from "../../../lib/reloadSafety";
import { composeKey } from "../../../lib/storage";
import { useViewer } from "../../session";
import { cacheReplyTarget, type Status } from "../api/compose";
import { type ComposeState, DraftStore, hasUnuploadedAttachment } from "./draftStore";
import { directMessageText, replyMentions } from "./policy";

/** The id of the post's text area, which a reply focuses. Only one form is on the page at a time. */
export const composeTextId = "compose-text";

/** What the compose provider shares; it changes only when the account or the sheet changes. */
export type ComposeController = {
  accountId: string;
  store: DraftStore;
  isSheetOpen: boolean;
  setSheetOpen: (isOpen: boolean) => void;
  /** Starts a reply to `status`: the form names it and starts with its mentions, then gets focus. */
  reply: (status: Status) => void;
  /** Quotes `status` in the draft (FR-114): the form shows it, then gets focus. */
  quote: (status: Status) => void;
  /** Starts a direct message to the account `acct`: the form mentions it with the direct visibility, then gets focus. */
  directMessage: (acct: string) => void;
};

export const ComposeContext = createContext<ComposeController | null>(null);

/** The controller for the signed-in viewer, or null while nobody is signed in. */
export function useComposeController(): ComposeController | null {
  const viewer = useViewer();
  const api = useApi();
  const queryClient = useQueryClient();
  const accountId = viewer.status === "signed-in" ? viewer.account.id : null;
  // One store per account: when the account changes, the store is replaced during rendering, as
  // react.dev describes for state that depends on a changed value ("Adjusting some state when a prop changes").
  const [store, setStore] = useState<{ accountId: string; store: DraftStore } | null>(null);
  if (accountId !== (store?.accountId ?? null)) {
    setStore(accountId === null ? null : { accountId, store: new DraftStore(composeKey(accountId)) });
  }
  const [isSheetOpen, setIsSheetOpen] = useState(false);
  const currentStore = store?.store;
  useEffect(() => {
    if (currentStore === undefined) return;
    return registerReloadBlocker(() => hasUnuploadedAttachment(currentStore.getSnapshot()));
  }, [currentStore]);
  if (store === null || accountId === null || store.accountId !== accountId) return null;
  const draftStore = store.store;
  return {
    accountId,
    store: draftStore,
    isSheetOpen,
    setSheetOpen: setIsSheetOpen,
    reply(status) {
      const { draft } = draftStore.getSnapshot();
      draftStore.edit({
        replyToId: status.id,
        visibility: null,
        text: draft.text.trim() === "" ? replyMentions(status, accountId) : draft.text,
      });
      cacheReplyTarget(api, queryClient, status);
      setIsSheetOpen(true);
    },
    quote(status) {
      draftStore.edit({ quoteId: status.id });
      cacheReplyTarget(api, queryClient, status);
      setIsSheetOpen(true);
    },
    directMessage(acct) {
      const { draft } = draftStore.getSnapshot();
      // A new conversation: it answers and quotes nothing, whatever the draft was for.
      draftStore.edit({
        text: directMessageText(draft.text, acct),
        visibility: "direct",
        replyToId: null,
        quoteId: null,
      });
      setIsSheetOpen(true);
    },
  };
}

/** The compose controller; null while nobody is signed in. */
export function useComposeContext(): ComposeController | null {
  return use(ComposeContext);
}

/** The compose state of the controller's store. */
export function useComposeState(store: DraftStore): ComposeState {
  return useSyncExternalStore(store.subscribe, store.getSnapshot);
}

/**
 * Returns `reply(status)`, which opens the form as a reply to `status`, or null while nobody is signed in
 * (the reply buttons of other features hide then).
 */
export function useReply(): ((status: Status) => void) | null {
  return useComposeContext()?.reply ?? null;
}

/**
 * Returns `quote(status)`, which opens the form quoting `status`, or null while nobody is signed in (the
 * quote buttons of other features hide then).
 */
export function useQuote(): ((status: Status) => void) | null {
  return useComposeContext()?.quote ?? null;
}

/**
 * Returns `directMessage(acct)`, which opens the form as a direct message to the account `acct`, or null while nobody
 * is signed in.
 */
export function useDirectMessage(): ((acct: string) => void) | null {
  return useComposeContext()?.directMessage ?? null;
}
