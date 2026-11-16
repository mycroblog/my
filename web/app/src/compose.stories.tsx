// SPDX-License-Identifier: AGPL-3.0-only

// The post composer of @my/app-features: a story per state of it, in the flow of the page; and the live flow, the app
// itself, where New post (or `n`), a post's Reply and the boost menu's Quote open the composer over the home timeline,
// and what is posted tops it. The language is the toolbar's locale (ja, else en).
//
// The server is the fake of @my/e2e, which answers the reads from the scenario and keeps the reader's drafts and their
// schedules; this file's own fake takes the other writes: posts and uploads (a file whose name has "fail" is refused,
// so that the retry shows; the others are processed for a moment); a post whose text has "#fail" is refused with the
// server's words, so that a failed post shows. Leaving the composer: close and Escape discard a written post, with
// Undo; the backdrop saves it as a draft, which New post opens again. The drafts' menu in the head saves it as a draft,
// or opens the drafts and scheduled posts, from which one opens in the composer.

import { AppDataProvider, createAppData, defaultComposeSettings, type PostSource, type Status } from "@my/app-data";
import {
  ComposeLayer,
  emptyComposeDraft,
  type ComposeDraft,
  type ComposeSpecimen,
  type ComposeTarget,
} from "@my/app-features";
import { useLocale } from "@my/design-system";
import { images } from "@my/design-system/samples";
import { createFakeApi, fakeHost, fakeOrigin, type FakeApi } from "@my/e2e/fake";
import { createClient } from "@mycroblog/mykit";
import type { Meta, StoryContext, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { AppRoot } from "./app";
import { minimumWidth, rightToLeft } from "./app.samples";
import { createAppRouter } from "./router";
import { useNarrowViewport } from "./shell/viewport";

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
    if (path === "/instance") {
      const instance = (await read("/instance")) as Record<string, unknown>;
      return json({ ...instance, limits: { max_post_characters: 500, require_media_description: false } });
    }
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
      // Not uploaded here: a file of a draft, which the fake of @my/e2e holds.
      if (!upload) return fake.serve(request);
      const ready = Date.now() >= upload.ready;
      return json({
        id: media[1],
        type: "image",
        state: ready ? "ready" : "processing",
        meta: centered,
        ...(ready ? { url: upload.url, preview_url: upload.url } : {}),
      });
    }
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
    edit: "今朝の尾根。三時に起きた甲斐がありました #山 #写真",
    marked:
      "Thanks @sam and @layla@social.example for the walk :sparkle: Photos at https://photos.example/ridge #山 #hiking",
    markedCw: "Trail photos :sparkle: lots of them",
    suggestAccount: "Lunch tomorrow with @m",
    suggestHashtag: "Day 4 of the sourdough experiment #ba",
    suggestEmoji: "Good morning :wa",
    suggestCw: "Trail photos :sp",
    later: "@sam The sourdough came out flat again. Trying your overnight proof this weekend.",
    // The server's words of a 422 that names a member of the request: its title, and its detail.
    refusedTitle: "This could not be done.",
    refusedDetail: "This value is not valid. Check what you entered.",
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
    edit: "今朝の尾根。三時に起きた甲斐がありました #山 #写真",
    marked:
      "@sam さん @layla@social.example さん、今日はありがとう :sparkle: 写真は https://photos.example/ridge に #山 #写真",
    markedCw: "登山の写真 :sparkle: たくさん",
    suggestAccount: "明日のお昼は @m",
    suggestHashtag: "今朝の尾根 #写",
    suggestEmoji: "おはよう :wa",
    suggestCw: "登山の写真 :sp",
    later: "@sam サワードウがまたぺたんこに。週末は教わった一晩発酵でやってみます。",
    refusedTitle: "この操作はできませんでした。",
    refusedDetail: "正しくない値です。入力した内容を確かめてください。",
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
  /** What it opens in: its phase, and what is open in it. */
  specimen?: ComposeSpecimen;
  /** It opens refused by the server, in the server's words, beside this input (field-errors.ts of @my/app-features). */
  refusedAt?: string;
};

