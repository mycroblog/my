// SPDX-License-Identifier: AGPL-3.0-only

// The anatomy of a post in the home timeline, in grayscale (ADR-0169 §9), on a short column of en and ja posts: an
// author header (avatar, name and time over the handle), then the body, the media and the actions at the full width,
// so that no empty column takes room on mobile. What is not yet a component (the avatar, the media, the glyphs) is
// drawn as a gray block; the actions are the Button of @my/design-system.

import { Button } from "@my/design-system";
import { rootPx, text } from "@my/design-tokens";
import { useState, type CSSProperties, type ReactNode } from "react";

const rem = (px: number) => `${px / rootPx}rem`;

type Visibility = "public" | "unlisted" | "followers";
type Post = {
  lang: "en" | "ja";
  name: string;
  handle: string;
  time: string;
  visibility: Visibility;
  context?: { kind: "boost" | "reply"; who: string };
  cw?: string;
  body: ReactNode;
  media?: string;
  counts: { replies: number; boosts: number; favourites: number };
};

const visibilityLabel: Record<Visibility, string> = {
  public: "Public",
  unlisted: "Quiet public",
  followers: "Followers only",
};

const muted = "var(--color-muted-text)";
const block = "var(--color-surface-raised-pressed)";

// The avatar's size and its gap to the name, in px.
const avatar = 40;
const avatarGap = 10;

// One column that may shrink below its content, so that a long handle truncates instead of widening the post.
const one = "minmax(0, 1fr)";

const caption: CSSProperties = {
  color: muted,
  fontSize: `${text.caption.size}rem`,
  lineHeight: text.caption.lineHeight,
};

// A link, a mention or a hashtag: in grayscale, the text's own color, underlined.
const A = ({ children }: { children: ReactNode }) => (
  <a href="#post" style={{ color: "inherit", textDecoration: "underline", textUnderlineOffset: "0.15em" }}>
    {children}
  </a>
);

// A custom emoji, drawn as a gray square of the line's size.
const Emoji = ({ name }: { name: string }) => (
  <span
    role="img"
    aria-label={`:${name}:`}
    style={{
      background: block,
      borderRadius: 3,
      display: "inline-block",
      height: "1.2em",
      verticalAlign: "-0.25em",
      width: "1.2em",
    }}
  />
);

// A glyph, drawn as a gray square, until the icons are decided.
const Glyph = ({ size = 16, label }: { size?: number; label?: string }) => (
  <span
    role={label ? "img" : undefined}
    aria-label={label}
    aria-hidden={label ? undefined : true}
    style={{
      background: muted,
      borderRadius: 3,
      display: "inline-block",
      flex: "none",
      height: rem(size),
      opacity: 0.6,
      width: rem(size),
    }}
  />
);

const Avatar = ({ size }: { size: number }) => (
  <div
    aria-hidden
    style={{ background: block, borderRadius: rem(size / 4), flex: "none", height: rem(size), width: rem(size) }}
  />
);

