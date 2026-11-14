// SPDX-License-Identifier: AGPL-3.0-only

// The post composer of @my/app-features: a story per state of it, in the flow of the page; and the live flow, the app
// itself, where New post (or `n`), a post's Reply and the boost menu's Quote open the composer over the home timeline,
// and what is posted tops it. The language is the toolbar's locale (ja, else en).
//
// The server is the fake of @my/e2e, which answers the reads from the scenario; this file's own fake takes the writes:
// posts, uploads (a file whose name has "fail" is refused, so that the retry shows; the others are processed for a
// moment) and drafts; a post whose text has "#fail" is refused with the server's words, so that a failed post shows.
// Leaving the composer: close and Escape discard a written post, with Undo; the backdrop saves it as a draft, which New
// post opens again.

import { AppDataProvider, createAppData, defaultComposeSettings, type Status } from "@my/app-data";
import { ComposeLayer, emptyComposeDraft, type ComposeDraft, type ComposeTarget } from "@my/app-features";
import { useLocale } from "@my/design-system";
import { images } from "@my/design-system/samples";
import { createFakeApi, fakeHost, fakeOrigin, type FakeApi } from "@my/e2e/fake";
import { createClient } from "@mycroblog/mykit";
import type { Meta, StoryContext, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { waitFor } from "storybook/test";
import { AppRoot } from "./app";
import { minimumWidth, rightToLeft } from "./app.samples";
import { createAppRouter } from "./router";

const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": status >= 400 ? "application/problem+json" : "application/json", ETag: '"1"' },
  });
const notFound = () => json({ type: "about:blank", title: "Not Found", status: 404 }, 404);
const refused = (detail: string) =>
  json({ type: "/api/v1/problems/status-rejected", title: "Unprocessable Content", status: 422, detail }, 422);

/** A file's metadata, centered. */
const centered = { focus: { x: 0, y: 0 } };

