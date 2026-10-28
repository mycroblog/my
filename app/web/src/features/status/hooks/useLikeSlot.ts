// SPDX-License-Identifier: AGPL-3.0-only
// The viewer's Like slot of a post (ADR-0052 §1): a favourite or one emoji reaction, never both. Favouriting,
// reacting, changing the emoji and taking either back all change the one slot, optimistically in every cached copy
// of the post (web-client/README.md S8), one change at a time per post in the order pressed, so that the last press wins.
import { useAnnouncer, useToast } from "@my/ui";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { type Translate, useT } from "../../../lib/i18n";
import { type LikeSlot, likeSlotOf } from "../api/cache";
import { cacheLikeSlotAnswer, cacheLikeSlotPending, restoreLikeSlot, setLikeSlot, type Status } from "../api/status";

export type LikeSlotControl = {
  /** What fills the slot now, as the cache has it. */
  slot: LikeSlot;
  /** Fills the slot with `slot`: favourites, reacts with an emoji, or takes back what is there. */
  set: (slot: LikeSlot) => void;
};

/** What is announced when the slot changes from `previous` to `slot`. */
function slotMessage(t: Translate, slot: LikeSlot, previous: LikeSlot): string {
  switch (slot.kind) {
    case "favourite":
      return t("client:status.favourite.on");
    case "reaction":
      return t("client:status.reaction.on", { emoji: emojiText(slot) });
    case "none":
      return previous.kind === "favourite" ? t("client:status.favourite.off") : t("client:status.reaction.off");
  }
}

/** The emoji of a reaction as a sentence says it: the Unicode emoji, or the `:shortcode:` of a custom emoji. */
function emojiText(slot: Extract<LikeSlot, { kind: "reaction" }>): string {
  return slot.emoji.shortcode === undefined ? slot.emoji.name : `:${slot.emoji.shortcode}:`;
}

/**
 * The Like slot of `post`. Read `post` from the query cache on every render; the hook never copies it (S2). A
 * refused change puts the slot back as it was and says why in a toast (E2); each result is announced (Y7).
 */
export function useLikeSlot(post: Status): LikeSlotControl {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const announce = useAnnouncer();
  const toast = useToast();
  const mutationKey = ["status", post.id, "like-slot"] as const;
  const { mutate } = useMutation({
    mutationKey,
    // One request at a time per post, in the order pressed (TanStack Query, "Mutation Scopes").
    scope: { id: `status:${post.id}:like-slot` },
    mutationFn: ({ slot, previous }: { slot: LikeSlot; previous: LikeSlot }) =>
      setLikeSlot(api, post.id, slot, previous),
    onMutate: ({ slot }) => cacheLikeSlotPending(api, queryClient, post.id, slot),
    onSuccess: (answer, { slot, previous }) => {
      // The last press is the only one still running.
      const last = queryClient.isMutating({ mutationKey, exact: true }) <= 1;
      cacheLikeSlotAnswer(queryClient, post.id, answer, last);
      announce(slotMessage(t, slot, previous));
    },
    onError: (error, { previous }) => {
      restoreLikeSlot(queryClient, post.id, previous);
      toast.show({ message: t("client:status.action.failed", { reason: errorMessage(error) }), tone: "danger" });
    },
  });
  const slot = likeSlotOf(post);
  return {
    slot,
    set: (next) => {
      mutate({ slot: next, previous: slot });
    },
  };
}
