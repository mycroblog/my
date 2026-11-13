// SPDX-License-Identifier: AGPL-3.0-only

// The rough parts of the home timeline screen (#3005), on the catalog only while their issues stay open: the post
// (#2988) and the column with the keyboard moving between posts (#2989), with a quoted post, emoji reactions and
// translation. They use the built parts of @my/design-system, among them the disclosure of a content warning, and the
// one content renderer of @my/app-components for the text, and follow the decisions of the post's anatomy: the author
// header over a body at the full width, the extent shown by a surface on hover and focus only.

import { PostContent } from "@my/app-components";
import {
  Avatar,
  CustomEmoji,
  Disclosure,
  Icon,
  IconButton,
  markedSurface,
  MenuButton,
  MenuGroup,
  MenuItem,
  RelativeTime,
} from "@my/design-system";
import { iconButton, rootPx, text } from "@my/design-tokens";
import { useState, type KeyboardEvent } from "react";
import { focusRing, MoreItems, moreLabels, type Lang } from "../Posts";
import { serverHtml } from "../serverHtml";
import { ago, caption, NameLine } from "../Times";
import { emojis, translationProvider, type Media, type QuotedPost, type Reaction, type TimelinePost } from "./fixtures";
import { strings } from "./strings";

const rem = (px: number) => `${px / rootPx}rem`;

// One column that may shrink below its content, so that a long word or handle wraps or truncates in it.
const one = "minmax(0, 1fr)";

/** The post's text, rendered by the server from the fixture's text, through the one content renderer. */
function PostText({ value }: { value: string }) {
  return <PostContent html={serverHtml(value)} emojis={emojis} />;
}

/** A reaction's emoji: a :shortcode: of the post's custom emoji as its image, a Unicode emoji as text. */
function ReactionEmoji({ value }: { value: string }) {
  const custom = emojis.find((e) => `:${e.shortcode}:` === value);
  return custom ? <CustomEmoji shortcode={custom.shortcode} url={custom.url} staticUrl={custom.static_url} /> : value;
}

/**
 * The images at the column's full width, 16:9 up to a height, so that the next post shows on the same screen at PC:
 * one alone, two side by side, four in a grid; no border, the corners of a control.
 */
function MediaGrid({ media }: { media: Media[] }) {
  const n = media.length;
  return (
    <div style={{ containerType: "inline-size" }}>
      <div
        style={{
          borderRadius: "var(--radius-control)",
          display: "grid",
          gap: rem(4),
          gridTemplateColumns: n === 1 ? one : `${one} ${one}`,
          gridTemplateRows: n > 2 ? `${one} ${one}` : one,
          height: `min(56.25cqw, ${rem(320)})`,
          overflow: "hidden",
        }}
      >
        {media.map((m) => (
          <img
            key={m.src + m.alt}
            src={m.src}
            alt={m.alt}
            style={{ display: "block", height: "100%", minHeight: 0, objectFit: "cover", width: "100%" }}
          />
        ))}
      </div>
    </div>
  );
}

// The tile's inline padding: the actions reach that far past the body, so the first glyph lines up with the body.
const tilePadding = `${iconButton.paddingInline}rem`;

// A count with the reader's own action as it is now, the fixture's count holding it as it was.
const counted = (count: number, on: boolean, was: boolean | undefined) => count + (on ? 1 : 0) - (was ? 1 : 0);

/**
 * The actions: reply, boost, favourite and react close together at the start, more at the end. Boost opens a menu of
 * boost (its glyph on while boosted) and quote, and keeps its count and its on state. React opens no picker yet.
 */
