// SPDX-License-Identifier: AGPL-3.0-only

// The post composer of @my/app-features at /app/compose on the catalog's dev server, on a fake server instead of a live
// one: each state of it in the flow of the page, one specimen after another under a catalog label whose id names it
// (`compose-<state>`, for `web:shot --section`), and `?state=<id>` keeps one; `?state=flow` is the app itself, where
// New post (or `n`), a post's Reply and the boost menu's Quote open the composer over the home timeline, and what is
// posted tops it. The language is the page's (`?lang=en|ja`), the scheme the page's.
//
// The fake server takes posts, uploads (a file whose name has "fail" is refused, so that the retry shows; the others
// are processed for a moment), and drafts; a post whose text has "#fail" is refused with the server's words, so that a
// failed post shows. Leaving the composer: close and Escape discard a written post, with Undo; the backdrop saves it as
// a draft, which New post opens again.

import { AppRoot, createAppRouter } from "@my/app";
import { ComposeLayer, emptyComposeDraft, type ComposeDraft, type ComposeTarget } from "@my/app-features";
import { AppDataProvider, createAppData, defaultComposeSettings, type Status } from "@my/app-data";
import { useLocale } from "@my/design-system";
import { createClient } from "@mycroblog/mykit";
import type { ReactNode } from "react";
import { posts, statusOf } from "../timeline/fixtures";

const params = new URLSearchParams(location.search);
const onlyState = params.get("state");

const server = "https://my.example";
const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": status >= 400 ? "application/problem+json" : "application/json", ETag: '"1"' },
  });

// What the fake server holds, newest first: the posts posted on it, then the samples.
const served = posts.map((post) => statusOf(post, post.id) as unknown as Status);
const uploads = new Map<string, { url: string; ready: number; description?: string }>();
let made = 0;

/** A file's metadata, centered. */
const meta = { focus: { x: 0, y: 0 } };

