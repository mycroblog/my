// SPDX-License-Identifier: AGPL-3.0-only

// A post of a timeline (#2981's anatomy), drawn from a status as the API gives it (status.ts) with the parts of
// @my/design-system and the one content renderer: the context line (who boosted it, or whom it replies to), the author
// header (the avatar, the name with the visibility and the time at the end, and "edited" after the time of an edited
// post, which opens its edit history, over the handle), the text, or a content warning over it, the poll (open to the
// reader's vote, else its results), the media (covered while sensitive; a picture opens the media viewer, which the
// post holds open), or else the card of its first link, the quoted post as a reduced preview, the emoji reactions, the
// translate line, and the actions (reply, boost with its menu of boost and quote, favourite, react, and more at the
// end, whose menu holds edit, delete and redraft on one's own post), whose keys
// (post.reply, post.boost, post.favourite, post.more) act while focus is inside it. A screen may add a line of its own
// over it (`context`: pinned, or the heading of a thread's replies), lines under the reactions (`details`: a focused
// post's date and counts), and a line after the actions (`after`: a thread's fold), inside its surface. Its author's name leads to their profile and its time to the post's own page (paths.ts). It shows its
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
  EmojiPicker,
  EmojiPickerPopover,
  EmojiPickerSheet,
  Entry,
  IconButton,
  LinkCard,
  LivePart,
  MediaGrid,
  MediaViewer,
  MenuButton,
  MenuGroup,
  MenuItem,
  ModalPresence,
  PollBox,
  PollChoices,
  PollFoot,
  PollResults,
  preloadEmojiPicker,
  Stack,
  Stamp,
  TextButton,
  useAction,
  useLocale,
  type EmojiPickerProps,
  type IconName,
  type MediaItem,
} from "@my/design-system";
import { Component, Fragment, useId, useState, type ReactNode } from "react";
import { AccountName as Name, handleOf, nameOf } from "./account";
import { languageName, stringsIn } from "./locale";
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
        | "redraft"
        | "history";
      id: string;
    }
  | { type: "reaction"; id: string; name: string }
  | { type: "vote"; id: string; poll: string; choices: number[] };

export type PostProps = {
  /** The status, as the API gives it; a boost shows the status it boosts. */
  status: PostStatus;
  /** The reader wrote it: its more menu edits and deletes instead of acting on its author. */
  own?: boolean | undefined;
  /** The post in the reader's language, shown in place of its text while given. */
  translation?: PostTranslation | undefined;
  /** The post may be translated into the reader's language: it offers to. */
  translatable?: boolean | undefined;
  /**
   * Its translation is on its way: true, or the share (0 to 1) of the model the browser's translator is downloading
   * before it translates.
   */
  translating?: boolean | number | undefined;
  /** Why the last translation asked for failed, and where asking again goes, when it may succeed. */
  translationFailure?: TranslationFailure | undefined;
  /** What the reader asked of it. */
  onAction?: ((action: PostAction) => void) | undefined;
  /**
   * A reader without an account: the actions that need one (reply, boost, quote, favourite, react, bookmark, and
   * acting on the author) lead to the sign-in with the post they were pressed on, in place of `onAction`; the others
   * (translating, the post's link, sharing) stay `onAction`'s. The actions show as for anyone.
   */
  signIn?: ((status: PostStatus) => void) | undefined;
  /** A line of the screen's own over the post, inside it (that it is pinned): a `CaptionLine`. */
  context?: ReactNode;
  /** The screen's own lines under the body, before the actions (a focused post's date and counts). */
  details?: ReactNode;
  /** The visibility and the time at the end of the author's line; false where `details` says them in full. */
  stamp?: boolean | undefined;
  /** The screen's own line after the actions, inside it (a fold of the posts that follow it in a thread). */
  after?: ReactNode;
  /** The choices of the reader's vote in its poll while the vote is in flight. */
  voting?: readonly number[] | undefined;
  /** The emoji picker the react action opens; without one, react is `onAction`'s. */
  picker?: PostPicker | undefined;
  /** How its link card shows (the reader's preference): the image above the text, or a square beside it. */
  cardSize?: "large" | "small" | undefined;
};

