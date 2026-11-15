// SPDX-License-Identifier: AGPL-3.0-only

// A post of a timeline (#2981's anatomy), drawn from a status as the API gives it (status.ts) with the parts of
// @my/design-system and the one content renderer: the context line (who boosted it, or whom it replies to), the author
// header (the avatar, the name with the visibility and the time at the end, and "edited" after the time of an edited
// post, which opens its edit history, over the handle), the text, or a content warning over it, the poll (open to the
// reader's vote, else its results), the images, the quoted post as a reduced preview, the emoji reactions, the
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
  MediaGrid,
  MenuButton,
  MenuGroup,
  MenuItem,
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
import { Fragment, useId, useState, type ReactNode } from "react";
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

type T = ReturnType<typeof stringsIn>["t"];

// The first of the characters no text holds (the private use area), each standing for a part drawn by React inside a
// formatted string.
const firstSlot = 0xe000;
const slotPattern = /([-])/u;

/**
 * The string `id` with each of `parts` drawn by React in its argument's place, so that a name keeps its custom emoji
 * and its isolation inside a sentence, and no transform of the message (a pseudo-locale's) reaches it.
 */
export function format(t: T, id: MessageId, parts: Readonly<Record<string, ReactNode>>): ReactNode {
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

/** The note of a translation: the language it was written in and the provider, as far as they are known. */
function translatedNote(translation: PostTranslation, t: T, lang: string): ReactNode {
  const source = known(translation.source_language);
  const name = source === undefined ? undefined : languageName(source, lang);
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
  onAction,
  signIn,
  context,
  details,
  stamp = true,
  after,
  voting,
  picker,
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
      <Poll status={shown} translation={translation} own={own} voting={voting} t={t} lang={lang} act={act} />
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
        <Disclosure summary={<EmojiText text={cw} emojis={shown.emojis} />} icon="hidden">
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
    </Entry>
  );
}