/** A moment `days` days from now at `hour` o'clock, local time, as an ISO 8601 string. */
const dayAt = (days: number, hour: number) => {
  const d = new Date();
  d.setDate(d.getDate() + days);
  d.setHours(hour, 0, 0, 0);
  return d.toISOString();
};

const nw = (): ComposeTarget => ({ kind: "new" });
/** The source of the post of `id`, bare: the state's draft writes what it holds. */
const sourceOf = (id: string): PostSource => ({
  id,
  text: "",
  spoiler_text: "",
  language: "und",
  visibility: "public",
  sensitive: false,
  media: [],
});
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
  marks: {
    name: "Mentions, hashtags, a URL and custom emoji",
    target: nw,
    draft: (d, w) => ({ ...d, cw: w.markedCw, text: w.marked }),
  },
  "suggest-account": {
    name: "Suggesting accounts",
    target: nw,
    draft: (d, w) => ({ ...d, text: w.suggestAccount }),
  },
  "suggest-hashtag": {
    name: "Suggesting hashtags",
    target: nw,
    draft: (d, w) => ({ ...d, text: w.suggestHashtag }),
  },
  "suggest-emoji": { name: "Suggesting emoji", target: nw, draft: (d, w) => ({ ...d, text: w.suggestEmoji }) },
  "suggest-cw": {
    name: "Suggesting emoji in the content warning",
    target: nw,
    draft: (d, w) => ({ ...d, cw: w.suggestCw, text: w.cwText }),
  },
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
  // The reader's (yuki's) post with its image, opened from its source to edit.
  edit: {
    name: "Edit",
    target: (fake) => ({ kind: "edit", id: idOf(fake, "yuki-ridge"), source: sourceOf(idOf(fake, "yuki-ridge")) }),
    draft: (d, w) => ({ ...d, text: w.edit, language: "ja", attachments: [image(0, "ready", w.alt)] }),
  },
  // Sending it later (ADR-0156): a time set, the form of the time open, a time too near, the menu of the ways to send
  // it later open, a draft whose scheduled post failed, and a save refused for a newer draft saved elsewhere.
  scheduled: {
    name: "Scheduled",
    target: nw,
    draft: (d, w) => ({ ...typed(d, w), scheduledAt: dayAt(1, 9) }),
  },
  "schedule-form": { name: "Schedule's form", target: nw, draft: typed, specimen: { opened: "schedule" } },
  "schedule-change": {
    name: "Schedule's form, a time set",
    target: nw,
    draft: (d, w) => ({ ...typed(d, w), scheduledAt: dayAt(1, 9) }),
    specimen: { opened: "schedule" },
  },
  "schedule-too-soon": {
    name: "Schedule too soon",
    target: nw,
    draft: (d, w) => ({ ...typed(d, w), scheduledAt: new Date(Date.now() + 2 * 60_000).toISOString() }),
    specimen: { opened: "schedule" },
  },
  "drafts-menu": { name: "Drafts' menu", target: nw, draft: typed, specimen: { opened: "drafts" } },
  "schedule-failed": {
    name: "Scheduled post failed",
    target: nw,
    draft: (d, w) => ({ ...d, text: w.later, failure: { publishAt: dayAt(-1, 9), reason: "rejected" } }),
  },
  conflict: { name: "Newer draft elsewhere", target: nw, draft: typed, specimen: { phase: { kind: "conflict" } } },
  gone: { name: "Draft gone from the server", target: nw, draft: typed, specimen: { phase: { kind: "gone" } } },
  // Refused by the server for the value of one input it named: what is wrong stands under that input, the problem's
  // title alone before Post.
  "refused-cw": {
    name: "Content warning refused",
    target: nw,
    draft: (d, w) => ({ ...d, cw: w.cw, text: w.cwText }),
    refusedAt: "cw",
  },
  "refused-option": { name: "Poll option refused", target: nw, draft: poll(false), refusedAt: "option:1" },
  "refused-description": {
    name: "Image description refused",
    target: nw,
    draft: (d, w) => ({ ...d, text: w.images, attachments: [image(0, "ready", w.alt), image(1, "ready", w.alt)] }),
    refusedAt: "media:sample-1",
  },
  // Being sent: Close, Post and the tools wait for the answer.
  posting: { name: "Posting", target: nw, draft: typed, specimen: { phase: { kind: "posting" } } },
  // The chunk of the composer's view, or of its editor, could not be fetched (the network).
  "unloaded-composer": {
    name: "Post form not loaded",
    target: nw,
    draft: typed,
    specimen: { unloaded: "composer" },
  },
  "unloaded-editor": {
    name: "Editor not loaded",
    target: nw,
    draft: (d, w) => ({ ...d, cw: w.cw, text: w.cwText }),
    specimen: { unloaded: "editor" },
  },
} satisfies Record<string, State>;

