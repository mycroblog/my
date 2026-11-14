// SPDX-License-Identifier: AGPL-3.0-only

// A post of a timeline (#2981's anatomy), drawn from a status as the API gives it (status.ts) with the parts of
// @my/design-system and the one content renderer: the context line (who boosted it, or whom it replies to), the author
// header (the avatar, the name with the visibility and the time at the end, over the handle), the text, or a content
// warning over it, the images, the quoted post as a reduced preview, the emoji reactions, the translate line, and the
// actions (reply, boost with its menu of boost and quote, favourite, react, and more at the end), whose keys
// (post.reply, post.boost, post.favourite, post.more) act while focus is inside it. A screen may add a line of its own
// over it (`context`: pinned) and lines under the reactions (`details`: a focused post's date and counts), inside its
// surface. Its author's name leads to their profile and its time to the post's own page (paths.ts). It shows its
// extent by a surface only while hovered or focused (Entry). It fetches nothing and keeps no state of the server's: each
// action is a callback, and what the action changes comes back in the next status.

import {
  ActionRow,
  Avatar,
  Byline,
  CaptionLine,
  CaptionName,
  Chip,
  ChipRow,
  CustomEmoji,
  Disclosure,
  Embed,
  Entry,
  IconButton,
  MediaGrid,
  MenuButton,
  MenuGroup,
  MenuItem,
  Stack,
  Stamp,
  TextButton,
  useAction,
  useLocale,
  type IconName,
  type MediaItem,
} from "@my/design-system";
import { Fragment, useId, useState, type ReactNode } from "react";
import { AccountName as Name, handleOf, nameOf } from "./account";
import { stringsIn } from "./locale";
import type { MessageId } from "./messages.gen";
import { accountPath, statusPath } from "./paths";
import { EmojiText, PostContent } from "./post-content";
import type { PostMedia, PostReaction, PostStatus, PostTranslation, PostVisibility } from "./status";

/** What the reader asked of a post, on the status it shows (a boost's boosted status). */
export type PostAction =
  | {
      type:
        | "reply"
        | "boost"
        | "quote"
        | "favourite"
        | "react"
        | "translate"
        | "original"
        | "copy-link"
        | "open-original"
        | "share"
        | "bookmark"
        | "edit"
        | "delete"
        | "mute"
        | "block"
        | "report";
      id: string;
    }
  | { type: "reaction"; id: string; name: string };

export type PostProps = {
  /** The status, as the API gives it; a boost shows the status it boosts. */
  status: PostStatus;
  /** The reader wrote it: its more menu edits and deletes instead of acting on its author. */
  own?: boolean | undefined;
  /** The post in the reader's language, shown in place of its text while given. */
  translation?: PostTranslation | undefined;
  /** The post may be translated into the reader's language: it offers to. */
  translatable?: boolean | undefined;
  /** What the reader asked of it. */
  onAction?: ((action: PostAction) => void) | undefined;
  /** A line of the screen's own over the post, inside it (that it is pinned): a `CaptionLine`. */
  context?: ReactNode;
  /** The screen's own lines under the body, before the actions (a focused post's date and counts). */
  details?: ReactNode;
  /** The visibility and the time at the end of the author's line; false where `details` says them in full. */
  stamp?: boolean | undefined;
};

type T = ReturnType<typeof stringsIn>["t"];

// The first of the characters no text holds (the private use area), each standing for a part drawn by React inside a
// formatted string.
const firstSlot = 0xe000;
const slotPattern = /([-])/u;

/**
 * The string `id` with each of `parts` drawn by React in its argument's place, so that a name keeps its custom emoji
 * and its isolation inside a sentence, and no transform of the message (a pseudo-locale's) reaches it.
 */
