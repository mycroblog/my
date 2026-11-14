// SPDX-License-Identifier: AGPL-3.0-only

// The rough post composer (#3046) at /app/compose on the catalog's dev server, to judge the whole flow before it is
// built: each state of the composer over the app's frame and a few sample posts, one specimen after another under a
// catalog label whose id names it (`compose-<state>`, for `web:shot --section`); `?state=<id>` keeps one. The state
// `flow` is the live one: the frame and the posts, where New post (or `n`), a post's Reply and the boost menu's Quote
// open the composer. The language is the page's (`?lang=en|ja`), the scheme the page's.
//
// Leaving the composer: close and Escape discard a written post, and a notice offers to undo it for a moment; a press
// on the backdrop (or, on a phone, the system's back gesture) leaves it by accident, and the post is saved as a
// draft (ADR-0156) without a word. A specimen shows the state it leaves, and Undo, or New post, opens it again.
//
// The software keyboard: on a phone the row that sends the post stays right above it. For real, the page's viewport
// says `interactive-widget=resizes-content`, so that 100dvh ends at the keyboard (Chrome on Android), and the composer
// subtracts `env(keyboard-inset-height)` where the browser overlays the keyboard instead (with the Virtual Keyboard
// API's overlaysContent), or the height the visualViewport loses (Safari on iOS), as --cmp-keyboard. The `keyboard`
// state draws a keyboard of a phone's height and sets --cmp-keyboard to it.

import { keymap } from "@my/app";
import { Post, type PostAction } from "@my/app-components";
import {
  Frame,
  FrameAction,
  FrameAside,
  FrameFloat,
  FrameMain,
  FrameMark,
  FrameSection,
  FrameSide,
  FrameTabBar,
  NavItem,
  NavList,
  ShortcutProvider,
  useAction,
  useLocale,
} from "@my/design-system";
import { namesCommandKey } from "@my/platform";
import { useEffect, useId, useState, type ReactNode } from "react";
import wordmarkDark from "../../../brand/Wordmark-DarkBg.svg";
import wordmarkLight from "../../../brand/Wordmark-Primary-Color.svg";
import { posts, statusOf } from "../timeline/fixtures";
import { Composer, emptyDraft, maxChars, type Draft, type Language, type Leave } from "./Composer";
import { stringsFor } from "./strings";
import "./compose.css";

const params = new URLSearchParams(location.search);
const onlyState = params.get("state");

const ja = (locale: string) => locale.startsWith("ja");

const byId = (id: string) => {
  const post = posts.find((p) => p.id === id);
  if (!post) throw new Error(`no sample post ${id}`);
  return post;
};

// The posts behind the composer, and the ones it replies to and quotes.
const behind = posts.slice(0, 4);
const replied = byId("alice-reply");
const quoted = byId("ravi-quote").quote;
const scenes = posts.flatMap((p) => p.media ?? []);

