// SPDX-License-Identifier: AGPL-3.0-only

// Who reacted to a post (FR-116), on the post's own page under the focused post: a row per emoji, in the order of its
// reactions; each row its emoji and count, the faces of the first accounts the server answered, and a chevron, the
// whole row a link to that emoji's list. `ReactorsTitle` heads that list, on a screen of its own: the accounts that
// reacted with one emoji, a row each.

import { Avatar, CustomEmoji, FacesRow, ListTitle, useLocale } from "@my/design-system";
import type { ReactNode } from "react";
import { nameOf } from "./account";
import { format } from "./format";
import { stringsIn } from "./locale";
import { reactorsPath } from "./paths";
import type { PostAccount, PostReaction, PostStatus } from "./status";

/** A reaction to a post with one emoji, and the first accounts that reacted with it. */
export type Reactors = { reaction: PostReaction; accounts: readonly PostAccount[] };

/** A reaction's emoji: a custom one as its image, a Unicode one as text. */
const emojiOf = (r: PostReaction): ReactNode =>
  r.emoji === undefined ? (
    r.name
  ) : (
    <CustomEmoji shortcode={r.emoji.shortcode} url={r.emoji.url} staticUrl={r.emoji.static_url} />
  );

/** A reaction's emoji as words: a Unicode emoji, or a custom one's `:shortcode:`. */
const wordOf = (r: PostReaction) => (r.emoji === undefined ? r.name : `:${r.emoji.shortcode}:`);

/** The rows of who reacted to `status`, one per emoji, with the first accounts of each that the server answered. */
export function WhoReacted({ status, reactors }: { status: PostStatus; reactors: readonly Reactors[] }) {
  const { t, lang } = stringsIn(useLocale().locale);
  const number = new Intl.NumberFormat(lang);
  return (
    <div lang={lang}>
      {reactors.map(({ reaction, accounts }) => {
        const { count } = reaction;
        if (count === 0) return null;
        return (
          <FacesRow
            key={reaction.name}
            href={reactorsPath(status, reaction.name)}
            label={t("post.reacted_row", { count, emoji: wordOf(reaction) })}
            glyph={emojiOf(reaction)}
            count={number.format(count)}
            faces={accounts.map((a) => (
              <Avatar key={a.id} name={nameOf(a)} src={a.avatar?.small.url} size="post" decorative />
            ))}
          />
        );
      })}
    </div>
  );
}

/** The title of one emoji's list: its words with the emoji drawn in them, and how many reacted with it. */
export function ReactorsTitle({ reaction, count }: { reaction: PostReaction; count: number }) {
  const { t, lang } = stringsIn(useLocale().locale);
  const shown = new Intl.NumberFormat(lang).format(count);
  return <ListTitle count={shown}>{format(t, "post.reacted_with", { emoji: emojiOf(reaction) })}</ListTitle>;
}