type StateId = keyof typeof states;

/**
 * One state of the composer in the flow of the page; `text`, when given, is the text written in it. Its emoji picker
 * opens in a sheet on a phone's viewport, as the shell decides, or with `pickerSheet`; else in a popover.
 */
function Specimen({ id, text, pickerSheet = false }: { id: StateId; text: string; pickerSheet?: boolean | undefined }) {
  const { locale } = useLocale();
  const narrow = useNarrowViewport();
  const [{ fake, data }] = useState(createComposeApp);
  const state: State = states[id];
  const lang = locale.startsWith("ja") ? "ja" : "en";
  const target = state.target(fake);
  const drafted = state.draft(
    { ...emptyComposeDraft(target, defaultComposeSettings, `compose-${id}`), language: lang },
    words[lang],
  );
  const initial = text === "" ? drafted : { ...drafted, text };
  const w = words[lang];
  const specimen: ComposeSpecimen | undefined =
    state.refusedAt === undefined
      ? state.specimen
      : {
          ...state.specimen,
          phase: { kind: "failed", reason: w.refusedTitle, field: { input: state.refusedAt, text: w.refusedDetail } },
        };
  return (
    <AppDataProvider data={data}>
      <div style={{ maxInlineSize: "37.5rem", marginInline: "auto" }}>
        <ComposeLayer
          target={target}
          initial={initial}
          inline
          pickerSheet={pickerSheet || narrow}
          specimen={specimen}
          onOpen={() => undefined}
          onClose={() => undefined}
        />
      </div>
    </AppDataProvider>
  );
}

/** The app itself, on the composer's fake server, at the home timeline. */
function Flow() {
  const [{ router, data }] = useState(() => {
    const { data } = createComposeApp();
    const router = createAppRouter({
      data,
      config: { apiOrigin: fakeOrigin, instanceName: fakeHost, defaultLocale: "en", clientId: "my-web" },
      at: "/",
    });
    return { router, data };
  });
  return <AppRoot router={router} visitor={data} />;
}

/**
 * Waits until the composer shows what the fake server answers: its text field, and the post a reply or a quote is
 * of; so that the story tests check it as the reader sees it.
 */
const settle = async ({ canvasElement, parameters }: StoryContext) => {
  const state = composeOf(parameters);
  // The handle of the author of the post a reply or a quote is of (sam-bread, layla-jokes), which shows once it came.
  const of = state === "reply" ? "@sam" : state === "quote" ? "@layla" : "";
  // A state whose chunk was not loaded shows its alert in the text field's place.
  const field = state.startsWith("unloaded-") ? '[role="alert"]' : '[role="textbox"][aria-multiline="true"]';
  await waitFor(
    () => {
      const ready = canvasElement.querySelector(field) !== null;
      if (!ready || !canvasElement.textContent.includes(of))
        throw new Error(`the composer does not show its ${state} state yet`);
    },
    { timeout: 5000 },
  );
};

/** Waits until the composer shows its state, then that the server's words describe the input it refused, marked invalid. */
const refusedBeside = async (context: StoryContext) => {
  await settle(context);
  const said = [words.en.refusedDetail, words.ja.refusedDetail];
  await waitFor(
    () => {
      const doc = context.canvasElement.ownerDocument;
      const described = [...context.canvasElement.querySelectorAll('[aria-invalid="true"]')].some((input) => {
        const id = input.getAttribute("aria-describedby");
        const text = id ? (doc.getElementById(id)?.textContent ?? "") : "";
        return said.some((s) => text.includes(s));
      });
      if (!described) throw new Error("the refusal does not describe the input it is about");
    },
    { timeout: 5000 },
  );
};