const escape = (s: string) => s.replace(/[&<>"]/g, (c) => `&#${String(c.charCodeAt(0))};`);

/** The status a post makes, from the reader's sample post and what was sent. */
function posted(body: Record<string, unknown>): Status {
  const str = (v: unknown, fallback = "") => (typeof v === "string" ? v : fallback);
  made += 1;
  const base = served.find((s) => s.id === "own") ?? served[0];
  if (!base) throw new Error("the catalog has no sample posts");
  const media = (body["media"] as { id: string; description?: string }[] | undefined) ?? [];
  const quoted = served.find((s) => s.id === body["quoted_status_id"]);
  return {
    ...base,
    id: `posted-${String(made)}`,
    created_at: new Date().toISOString(),
    content: str(body["text"])
      .split("\n")
      .map((line) => `<p>${escape(line)}</p>`)
      .join(""),
    spoiler_text: str(body["spoiler_text"]),
    visibility: body["visibility"] as Status["visibility"],
    language: str(body["language"], "en"),
    media_attachments: media.map((m) => {
      const url = uploads.get(m.id)?.url ?? "";
      return {
        id: m.id,
        type: "image",
        state: "ready",
        meta,
        url,
        preview_url: url,
        ...(m.description ? { description: m.description } : {}),
      };
    }),
    replies_count: 0,
    boosts_count: 0,
    favourites_count: 0,
    reactions: [],
    favourited: false,
    boosted: false,
    ...(typeof body["in_reply_to_id"] === "string" ? { in_reply_to_id: body["in_reply_to_id"] } : {}),
    ...(quoted ? { quote: { state: "accepted", quoted_status: quoted } } : { quote: undefined }),
  } as Status;
}

const refused = (detail: string) =>
  json({ type: "/api/v1/problems/status-rejected", title: "Unprocessable Content", status: 422, detail }, 422);

/** The fake server. */
async function serve(request: Request): Promise<Response> {
  const url = new URL(request.url);
  const path = url.pathname.replace("/api/v1", "");
  const method = request.method;
  if (path === "/timelines/home") {
    const cursor = url.searchParams.get("cursor");
    const from = cursor === null ? 0 : served.findIndex((s) => s.id === cursor) + 1;
    const items = served.slice(from, from + 6);
    const last = items.at(-1);
    return json({ items, ...(last && from + 6 < served.length ? { next_cursor: last.id } : {}) });
  }
  if (path === "/preferences")
    return json({
      default_visibility: "public",
      default_interaction_policy: {
        reply: { automatic: "everyone" },
        announce: { automatic: "everyone" },
        quote: { automatic: "everyone" },
      },
      default_post_language: "und",
      require_media_description: false,
    });
  if (path === "/instance")
    return json({ domain: "my.example", limits: { max_post_characters: 500, require_media_description: false } });
  if (path === "/accounts/verify_credentials") return json({ id: "kumi", acct: "kumi" });
  if (path === "/statuses" && method === "POST") {
    const body = (await request.json()) as Record<string, unknown>;
    await new Promise((resolve) => setTimeout(resolve, 800));
    if (typeof body["text"] === "string" && body["text"].includes("#fail"))
      return refused(document.documentElement.lang.startsWith("ja") ? "投稿が断られました。" : "The post was refused.");
    const status = posted(body);
    served.unshift(status);
    return json(status, 201);
  }
  const status = /^\/statuses\/([^/]+)$/.exec(path);
  if (status && method === "GET") {
    const found = served.find((s) => s.id === status[1]);
    return found ? json(found) : json({ type: "about:blank", title: "Not Found", status: 404 }, 404);
  }
  if (path === "/media" && method === "POST") {
    const file = (await request.formData()).get("file");
    if (!(file instanceof File) || file.name.includes("fail"))
      return json({ type: "about:blank", title: "Unsupported Media Type", status: 415 }, 415);
    made += 1;
    const id = `media-${String(made)}`;
    uploads.set(id, { url: URL.createObjectURL(file), ready: Date.now() + 1500 });
    return json({ id, type: "image", state: "processing", meta }, 202);
  }
  const media = /^\/media\/([^/]+)$/.exec(path);
  if (media?.[1]) {
    const upload = uploads.get(media[1]);
    if (!upload) return json({ type: "about:blank", title: "Not Found", status: 404 }, 404);
    const ready = Date.now() >= upload.ready;
    return json({
      id: media[1],
      type: "image",
      state: ready ? "ready" : "processing",
      meta,
      ...(ready ? { url: upload.url, preview_url: upload.url } : {}),
    });
  }
  if (path === "/drafts" && method === "POST") return json({ id: "draft-1", visibility: "public" }, 201);
  if (path.startsWith("/drafts/") && method === "PUT") return json({ id: path.slice("/drafts/".length) });
  if (path.startsWith("/drafts/") && method === "DELETE") return new Response(null, { status: 204 });
  return json({ type: "about:blank", title: "Not Found", status: 404 }, 404);
}

const data = createAppData(createClient({ server, retry: { disabled: true }, fetch: serve }));

const words = {
  en: {
    typing: "Walked along the river this morning. The first frost turned every leaf on the path silver, and the",
    long: "Notes from the first month of running a small server, for anyone who is thinking about it. The hardware was the easy part: an old laptop and a backup drive did the job, and the setup took one evening. What took longer was deciding what kind of place it should be. We wrote three rules, read them aloud at the first meetup, and changed two of them a week later. The timeline is slow and that is the point. People reply to each other instead of broadcasting, and nobody has asked for a trending page yet. If you start one, write down why, and keep the rules short.",
    cw: "Spoilers for the season finale",
    cwText: "I did not expect the lighthouse keeper to be the narrator all along. Rewatching the first episode now.",
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

const scenes = posts.flatMap((p) => p.media ?? []).map((m) => m.src);
const image = (n: number, state: "ready" | "uploading" | "processing" | "failed", description = "") => ({
  key: `sample-${String(n)}`,
  preview: scenes[n % scenes.length],
  state,
  description,
  ...(state === "failed" ? { file: new File([], "photo.jpg") } : {}),
  ...(state === "ready"
    ? { media: { id: `sample-${String(n)}`, type: "image" as const, state: "ready" as const, meta } }
    : {}),
});

type State = { id: string; name: string; target: ComposeTarget; draft: (base: ComposeDraft) => ComposeDraft };

function states(locale: string): State[] {
  const w = locale.startsWith("ja") ? words.ja : words.en;
  const typed = (d: ComposeDraft) => ({ ...d, text: w.typing });
  const poll = (multiple: boolean) => (d: ComposeDraft) => ({
    ...d,
    text: w.poll,
    poll: { options: w.options, duration: 259200, multiple, hideTotals: !multiple },
  });
  const nw: ComposeTarget = { kind: "new" };
  return [
    { id: "empty", name: "Empty", target: nw, draft: (d) => d },
    { id: "typing", name: "Typing", target: nw, draft: typed },
    { id: "near-limit", name: "Near the limit", target: nw, draft: (d) => ({ ...d, text: cut(w.long, 488) }) },
    { id: "over-limit", name: "Over the limit", target: nw, draft: (d) => ({ ...d, text: cut(w.long, 508) }) },
    { id: "visibility-local", name: "Local", target: nw, draft: (d) => ({ ...typed(d), visibility: "local" }) },
    {
      id: "visibility-private",
      name: "Followers only",
      target: nw,
      draft: (d) => ({ ...typed(d), visibility: "private" }),
    },
    {
      id: "permissions",
      name: "Replies, boosts or quotes limited",
      target: nw,
      draft: (d) => ({ ...typed(d), policy: { reply: "everyone", announce: "followers", quote: "nobody" } }),
    },
    {
      id: "visibility-direct",
      name: "Direct",
      target: nw,
      draft: (d) => ({ ...d, text: w.direct, visibility: "direct" }),
    },
    { id: "cw", name: "Content warning", target: nw, draft: (d) => ({ ...d, cw: w.cw, text: w.cwText }) },
    {
      id: "images",
      name: "Images: described, not yet, processing, failed",
      target: nw,
      draft: (d) => ({
        ...d,
        text: w.images,
        attachments: [image(0, "ready", w.alt), image(1, "ready"), image(2, "processing"), image(3, "failed")],
      }),
    },
    { id: "reply", name: "Reply", target: { kind: "reply", id: "alice-reply" }, draft: (d) => d },
    {
      id: "quote",
      name: "Quote of a post on another server",
      target: { kind: "quote", id: "mika-long" },
      draft: (d) => ({ ...d, text: w.quote }),
    },
    { id: "poll", name: "Poll", target: nw, draft: poll(false) },
    { id: "poll-multiple", name: "Poll, several choices", target: nw, draft: poll(true) },
  ];
}

/** One state, under its label, in the flow of the page. */
function Specimen({ state, locale }: { state: State; locale: string }) {
  const id = `compose-${state.id}`;
  const lang = locale.startsWith("ja") ? "ja" : "en";
  const initial = state.draft({ ...emptyComposeDraft(state.target, defaultComposeSettings, id), language: lang });
  return (
    <section aria-labelledby={id} style={{ maxInlineSize: "37.5rem", marginInline: "auto", paddingBlockEnd: "1rem" }}>
      <h2 id={id} style={label}>
        <a
          href={`?${new URLSearchParams({ ...Object.fromEntries(params), state: state.id }).toString()}`}
          style={{ color: "inherit" }}
        >
          {state.name}
        </a>
      </h2>
      <ComposeLayer target={state.target} initial={initial} inline onOpen={() => undefined} onClose={() => undefined} />
    </section>
  );
}

// The catalog's chrome, not the composer's: a label over each specimen, and the index of the states.
const label = {
  margin: 0,
  padding: "0.25rem 1rem",
  borderBlockStart: "1px dashed var(--color-muted-text)",
  color: "var(--color-muted-text)",
  fontFamily: "ui-monospace, monospace",
  fontSize: "var(--text-caption)",
  fontWeight: 400,
} as const;

function Index({ locale }: { locale: string }) {
  const href = (state?: string) => {
    const next = new URLSearchParams(location.search);
    if (state) next.set("state", state);
    else next.delete("state");
    return `?${next.toString()}`;
  };
  const link = (state: string | undefined, name: string) => (
    <a
      key={state ?? "all"}
      href={href(state)}
      aria-current={state === (onlyState ?? undefined) ? "page" : undefined}
      style={{ color: "inherit" }}
    >
      {name}
    </a>
  );
  return (
    <nav
      aria-label="Composer states"
      style={{
        display: "flex",
        flexWrap: "wrap",
        gap: "0.25rem 0.75rem",
        padding: "0.75rem 1rem",
        color: "var(--color-muted-text)",
        fontFamily: "var(--font-sans)",
        fontSize: "var(--text-caption)",
      }}
    >
      {link(undefined, "All")}
      {link("flow", "Live flow")}
      {states(locale).map((s) => link(s.id, s.name))}
    </nav>
  );
}

const router = createAppRouter({
  data,
  config: { apiOrigin: server, instanceName: "my.example", defaultLocale: "en", clientId: "my-web" },
  at: "/",
});

export function ComposePreview(): ReactNode {
  const { locale } = useLocale();
  if (onlyState === "flow") return <AppRoot router={router} />;
  return (
    <AppDataProvider data={data}>
      <div style={{ background: "var(--color-surface)", minBlockSize: "100dvh" }}>
        <Index locale={locale} />
        {states(locale)
          .filter((s) => !onlyState || s.id === onlyState)
          .map((s) => (
            <Specimen key={s.id} state={s} locale={locale} />
          ))}
      </div>
    </AppDataProvider>
  );
}
