// SPDX-License-Identifier: AGPL-3.0-only
// The emoji reactions to a post (FR-116, ADR-0052 §5): the counts per emoji under the post, which the viewer joins
// or leaves by pressing one, and the accounts that reacted.
import { type ImageSources, useAnnouncer } from "@my/ui";
import { useInfiniteQuery } from "@tanstack/react-query";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { handleOf } from "../../../lib/handle";
import { imageSources } from "../../../lib/images";
import { useT } from "../../../lib/i18n";
import { profilePath } from "../../../lib/paths";
import { useViewer } from "../../session";
import { reactorsQueryOptions, type ReactionEmoji, type Status } from "../api/status";
import { useLikeSlot } from "./useLikeSlot";

/** The reactions to a post with one emoji, as the row under the post shows them. */
export type ReactionChip = {
  /** The emoji's name (ReactionEmoji.name), unique within the post. */
  key: string;
  emoji: ReactionEmoji;
  /** The emoji in words: the Unicode emoji, `:shortcode:`, or `:shortcode@domain:` for another server's. */
  text: string;
  count: number;
  /** Whether the viewer reacted with it. */
  isSelected: boolean;
  /**
   * Joins (true) or leaves (false) the reaction, in place of the viewer's favourite or other reaction; null when the
   * viewer cannot: while nobody is signed in, and for a custom emoji of another server, which is shown only (ADR-0052 §2).
   */
  onChange: ((on: boolean) => void) | null;
};

/** The emoji of a reaction in words (ReactionChip.text). */
function reactionText(emoji: ReactionEmoji): string {
  if (emoji.shortcode === undefined) return emoji.name;
  return emoji.domain === undefined ? `:${emoji.shortcode}:` : `:${emoji.shortcode}@${emoji.domain}:`;
}

/**
 * The reactions to `status` (for a boost, to the boosted post), most used first as the server counts them. Read
 * `status` from the query cache on every render; the hook never copies it (S2).
 */
export function useStatusReactions(status: Status): ReactionChip[] {
  const post = status.boost ?? status;
  const viewer = useViewer();
  const likeSlot = useLikeSlot(post);
  const signedIn = viewer.status === "signed-in";
  return post.reactions.map((reaction) => {
    const { count, me, ...emoji } = reaction;
    return {
      key: reaction.name,
      emoji,
      text: reactionText(emoji),
      count,
      isSelected: me === true,
      onChange:
        !signedIn || emoji.domain !== undefined
          ? null
          : (on) => {
              likeSlot.set(on ? { kind: "reaction", emoji } : { kind: "none" });
            },
    };
  });
}

/** One account that reacted, as the list of reactions shows it. */
type Reactor = {
  key: string;
  id: string;
  name: string;
  emojis: Status["account"]["emojis"];
  handle: string;
  href: string;
  avatar: ImageSources | undefined;
  emoji: ReactionEmoji;
  emojiText: string;
};

export type ReactorsState =
  | { status: "loading" }
  | { status: "error"; error: string; retry: () => void }
  | {
      status: "loaded";
      items: readonly Reactor[];
      hasMore: boolean;
      isLoadingMore: boolean;
      loadMore: () => void;
      /** Why the last page failed to load (localized), or null. */
      loadMoreError: string | null;
    };

/**
 * The accounts that reacted to post `id`, newest first, read while `enabled` (the list is open). The next pages load
 * on demand; a failure of one is announced and kept with the accounts already shown (web-client/README.md E2).
 */
export function useReactors(id: string, enabled: boolean): ReactorsState {
  const api = useApi();
  const t = useT();
  const announce = useAnnouncer();
  const query = useInfiniteQuery({ ...reactorsQueryOptions(api, id), enabled, retry: false });
  // A failed next page leaves the query in error with the pages already loaded, which stay shown.
  if (query.data === undefined) {
    if (!query.isError) return { status: "loading" };
    return {
      status: "error",
      error: errorMessage(query.error),
      retry: () => {
        void query.refetch();
      },
    };
  }
  return {
    status: "loaded",
    items: query.data.pages.flatMap((page) =>
      page.items.map((item) => ({
        key: `${item.account.id}:${item.emoji.name}`,
        id: item.account.id,
        name: item.account.display_name || item.account.username,
        emojis: item.account.emojis,
        handle: handleOf(item.account.acct),
        href: profilePath(item.account.acct),
        avatar: imageSources(item.account.avatar),
        emoji: item.emoji,
        emojiText: reactionText(item.emoji),
      })),
    ),
    hasMore: query.hasNextPage,
    isLoadingMore: query.isFetchingNextPage,
    loadMore: () => {
      if (!query.hasNextPage || query.isFetchingNextPage) return;
      void query.fetchNextPage().then((result) => {
        if (result.isFetchNextPageError)
          announce(t("client:reaction.list.load_more_failed", { reason: errorMessage(result.error) }), "assertive");
      });
    },
    loadMoreError: query.isFetchNextPageError ? errorMessage(query.error) : null,
  };
}