function Actions({ post, lang }: { post: TimelinePost; lang: Lang }) {
  const s = strings[lang];
  const [boosted, setBoosted] = useState(post.boosted ?? false);
  const [favourited, setFavourited] = useState(post.favourited ?? false);
  return (
    <div style={{ display: "flex", gap: rem(4), marginInline: `calc(-1 * ${tilePadding})` }}>
      <IconButton icon="reply" label={s.reply} count={post.counts.replies} />
      <MenuButton
        icon="boost"
        label={s.boost}
        count={counted(post.counts.boosts, boosted, post.boosted)}
        pressed={boosted}
        align="start"
      >
        <MenuGroup>
          <MenuItem
            id="boost"
            icon="boost"
            on={boosted}
            onAction={() => {
              setBoosted(!boosted);
            }}
          >
            {s.boost}
          </MenuItem>
          <MenuItem id="quote" icon="quote">
            {s.quote}
          </MenuItem>
        </MenuGroup>
      </MenuButton>
      <IconButton
        icon="favourite"
        label={s.favourite}
        count={counted(post.counts.favourites, favourited, post.favourited)}
        pressed={favourited}
        onPress={() => {
          setFavourited(!favourited);
        }}
      />
      <IconButton icon="react" label={s.react} />
      <span style={{ display: "inline-flex", marginInlineStart: "auto" }}>
        <MenuButton icon="more" label={moreLabels[lang].more}>
          <MoreItems post={post.author} lang={lang} own={post.own ?? false} bookmarked={post.bookmarked ?? false} />
        </MenuButton>
      </span>
    </div>
  );
}

// The context line, in the reader's language: who boosted the post, or whom it replies to.
function Context({ post, lang }: { post: TimelinePost; lang: Lang }) {
  const s = strings[lang];
  const line = post.boostedBy
    ? { icon: "boost" as const, text: s.boosted(post.boostedBy) }
    : post.replyTo
      ? { icon: "reply" as const, text: s.replying(`@${post.replyTo.split("@")[1] ?? post.replyTo}`) }
      : null;
  if (!line) return null;
  return (
    <div lang={lang} style={{ ...caption, alignItems: "center", display: "flex", gap: rem(6) }}>
      <Icon name={line.icon} place="caption" />
      {line.text}
    </div>
  );
}