const words = {
  en: {
    typing: "Walked along the river this morning. The first frost turned every leaf on the path silver, and the",
    long: "Notes from the first month of running a small server, for anyone who is thinking about it. The hardware was the easy part: an old laptop and a backup drive did the job, and the setup took one evening. What took longer was deciding what kind of place it should be. We wrote three rules, read them aloud at the first meetup, and changed two of them a week later. The timeline is slow and that is the point. People reply to each other instead of broadcasting, and nobody has asked for a trending page yet. If you start one, write down why, and keep the rules short.",
    cw: "Spoilers for the season finale",
    cwText: "I did not expect the lighthouse keeper to be the narrator all along. Rewatching the first episode now.",
    reply: "@alice@example.social Nine works. I will bring the thermos.",
    quote: "Bookmarking this before my move next month.",
    images: "The view from the ridge before the fog came in.",
    direct: "@mika@mstdn.example.jp the slides from today are in the shared folder.",
    poll: "Which day works for the next meetup?",
    options: ["Saturday morning", "Saturday evening", "Sunday afternoon"],
    alt: "Low hills under a pale morning sky, a sun just above the ridge",
  },
  ja: {
    typing: "今朝は川沿いを歩いた。初霜で道の葉っぱがぜんぶ銀色になっていて、",
    long: "小さなサーバーを一か月動かしてみての覚え書き。これから立てようと思っている人のために。機材はいちばん簡単だった。古いノートパソコンとバックアップ用のディスクで足りて、設定はひと晩で終わった。時間がかかったのは、どんな場所にしたいかを決めることのほう。ルールを三つ書いて、最初のオフ会で読み上げて、一週間後に二つ書き直した。タイムラインはゆっくり流れるけれど、それでいい。みんな発信するより返事をしていて、トレンドの画面がほしいと言った人はまだいない。もし立てるなら、なぜ立てるのかを書いておくこと。ルールは短く。それから、ひとりで抱えないこと。困ったら手伝ってくれる人は思ったよりいる。サーバーの名前は三回変えたけれど、最初の名前がいちばん好きだったので、結局それに戻した。次は絵文字を増やす予定。みんなが描いた絵をそのまま使えるようにしたい。週末に集まって描く会をやるので、来られる人はぜひ。お茶とお菓子は用意します。場所はいつもの公民館の二階、午後二時から。道に迷ったら連絡してください。サーバー代のチップも少しずつ届いていて、とてもありがたい。来年もこの調子で続けたい。まずは冬を越すところから。寒くなってきたので、みなさんもお気をつけて。ではまた。",
    cw: "最終回のネタバレ",
    cwText: "灯台守がずっと語り手だったとは思わなかった。いま第一話を見直している。",
    reply: "@alice@example.social 9時で大丈夫。水筒を持っていくね。",
    quote: "来月の引っ越し前に読んでおく。",
    images: "霧が出る前の尾根からの眺め。",
    direct: "@mika@mstdn.example.jp 今日のスライドは共有フォルダに入れました。",
    poll: "次のオフ会、どの日がいい？",
    options: ["土曜の午前", "土曜の夜", "日曜の午後"],
    alt: "淡い朝の空の下の低い丘。尾根のすぐ上に太陽",
  },
};

/** A text of `n` characters, cut from the long sample. */
const cut = (text: string, n: number) => Array.from(text.repeat(2)).slice(0, n).join("");

/** How a specimen starts: the composer open with a draft, or left one way or the other. */
type State = { id: string; name: string; draft: Draft; left?: "discarded" | "saved"; keyboard?: boolean };

function states(locale: string): State[] {
  const lang: Language = ja(locale) ? "ja" : "en";
  const w = words[lang];
  const base = emptyDraft(lang);
  const typed = { ...base, text: w.typing };
  const quoting = quoted
    ? { ...base, kind: "quote" as const, quote: { post: quoted, remote: true }, text: w.quote }
    : base;
  return [
    { id: "empty", name: "Empty", draft: base },
    { id: "typing", name: "Typing", draft: typed },
    { id: "keyboard", name: "Typing on a phone, the keyboard up", draft: typed, keyboard: true },
    { id: "near-limit", name: "Near the limit", draft: { ...base, text: cut(w.long, maxChars - 12) } },
    { id: "over-limit", name: "Over the limit", draft: { ...base, text: cut(w.long, maxChars + 8) } },
    { id: "audience", name: "Choosing the audience", draft: { ...typed, pop: "audience" } },
    {
      id: "permissions",
      name: "Who can reply, boost and quote",
      draft: { ...typed, policy: { reply: "everyone", boost: "followers", quote: "nobody" }, pop: "permissions" },
    },
    { id: "visibility-local", name: "Local", draft: { ...typed, visibility: "local" } },
    { id: "visibility-unlisted", name: "Quiet public", draft: { ...typed, visibility: "unlisted" } },
    { id: "visibility-private", name: "Followers only", draft: { ...typed, visibility: "private" } },
    { id: "visibility-direct", name: "Direct", draft: { ...base, text: w.direct, visibility: "direct" } },
    { id: "cw", name: "Content warning", draft: { ...base, cw: w.cw, text: w.cwText } },
    {
      id: "images",
      name: "Images",
      draft: {
        ...base,
        text: w.images,
        images: [
          { src: scenes[0]?.src ?? "", alt: w.alt },
          { src: scenes[1]?.src ?? "", alt: "" },
          { src: scenes[2]?.src ?? "", alt: "", uploading: true },
        ],
      },
    },
    {
      id: "images-required",
      name: "A description missing, which the server asks for",
      draft: {
        ...base,
        text: w.images,
        images: [
          { src: scenes[0]?.src ?? "", alt: w.alt },
          { src: scenes[1]?.src ?? "", alt: "" },
        ],
        descriptionsRequired: true,
        checked: true,
      },
    },
    { id: "reply", name: "Reply", draft: { ...base, kind: "reply", replyTo: replied, text: w.reply } },
    { id: "quote", name: "Quote", draft: quoting },
    { id: "quote-audience", name: "Quote, choosing the audience", draft: { ...quoting, pop: "audience" } },
    { id: "language", name: "Language", draft: { ...typed, pop: "language" } },
    {
      id: "poll",
      name: "Poll",
      draft: { ...base, text: w.poll, poll: { options: w.options, duration: 3, multiple: false, hideTotals: true } },
    },
    {
      id: "poll-multiple",
      name: "Poll, several choices",
      draft: { ...base, text: w.poll, poll: { options: w.options, duration: 3, multiple: true, hideTotals: false } },
    },
    { id: "submitting", name: "Submitting", draft: { ...typed, phase: "submitting" } },
    { id: "failed", name: "Failed", draft: { ...typed, phase: "failed", failNext: true } },
    { id: "discarded", name: "Closed with × or Escape: discarded, with Undo", draft: typed, left: "discarded" },
    { id: "saved", name: "Left by the backdrop: saved as a draft, silently", draft: typed, left: "saved" },
  ];
}