function format(t: T, id: MessageId, parts: Readonly<Record<string, ReactNode>>): ReactNode {
  const names = Object.keys(parts);
  const text = t(id, Object.fromEntries(names.map((name, i) => [name, String.fromCodePoint(firstSlot + i)])));
  return text.split(slotPattern).map((piece, i) => {
    if (i % 2 === 0) return piece;
    const name = names[(piece.codePointAt(0) ?? firstSlot) - firstSlot] ?? "";
    return <Fragment key={i}>{parts[name]}</Fragment>;
  });
}

// Each visibility's glyph, and the name it says for assistive technology.
export const visibilities: Record<PostVisibility, { icon: IconName; label: MessageId }> = {
  public: { icon: "public", label: "post.visibility.public" },
  local: { icon: "local", label: "post.visibility.local" },
  unlisted: { icon: "unlisted", label: "post.visibility.unlisted" },
  private: { icon: "followers", label: "post.visibility.private" },
  direct: { icon: "direct", label: "post.visibility.direct" },
};

/** A language tag that is known, for a lang attribute. */
const known = (tag: string) => (tag === "und" || tag === "" ? undefined : tag);

/** The context line: who boosted it, or whom it replies to; none for a post that is neither. */
function Context({ status, shown, t, lang }: { status: PostStatus; shown: PostStatus; t: T; lang: string }) {
  if (status.boost !== undefined)
    return (
      <CaptionLine icon="boost" lang={lang} oneLine>
        {format(t, "post.boosted_by", {
          name: (
            <CaptionName>
              <Name account={status.account} />
            </CaptionName>
          ),
        })}
      </CaptionLine>
    );
  const to = shown.in_reply_to_account_id;
  if (to === undefined) return null;
  const username = to === shown.account.id ? shown.account.username : shown.mentions.find((m) => m.id === to)?.username;
  if (username === undefined) return null;
  return (
    <CaptionLine icon="reply" lang={lang}>
      {format(t, "post.replying_to", { handle: <bdi dir="ltr">@{username}</bdi> })}
    </CaptionLine>
  );
}

/** The images: each one's preview rendition, with its description (translated, while the post is) as its text. */
function mediaItems(media: readonly PostMedia[], translation: PostTranslation | undefined): MediaItem[] {
  return media.flatMap((m) => {
    const src = m.preview_url ?? m.url;
    if (src === undefined) return [];
    const alt = translation?.media_attachments.find((a) => a.id === m.id)?.description ?? m.description ?? "";
    return [{ id: m.id, src, alt }];
  });
}

/** The quoted post, one level deep: its author on one line, its text, and its images as small previews. */
function Quote({ status }: { status: PostStatus }) {
  const name = useId();
  return (
    <Embed lang={known(status.language)}>
      <Byline
        compact
        avatar={
          <Avatar name={nameOf(status.account)} src={status.account.avatar?.small.url} size="inline" decorative />
        }
        name={<Name account={status.account} />}
        nameId={name}
        handle={handleOf(status.account)}
        end={<Stamp date={status.created_at} />}
      />
      <PostContent html={status.content} emojis={status.emojis} lang={known(status.language)} />
      <MediaGrid items={mediaItems(status.media_attachments, undefined)} size="preview" />
    </Embed>
  );
}

/** A reaction's emoji: a custom one as its image, a Unicode one as text. */
const glyphOf = (r: PostReaction) =>
  r.emoji === undefined ? (
    r.name
  ) : (
    <CustomEmoji shortcode={r.emoji.shortcode} url={r.emoji.url} staticUrl={r.emoji.static_url} />
  );

/** The emoji reactions (FR-116): a chip per emoji with its count, the reader's own knocked out. */
function Reactions({ status, t, lang, act }: { status: PostStatus; t: T; lang: string; act: Act }) {
  if (status.reactions.length === 0) return null;
  return (
    <ChipRow label={t("post.reactions")} lang={lang}>
      {status.reactions.map((r) => {
        const emoji = r.emoji === undefined ? r.name : `:${r.emoji.shortcode}:`;
        const mine = r.me ?? false;
        return (
          <Chip
            key={r.name}
            glyph={glyphOf(r)}
            count={r.count}
            pressed={mine}
            label={t(mine ? "post.reaction_mine" : "post.reaction", { emoji, count: r.count })}
            onPress={() => {
              act({ type: "reaction", id: status.id, name: r.name });
            }}
          />
        );
      })}
    </ChipRow>
  );
}

