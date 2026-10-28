// SPDX-License-Identifier: AGPL-3.0-only
// The requests tab of the custom emoji screen (ADR-0070 §2, §9, manage_custom_emojis): the requests in one state,
// the number of the pending ones for the tab, and approving or rejecting one. What a decision did is announced and
// kept in a StatusMessage; a failure stays next to its cause.
import { useNotice } from "@my/ui";
import { useInfiniteQuery, useMutation, useQueryClient, useSuspenseInfiniteQuery } from "@tanstack/react-query";
import { useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import { shellQueryRetry } from "../../../lib/query";
import { type PagedPages, toPagedList } from "../../moderation";
import {
  type AdminCustomEmojiRequest,
  adminCustomEmojiRequestsQueryOptions,
  approveCustomEmojiRequestMutation,
  cacheDecidedCustomEmojiRequest,
  type CustomEmojiRequestState,
  rejectCustomEmojiRequestMutation,
} from "../api/customEmojiRequests";

/** A request for a sentence: its `:shortcode:`, isolated left to right from the text around it (U+2066 … U+2069). */
export function requestInText(request: Pick<AdminCustomEmojiRequest, "shortcode">): string {
  return `⁦:${request.shortcode}:⁩`;
}

/** The requests in `state`, newest first. */
export function useAdminCustomEmojiRequests(state: CustomEmojiRequestState): PagedPages<AdminCustomEmojiRequest> {
  const api = useApi();
  return toPagedList(useSuspenseInfiniteQuery(adminCustomEmojiRequestsQueryOptions(api, state)));
}

/**
 * How many requests await a decision, for the tab (ADR-0070 §9): those of the first page of the pending ones, with
 * `more` when there are more pages; undefined while they load or when they fail to (the tab then shows no count).
 */
export function usePendingCustomEmojiRequestCount(): { count: number; more: boolean } | undefined {
  const api = useApi();
  const query = useInfiniteQuery({ ...adminCustomEmojiRequestsQueryOptions(api, "pending"), retry: shellQueryRetry });
  const first = query.data?.pages[0];
  if (first === undefined) return undefined;
  return { count: first.items.length, more: first.next_cursor !== undefined };
}

export type CustomEmojiRequestActions = {
  /** Approves a pending request; resolves with whether it succeeded, the failure in `error`. */
  approve: (request: AdminCustomEmojiRequest) => Promise<boolean>;
  /** Rejects a pending request with `reason` ("" for none); resolves with whether it succeeded. */
  reject: (request: AdminCustomEmojiRequest, reason: string) => Promise<boolean>;
  /** The request being decided. */
  pendingId: string | undefined;
  /** Why the last decision failed, localized (the server's detail of a 409 says why), until the next one. */
  error: string | undefined;
  /** What the last decision did, for the list's StatusMessage. */
  status: string | undefined;
};

/**
 * Deciding the requests of the list. `onDecided(id)` is called once a decided request is about to leave the pending
 * list (to keep focus, lib/useFocusAfterRemoval).
 */
export function useCustomEmojiRequestActions(onDecided: (id: string) => void): CustomEmojiRequestActions {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const notice = useNotice();
  const [status, setStatus] = useState<string | undefined>(undefined);
  const approve = useMutation(approveCustomEmojiRequestMutation(api));
  const reject = useMutation(rejectCustomEmojiRequestMutation(api));
  const decided = async (request: AdminCustomEmojiRequest, message: string) => {
    onDecided(request.id);
    await cacheDecidedCustomEmojiRequest(queryClient, request);
    setStatus(message);
    notice.inPlace(message);
  };
  const failed = [approve, reject].find((mutation) => mutation.isError);
  return {
    approve: async (request) => {
      reject.reset();
      let saved: AdminCustomEmojiRequest;
      try {
        saved = await approve.mutateAsync(request.id);
      } catch {
        return false;
      }
      await decided(saved, t("client:admin.custom_emoji_requests.approved", { emoji: requestInText(saved) }));
      return true;
    },
    reject: async (request, reason) => {
      approve.reset();
      let saved: AdminCustomEmojiRequest;
      try {
        saved = await reject.mutateAsync({ id: request.id, reason: reason.trim() });
      } catch {
        return false;
      }
      await decided(saved, t("client:admin.custom_emoji_requests.rejected", { emoji: requestInText(saved) }));
      return true;
    },
    pendingId: approve.isPending ? approve.variables : reject.isPending ? reject.variables.id : undefined,
    error: failed === undefined ? undefined : errorMessage(failed.error),
    status,
  };
}