/**
 * The refusal beside the poll's second option, then one more letter typed in that option: the option is valid again,
 * and neither the server's words nor its title stand anywhere in the composer.
 */
const refusedThenEdited = async (context: StoryContext) => {
  await refusedBeside(context);
  const canvas = within(context.canvasElement);
  const option = canvas.getByRole("textbox", { name: "Option 2" });
  await expect(option).toHaveAttribute("aria-invalid", "true");
  await expect(canvas.getByRole("alert")).toHaveTextContent(/^This could not be done\.$/);
  await userEvent.type(option, "s");
  await waitFor(() => expect(option).not.toHaveAttribute("aria-invalid", "true"));
  await expect(canvas.queryByText(words.en.refusedDetail)).toBeNull();
  await expect(canvas.queryByText(words.en.refusedTitle)).toBeNull();
};

/** The state of the composer a story shows, from its parameters. */
const composeOf = (parameters: StoryContext["parameters"]): StateId =>
  ((parameters as { compose?: StateId }).compose ?? "empty") satisfies StateId;

/** A story of the composer in `id`. */
const shows = (id: StateId): Story => ({ parameters: { compose: id } });

/**
 * Opens the composer's emoji picker, as a press on its button does, and waits until it shows the Unicode emoji and
 * (in a sheet) has risen, so that the story tests check it as the reader sees it.
 */
const openEmoji = async (context: StoryContext) => {
  await settle(context);
  const doc = context.canvasElement.ownerDocument;
  // The button at the end of the text's first line, in any locale.
  context.canvasElement
    .querySelector<HTMLButtonElement>('div:has(> [role="textbox"][aria-multiline="true"]) ~ div button')
    ?.click();
  await waitFor(
    () => {
      const grid = doc.querySelector('[role="grid"]');
      const panel = grid?.closest('[role="dialog"]')?.parentElement;
      const style = panel ? getComputedStyle(panel) : undefined;
      if (grid === null || grid.querySelectorAll('[role="gridcell"]').length < 20)
        throw new Error("the emoji picker shows no emoji yet");
      if (style?.opacity !== "1" || !["none", "matrix(1, 0, 0, 1, 0, 0)"].includes(style.transform))
        throw new Error("the emoji picker has not appeared yet");
    },
    { timeout: 5000 },
  );
};

/**
 * Puts the caret at the end of the field whose word is suggested for (the content warning's for `suggest-cw`, else the
 * text's), as a reader who typed it, and waits until the suggestions list their options.
 */
const suggest = async (context: StoryContext) => {
  await settle(context);
  const multiline = composeOf(context.parameters) === "suggest-cw" ? "false" : "true";
  const field = await waitFor(
    () => {
      const found = context.canvasElement.querySelector<HTMLElement>(
        `[role="textbox"][aria-autocomplete][aria-multiline="${multiline}"]`,
      );
      if (found === null) throw new Error("the composer has no field to suggest in");
      return found;
    },
    { timeout: 5000 },
  );
  // As a reader presses End: the caret goes to the end of the text, which tells the suggestions where it is.
  field.focus();
  const selection = field.ownerDocument.getSelection();
  selection?.selectAllChildren(field);
  selection?.collapseToEnd();
  await waitFor(
    () => {
      if (context.canvasElement.querySelectorAll('[role="listbox"] [role="option"]').length === 0)
        throw new Error("the composer suggests nothing yet");
    },
    { timeout: 5000 },
  );
};

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
  render: ({ text }, { id, parameters }) => (
    <Specimen
      key={id}
      id={composeOf(parameters)}
      text={text}
      pickerSheet={(parameters as { pickerSheet?: boolean }).pickerSheet}
    />
  ),
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
/**
 * What the server will read, marked behind the text as it is typed: mentions, hashtags, a URL and a custom emoji; in
 * the content warning, its custom emoji alone.
 */
