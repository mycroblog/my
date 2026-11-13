// SPDX-License-Identifier: AGPL-3.0-only

// The anatomy of a post in the home timeline, in grayscale (ADR-0169 §9), on a short column of en and ja posts: an
// author header (avatar, then the name with the visibility and the time at the end, over the handle), then the body, the media and the actions at the full width,
// so that no empty column takes room on mobile. The media, not yet a component, is drawn as a gray block; the avatar,
// the actions (the IconButton) and the glyphs are those of @my/design-system. Custom emoji wait for the post's text
// renderer.

import {
  Avatar,
  Button,
  Icon,
  IconButton,
  icons,
  markedSurface,
  MenuButton,
  MenuGroup,
  MenuItem,
  MenuPanel,
  type IconName,
  type IconPlace,
} from "@my/design-system";
import { Preview, PreviewItems, type PreviewState } from "@my/design-system/preview";
import { iconButton, rootPx, text, type Scheme } from "@my/design-tokens";
import { useState, type CSSProperties, type ReactNode } from "react";
import { AvatarSpecimen, images } from "./Avatars";
import { DisclosureSpecimen } from "./Disclosures";
import { ago, caption, NameLine, Stamp, TimeSpecimen, visibilityLabel, type Visibility } from "./Times";

const rem = (px: number) => `${px / rootPx}rem`;

const minute = 60_000;
const hour = 60 * minute;
const day = 24 * hour;

type Post = {
  lang: "en" | "ja";
  name: string;
  handle: string;
  /** How long ago it was posted, in ms; the time is that long before the page renders. */
  ago: number;
  avatar?: string;
  visibility: Visibility;
  context?: { kind: "boost" | "reply"; who: string };
  cw?: string;
  body: ReactNode;
  media?: string;
  counts: { replies: number; boosts: number; favourites: number };
};

const muted = "var(--color-muted-text)";
const block = "var(--color-surface-raised-pressed)";

// The avatar's gap to the name, in px.
const avatarGap = 10;

// One column that may shrink below its content, so that a long handle truncates instead of widening the post.
const one = "minmax(0, 1fr)";

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
    ago: 3 * minute,
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
    ago: 12 * minute,
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
    ago: hour,
    avatar: images.logo,
    visibility: "unlisted",
    context: { kind: "boost", who: "Kenji" },
    body: (
      <>
        This week: three servers federate their first polls, and a guide to moving your account.{" "}
        <A>social.example.org/weekly/42</A>
      </>
    ),
    counts: { replies: 18, boosts: 1_234, favourites: 12_840 },
  },
  {
    lang: "ja",
    name: "たくや",
    handle: "@takuya@example.jp",
    ago: 26 * hour,
    visibility: "followers",
    cw: "映画の結末に触れています",
    body: <>最後の十分で全部ひっくり返るの、二回目に観るとちゃんと伏線があってすごい。</>,
    counts: { replies: 0, boosts: 0, favourites: 3 },
  },
  {
    lang: "en",
    name: "Ravi",
    handle: "@ravi@fedi.example.net",
    ago: 9 * day,
    visibility: "direct",
    body: <>First snow on the ridge this morning.</>,
    media: "A snowy ridge under a pale sky, with pine trees in the foreground",
    counts: { replies: 3, boosts: 5, favourites: 41 },
  },
];

export type Lang = "en" | "ja";

// The menus' sample labels (more, and boost's boost and quote); the design system's strings are not built yet. A
// label whose action asks for a confirmation (block, report, delete) ends in "…".
export const moreLabels = {
  en: {
    boost: "Boost",
    quote: "Quote",
    more: "More",
    link: "Copy link",
    open: "Open original page",
    share: "Share",
    bookmark: "Bookmark",
    unbookmark: "Remove bookmark",
    edit: "Edit",
    mute: "Mute",
    block: "Block…",
    report: "Report…",
    delete: "Delete…",
  },
  ja: {
    boost: "ブースト",
    quote: "引用",
    more: "その他",
    link: "リンクをコピー",
    open: "元のページを開く",
    share: "共有",
    bookmark: "ブックマーク",
    unbookmark: "ブックマークを外す",
    edit: "編集",
    mute: "ミュート",
    block: "ブロック…",
    report: "通報…",
    delete: "削除…",
  },
} as const;

// The account part of a handle: "@mika" of "@mika@mstdn.example.jp".
const shortHandle = (h: string) => `@${h.split("@")[1] ?? h}`;