const posts: Post[] = [
  {
    lang: "en",
    name: "Alice Chen",
    handle: "@alice@example.social",
    time: "3m",
    visibility: "public",
    context: { kind: "reply", who: "@mika" },
    body: (
      <>
        Morning! The bakery on the corner finally reopened <Emoji name="croissant" /> See you there at nine?
      </>
    ),
    counts: { replies: 2, boosts: 0, favourites: 5 },
  },
  {
    lang: "ja",
    name: "佐藤 美香",
    handle: "@mika@mstdn.example.jp",
    time: "12分",
    visibility: "public",
    body: (
      <>
        週末に小さなサーバーを立ててみました。最初はひとりで使うつもりでしたが、
        <A>@alice@example.social</A>{" "}
        さんに声をかけたら友だちが三人来てくれて、いまはタイムラインがちょうどいい速さで流れています。大きなサーバーにいたころは追いきれなかった会話が、ここでは全部読めるのがうれしい。設定の手順はブログにまとめました{" "}
        <A>example.jp/blog/small-server</A> <A>#小さなサーバー</A> <Emoji name="tada" />
      </>
    ),
    counts: { replies: 4, boosts: 12, favourites: 38 },
  },
  {
    lang: "en",
    name: "Open Source Weekly",
    handle: "@news@social.example.org",
    time: "1h",
    visibility: "unlisted",
    context: { kind: "boost", who: "Kenji" },
    body: (
      <>
        This week: three servers federate their first polls, and a guide to moving your account.{" "}
        <A>social.example.org/weekly/42</A>
      </>
    ),
    counts: { replies: 1, boosts: 27, favourites: 64 },
  },
  {
    lang: "ja",
    name: "たくや",
    handle: "@takuya@example.jp",
    time: "2時間",
    visibility: "followers",
    cw: "映画の結末に触れています",
    body: <>最後の十分で全部ひっくり返るの、二回目に観るとちゃんと伏線があってすごい。</>,
    counts: { replies: 0, boosts: 0, favourites: 3 },
  },
  {
    lang: "en",
    name: "Ravi",
    handle: "@ravi@fedi.example.net",
    time: "4h",
    visibility: "public",
    body: <>First snow on the ridge this morning.</>,
    media: "A snowy ridge under a pale sky, with pine trees in the foreground",
    counts: { replies: 3, boosts: 5, favourites: 41 },
  },
];

// The actions, as quiet timeline buttons: a glyph and a count. Zero shows no count.
function Actions({ post }: { post: Post }) {
  const action = (label: string, count?: number) => (
    <Button kind="quiet" size="timeline" aria-label={count ? `${label} (${count})` : label}>
      <span style={{ alignItems: "center", display: "inline-flex", gap: rem(6), fontWeight: 400 }}>
        <Glyph />
        {count ? <span style={{ color: muted, fontVariantNumeric: "tabular-nums" }}>{count}</span> : null}
      </span>
    </Button>
  );
  return (
    <div
      style={{
        display: "flex",
        justifyContent: "space-between",
        marginInlineStart: `calc(-1 * var(--control-timeline-padding-inline))`,
        maxWidth: rem(320),
      }}
    >
      {action("Reply", post.counts.replies)}
      {action("Boost", post.counts.boosts)}
      {action("Favourite", post.counts.favourites)}
      {action("More")}
    </div>
  );
}

// The context line: who boosted it, or whom it replies to, from the post's edge, where the body starts.
const Context = ({ post }: { post: Post }) =>
  post.context ? (
    <div lang="en" style={{ ...caption, alignItems: "center", display: "flex", gap: rem(6) }}>
      <Glyph size={12} />
      {post.context.kind === "boost" ? `${post.context.who} boosted` : `Replying to ${post.context.who}`}
    </div>
  ) : null;

// The body, folded under its content warning when it has one.
function Body({ post }: { post: Post }) {
  const body = (
    <p
      style={{
        fontSize: `${text.body.size}rem`,
        lineHeight: text.body.lineHeight,
        margin: 0,
        overflowWrap: "anywhere",
      }}
    >
      {post.body}
    </p>
  );
  const media = post.media ? (
    <div
      role="img"
      aria-label={post.media}
      style={{
        aspectRatio: "16 / 9",
        background: block,
        borderRadius: "var(--radius-control)",
        // Capped, so that the next post's author shows on the same screen at PC.
        maxHeight: rem(240),
        width: "100%",
      }}
    />
  ) : null;
  if (post.cw)
    return (
      <div
        style={{
          alignItems: "center",
          background: "var(--color-surface-raised)",
          borderRadius: "var(--radius-control)",
          display: "flex",
          flexWrap: "wrap",
          gap: rem(8),
          justifyContent: "space-between",
          // The half-leading a line of text leaves below it, so the actions stand as far from it as from a body.
          marginBottom: rem(4),
          padding: `${rem(8)} ${rem(8)} ${rem(8)} ${rem(12)}`,
        }}
      >
        <span style={{ fontSize: `${text.body.size}rem`, lineHeight: text.body.lineHeight }}>{post.cw}</span>
        <Button kind="secondary" size="timeline" lang="en">
          Show
        </Button>
      </div>
    );
  return (
    <div style={{ display: "grid", gap: rem(8), gridTemplateColumns: one }}>
      {body}
      {media}
    </div>
  );
}

