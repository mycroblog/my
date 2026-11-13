// SPDX-License-Identifier: AGPL-3.0-only

// The anatomy of a post in the home timeline, in grayscale (ADR-0169 §9), on a short column of en and ja posts: an
// author header (avatar, name and time over the handle), then the body, the media and the actions at the full width,
// so that no empty column takes room on mobile. The media, not yet a component, is drawn as a gray block; the avatar,
// the actions (the Button) and the glyphs are those of @my/design-system. Custom emoji wait for the post's text
// renderer.

import { Avatar, Button, Icon, icons, type IconName, type IconPlace } from "@my/design-system";
import { rootPx, text, type Scheme } from "@my/design-tokens";
import { useState, type CSSProperties, type ReactNode } from "react";
import { AvatarSpecimen, images } from "./Avatars";

const rem = (px: number) => `${px / rootPx}rem`;

type Visibility = "public" | "unlisted" | "followers" | "direct";
type Post = {
  lang: "en" | "ja";
  name: string;
  handle: string;
  time: string;
  avatar?: string;
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
  direct: "Direct",
};

const muted = "var(--color-muted-text)";
const block = "var(--color-surface-raised-pressed)";

// The avatar's gap to the name, in px.
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

// An icon of @my/design-system at the size of its place. Where the glyph alone carries meaning (the visibility), a
// wrapper names it.
function Glyph({
  meaning,
  on = false,
  size = "action",
  label,
}: {
  meaning: IconName;
  on?: boolean | undefined;
  size?: IconPlace;
  label?: string;
}) {
  const glyph = <Icon name={meaning} place={size} on={on} />;
  if (!label) return glyph;
  return (
    <span role="img" aria-label={label} style={{ display: "inline-flex", flex: "none" }}>
      {glyph}
    </span>
  );
}

const posts: Post[] = [
  {
    lang: "en",
    name: "Alice Chen",
    handle: "@alice@example.social",
    time: "3m",
    avatar: images.dusk,
    visibility: "public",
    context: { kind: "reply", who: "@mika" },
    body: <>Morning! The bakery on the corner finally reopened. See you there at nine?</>,
    counts: { replies: 2, boosts: 0, favourites: 5 },
  },
  {
    lang: "ja",
    name: "佐藤 美香",
    handle: "@mika@mstdn.example.jp",
    time: "12分",
    avatar: images.snow,
    visibility: "public",
    body: (
      <>
        週末に小さなサーバーを立ててみました。最初はひとりで使うつもりでしたが、
        <A>@alice@example.social</A>{" "}
        さんに声をかけたら友だちが三人来てくれて、いまはタイムラインがちょうどいい速さで流れています。大きなサーバーにいたころは追いきれなかった会話が、ここでは全部読めるのがうれしい。設定の手順はブログにまとめました{" "}
        <A>example.jp/blog/small-server</A> <A>#小さなサーバー</A>
      </>
    ),
    counts: { replies: 4, boosts: 12, favourites: 38 },
  },
  {
    lang: "en",
    name: "Open Source Weekly",
    handle: "@news@social.example.org",
    time: "1h",
    avatar: images.logo,
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
    visibility: "direct",
    body: <>First snow on the ridge this morning.</>,
    media: "A snowy ridge under a pale sky, with pine trees in the foreground",
    counts: { replies: 3, boosts: 5, favourites: 41 },
  },
];

// The actions, as quiet timeline buttons: a glyph and a count. Zero shows no count. `on` shows the boost or the
// favourite done.
type Done = "boost" | "favourite";
function Actions({ post, on = [] }: { post: Post; on?: Done[] }) {
  const action = (label: string, meaning: IconName, count?: number) => (
    <Button kind="quiet" size="timeline" aria-label={count ? `${label} (${count})` : label}>
      <span style={{ alignItems: "center", color: muted, display: "inline-flex", gap: rem(6), fontWeight: 400 }}>
        <Glyph meaning={meaning} on={(on as IconName[]).includes(meaning)} />
        {count ? <span style={{ fontVariantNumeric: "tabular-nums" }}>{count}</span> : null}
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
      {action("Reply", "reply", post.counts.replies)}
      {action(on.includes("boost") ? "Boosted" : "Boost", "boost", post.counts.boosts)}
      {action(on.includes("favourite") ? "Favourited" : "Favourite", "favourite", post.counts.favourites)}
      {action("More", "more")}
    </div>
  );
}