export const Marks: Story = shows("marks");
export const MarksRightToLeft: Story = { ...shows("marks"), globals: rightToLeft };
/** `@m` typed: the accounts the server found, the followed first, under the caret's line, the first active. */
export const SuggestAccounts: Story = { ...shows("suggest-account"), play: suggest };
export const SuggestAccountsRightToLeft: Story = { ...shows("suggest-account"), play: suggest, globals: rightToLeft };
/** `#ba` (`#写` in ja): the hashtags the server found, the most used first, with how many posts carry each. */
export const SuggestHashtags: Story = { ...shows("suggest-hashtag"), play: suggest };
/** `:wa`: the custom emoji, then the Unicode ones, that match. */
export const SuggestEmoji: Story = { ...shows("suggest-emoji"), play: suggest };
/** `:sp` in the content warning: its emoji, under it. */
export const SuggestEmojiInWarning: Story = { ...shows("suggest-cw"), play: suggest };
export const Images: Story = shows("images");
export const Reply: Story = shows("reply");
export const Quote: Story = shows("quote");
export const Poll: Story = shows("poll");
export const PollSeveralChoices: Story = shows("poll-multiple");
/**
 * Editing the reader's post: Edit post and Save tell it; the poll, the audience and who may interact leave the row,
 * and one muted line over the text says the audience stays as posted.
 */
export const Edit: Story = shows("edit");
/** A time set: one line over the row says when it goes out, and Schedule takes Post's place. */
export const Scheduled: Story = shows("scheduled");
/** The form of the time open: one field of the date and the time, the reader's zone and the server's limits under it. */
export const ScheduleForm: Story = shows("schedule-form");
/** The form of a time already set: Don't schedule beside Set time takes it away. */
export const ScheduleChange: Story = shows("schedule-change");
/** A time nearer than the server takes: the field says so in place of the limits. */
export const ScheduleTooSoon: Story = shows("schedule-too-soon");
/** The drafts' menu in the head open: keep this post as a draft, or open the drafts and scheduled posts. */
export const DraftsMenu: Story = shows("drafts-menu");
/** A draft whose scheduled post failed, opened again: why, before Post. */
export const ScheduleFailed: Story = shows("schedule-failed");
/** A save refused for a newer draft saved on another device: it says so, and opens the newer one. */
export const NewerDraft: Story = shows("conflict");
/** The draft is no longer on the server: what was written stays, and Post reads Post as new, which posts it so. */
export const DraftGone: Story = shows("gone");
/** Being sent: Post shows it is sending, and Close stays in its place, disabled. */
export const Posting: Story = shows("posting");
export const WarningRefused: Story = { ...shows("refused-cw"), play: refusedBeside };
export const PollOptionRefused: Story = { ...shows("refused-option"), play: refusedBeside };
export const DescriptionRefused: Story = { ...shows("refused-description"), play: refusedBeside };
/** A poll's option refused, then edited: the next letter typed in it clears what the server said. */
export const PollOptionRefusedEdited: Story = {
  ...shows("refused-option"),
  globals: { locale: "en" },
  play: refusedThenEdited,
};
/** The composer's view could not be fetched: its frame as tall as one line, that it could not be loaded, and Try again. */
export const PostFormNotLoaded: Story = shows("unloaded-composer");
/**
 * The editors could not be fetched: once, in the text's place, that it could not be loaded, and Try again; the content
 * warning's band empty, the count away and Post disabled.
 */
export const EditorNotLoaded: Story = shows("unloaded-editor");
export const RightToLeft: Story = { ...shows("typing"), globals: rightToLeft };
export const MinimumWidth: Story = { ...shows("typing"), globals: minimumWidth };
/** A time set at the narrowest width: the row keeps its tools, the count and Schedule on one line. */
export const ScheduledMinimumWidth: Story = { ...shows("scheduled"), globals: minimumWidth };
/** The emoji button pressed: the picker in a popover beside it, from the tablet width. */
export const EmojiPicker: Story = { ...shows("typing"), play: openEmoji };
/** The same on a phone: the picker in a sheet from the bottom. */
export const EmojiPickerSheet: Story = {
  parameters: { compose: "typing", pickerSheet: true },
  play: openEmoji,
  globals: minimumWidth,
};
/** The app itself: New post (or `n`), a post's Reply and the boost menu's Quote open the composer. */
export const LiveFlow: Story = { render: () => <Flow />, play: settleFlow };