const escape = (s: string) => s.replace(/[&<>"]/g, (c) => `&#${String(c.charCodeAt(0))};`);
const str = (v: unknown, fallback = "") => (typeof v === "string" ? v : fallback);

/** The fake of @my/e2e, with the composer's writes kept in memory and its settings answered. */
function composeFake(fake: FakeApi): (request: Request) => Promise<Response> {
  const uploads = new Map<string, { url: string; ready: number }>();
  const posted: Status[] = [];
  let made = 0;

  const read = async (path: string): Promise<unknown> =>
    (await fake.serve(new Request(`${fakeOrigin}/api/v1${path}`))).json();

  /** The status a post makes, from a status of the home timeline and what was sent. */
  async function post(body: Record<string, unknown>): Promise<Status> {
    made += 1;
    const home = (await read("/timelines/home")) as { items: Status[] };
    const me = (await read("/accounts/verify_credentials")) as Status["account"];
    const base = home.items.find((s) => s.boost === undefined);
    if (base === undefined) throw new Error("the fake's home timeline holds no post of its own");
    const media = (body["media"] as { id: string; description?: string }[] | undefined) ?? [];
    const quotedId = str(body["quoted_status_id"]);
    const quoted = quotedId === "" ? undefined : ((await read(`/statuses/${quotedId}`)) as Status);
    return {
      ...base,
      id: `posted-${String(made)}`,
      account: me,
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
          meta: centered,
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
      quote: quoted === undefined ? undefined : { state: "accepted", quoted_status: quoted },
    } as Status;
  }

  return async (request) => {
    const url = new URL(request.url);
    const path = url.pathname.replace("/api/v1", "");
    const method = request.method;
    if (path === "/timelines/home" && url.searchParams.get("cursor") === null && posted.length > 0) {
      const page = (await (await fake.serve(request)).json()) as { items: Status[] };
      return json({ ...page, items: [...posted, ...page.items] });
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
      return json({ domain: fakeHost, limits: { max_post_characters: 500, require_media_description: false } });
    if (path === "/statuses" && method === "POST") {
      const body = (await request.json()) as Record<string, unknown>;
      await new Promise((resolve) => setTimeout(resolve, 800));
      if (str(body["text"]).includes("#fail"))
        return refused(
          document.documentElement.lang.startsWith("ja") ? "投稿が断られました。" : "The post was refused.",
        );
      const status = await post(body);
      posted.unshift(status);
      return json(status, 201);
    }
    const own = /^\/statuses\/(posted-[^/]+)$/.exec(path);
    if (own && method === "GET") {
      const found = posted.find((s) => s.id === own[1]);
      return found ? json(found) : notFound();
    }
    if (path === "/media" && method === "POST") {
      const file = (await request.formData()).get("file");
      if (!(file instanceof File) || file.name.includes("fail"))
        return json({ type: "about:blank", title: "Unsupported Media Type", status: 415 }, 415);
      made += 1;
      const id = `media-${String(made)}`;
      uploads.set(id, { url: URL.createObjectURL(file), ready: Date.now() + 1500 });
      return json({ id, type: "image", state: "processing", meta: centered }, 202);
    }
    const media = /^\/media\/([^/]+)$/.exec(path);
    if (media?.[1]) {
      const upload = uploads.get(media[1]);
      if (!upload) return notFound();
      const ready = Date.now() >= upload.ready;
      return json({
        id: media[1],
        type: "image",
        state: ready ? "ready" : "processing",
        meta: centered,
        ...(ready ? { url: upload.url, preview_url: upload.url } : {}),
      });
    }
    if (path === "/drafts" && method === "POST") return json({ id: "draft-1", visibility: "public" }, 201);
    if (path.startsWith("/drafts/") && method === "PUT") return json({ id: path.slice("/drafts/".length) });
    if (path.startsWith("/drafts/") && method === "DELETE") return new Response(null, { status: 204 });
    return fake.serve(request);
  };
}

/** A new app on a new fake server, as the composer's stories use it. */
function createComposeApp() {
  const fake = createFakeApi();
  const data = createAppData(createClient({ server: fakeOrigin, retry: { disabled: true }, fetch: composeFake(fake) }));
  return { fake, data };
}

const words = {
  en: {
    typing: "Walked along the river this morning. The first frost turned every leaf on the path silver, and the",
    long: "Notes from the first month of running a small server, for anyone who is thinking about it. The hardware was the easy part: an old laptop and a backup drive did the job, and the setup took one evening. What took longer was deciding what kind of place it should be. We wrote three rules, read them aloud at the first meetup, and changed two of them a week later. The timeline is slow and that is the point. People reply to each other instead of broadcasting, and nobody has asked for a trending page yet. If you start one, write down why, and keep the rules short.",
    cw: "Spoilers for the season finale",
    cwText: "I did not expect the lighthouse keeper to be the narrator all along. Rewatching the first episode now.",
    quote: "Bookmarking this before my move next month.",
    images: "The view from the ridge before the fog came in.",
    direct: "@sam the slides from today are in the shared folder.",
    poll: "Which day works for the next meetup?",
    options: ["Saturday morning", "Saturday evening", "Sunday afternoon"],
    alt: "Low hills under a dusk sky, a sun just above the ridge",
  },
  ja: {
    typing: "今朝は川沿いを歩いた。初霜で道の葉っぱがぜんぶ銀色になっていて、",
    long: "小さなサーバーを一か月動かしてみての覚え書き。これから立てようと思っている人のために。機材はいちばん簡単だった。古いノートパソコンとバックアップ用のディスクで足りて、設定はひと晩で終わった。時間がかかったのは、どんな場所にしたいかを決めることのほう。ルールを三つ書いて、最初のオフ会で読み上げて、一週間後に二つ書き直した。タイムラインはゆっくり流れるけれど、それでいい。みんな発信するより返事をしていて、トレンドの画面がほしいと言った人はまだいない。もし立てるなら、なぜ立てるのかを書いておくこと。ルールは短く。それから、ひとりで抱えないこと。困ったら手伝ってくれる人は思ったよりいる。サーバーの名前は三回変えたけれど、最初の名前がいちばん好きだったので、結局それに戻した。次は絵文字を増やす予定。みんなが描いた絵をそのまま使えるようにしたい。週末に集まって描く会をやるので、来られる人はぜひ。お茶とお菓子は用意します。場所はいつもの公民館の二階、午後二時から。道に迷ったら連絡してください。サーバー代のチップも少しずつ届いていて、とてもありがたい。来年もこの調子で続けたい。まずは冬を越すところから。寒くなってきたので、みなさんもお気をつけて。ではまた。",
    cw: "最終回のネタバレ",
    cwText: "灯台守がずっと語り手だったとは思わなかった。いま第一話を見直している。",
    quote: "来月の引っ越し前に読んでおく。",
    images: "霧が出る前の尾根からの眺め。",
    direct: "@sam 今日のスライドは共有フォルダに入れました。",
    poll: "次のオフ会、どの日がいい？",
    options: ["土曜の午前", "土曜の夜", "日曜の午後"],
    alt: "夕暮れの空の下の低い丘。尾根のすぐ上に太陽",
  },
};

/** A text of `n` characters, cut from the long sample. */
const cut = (text: string, n: number) => Array.from(text.repeat(2)).slice(0, n).join("");

const scenes = [images.dusk, images.snow, images.logo];
const image = (n: number, state: "ready" | "uploading" | "processing" | "failed", description = "") => ({
  key: `sample-${String(n)}`,
  preview: scenes[n % scenes.length],
  state,
  description,
  ...(state === "failed" ? { file: new File([], "photo.jpg") } : {}),
  ...(state === "ready"
    ? { media: { id: `sample-${String(n)}`, type: "image" as const, state: "ready" as const, meta: centered } }
    : {}),
});

/** The ID of the scenario post with `key`, from the app's address of it. */
const idOf = (fake: FakeApi, key: string) => fake.postPath(key).split("/").at(-1) ?? "";

type State = {
  name: string;
  target: (fake: FakeApi) => ComposeTarget;
  draft: (base: ComposeDraft, w: (typeof words)["en"]) => ComposeDraft;
};

const nw = (): ComposeTarget => ({ kind: "new" });
const typed = (d: ComposeDraft, w: (typeof words)["en"]) => ({ ...d, text: w.typing });
const poll = (multiple: boolean) => (d: ComposeDraft, w: (typeof words)["en"]) => ({
  ...d,
  text: w.poll,
  poll: { options: w.options, duration: 259200, multiple, hideTotals: !multiple },
});

const states = {
  empty: { name: "Empty", target: nw, draft: (d) => d },
  typing: { name: "Typing", target: nw, draft: typed },
  "near-limit": { name: "Near the limit", target: nw, draft: (d, w) => ({ ...d, text: cut(w.long, 488) }) },
  "over-limit": { name: "Over the limit", target: nw, draft: (d, w) => ({ ...d, text: cut(w.long, 508) }) },
  "visibility-local": { name: "Local", target: nw, draft: (d, w) => ({ ...typed(d, w), visibility: "local" }) },
  "visibility-private": {
    name: "Followers only",
    target: nw,
    draft: (d, w) => ({ ...typed(d, w), visibility: "private" }),
  },
  permissions: {
    name: "Replies, boosts or quotes limited",
    target: nw,
    draft: (d, w) => ({ ...typed(d, w), policy: { reply: "everyone", announce: "followers", quote: "nobody" } }),
  },
  "visibility-direct": {
    name: "Direct",
    target: nw,
    draft: (d, w) => ({ ...d, text: w.direct, visibility: "direct" }),
  },
  cw: { name: "Content warning", target: nw, draft: (d, w) => ({ ...d, cw: w.cw, text: w.cwText }) },
  images: {
    name: "Images: described, not yet, processing, failed",
    target: nw,
    draft: (d, w) => ({
      ...d,
      text: w.images,
      attachments: [image(0, "ready", w.alt), image(1, "ready"), image(2, "processing"), image(3, "failed")],
    }),
  },
  reply: { name: "Reply", target: (fake) => ({ kind: "reply", id: idOf(fake, "sam-bread") }), draft: (d) => d },
  quote: {
    name: "Quote",
    target: (fake) => ({ kind: "quote", id: idOf(fake, "layla-jokes") }),
    draft: (d, w) => ({ ...d, text: w.quote }),
  },
  poll: { name: "Poll", target: nw, draft: poll(false) },
  "poll-multiple": { name: "Poll, several choices", target: nw, draft: poll(true) },
} satisfies Record<string, State>;

type StateId = keyof typeof states;

/** One state of the composer in the flow of the page; `text`, when given, is the text written in it. */
function Specimen({ id, text }: { id: StateId; text: string }) {
  const { locale } = useLocale();
  const [{ fake, data }] = useState(createComposeApp);
  const state: State = states[id];
  const lang = locale.startsWith("ja") ? "ja" : "en";
  const target = state.target(fake);
  const drafted = state.draft(
    { ...emptyComposeDraft(target, defaultComposeSettings, `compose-${id}`), language: lang },
    words[lang],
  );
  const initial = text === "" ? drafted : { ...drafted, text };
  return (
    <AppDataProvider data={data}>
      <div style={{ maxInlineSize: "37.5rem", marginInline: "auto" }}>
        <ComposeLayer target={target} initial={initial} inline onOpen={() => undefined} onClose={() => undefined} />
      </div>
    </AppDataProvider>
  );
}

/** The app itself, on the composer's fake server, at the home timeline. */
function Flow() {
  const [router] = useState(() =>
    createAppRouter({
      data: createComposeApp().data,
      config: { apiOrigin: fakeOrigin, instanceName: fakeHost, defaultLocale: "en", clientId: "my-web" },
      at: "/",
    }),
  );
  return <AppRoot router={router} />;
}

/**
 * Waits until the composer shows what the fake server answers: its text field, and the post a reply or a quote is
 * of; so that the story tests check it as the reader sees it.
 */
const settle = async ({ canvasElement, parameters }: StoryContext) => {
  const state = composeOf(parameters);
  // The handle of the author of the post a reply or a quote is of (sam-bread, layla-jokes), which shows once it came.
  const of = state === "reply" ? "@sam" : state === "quote" ? "@layla" : "";
  await waitFor(
    () => {
      const ready = canvasElement.querySelector('[role="textbox"], textarea') !== null;
      if (!ready || !canvasElement.textContent.includes(of))
        throw new Error(`the composer does not show its ${state} state yet`);
    },
    { timeout: 5000 },
  );
};

/** The state of the composer a story shows, from its parameters. */
const composeOf = (parameters: StoryContext["parameters"]): StateId =>
  ((parameters as { compose?: StateId }).compose ?? "empty") satisfies StateId;

/** A story of the composer in `id`. */
const shows = (id: StateId): Story => ({ parameters: { compose: id } });

/** Waits until the app shows the home timeline's posts. */
const settleFlow = async ({ canvasElement }: { canvasElement: HTMLElement }) => {
  await waitFor(
    () => {
      if (canvasElement.querySelector("article") === null) throw new Error("the home timeline shows no post yet");
    },
    { timeout: 5000 },
  );
};

// The composer's state is the story's (`parameters.compose`); the one knob is the text written in it, which replaces
// the state's own while given. Tagged `themes`, the story tests run each in dark too.
const meta = {
  title: "Compose",
  tags: ["themes"],
  parameters: { screen: true, layout: "fullscreen" },
  args: { text: "" },
  argTypes: { text: { control: "text", description: "The text written in the composer, in place of the state's" } },
  play: settle,
  render: ({ text }, { id, parameters }) => <Specimen key={id} id={composeOf(parameters)} text={text} />,
} satisfies Meta<{ text: string }>;
export default meta;

type Story = StoryObj<typeof meta>;

export const Empty: Story = shows("empty");
export const Typing: Story = shows("typing");
export const NearLimit: Story = shows("near-limit");
export const OverLimit: Story = shows("over-limit");
export const Local: Story = shows("visibility-local");
export const FollowersOnly: Story = shows("visibility-private");
export const Permissions: Story = shows("permissions");
export const Direct: Story = shows("visibility-direct");
export const ContentWarning: Story = shows("cw");
export const Images: Story = shows("images");
export const Reply: Story = shows("reply");
export const Quote: Story = shows("quote");
export const Poll: Story = shows("poll");
export const PollSeveralChoices: Story = shows("poll-multiple");
export const RightToLeft: Story = { ...shows("typing"), globals: rightToLeft };
export const MinimumWidth: Story = { ...shows("typing"), globals: minimumWidth };
/** The app itself: New post (or `n`), a post's Reply and the boost menu's Quote open the composer. */
export const LiveFlow: Story = { render: () => <Flow />, play: settleFlow };