/** The note of a translation: the language it was written in and the provider, as far as they are known. */
function translatedNote(translation: PostTranslation, t: T, lang: string): ReactNode {
  const source = known(translation.source_language);
  const name = source === undefined ? undefined : new Intl.DisplayNames([lang], { type: "language" }).of(source);
  // A provider's name is a proper name, and a language's is the platform's: both isolated, in their own direction.
  const language = name === undefined ? undefined : <bdi>{name}</bdi>;
  const provider = translation.provider === undefined ? undefined : <bdi>{translation.provider.name}</bdi>;
  if (language !== undefined && provider !== undefined)
    return format(t, "post.translated_from_by", { language, provider });
  if (provider !== undefined) return format(t, "post.translated_by", { provider });
  if (language !== undefined) return format(t, "post.translated_from", { language });
  return t("post.translated");
}

type Act = (action: PostAction) => void;

/** The more menu: what reaches the post, the bookmark, then edit and delete for one's own, or the author's account. */
function MoreItems({ status, own, t, act }: { status: PostStatus; own: boolean; t: T; act: Act }) {
  const item = (type: Exclude<PostAction["type"], "reaction">, icon: IconName, label: string, on = false) => (
    <MenuItem
      id={type}
      icon={icon}
      on={on}
      onAction={() => {
        act({ type, id: status.id });
      }}
    >
      {label}
    </MenuItem>
  );
  const bookmarked = status.bookmarked ?? false;
  return (
    <>
      <MenuGroup>
        {item("copy-link", "link", t("post.copy_link"))}
        {status.account.acct.includes("@") && item("open-original", "open", t("post.open_original"))}
        {item("share", "share", t("post.share"))}
      </MenuGroup>
      <MenuGroup>
        {item("bookmark", "bookmark", t(bookmarked ? "post.unbookmark" : "post.bookmark"), bookmarked)}
      </MenuGroup>
      {own ? (
        <>
          <MenuGroup>{item("edit", "edit", t("post.edit"))}</MenuGroup>
          <MenuGroup>{item("delete", "delete", t("post.delete"))}</MenuGroup>
        </>
      ) : (
        <MenuGroup title={`@${status.account.username || status.account.domain}`}>
          {item("mute", "mute", t("post.mute"))}
          {item("block", "block", t("post.block"))}
          {item("report", "report", t("post.report"))}
        </MenuGroup>
      )}
    </>
  );
}