const ellipsis = { minWidth: 0, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" } as const;

/**
 * The quoted post, embedded one level deep (ADR-0069): on a raised surface, its author on one line (the inline avatar,
 * the name, the handle, the time at the end), then its text and its images as small previews, so that it never
 * outweighs the quoting post.
 */
function Quote({ quote }: { quote: QuotedPost }) {
  return (
    <div
      lang={quote.lang}
      style={{
        background: "var(--color-surface-raised)",
        borderRadius: "var(--radius-control)",
        display: "grid",
        gap: rem(6),
        gridTemplateColumns: one,
        padding: rem(12),
      }}
    >
      <div style={{ alignItems: "center", display: "flex", fontSize: `${text.body.size}rem`, gap: rem(6) }}>
        <Avatar name={quote.author.name} src={quote.author.avatar} size="inline" decorative />
        <span style={{ ...ellipsis, flex: "0 1 auto", fontWeight: 700 }}>{quote.author.name}</span>
        <span style={{ ...caption, ...ellipsis, flex: "1 1 0" }}>{quote.author.handle}</span>
        <span style={{ ...caption, flex: "none", marginInlineStart: "auto" }}>
          <RelativeTime date={ago(quote.ago)} />
        </span>
      </div>
      <PostText value={quote.text} />
      {quote.media && (
        <div style={{ display: "flex", gap: rem(4) }}>
          {quote.media.map((m) => (
            <img
              key={m.src + m.alt}
              src={m.src}
              alt={m.alt}
              style={{
                borderRadius: "var(--radius-control)",
                display: "block",
                height: rem(72),
                objectFit: "cover",
                width: rem(96),
              }}
            />
          ))}
        </div>
      )}
    </div>
  );
}

/**
 * The emoji reactions (FR-116): a chip per emoji with its count. The reader's own is the chip knocked out, the text
 * color as its fill, with no hue; pressing another moves it there, pressing one's own takes it back.
 */
function Reactions({ reactions, lang }: { reactions: Reaction[]; lang: Lang }) {
  const s = strings[lang];
  const [mine, setMine] = useState(reactions.find((r) => r.mine)?.emoji ?? null);
  const count = (r: Reaction) => r.count - (r.mine ? 1 : 0) + (mine === r.emoji ? 1 : 0);
  return (
    <div role="group" aria-label={s.reactions} lang={lang} style={{ display: "flex", flexWrap: "wrap", gap: rem(6) }}>
      {reactions.map((r) => {
        const on = mine === r.emoji;
        return (
          <button
            key={r.emoji}
            type="button"
            aria-pressed={on}
            aria-label={s.reacted(r.emoji, count(r), on)}
            onClick={() => {
              setMine(on ? null : r.emoji);
            }}
            style={{
              alignItems: "center",
              background: on ? "var(--color-text)" : "var(--color-surface-raised)",
              border: 0,
              borderRadius: "var(--radius-control)",
              color: on ? "var(--color-surface)" : "var(--color-text)",
              cursor: "pointer",
              display: "inline-flex",
              fontFamily: "inherit",
              fontSize: `${text.caption.size}rem`,
              gap: rem(4),
              height: rem(28),
              padding: `0 ${rem(8)}`,
            }}
          >
            <span aria-hidden="true" style={{ fontSize: "1.25em", lineHeight: 1 }}>
              <ReactionEmoji value={r.emoji} />
            </span>
            <span aria-hidden="true" style={{ fontVariantNumeric: "tabular-nums" }}>
              {count(r)}
            </span>
          </button>
        );
      })}
    </div>
  );
}

const quietLink = {
  background: "none",
  border: 0,
  color: "inherit",
  cursor: "pointer",
  font: "inherit",
  padding: 0,
  textDecoration: "underline",
  textUnderlineOffset: "0.15em",
} as const;

/**
 * The text, translated into the reader's language when they ask (ADR-0148): a post in another language offers
 * "Translate" under its text; translated, it names the language and the provider, with the way back to the original.
 */
function TranslatableText({ post, lang }: { post: TimelinePost; lang: Lang }) {
  const s = strings[lang];
  const translation = post.lang === lang ? undefined : post.translation?.[lang];
  const [translated, setTranslated] = useState(post.translated === true && translation !== undefined);
  if (translation === undefined) return <PostText value={post.text} />;
  const language = new Intl.DisplayNames([lang], { type: "language" }).of(post.lang) ?? post.lang;
  return (
    <div style={{ display: "grid", gap: rem(4), gridTemplateColumns: one }}>
      <div lang={translated ? lang : post.lang}>
        <PostText value={translated ? translation : post.text} />
      </div>
      {/* One line of text that wraps between words; the separator and the action never part. */}
      <p lang={lang} style={{ ...caption, margin: 0 }}>
        <span style={{ display: "inline-flex", marginInlineEnd: rem(6), verticalAlign: "-0.15em" }}>
          <Icon name="translate" place="caption" />
        </span>
        {translated && `${s.translatedFrom(language, translationProvider)} `}
        <span style={{ whiteSpace: "nowrap" }}>
          {translated && "· "}
          <button
            type="button"
            style={quietLink}
            onClick={() => {
              setTranslated(!translated);
            }}
          >
            {translated ? s.original : s.translate}
          </button>
        </span>
      </p>
    </div>
  );
}

// The room a hovered or focused post's surface reaches beyond its content, in px (Posts.tsx's `reach`).
const reach = 14;

/**
 * A post: the context line, the author header (avatar, the name with the visibility and the time at the end, over the
 * handle), then the text and images (behind the disclosure of a content warning) and the actions at the full width. At
 * rest it has no edge; hovered or focused, it shows its extent with a surface, and keyboard focus adds the ring.
 */
function PostView({ post, lang }: { post: TimelinePost; lang: Lang }) {
  const [hovered, setHovered] = useState(false);
  const [focused, setFocused] = useState(false);
  const marked = hovered || focused;
  const content = (
    <>
      <TranslatableText post={post} lang={lang} />
      {post.media && <MediaGrid media={post.media} />}
      {post.quote && <Quote quote={post.quote} />}
    </>
  );
  return (
    <article
      lang={post.lang}
      tabIndex={0}
      data-post=""
      {...markedSurface(marked)}
      onPointerEnter={() => {
        setHovered(true);
      }}
      onPointerLeave={() => {
        setHovered(false);
      }}
      onFocus={(e) => {
        setFocused(e.target === e.currentTarget && e.currentTarget.matches(":focus-visible"));
      }}
      onBlur={() => {
        setFocused(false);
      }}
      style={{
        background: marked ? "var(--color-surface-hover)" : undefined,
        borderRadius: rem(8),
        boxShadow: focused ? focusRing : undefined,
        display: "grid",
        gap: rem(8),
        gridTemplateColumns: one,
        margin: rem(-reach),
        outline: "none",
        padding: rem(reach),
        scrollMarginBlock: rem(72),
      }}
    >
      <Context post={post} lang={lang} />
      <div style={{ alignItems: "center", display: "flex", gap: rem(10) }}>
        <Avatar name={post.author.name} src={post.author.avatar} size="post" decorative />
        <div style={{ display: "grid", flex: "1 1 0", fontSize: `${text.body.size}rem`, minWidth: 0 }}>
          <NameLine name={post.author.name} visibility={post.visibility} date={ago(post.ago)} />
          <span
            style={{
              ...caption,
              lineHeight: 1.3,
              overflow: "hidden",
              textOverflow: "ellipsis",
              whiteSpace: "nowrap",
            }}
          >
            {post.author.handle}
          </span>
        </div>
      </div>
      {post.cw ? (
        <Disclosure summary={post.cw} icon="hidden">
          <div style={{ display: "grid", gap: rem(8), gridTemplateColumns: one }}>{content}</div>
        </Disclosure>
      ) : (
        content
      )}
      {post.reactions && <Reactions reactions={post.reactions} lang={lang} />}
      <Actions post={post} lang={lang} />
    </article>
  );
}

// Keys that move between posts: j and k, or the arrows while a post itself has focus.
const next = new Set(["j", "ArrowDown"]);
const previous = new Set(["k", "ArrowUp"]);

/**
 * The column: the posts separated by space alone. j and k (or the arrows on a focused post) move focus to the next or
 * the previous post, from inside a post too; typing into a field is left alone.
 */
export function Column({ posts, lang, label }: { posts: TimelinePost[]; lang: Lang; label: string }) {
  const onKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    const target = e.target as HTMLElement;
    if (e.altKey || e.ctrlKey || e.metaKey || target.closest("input, textarea, [contenteditable], [role=menu]")) return;
    const arrow = e.key.startsWith("Arrow");
    if (!next.has(e.key) && !previous.has(e.key)) return;
    if (arrow && !target.matches("[data-post]")) return;
    const all = Array.from(e.currentTarget.querySelectorAll<HTMLElement>("[data-post]"));
    const current = target.closest<HTMLElement>("[data-post]");
    const i = current ? all.indexOf(current) : -1;
    const to = all[next.has(e.key) ? i + 1 : Math.max(0, i - 1)];
    if (!to) return;
    e.preventDefault();
    to.focus();
    to.scrollIntoView({ block: "nearest" });
  };
  return (
    <div
      role="feed"
      aria-label={label}
      onKeyDown={onKeyDown}
      style={{ display: "grid", gap: rem(28), gridTemplateColumns: one }}
    >
      {posts.map((post) => (
        <PostView key={post.id} post={post} lang={lang} />
      ))}
    </div>
  );
}