/**
 * The emoji picker as the app gives it: the emoji (the reader's frequent ones, the server's, Unicode's), their search,
 * the strings, and where it opens, which the app decides by the viewport: a sheet on a phone, titled with `title` and
 * closed by `closeLabel`'s button, else a popover beside the react button.
 */
export type PostPicker = Omit<EmojiPickerProps, "onSelect" | "current" | "fill" | "defaultQuery" | "autoFocus"> & {
  sheet: boolean;
  title: string;
  closeLabel: string;
  /** Asked for when the react button is hovered, focused or pressed: the emoji are loaded before the picker opens. */
  preload?: (() => void) | undefined;
  /** Told when the picker opens. */
  onOpen?: (() => void) | undefined;
};

/**
 * Why a translation failed: over the reader's or the server's monthly limit, a visibility the server's provider may
 * not receive, a language it does not translate, the provider down, or the browser's translator unable to.
 */
export type TranslationFailure = {
  reason: "limit" | "visibility" | "language" | "failed" | "browser";
  /**
   * Where another press goes, when it may succeed: to the browser's translator after the server refused
   * (`browser`), or to the server again, whose provider may answer (`server`); false when nothing may.
   */
  retry: "browser" | "server" | false;
};

const failures: Record<TranslationFailure["reason"], MessageId> = {
  limit: "post.translation_limit",
  visibility: "post.translation_visibility",
  language: "post.translation_language",
  failed: "post.translation_failed",
  browser: "post.translation_browser",
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
export function format(
  t: T,
  id: MessageId,
  parts: Readonly<Record<string, ReactNode>>,
  held: readonly string[] = [],
): ReactNode {
  const names = Object.keys(parts);
  const text = t(id, Object.fromEntries(names.map((name, i) => [name, String.fromCodePoint(firstSlot + i)])));
  const pieces = text.split(slotPattern);
  const nameAt = (slot: string | undefined) =>
    slot === undefined ? undefined : names[(slot.codePointAt(0) ?? firstSlot) - firstSlot];
  return pieces.map((piece, i) => {
    // The words beside a held part keep to it ("by LibreTranslate", "LibreTranslate が"): the spaces do not break.
    if (i % 2 === 0) {
      const before = held.includes(nameAt(pieces[i + 1]) ?? "") ? piece.replace(/ $/u, " ") : piece;
      return held.includes(nameAt(pieces[i - 1]) ?? "") ? before.replace(/^ /u, " ") : before;
    }
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

/**
 * The media: each one's preview rendition (a video's or a sound's poster), the file itself, its kind and its length,
 * with its description (translated, while the post is) as its text. An image without either rendition is left out.
 */
function mediaItems(media: readonly PostMedia[], translation: PostTranslation | undefined): MediaItem[] {
  return media.flatMap((m) => {
    const src = m.preview_url ?? (m.type === "image" ? m.url : undefined);
    if (src === undefined && m.type === "image") return [];
    const alt = translation?.media_attachments.find((a) => a.id === m.id)?.description ?? m.description ?? "";
    return [
      { id: m.id, src: src ?? "", alt, kind: m.type, href: m.url, duration: m.meta?.duration, blurhash: m.blurhash },
    ];
  });
}

const isPicture = (m: MediaItem) => m.kind === "image" || m.kind === "gifv" || m.kind === "video";

/** A part of a post that may fail on its own: an error while it draws leaves nothing in its place. */
class Contained extends Component<{ children: ReactNode }, { failed: boolean }> {
  override state = { failed: false };
  static getDerivedStateFromError() {
    return { failed: true };
  }
  override render() {
    return this.state.failed ? null : this.props.children;
  }
}

/**
 * The link card, when the post has neither media nor a quote (ADR-0062 §1); its author on an inline avatar. A card
 * that fails to draw (a page's data the card cannot take) leaves the post without it, never breaks the post (§8).
 */
function Card({ status, size }: { status: PostStatus; size: "large" | "small" }) {
  const card = status.card;
  if (card === undefined || status.media_attachments.length > 0 || status.quote?.quoted_status !== undefined)
    return null;
  const author = card.author;
  const image = card.image;
  return (
    <LinkCard
      href={card.url}
      title={card.title}
      description={card.description}
      site={card.site_name}
      size={size}
      sensitive={status.sensitive ?? false}
      lang={card.language}
      image={
        image === undefined
          ? undefined
          : {
              src: image.url,
              width: image.width,
              height: image.height,
              alt: image.description,
              blurhash: image.blurhash,
            }
      }
      author={
        author === undefined
          ? undefined
          : {
              name: nameOf(author),
              shown: (
                <>
                  <Avatar name={nameOf(author)} src={author.avatar?.small.url} size="inline" decorative />
                  <bdi>
                    <Name account={author} />
                  </bdi>
                </>
              ),
            }
      }
    />
  );
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
      <MediaGrid
        items={mediaItems(status.media_attachments, undefined)}
        size="preview"
        sensitive={status.sensitive ?? false}
      />
    </Embed>
  );
}

/** How long until `end` as the platform says it in `lang`, narrow, in whole units: "in 2d", "2日後". */
function timeUntil(end: Date, now: Date, lang: string): string {
  const minutes = Math.max(1, Math.floor((end.getTime() - now.getTime()) / 60_000));
  const rtf = new Intl.RelativeTimeFormat(lang, { numeric: "always", style: "narrow" });
  if (minutes < 60) return rtf.format(minutes, "minute");
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return rtf.format(hours, "hour");
  return rtf.format(Math.floor(hours / 24), "day");
}

/**
 * The poll (FR-112): open to the reader, its options to choose and Vote; voted, ended or the reader's own, its results,
 * each option's share unless its author hides them until it ends. Its line says who voted and when it ends.
 */
function Poll({
  status,
  translation,
  own,
  voting,
  t,
  lang,
  act,
}: {
  status: PostStatus;
  translation: PostTranslation | undefined;
  own: boolean;
  voting: readonly number[] | undefined;
  t: T;
  lang: string;
  act: Act;
}) {
  const [chosen, setChosen] = useState<number[]>([]);
  const poll = status.poll;
  if (poll === undefined) return null;
  const title = (i: number, text: string) => (
    <EmojiText text={translation?.poll_options?.[i] ?? text} emojis={status.emojis} />
  );
  const answered = poll.expired || (poll.voted ?? false) || own;
  const people =
    poll.voters_count === undefined
      ? t("post.poll_votes", { count: poll.votes_count })
      : t("post.poll_voters", { count: poll.voters_count });
  const hidden = poll.options.some((o) => o.votes_count === undefined);
  const when = poll.expired
    ? t("post.poll_ended")
    : poll.expires_at === undefined
      ? undefined
      : t("post.poll_ends", { time: timeUntil(new Date(poll.expires_at), new Date(), lang) });
  const line = (parts: (string | undefined)[]) => parts.filter((p) => p !== undefined);
  if (!answered) {
    const picked = voting ?? chosen;
    return (
      <PollBox label={t("post.poll")}>
        <PollChoices
          label={t("post.poll_options")}
          multiple={poll.multiple}
          items={poll.options.map((o, i) => ({ key: String(i), label: title(i, o.title) }))}
          chosen={picked}
          onChange={setChosen}
          disabled={voting !== undefined}
        />
        <PollFoot
          vote={{
            label: t("post.poll_vote"),
            disabled: picked.length === 0,
            busy: voting !== undefined,
            onPress: () => {
              act({ type: "vote", id: status.id, poll: poll.id, choices: chosen });
            },
          }}
          parts={line([poll.multiple ? t("post.poll_several") : undefined, people, when])}
        />
      </PollBox>
    );
  }
  const total = poll.multiple ? (poll.voters_count ?? poll.votes_count) : poll.votes_count;
  const percent = new Intl.NumberFormat(lang, { style: "percent", maximumFractionDigits: 0 });
  const most = Math.max(...poll.options.map((o) => o.votes_count ?? 0));
  const mine = poll.own_votes ?? [];
  return (
    <PollBox label={t("post.poll")}>
      <PollResults
        multiple={poll.multiple}
        mineLabel={t("post.poll_mine")}
        items={poll.options.map((o, i) => {
          const value = o.votes_count === undefined ? undefined : total === 0 ? 0 : o.votes_count / total;
          return {
            key: String(i),
            label: title(i, o.title),
            share: value === undefined ? undefined : { value, text: percent.format(value) },
            mine: mine.includes(i),
            leading: poll.expired && most > 0 && o.votes_count === most,
          };
        })}
      />
      <PollFoot parts={line([hidden ? t("post.poll_hidden") : undefined, people, when])} />
    </PollBox>
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

/**
 * The translate line, in one place under the text whatever its state, which assistive technology hears change: the
 * translation's note and Show original; else the translation on its way, a spinner in the glyph's place (with the
 * browser's model downloading, how much has come, which assistive technology hears by quarters); else why it failed,
 * in the text's color, with Try again when the server may answer next time, or Translate in this browser when the
 * browser takes over; else Translate, while the post may be translated; else nothing.
 */
function TranslateLine({
  translation,
  translatable,
  translating,
  failure,
  t,
  lang,
  on,
}: {
  translation: PostTranslation | undefined;
  translatable: boolean;
  translating: boolean | number;
  failure: TranslationFailure | undefined;
  t: T;
  lang: string;
  on: (type: "translate" | "original") => () => void;
}) {
  const line = () => {
    if (translation !== undefined)
      return (
        <CaptionLine
          icon="translate"
          lang={lang}
          action={<TextButton onPress={on("original")}>{t("post.original")}</TextButton>}
        >
          {holdEnd(translatedNote(translation, t, lang), lang)}
        </CaptionLine>
      );
    if (translating !== false) {
      if (typeof translating !== "number")
        return (
          <CaptionLine icon="translate" lang={lang} busy>
            {t("post.translating")}
          </CaptionLine>
        );
      const percent = new Intl.NumberFormat(lang, { style: "percent", maximumFractionDigits: 0 });
      // Assistive technology hears the download start, then each quarter, never each percent.
      const step = Math.floor(Math.min(1, Math.max(0, translating)) * 4) / 4;
      return (
        <CaptionLine
          icon="translate"
          lang={lang}
          busy
          spoken={t("post.translation_downloading", { percent: percent.format(step) })}
        >
          {t("post.translation_downloading", { percent: percent.format(translating) })}
        </CaptionLine>
      );
    }
    if (failure !== undefined) {
      const reason = t(failures[failure.reason]);
      // Over the limit with nothing else to try, the reader learns when they may again.
      const text =
        failure.reason === "limit" && failure.retry === false
          ? `${reason} ${t("post.translation_next_month")}`
          : reason;
      return (
        <CaptionLine
          icon="error"
          lang={lang}
          alert
          action={
            failure.retry === false ? undefined : (
              <TextButton onPress={on("translate")}>
                {t(failure.retry === "browser" ? "post.translation_in_browser" : "post.translation_retry")}
              </TextButton>
            )
          }
        >
          {failure.retry === false ? text : holdEnd(text, lang)}
        </CaptionLine>
      );
    }
    if (translatable)
      return (
        <CaptionLine
          icon="translate"
          lang={lang}
          action={<TextButton onPress={on("translate")}>{t("post.translate")}</TextButton>}
        />
      );
    return null;
  };
  return <LivePart>{line()}</LivePart>;
}

/**
 * A caption's text with its last word unbroken (word joiners between its characters), so that the middle dot held to
 * it before an action never takes part of a word to the next line ("翻訳 ·", not "翻 / 訳 ·").
 */
function holdEnd(text: ReactNode, lang: string): ReactNode {
  if (Array.isArray(text)) {
    const pieces: ReactNode[] = [...(text as ReactNode[])];
    pieces[pieces.length - 1] = holdEnd(pieces.at(-1), lang);
    return pieces;
  }
  if (typeof text !== "string") return text;
  const words = [...new Intl.Segmenter(lang, { granularity: "word" }).segment(text)];
  const last = words.findLastIndex((w) => w.isWordLike === true);
  if (last < 0) return text;
  const start = words[last]?.index ?? 0;
  const characters = new Intl.Segmenter(lang, { granularity: "grapheme" }).segment(text.slice(start));
  return text.slice(0, start) + Array.from(characters, (c) => c.segment).join("⁠");
}

/** The note of a translation: the language it was written in and the provider, as far as they are known. */
function translatedNote(translation: PostTranslation, t: T, lang: string): ReactNode {
  const source = known(translation.source_language);
  const name = source === undefined ? undefined : languageName(source, lang);
  // A provider's name is a proper name, and a language's is the platform's: both isolated, in their own direction.
  const language = name === undefined ? undefined : <bdi>{name}</bdi>;
  // The provider's name never breaks inside, and keeps the word before it ("by LibreTranslate").
  const provider =
    translation.provider === undefined ? undefined : <bdi>{translation.provider.name.replaceAll(" ", " ")}</bdi>;
  if (language !== undefined && provider !== undefined)
    return format(t, "post.translated_from_by", { language, provider }, ["provider"]);
  if (provider !== undefined) return format(t, "post.translated_by", { provider }, ["provider"]);
  if (language !== undefined) return format(t, "post.translated_from", { language });
  return t("post.translated");
}

type Act = (action: PostAction) => void;

/** The actions that need an account, which a reader without one is led to sign in for. */
const needsAccount: ReadonlySet<PostAction["type"]> = new Set([
  "reply",
  "boost",
  "quote",
  "favourite",
  "react",
  "reaction",
  "bookmark",
  "edit",
  "delete",
  "redraft",
  "vote",
]);

/**
 * The more menu: what reaches the post (its link, its original page when it came from another server, sharing it),
 * the bookmark, then for one's own edit (none with a poll), and apart and last delete and delete-and-redraft, which ask
 * first.
 */
function MoreItems({ status, own, t, act }: { status: PostStatus; own: boolean; t: T; act: Act }) {
  const item = (type: Exclude<PostAction["type"], "reaction" | "vote">, icon: IconName, label: string, on = false) => (
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
      {own && (
        <>
          {/* The server edits no post with a poll: its votes belong to the poll as posted. */}
          {status.poll === undefined && <MenuGroup>{item("edit", "edit", t("post.edit"))}</MenuGroup>}
          <MenuGroup>
            {item("redraft", "redraft", t("post.redraft"))}
            {item("delete", "delete", t("post.delete"))}
          </MenuGroup>
        </>
      )}
    </>
  );
}

/**
 * The key the picker tells the reader's own reaction by: a Unicode emoji without its skin tone, or a custom one's
 * `:shortcode:`.
 */
const keyOf = (r: PostReaction) =>
  r.shortcode === undefined ? r.name.replace(/[\u{1F3FB}-\u{1F3FF}]/gu, "") : `:${r.shortcode}:`;

/**
 * The react action: with a picker, it opens it (a popover beside the button, or a sheet on a phone, as the app says),
 * the reader's own reaction knocked out in it; picking an emoji reacts with it (moving the reader's reaction there, or
 * taking it back if it was theirs) and closes it. Without one, it asks the screen.
 */
function ReactAction({
  status,
  t,
  act,
  picker,
}: {
  status: PostStatus;
  t: T;
  act: Act;
  picker: PostPicker | undefined;
}) {
  const [open, setOpen] = useState(false);
  if (picker === undefined)
    return (
      <IconButton
        icon="react"
        label={t("post.react")}
        onPress={() => {
          act({ type: "react", id: status.id });
        }}
      />
    );
  const { sheet, title, closeLabel, preload, onOpen, ...rest } = picker;
  const show = (shown: boolean) => {
    if (shown) onOpen?.();
    setOpen(shown);
  };
  const mine = status.reactions.find((r) => r.me === true);
  // A post takes one reaction, so a pick closes the picker even with Shift held (ADR-0051 §6).
  const content = (
    <EmojiPicker
      {...rest}
      fill={sheet}
      autoFocus
      current={mine === undefined ? undefined : keyOf(mine)}
      onSelect={(emoji, text) => {
        act({ type: "reaction", id: status.id, name: "image" in emoji ? emoji.key.slice(1, -1) : text });
        setOpen(false);
      }}
    />
  );
  // The picker's code and its emoji are fetched as the reader reaches for the button, before it opens.
  const warm = () => {
    preloadEmojiPicker();
    preload?.();
  };
  const button = (
    <IconButton
      icon="react"
      label={t("post.react")}
      onHoverStart={warm}
      onFocus={warm}
      onPressStart={warm}
      {...(sheet
        ? {
            onPress: () => {
              show(true);
            },
          }
        : {})}
    />
  );
  if (sheet)
    return (
      <>
        {button}
        <EmojiPickerSheet
          open={open}
          onClose={() => {
            setOpen(false);
          }}
          title={title}
          closeLabel={closeLabel}
        >
          {content}
        </EmojiPickerSheet>
      </>
    );
  return (
    <EmojiPickerPopover label={t("post.react")} trigger={button} open={open} onOpenChange={show}>
      {content}
    </EmojiPickerPopover>
  );
}

/** The actions: reply, boost (a menu of boost and quote, with its count and on state), favourite and react, more. */
function Actions({
  status,
  own,
  t,
  lang,
  act,
  picker,
}: {
  status: PostStatus;
  own: boolean;
  t: T;
  lang: string;
  act: Act;
  picker: PostPicker | undefined;
}) {
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
      <ReactAction status={status} t={t} act={act} picker={picker} />
    </ActionRow>
  );
}

export function Post({
  status,
  own = false,
  translation,
  translatable = false,
  translating = false,
  translationFailure,
  onAction,
  signIn,
  context,
  details,
  stamp = true,
  after,
  voting,
  picker,
  cardSize = "large",
}: PostProps) {
  const { t, lang } = stringsIn(useLocale().locale);
  const name = useId();
  const shown = status.boost ?? status;
  const act: Act = (action) => {
    if (signIn !== undefined && needsAccount.has(action.type)) signIn(shown);
    else onAction?.(action);
  };
  const textLang = known(translation?.language ?? shown.language);
  const cw = translation?.spoiler_text || shown.spoiler_text;
  const quoted = shown.quote?.quoted_status;
  const on = (type: "translate" | "original" | "history") => () => {
    act({ type, id: shown.id });
  };
  const translateLine = (
    <TranslateLine
      translation={translation}
      translatable={translatable}
      translating={translating}
      failure={translationFailure}
      t={t}
      lang={lang}
      on={on}
    />
  );
  // A post behind a content warning has its translate line in the warning's band, under the warning, in one place
  // folded or open, so that the warning can be translated before it is opened; any other, under its text.
  const translates =
    translation !== undefined || translating !== false || translationFailure !== undefined || translatable;
  // The media viewer, open at one of the post's pictures; the modal gives the focus back to what opened it.
  const [viewing, setViewing] = useState<number | null>(null);
  const media = mediaItems(shown.media_attachments, translation);
  const pictures = media.filter(isPicture);
  const content = (
    <>
      <Stack gap="line">
        <PostContent html={translation?.content ?? shown.content} emojis={shown.emojis} lang={textLang} />
        {!cw && translateLine}
      </Stack>
      <Poll status={shown} translation={translation} own={own} voting={voting} t={t} lang={lang} act={act} />
      <MediaGrid
        items={media}
        sensitive={shown.sensitive ?? false}
        onOpen={(index) => {
          setViewing(index);
        }}
      />
      <Contained key={shown.card?.url}>
        <Card status={shown} size={cardSize} />
      </Contained>
      {quoted !== undefined && <Quote status={quoted} />}
    </>
  );
  const viewer = (
    <ModalPresence>
      {viewing !== null && (
        <MediaViewer
          items={pictures}
          index={viewing}
          onIndexChange={setViewing}
          onClose={() => {
            setViewing(null);
          }}
        />
      )}
    </ModalPresence>
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
              edited={
                shown.edited_at === undefined
                  ? undefined
                  : { text: t("post.edited"), label: t("post.edited_mark"), onPress: on("history") }
              }
            />
          ) : undefined
        }
      />
      {cw ? (
        <Disclosure
          summary={
            translates ? (
              <Stack gap="line" lang={textLang}>
                <span>
                  <EmojiText text={cw} emojis={shown.emojis} />
                </span>
                {translateLine}
              </Stack>
            ) : (
              <EmojiText text={cw} emojis={shown.emojis} />
            )
          }
          icon="hidden"
        >
          <Stack lang={textLang}>{content}</Stack>
        </Disclosure>
      ) : (
        content
      )}
      <Reactions status={shown} t={t} lang={lang} act={act} />
      {details}
      <Actions
        status={shown}
        own={own}
        t={t}
        lang={lang}
        act={act}
        picker={signIn === undefined ? picker : undefined}
      />
      {after}
      {viewer}
    </Entry>
  );
}