/** The actions: reply, boost (a menu of boost and quote, with its count and on state), favourite and react, more. */
function Actions({ status, own, t, lang, act }: { status: PostStatus; own: boolean; t: T; lang: string; act: Act }) {
  const on = (type: "reply" | "boost" | "quote" | "favourite" | "react") => () => {
    act({ type, id: status.id });
  };
  const boosted = status.boosted ?? false;
  const [moreOpen, setMoreOpen] = useState(false);
  // The keys of the focused post: reply, boost (or take it back) and favourite at once, and more opens its menu.
  // The help lists them once for every post, under the post's group.
  const keyed = (label: string) => ({ scope: "focus" as const, label, groupLabel: t("post.shortcuts") });
  const reply = useAction("post.reply", on("reply"), keyed(t("post.reply")));
  const boost = useAction("post.boost", on("boost"), keyed(t("post.boost")));
  const favourite = useAction("post.favourite", on("favourite"), keyed(t("post.favourite")));
  const more = useAction(
    "post.more",
    () => {
      setMoreOpen(true);
    },
    keyed(t("post.more_shortcut")),
  );
  return (
    <ActionRow
      lang={lang}
      end={
        <MenuButton icon="more" label={t("post.more")} open={moreOpen} onOpenChange={setMoreOpen} {...more}>
          <MoreItems status={status} own={own} t={t} act={act} />
        </MenuButton>
      }
    >
      <IconButton icon="reply" label={t("post.reply")} count={status.replies_count} onPress={on("reply")} {...reply} />
      <MenuButton
        icon="boost"
        label={t("post.boost")}
        count={status.boosts_count}
        pressed={boosted}
        align="start"
        {...boost}
      >
        <MenuGroup>
          <MenuItem id="boost" icon="boost" on={boosted} onAction={on("boost")}>
            {t(boosted ? "post.unboost" : "post.boost")}
          </MenuItem>
          <MenuItem id="quote" icon="quote" onAction={on("quote")}>
            {t("post.quote")}
          </MenuItem>
        </MenuGroup>
      </MenuButton>
      <IconButton
        icon="favourite"
        label={t("post.favourite")}
        count={status.favourites_count}
        pressed={status.favourited ?? false}
        onPress={on("favourite")}
        {...favourite}
      />
      <IconButton icon="react" label={t("post.react")} onPress={on("react")} />
    </ActionRow>
  );
}

export function Post({
  status,
  own = false,
  translation,
  translatable = false,
  onAction,
  context,
  details,
  stamp = true,
}: PostProps) {
  const { t, lang } = stringsIn(useLocale().locale);
  const name = useId();
  const shown = status.boost ?? status;
  const act: Act = (action) => {
    onAction?.(action);
  };
  const textLang = known(translation?.language ?? shown.language);
  const cw = translation?.spoiler_text || shown.spoiler_text;
  const quoted = shown.quote?.quoted_status;
  const on = (type: "translate" | "original") => () => {
    act({ type, id: shown.id });
  };
  const content = (
    <>
      <Stack gap="line">
        <PostContent html={translation?.content ?? shown.content} emojis={shown.emojis} lang={textLang} />
        {translation !== undefined ? (
          <CaptionLine
            icon="translate"
            lang={lang}
            action={<TextButton onPress={on("original")}>{t("post.original")}</TextButton>}
          >
            {translatedNote(translation, t, lang)}
          </CaptionLine>
        ) : (
          translatable && (
            <CaptionLine
              icon="translate"
              lang={lang}
              action={<TextButton onPress={on("translate")}>{t("post.translate")}</TextButton>}
            />
          )
        )}
      </Stack>
      <MediaGrid items={mediaItems(shown.media_attachments, translation)} />
      {quoted !== undefined && <Quote status={quoted} />}
    </>
  );
  return (
    <Entry lang={known(shown.language)} labelledBy={name}>
      {context}
      <Context status={status} shown={shown} t={t} lang={lang} />
      <Byline
        avatar={<Avatar name={nameOf(shown.account)} src={shown.account.avatar?.small.url} size="post" decorative />}
        name={<Name account={shown.account} />}
        nameId={name}
        handle={handleOf(shown.account)}
        href={accountPath(shown.account)}
        end={
          stamp ? (
            <Stamp
              glyph={{ icon: visibilities[shown.visibility].icon, label: t(visibilities[shown.visibility].label) }}
              date={shown.created_at}
              href={statusPath(shown)}
            />
          ) : undefined
        }
      />
      {cw ? (
        <Disclosure summary={<EmojiText text={cw} emojis={shown.emojis} />} icon="hidden">
          <Stack lang={textLang}>{content}</Stack>
        </Disclosure>
      ) : (
        content
      )}
      <Reactions status={shown} t={t} lang={lang} act={act} />
      {details}
      <Actions status={shown} own={own} t={t} lang={lang} act={act} />
    </Entry>
  );
}
