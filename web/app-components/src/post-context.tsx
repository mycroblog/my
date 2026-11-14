// SPDX-License-Identifier: AGPL-3.0-only

// A post as the context of what the reader writes (the composer's reply or quote, #3046): the post replied to, its
// author on one line over the start of its text, muted; or the quoted post as the new post will carry it, a reduced
// preview on the raised surface, with its remove. Both above the text. The text is drawn by the one content renderer.

import { Avatar, Byline, ComposerQuote, ComposerReply, Embed, MediaGrid, Stamp } from "@my/design-system";
import { EmojiText, PostContent } from "./post-content";
import type { PostAccount, PostStatus } from "./status";

const handleOf = (account: PostAccount) =>
  account.username === "" ? account.domain : `@${account.username}@${account.domain}`;
const nameOf = (account: PostAccount) => account.display_name || account.username || account.domain;
const known = (tag: string) => (tag === "und" || tag === "" ? undefined : tag);

function AuthorLine({ status, time }: { status: PostStatus; time: boolean }) {
  return (
    <Byline
      compact
      avatar={<Avatar name={nameOf(status.account)} src={status.account.avatar?.small.url} size="inline" decorative />}
      name={<EmojiText text={nameOf(status.account)} emojis={status.account.emojis} />}
      handle={handleOf(status.account)}
      {...(time ? { end: <Stamp date={status.created_at} /> } : {})}
    />
  );
}

/** The post replied to, above the text; `note` says to assistive technology whom the reply is to. */
export function ReplyContext({ status, note }: { status: PostStatus; note: string }) {
  const shown = status.boost ?? status;
  return (
    <ComposerReply byline={<AuthorLine status={shown} time={false} />} note={note}>
      <PostContent html={shown.content} emojis={shown.emojis} lang={known(shown.language)} />
    </ComposerReply>
  );
}

/** The quoted post, above the text, as the post will carry it, with its remove. */
export function QuoteContext({
  status,
  removeLabel,
  onRemove,
}: {
  status: PostStatus;
  removeLabel: string;
  onRemove: () => void;
}) {
  const shown = status.boost ?? status;
  const media = shown.media_attachments.flatMap((m) => {
    const src = m.preview_url ?? m.url;
    return src === undefined ? [] : [{ id: m.id, src, alt: m.description ?? "" }];
  });
  return (
    <ComposerQuote removeLabel={removeLabel} onRemove={onRemove}>
      <Embed lang={known(shown.language)}>
        <AuthorLine status={shown} time />
        <PostContent html={shown.content} emojis={shown.emojis} lang={known(shown.language)} />
        <MediaGrid items={media} size="preview" />
      </Embed>
    </ComposerQuote>
  );
}

/** Whether the account is on another server: its acct names a domain. */
export const isRemote = (status: PostStatus) => (status.boost ?? status).account.acct.includes("@");