const ellipsis: CSSProperties = { minWidth: 0, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" };
// The name keeps its room before the handle gives up its own.
const name: CSSProperties = { ...ellipsis, fontWeight: 700, flex: "none", maxWidth: "65%" };
const handle: CSSProperties = { ...ellipsis, color: muted, flex: "0 1 auto" };

// The time with the visibility as a glyph, next to the name.
const Stamp = ({ post }: { post: Post }) => (
  <span style={{ ...caption, alignItems: "center", display: "flex", flex: "none", gap: rem(6) }}>
    <Glyph size={12} label={visibilityLabel[post.visibility]} />
    <time>{post.time}</time>
  </span>
);

type PostState = "hover" | "focus-visible";

// The button's two-tone focus ring, at the post's edge.
const focusRing = [
  "0 0 0 var(--focus-ring-gap) var(--color-surface)",
  "0 0 0 calc(var(--focus-ring-gap) + var(--focus-ring-width)) var(--color-accent-text)",
  "0 0 0 calc(var(--focus-ring-gap) + var(--focus-ring-width) + var(--focus-ring-halo)) color-mix(in srgb, var(--color-accent) var(--focus-ring-halo-share), transparent)",
].join(", ");

// The room the surface of a hovered or focused post reaches beyond its content, in px; the post's content stays where
// it is, so the space between posts at rest is unchanged.
const reach = 12;

// A post: the author is a header beside the avatar, the name and the time on one line over the handle, so the time
// stays by the name at any width; the body, the media and the actions take the whole width below it. At rest it has
// no edge; hovered or focused from the keyboard, it shows its extent with the surface-hover surface, and focus adds
// the ring. `shown` shows a state at rest, for the catalog.
function PostView({ post, shown }: { post: Post; shown?: PostState }) {
  const [hovered, setHovered] = useState(false);
  const [focused, setFocused] = useState(false);
  const focus = focused || shown === "focus-visible";
  const marked = hovered || focus || shown === "hover";
  return (
    <article
      lang={post.lang}
      tabIndex={0}
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
        boxShadow: focus ? focusRing : undefined,
        display: "grid",
        gap: rem(8),
        gridTemplateColumns: one,
        margin: rem(-reach),
        outline: "none",
        padding: rem(reach),
      }}
    >
      <Context post={post} />
      <div style={{ alignItems: "center", display: "flex", gap: rem(avatarGap) }}>
        <Avatar size={avatar} />
        <div style={{ display: "grid", flex: "1 1 0", minWidth: 0, fontSize: `${text.body.size}rem` }}>
          <div style={{ alignItems: "baseline", display: "flex", gap: rem(8), lineHeight: 1.3, minWidth: 0 }}>
            <span style={name}>{post.name}</span>
            <Stamp post={post} />
          </div>
          <span style={{ ...handle, ...caption, lineHeight: 1.3 }}>{post.handle}</span>
        </div>
      </div>
      <Body post={post} />
      <Actions post={post} />
    </article>
  );
}

const column: CSSProperties = { display: "grid", gap: rem(28), gridTemplateColumns: one, maxWidth: rem(600) };

// The column: the posts on one surface, separated by space alone, several times the space inside a post; no line,
// no card edge. Below it, one post in each of its states, shown at rest.
export function Posts() {
  const states: PostState[] = ["hover", "focus-visible"];
  return (
    <div style={{ color: "var(--color-text)", colorScheme: "light", display: "grid", gap: rem(40) }}>
      <div style={column}>
        {posts.map((post) => (
          <PostView key={post.handle} post={post} />
        ))}
      </div>
      <div style={column}>
        {states.map((state) => (
          <div key={state} style={{ display: "grid", gap: rem(20), gridTemplateColumns: one }}>
            <h4 style={{ ...caption, margin: 0 }}>{state}</h4>
            <PostView post={posts[2] as Post} shown={state} />
          </div>
        ))}
      </div>
    </div>
  );
}