// A post's more menu, in the reader's `lang`: what reaches the post (copy link, open original, share), then the
// bookmark toggle on its own; then for another's post the actions on its account (mute, block, report) last and
// apart, under a caption naming it, or for one's own post edit, then delete last and apart.
export function MoreItems({
  post,
  lang,
  own = false,
  bookmarked = false,
}: {
  post: { handle: string };
  lang: Lang;
  own?: boolean;
  bookmarked?: boolean;
}) {
  const l = moreLabels[lang];
  return (
    <>
      <MenuGroup>
        <MenuItem id="link" icon="link">
          {l.link}
        </MenuItem>
        {!own && (
          <MenuItem id="open" icon="open">
            {l.open}
          </MenuItem>
        )}
        <MenuItem id="share" icon="share">
          {l.share}
        </MenuItem>
      </MenuGroup>
      <MenuGroup>
        <MenuItem id="bookmark" icon="bookmark" on={bookmarked}>
          {bookmarked ? l.unbookmark : l.bookmark}
        </MenuItem>
      </MenuGroup>
      {own ? (
        <>
          <MenuGroup>
            <MenuItem id="edit" icon="edit">
              {l.edit}
            </MenuItem>
          </MenuGroup>
          <MenuGroup>
            <MenuItem id="delete" icon="delete">
              {l.delete}
            </MenuItem>
          </MenuGroup>
        </>
      ) : (
        <MenuGroup title={shortHandle(post.handle)}>
          <MenuItem id="mute" icon="mute">
            {l.mute}
          </MenuItem>
          <MenuItem id="block" icon="block">
            {l.block}
          </MenuItem>
          <MenuItem id="report" icon="report">
            {l.report}
          </MenuItem>
        </MenuGroup>
      )}
    </>
  );
}

// The tile's inline padding: the row reaches that far past the body on both sides, so that the first glyph lines up
// with the body's start and the last with its end.
const tilePadding = `${iconButton.paddingInline}rem`;