// The context line: who boosted it, or whom it replies to, from the post's edge, where the body starts.
const Context = ({ post }: { post: Post }) =>
  post.context ? (
    <div lang="en" style={{ ...caption, alignItems: "center", display: "flex", gap: rem(6) }}>
      <Glyph meaning={post.context.kind} size="caption" />
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
const Stamp = ({ visibility, time }: { visibility: Visibility; time: string }) => (
  <span style={{ ...caption, alignItems: "center", display: "flex", flex: "none", gap: rem(6) }}>
    <Glyph meaning={visibility} size="caption" label={visibilityLabel[visibility]} />
    <time>{time}</time>
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
        <Avatar name={post.name} src={post.avatar} size="post" decorative />
        <div style={{ display: "grid", flex: "1 1 0", minWidth: 0, fontSize: `${text.body.size}rem` }}>
          <div style={{ alignItems: "baseline", display: "flex", gap: rem(8), lineHeight: 1.3, minWidth: 0 }}>
            <span style={name}>{post.name}</span>
            <Stamp visibility={post.visibility} time={post.time} />
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
const sheetRow: CSSProperties = { display: "grid", gap: rem(6), gridTemplateColumns: one };
const wrapRow: CSSProperties = { alignItems: "center", display: "flex", flexWrap: "wrap", gap: `${rem(8)} ${rem(16)}` };

// The glyphs at their timeline sizes beside the decided type, in one scheme: the actions off and on, the visibility
// and the context glyphs at the caption, and the rest of what a timeline needs.
function Sheet({ scheme }: { scheme: Scheme }) {
  const sample = posts[1] as Post;
  const visibilities: Visibility[] = ["public", "unlisted", "followers", "direct"];
  const others: [IconName, string, boolean?][] = [
    ["bookmark", "Bookmark"],
    ["bookmark", "Bookmarked", true],
    ["share", "Share"],
    ["attach", "Attach"],
    ["emoji", "Emoji"],
    ["close", "Close"],
    ["back", "Back"],
  ];
  return (
    <div
      style={{
        background: "var(--color-surface)",
        border: "1px solid #ccc",
        borderRadius: 8,
        color: "var(--color-text)",
        colorScheme: scheme,
        display: "grid",
        flex: `1 1 ${rem(320)}`,
        gap: rem(16),
        gridTemplateColumns: one,
        padding: rem(16),
      }}
    >
      <div style={caption}>{scheme}</div>
      <div style={sheetRow}>
        <h4 style={{ ...caption, margin: 0 }}>Actions · off, boosted, favourited</h4>
        <Actions post={sample} />
        <Actions post={sample} on={["boost"]} />
        <Actions post={sample} on={["favourite"]} />
      </div>
      <div style={sheetRow}>
        <h4 style={{ ...caption, margin: 0 }}>Visibility, by the time</h4>
        <div style={{ ...wrapRow, fontSize: `${text.body.size}rem` }}>
          {visibilities.map((v) => (
            <span key={v} style={{ alignItems: "baseline", display: "flex", gap: rem(8) }}>
              <span style={{ fontWeight: 700 }}>{visibilityLabel[v]}</span>
              <Stamp visibility={v} time="12m" />
            </span>
          ))}
        </div>
      </div>
      <div style={sheetRow}>
        <h4 style={{ ...caption, margin: 0 }}>Context</h4>
        <Context post={posts[0] as Post} />
        <Context post={posts[2] as Post} />
      </div>
      <div style={sheetRow}>
        <h4 style={{ ...caption, margin: 0 }}>Others</h4>
        <div
          style={{ ...wrapRow, gap: rem(4), marginInlineStart: `calc(-1 * var(--control-timeline-padding-inline))` }}
        >
          {others.map(([meaning, label, on]) => (
            <Button key={label} kind="quiet" size="timeline" aria-label={label}>
              <span
                style={{ alignItems: "center", color: muted, display: "inline-flex", gap: rem(6), fontWeight: 400 }}
              >
                <Glyph meaning={meaning} on={on} />
                <span style={caption}>{label}</span>
              </span>
            </Button>
          ))}
        </div>
      </div>
    </div>
  );
}

// The icon table of @my/design-system: each meaning with its Tabler glyph, its on shape, and whether it flips in RTL.
function IconTable() {
  type Named = { displayName?: string | undefined };
  const entries: [string, { off: Named; on?: Named | "tile"; directional?: boolean }][] = Object.entries(icons);
  const onOf = (on: Named | "tile" | undefined) =>
    on === undefined ? "" : on === "tile" ? " / on: tile" : ` / on: ${on.displayName ?? ""}`;
  return (
    <div style={caption}>
      Tabler (@tabler/icons-react 3.51.0, MIT), stroke 2:{" "}
      {entries
        .map(([m, e]) => `${m} ${e.off.displayName}${onOf(e.on)}${e.directional ? " (flips in RTL)" : ""}`)
        .join(" · ")}
    </div>
  );
}

// The column: the posts on one surface, separated by space alone, several times the space inside a post; no line,
// no card edge. Below it, one post in each of its states, shown at rest, then the avatars and the glyphs in light and
// dark. The section follows the page's color scheme.
export function Posts() {
  const states: PostState[] = ["hover", "focus-visible"];
  return (
    <div style={{ color: "var(--color-text)", display: "grid", gap: rem(40) }}>
      <IconTable />
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
      <div style={{ display: "flex", flexWrap: "wrap", gap: rem(16) }}>
        <AvatarSpecimen scheme="light" />
        <AvatarSpecimen scheme="dark" />
      </div>
      <div style={{ display: "flex", flexWrap: "wrap", gap: rem(16) }}>
        <Sheet scheme="light" />
        <Sheet scheme="dark" />
      </div>
    </div>
  );
}