/** The places of the app's navigation, as the shell lists them. */
const places = [
  { href: "#home", icon: "home", en: "Home", ja: "ホーム" },
  { href: "#notifications", icon: "notifications", en: "Notifications", ja: "通知" },
  { href: "#search", icon: "search", en: "Search", ja: "検索" },
  { href: "#profile", icon: "profile", en: "Profile", ja: "プロフィール" },
  { href: "#settings", icon: "settings", en: "Settings", ja: "設定" },
] as const;

function Places({ lang }: { lang: "en" | "ja" }) {
  return (
    <NavList>
      {places.map((place, i) => (
        <NavItem key={place.href} href={place.href} icon={place.icon} label={place[lang]} current={i === 0} />
      ))}
    </NavList>
  );
}

/** The app's frame around the sample posts, as the shell places it; New post and `n` call `onCompose`. */
function Shell({ onCompose, onAction }: { onCompose: () => void; onAction?: (action: PostAction) => void }) {
  const lang = ja(useLocale().locale) ? "ja" : "en";
  const label = lang === "ja" ? "投稿する" : "New post";
  const keys = useAction("compose.open", onCompose, { scope: "screen", label });
  const action = <FrameAction icon="compose" label={label} onPress={onCompose} {...keys} />;
  const aboutId = useId();
  return (
    <Frame>
      <FrameSide label="Main">
        <FrameMark href="#home" label="mycroblog" light={wordmarkLight} dark={wordmarkDark} />
        {action}
        <Places lang={lang} />
      </FrameSide>
      <FrameMain>
        <div className="cmp-feed">
          {behind.map((post) => (
            <Post key={post.id} status={statusOf(post, post.id)} own={post.own} onAction={onAction} />
          ))}
        </div>
      </FrameMain>
      <FrameAside label={lang === "ja" ? "まわりのこと" : "Around you"}>
        <FrameSection id={aboutId} icon="about" href="#about" heading="my.example">
          <p>{lang === "ja" ? "小さなコミュニティのためのサーバー。" : "A server for a small community."}</p>
        </FrameSection>
      </FrameAside>
      <FrameTabBar label="Main">
        <Places lang={lang} />
      </FrameTabBar>
      <FrameFloat>{action}</FrameFloat>
    </Frame>
  );
}

/** The notice of a discarded post, for a moment, with Undo; neutral, since coral is not for it. */
function Discarded({ onUndo, live }: { onUndo: () => void; live: boolean }) {
  const s = stringsFor(useLocale().locale);
  return (
    <div className="cmp-notice" role="status" data-live={live ? "" : undefined}>
      <span>{s.discarded}</span>
      <button type="button" className="cmp-notice-action" onClick={onUndo}>
        {s.undo}
      </button>
    </div>
  );
}

/**
 * The composer over the frame, and what leaving it does: a discard shows the notice with Undo, and a press on the
 * backdrop saves the draft and closes. `live` lays it over the viewport (the flow); else over its specimen.
 */