// The actions, as icon buttons: a glyph and a count. Zero shows no count. Reply, boost and favourite sit close
// together at the start, more at the end; the buttons sit side by side and their touch areas reach only up and down,
// so they never overlap. `on` shows the boost or the favourite done; `shownOn` shows one button in a state at rest.
type Done = "boost" | "favourite";
type ShownOn = { meaning: IconName; state: PreviewState };
function Actions({ post, on = [], shownOn }: { post: Post; on?: Done[] | undefined; shownOn?: ShownOn | undefined }) {
  const action = (icon: IconName, label: string, count?: number, end = false) => {
    const button =
      icon === "more" && shownOn?.meaning !== "more" ? (
        <MenuButton icon="more" label={label}>
          <MoreItems post={post} lang="en" />
        </MenuButton>
      ) : (
        <IconButton
          icon={icon}
          label={label}
          count={count}
          pressed={icon === "boost" || icon === "favourite" ? on.includes(icon) : undefined}
        />
      );
    return (
      <span style={{ display: "inline-flex", ...(end ? { marginInlineStart: "auto" } : {}) }}>
        {shownOn?.meaning === icon ? <Preview state={shownOn.state}>{button}</Preview> : button}
      </span>
    );
  };
  return (
    <div style={{ display: "flex", gap: rem(4), marginInline: `calc(-1 * ${tilePadding})` }}>
      {action("reply", "Reply", post.counts.replies)}
      {action("boost", "Boost", post.counts.boosts)}
      {action("favourite", "Favourite", post.counts.favourites)}
      {action("more", "More", undefined, true)}
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

const handle: CSSProperties = {
  color: muted,
  minWidth: 0,
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
};

type PostState = "hover" | "focus-visible";

// The button's two-tone focus ring, at the post's edge.
export const focusRing = [
  "0 0 0 var(--focus-ring-gap) var(--color-surface)",
  "0 0 0 calc(var(--focus-ring-gap) + var(--focus-ring-width)) var(--color-accent-text)",
  "0 0 0 calc(var(--focus-ring-gap) + var(--focus-ring-width) + var(--focus-ring-halo)) color-mix(in srgb, var(--color-accent) var(--focus-ring-halo-share), transparent)",
].join(", ");

// The room the surface of a hovered or focused post reaches beyond its content, in px; the post's content stays where
// it is, so the space between posts at rest is unchanged. It holds an action's tile, which reaches past the body by
// its inline padding (6), and the tile's focus ring around it (7).
const reach = 14;

// A post: the author is a header beside the avatar, the name, then the visibility and the time at the line's end, over
// the handle; the time is in the reader's locale, the page's, whatever the post's language. The body, the media and the actions take the whole width below it. At rest it has
// no edge; hovered or focused from the keyboard, it shows its extent with the surface-hover surface, and focus adds
// the ring. `shown` shows a state at rest, for the catalog.
function PostView({
  post,
  shown,
  on,
  shownOn,
}: {
  post: Post;
  shown?: PostState | undefined;
  on?: Done[] | undefined;
  shownOn?: ShownOn | undefined;
}) {
  const [hovered, setHovered] = useState(false);
  const [focused, setFocused] = useState(false);
  const focus = focused || shown === "focus-visible";
  const marked = hovered || focus || shown === "hover";
  return (
    <article
      lang={post.lang}
      tabIndex={0}
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
          <NameLine name={post.name} visibility={post.visibility} date={ago(post.ago)} />
          <span style={{ ...handle, ...caption, lineHeight: 1.3 }}>{post.handle}</span>
        </div>
      </div>
      <Body post={post} />
      <Actions post={post} on={on} shownOn={shownOn} />
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
        <h4 style={{ ...caption, margin: 0 }}>
          Actions · off, boosted, favourited, both with large counts, no replies or boosts yet
        </h4>
        <Actions post={sample} />
        <Actions post={sample} on={["boost"]} />
        <Actions post={sample} on={["favourite"]} />
        <Actions post={posts[2] as Post} on={["boost", "favourite"]} />
        <Actions post={posts[3] as Post} />
      </div>
      <div style={sheetRow}>
        <h4 style={{ ...caption, margin: 0 }}>Counts · a reader in en, a reader in ja</h4>
        {(["en", "ja"] as const).map((locale) => (
          <div
            key={locale}
            style={{ display: "flex", flexWrap: "wrap", gap: rem(4), marginInline: `calc(-1 * ${tilePadding})` }}
          >
            {[7, 999, 1_234, 12_840, 1_250_000].map((n) => (
              <IconButton key={n} icon="favourite" label="Favourite" count={n} locale={locale} />
            ))}
          </div>
        ))}
      </div>
      <div style={sheetRow}>
        <h4 style={{ ...caption, margin: 0 }}>Visibility, by the time</h4>
        <div style={{ ...wrapRow, fontSize: `${text.body.size}rem` }}>
          {visibilities.map((v) => (
            <span key={v} style={{ alignItems: "baseline", display: "flex", gap: rem(8) }}>
              <span style={{ fontWeight: 700 }}>{visibilityLabel[v]}</span>
              <Stamp visibility={v} date={ago(12 * minute)} />
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

// One's own post, for the menu of one's own post.
const ownPost: Post = {
  lang: "en",
  name: "Kumi Ito",
  handle: "@kumi@my.example",
  ago: 5 * minute,
  visibility: "public",
  body: <>Trying out the new server. Hello, fediverse!</>,
  counts: { replies: 1, boosts: 0, favourites: 2 },
};

// The space between the more button and its menu, as the popover places it, in px.
const menuOffset = 4;

// A post with its more menu open at rest, in the reader's `lang`: the more button pressed, as while the menu is
// open, and the menu under it, its end at the button's end. `states` shows items hovered or focused from the keyboard.
function OpenMenu({
  post,
  lang,
  own = false,
  bookmarked = false,
  states = {},
}: {
  post: Post;
  lang: Lang;
  own?: boolean;
  bookmarked?: boolean;
  states?: Record<string, PreviewState>;
}) {
  return (
    <div lang={lang} style={{ display: "grid", gap: rem(menuOffset), gridTemplateColumns: one }}>
      <PostView post={post} shownOn={{ meaning: "more", state: "pressed" }} />
      <div style={{ justifySelf: "end", marginInlineEnd: `calc(-1 * ${tilePadding})` }}>
        <PreviewItems states={states}>
          <MenuPanel label={moreLabels[lang].more}>
            <MoreItems post={post} lang={lang} own={own} bookmarked={bookmarked} />
          </MenuPanel>
        </PreviewItems>
      </div>
    </div>
  );
}

// The more menu open at rest in one scheme and the reader's language: on another's post with one item hovered and
// one focused from the keyboard, and on one's own post, bookmarked; then the boost menu of a boosted post.
function MenuSpecimen({ scheme, lang }: { scheme: Scheme; lang: Lang }) {
  const panel = (children: ReactNode) => (
    <div
      style={{
        alignContent: "start",
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
      {children}
    </div>
  );
  return (
    <>
      {panel(
        <>
          <div style={caption}>
            {scheme} · {lang} · another&apos;s post · open hovered, block focused from the keyboard
          </div>
          <OpenMenu post={posts[1] as Post} lang={lang} states={{ open: "hover", block: "focus-visible" }} />
        </>,
      )}
      {panel(
        <>
          <div style={caption}>
            {scheme} · {lang} · one&apos;s own post, bookmarked
          </div>
          <OpenMenu post={ownPost} lang={lang} own bookmarked />
        </>,
      )}
      {panel(
        <>
          <div style={caption}>
            {scheme} · {lang} · the boost menu of a boosted post, its count and on state kept
          </div>
          <BoostMenu lang={lang} />
        </>,
      )}
    </>
  );
}

// The boost button's menu open at rest, in the reader's `lang`: the button, boosted and with its count, pressed as
// while the menu is open, and the menu under it, its start at the button's start: boost (on) and quote.
function BoostMenu({ lang }: { lang: Lang }) {
  const l = moreLabels[lang];
  return (
    <div lang={lang} style={{ display: "grid", gap: rem(menuOffset), justifyItems: "start" }}>
      <Preview state="pressed">
        <MenuButton icon="boost" label={l.boost} count={1_234} pressed align="start">
          <MenuItem id="boost" icon="boost" on>
            {l.boost}
          </MenuItem>
        </MenuButton>
      </Preview>
      <MenuPanel label={l.boost}>
        <MenuGroup>
          <MenuItem id="boost" icon="boost" on>
            {l.boost}
          </MenuItem>
          <MenuItem id="quote" icon="quote">
            {l.quote}
          </MenuItem>
        </MenuGroup>
      </MenuPanel>
    </div>
  );
}

// A post in a state, with one of its buttons in a state or toggled on, as the states column shows it.
type StateRow = { title: string; shown?: PostState; on?: Done[]; shownOn?: ShownOn };
const stateRows: StateRow[] = [
  { title: "hover", shown: "hover" },
  { title: "focus-visible", shown: "focus-visible" },
  { title: "hover · favourite hovered", shown: "hover", shownOn: { meaning: "favourite", state: "hover" } },
  { title: "hover · favourite pressed", shown: "hover", shownOn: { meaning: "favourite", state: "pressed" } },
  { title: "at rest · reply focused from the keyboard", shownOn: { meaning: "reply", state: "focus-visible" } },
  {
    title: "hover · boosted and favourited, boost hovered",
    shown: "hover",
    on: ["boost", "favourite"],
    shownOn: { meaning: "boost", state: "hover" },
  },
];

// The column: the posts on one surface, separated by space alone, several times the space inside a post; no line,
// no card edge. Below it, one post in each of its states and its buttons' states, shown at rest, then the disclosure,
// the times, the avatars and the glyphs in light and dark. The section follows the page's color scheme. The more menu is open at rest above the
// column, in light and dark; in the column, each more button opens it.
export function Posts() {
  return (
    <div style={{ color: "var(--color-text)", display: "grid", gap: rem(40) }}>
      <IconTable />
      <div id="menu" style={{ display: "flex", flexWrap: "wrap", gap: rem(16) }}>
        <MenuSpecimen scheme="light" lang="en" />
        <MenuSpecimen scheme="dark" lang="ja" />
      </div>
      <div style={column}>
        {posts.map((post) => (
          <PostView key={post.handle} post={post} />
        ))}
      </div>
      <div style={column}>
        {stateRows.map((row) => (
          <div key={row.title} style={{ display: "grid", gap: rem(20), gridTemplateColumns: one }}>
            <h4 style={{ ...caption, margin: 0 }}>{row.title}</h4>
            <PostView post={posts[2] as Post} shown={row.shown} on={row.on} shownOn={row.shownOn} />
          </div>
        ))}
      </div>
      <div id="disclosure" style={{ display: "flex", flexWrap: "wrap", gap: rem(16) }}>
        <DisclosureSpecimen scheme="light" lang="en" />
        <DisclosureSpecimen scheme="dark" lang="ja" />
      </div>
      <div id="time" style={{ display: "flex", flexWrap: "wrap", gap: rem(16) }}>
        <TimeSpecimen scheme="light" />
        <TimeSpecimen scheme="dark" />
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
