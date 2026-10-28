// SPDX-License-Identifier: AGPL-3.0-only
// The user's page of their custom emoji (ADR-0070 §2, §3, §4, create_custom_emojis): what they used of their limit,
// the form that requests a new emoji, their requests with withdrawing a pending one, and the emoji they made with
// changing their settings, requesting a new image, and deleting them. What each change did is announced and kept in a
// StatusMessage; a failure stays next to its cause.
import { useNotice } from "@my/ui";
import { useMutation, useQueryClient, useSuspenseInfiniteQuery } from "@tanstack/react-query";
import { type SyntheticEvent, useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { type Translate, useT } from "../../../lib/i18n";
import { type PagedList, toPagedList } from "../../moderation";
import { MediaProcessingError } from "../../settings";
import {
  cacheChangedOwnCustomEmoji,
  cacheDeletedOwnCustomEmoji,
  cacheRequestedCustomEmoji,
  cacheWithdrawnCustomEmojiRequest,
  type CustomEmojiRequest,
  deleteOwnCustomEmojiMutation,
  type OwnCustomEmoji,
  type OwnCustomEmojiChange,
  ownCustomEmojiRequestsQueryOptions,
  ownCustomEmojisQueryOptions,
  requestCustomEmojiMutation,
  updateOwnCustomEmojiMutation,
  withdrawCustomEmojiRequestMutation,
} from "../api/ownEmojis";

/** An emoji for a sentence: its `:shortcode:`, isolated left to right from the text around it (U+2066 … U+2069). */
export function emojiInText(emoji: { shortcode: string }): string {
  return `⁦:${emoji.shortcode}:⁩`;
}

/** Why a change failed, localized: the server's reason, or that it could not process the image. */
function failure(t: Translate, error: unknown): string {
  return error instanceof MediaProcessingError ? t("client:custom_emojis.image_failed") : errorMessage(error);
}

/**
 * The user's emoji, newest first, with what they used of their limit: `pending` is the part of `used` that their pending
 * requests take, and `limit` is undefined for no limit.
 */
export function useOwnCustomEmojis(): PagedList<OwnCustomEmoji> & {
  used: number;
  pending: number;
  limit: number | undefined;
} {
  const api = useApi();
  const query = useSuspenseInfiniteQuery(ownCustomEmojisQueryOptions(api));
  const first = query.data.pages[0];
  return { ...toPagedList(query), used: first?.used ?? 0, pending: first?.pending ?? 0, limit: first?.limit };
}

/** The user's requests in every state, newest first. */
export function useOwnCustomEmojiRequests(): PagedList<CustomEmojiRequest> {
  const api = useApi();
  return toPagedList(useSuspenseInfiniteQuery(ownCustomEmojiRequestsQueryOptions(api)));
}

/** The form of a request for a new emoji, filled in by the user. */
type Draft = { file: File | null; shortcode: string };

const emptyDraft: Draft = { file: null, shortcode: "" };

export type CustomEmojiRequestForm = {
  draft: Draft;
  change: (change: Partial<Draft>) => void;
  /** Uploads the image and sends the request, once the browser has checked the shortcode and an image is chosen. */
  submit: (event: SyntheticEvent<HTMLFormElement>) => void;
  isPending: boolean;
  /** That no image is chosen, after a submit without one. */
  missingImage: boolean;
  /** Why the request failed, localized; the draft stays. */
  error: string | undefined;
  /** What the last request did, for the form's StatusMessage. */
  status: string | undefined;
};

/** The form of a request for a new emoji. A success empties it and says so once the lists are fetched again. */
export function useCustomEmojiRequestForm(): CustomEmojiRequestForm {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const [draft, setDraft] = useState<Draft>(emptyDraft);
  const [missingImage, setMissingImage] = useState(false);
  const [status, setStatus] = useState<string | undefined>(undefined);
  const mutation = useMutation({
    ...requestCustomEmojiMutation(api),
    onSuccess: async (request) => {
      await cacheRequestedCustomEmoji(queryClient);
      setDraft(emptyDraft);
      setStatus(t("client:custom_emojis.requested", { emoji: emojiInText(request) }));
    },
  });
  return {
    draft,
    change: (change) => {
      setDraft((previous) => ({ ...previous, ...change }));
      if (change.file !== undefined) setMissingImage(false);
    },
    submit: (event) => {
      event.preventDefault();
      if (draft.file === null) {
        setMissingImage(true);
        return;
      }
      setStatus(undefined);
      mutation.mutate({ kind: "create", shortcode: draft.shortcode.trim(), file: draft.file });
    },
    isPending: mutation.isPending,
    missingImage,
    error: mutation.isError ? failure(t, mutation.error) : undefined,
    status,
  };
}

export type CustomEmojiRequestActions = {
  /** Withdraws a pending request. */
  withdraw: (request: CustomEmojiRequest) => void;
  /** The request being withdrawn. */
  pendingId: string | undefined;
  /** Why the last withdrawal failed, localized, until the next one. */
  error: string | undefined;
  /** What the last withdrawal did, for the list's StatusMessage. */
  status: string | undefined;
};

/** Withdrawing the user's pending requests; a withdrawn request keeps its row, which shows its new state. */
export function useCustomEmojiRequestActions(): CustomEmojiRequestActions {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const notice = useNotice();
  const [status, setStatus] = useState<string | undefined>(undefined);
  const withdraw = useMutation(withdrawCustomEmojiRequestMutation(api));
  return {
    withdraw: (request) => {
      withdraw.mutate(request.id, {
        onSuccess: (saved) => {
          void cacheWithdrawnCustomEmojiRequest(queryClient, saved).then(() => {
            const message = t("client:custom_emojis.withdrawn", { emoji: emojiInText(saved) });
            setStatus(message);
            notice.inPlace(message);
          });
        },
      });
    },
    pendingId: withdraw.isPending ? withdraw.variables : undefined,
    error: withdraw.isError ? errorMessage(withdraw.error) : undefined,
    status,
  };
}

export type OwnCustomEmojiActions = {
  /** Changes an emoji's settings; resolves with whether it succeeded, the failure in `error`. */
  update: (emoji: OwnCustomEmoji, change: OwnCustomEmojiChange) => Promise<boolean>;
  /** Uploads a new image of an emoji and requests it. */
  requestImage: (emoji: OwnCustomEmoji, file: File) => void;
  /** Deletes an emoji; resolves with whether it succeeded. */
  remove: (emoji: OwnCustomEmoji) => Promise<boolean>;
  /** The emoji being changed. */
  pendingId: string | undefined;
  /** Why the last change failed, localized, until the next change. */
  error: string | undefined;
  /** What the last change did, for the list's StatusMessage. */
  status: string | undefined;
};

/**
 * Changing the user's emoji. `onDeleted(id)` is called once a deleted emoji is about to leave the list (to keep
 * focus, lib/useFocusAfterRemoval); a changed emoji keeps its row, which shows the answer.
 */
export function useOwnCustomEmojiActions(onDeleted: (id: string) => void): OwnCustomEmojiActions {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const notice = useNotice();
  const [status, setStatus] = useState<string | undefined>(undefined);
  const report = (message: string) => {
    setStatus(message);
    notice.inPlace(message);
  };
  const update = useMutation(updateOwnCustomEmojiMutation(api));
  const image = useMutation(requestCustomEmojiMutation(api));
  const remove = useMutation(deleteOwnCustomEmojiMutation(api));
  const imageFor = image.variables?.kind === "image" ? image.variables.emojiId : undefined;
  const pendingId = remove.isPending
    ? remove.variables
    : update.isPending
      ? update.variables.id
      : image.isPending
        ? imageFor
        : undefined;
  const failed = [update, image, remove].find((mutation) => mutation.isError);
  return {
    update: async (emoji, change) => {
      image.reset();
      remove.reset();
      try {
        const saved = await update.mutateAsync({ id: emoji.id, change });
        await cacheChangedOwnCustomEmoji(queryClient, saved);
        report(t("client:custom_emojis.updated", { emoji: emojiInText(saved) }));
        return true;
      } catch {
        return false;
      }
    },
    requestImage: (emoji, file) => {
      update.reset();
      remove.reset();
      image.mutate(
        { kind: "image", emojiId: emoji.id, file },
        {
          onSuccess: () => {
            void cacheRequestedCustomEmoji(queryClient).then(() => {
              report(t("client:custom_emojis.image_requested", { emoji: emojiInText(emoji) }));
            });
          },
        },
      );
    },
    remove: async (emoji) => {
      update.reset();
      image.reset();
      try {
        await remove.mutateAsync(emoji.id);
      } catch {
        return false;
      }
      onDeleted(emoji.id);
      await cacheDeletedOwnCustomEmoji(queryClient, emoji.id);
      report(t("client:custom_emojis.deleted", { emoji: emojiInText(emoji) }));
      return true;
    },
    pendingId,
    error: failed === undefined ? undefined : failure(t, failed.error),
    status,
  };
}