function Stage({
  initial,
  left,
  live,
  onAction,
  keyboard = false,
}: {
  initial: Draft | undefined;
  left?: "discarded" | "saved" | undefined;
  live: boolean;
  onAction?: (open: (d: Draft) => void) => (action: PostAction) => void;
  keyboard?: boolean;
}) {
  const lang: Language = ja(useLocale().locale) ? "ja" : "en";
  const [open, setOpen] = useState<Draft | undefined>(left === undefined ? initial : undefined);
  const [discarded, setDiscarded] = useState<Draft | undefined>(left === "discarded" ? initial : undefined);
  const [opened, setOpened] = useState(0);
  const show = (d: Draft) => {
    setOpened((n) => n + 1);
    setDiscarded(undefined);
    setOpen(d);
  };
  // The notice goes after a moment in the flow; a specimen keeps it, to be seen.
  useEffect(() => {
    if (!live || discarded === undefined) return;
    const t = setTimeout(() => {
      setDiscarded(undefined);
    }, 6000);
    return () => {
      clearTimeout(t);
    };
  }, [live, discarded]);
  const onLeave = (how: Leave, d: Draft) => {
    setOpen(undefined);
    if (how === "discarded") setDiscarded(d);
  };
  const shell = (
    <Shell
      onCompose={() => {
        show(emptyDraft(lang));
      }}
      {...(onAction ? { onAction: onAction(show) } : {})}
    />
  );
  return (
    <div className="cmp-stage" data-live={live ? "" : undefined} data-keyboard={keyboard ? "" : undefined}>
      {live ? (
        shell
      ) : (
        <div className="cmp-behind" data-open={open ? "" : undefined} inert={open !== undefined}>
          {shell}
        </div>
      )}
      {open && (
        <div
          className="cmp-layer"
          data-live={live ? "" : undefined}
          onPointerDown={(e) => {
            // A press on the backdrop, outside the composer, leaves it by accident: the post is kept as a draft.
            if (e.target === e.currentTarget) setOpen(undefined);
          }}
        >
          <Composer key={opened} initial={open} onLeave={onLeave} />
        </div>
      )}
      {keyboard && open && <div className="cmp-keyboard" aria-hidden="true" />}
      {discarded && (
        <Discarded
          live={live}
          onUndo={() => {
            show(discarded);
          }}
        />
      )}
    </div>
  );
}

/** One state, under its label. */
function Specimen({ state }: { state: State }) {
  const id = `compose-${state.id}`;
  return (
    <section aria-labelledby={id}>
      <h2 id={id} className="cmp-specimen-label">
        <a href={`?${new URLSearchParams({ ...Object.fromEntries(params), state: state.id }).toString()}`}>
          {state.name}
        </a>
      </h2>
      <Stage initial={state.draft} left={state.left} live={false} keyboard={state.keyboard === true} />
    </section>
  );
}

/** The live flow: the frame and the posts, where New post, Reply and Quote open the composer over them. */
function Flow() {
  const lang: Language = ja(useLocale().locale) ? "ja" : "en";
  return (
    <Stage
      initial={undefined}
      live
      onAction={(open) => (action) => {
        const post = posts.find((p) => p.id === action.id);
        if (!post) return;
        if (action.type === "reply")
          open({ ...emptyDraft(lang), kind: "reply", replyTo: post, text: `${post.author.handle} ` });
        if (action.type === "quote")
          open({
            ...emptyDraft(lang),
            kind: "quote",
            quote: { post, remote: !post.author.handle.endsWith("@my.example") },
          });
      }}
    />
  );
}

function Index() {
  const { locale } = useLocale();
  const href = (state?: string) => {
    const next = new URLSearchParams(location.search);
    if (state) next.set("state", state);
    else next.delete("state");
    return `?${next.toString()}`;
  };
  return (
    <nav className="cmp-index" aria-label="Composer states">
      <a href={href()} aria-current={onlyState ? undefined : "page"}>
        All
      </a>
      <a href={href("flow")} aria-current={onlyState === "flow" ? "page" : undefined}>
        Live flow
      </a>
      {states(locale).map((state) => (
        <a key={state.id} href={href(state.id)} aria-current={state.id === onlyState ? "page" : undefined}>
          {state.name}
        </a>
      ))}
    </nav>
  );
}

function Specimens() {
  const { locale } = useLocale();
  return states(locale)
    .filter((state) => !onlyState || state.id === onlyState)
    .map((state) => <Specimen key={state.id} state={state} />);
}

export function ComposePreview(): ReactNode {
  return (
    <ShortcutProvider keymap={keymap} mac={namesCommandKey()}>
      {onlyState === "flow" ? (
        <Flow />
      ) : (
        <>
          <Index />
          <Specimens />
        </>
      )}
    </ShortcutProvider>
  );
}
