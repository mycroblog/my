// SPDX-License-Identifier: AGPL-3.0-only

// The post composer of @my/app-features: a story per state of it, in the flow of the page; and the live flow, the app
// itself, where New post (or `n`), a post's Reply and the boost menu's Quote open the composer over the home timeline,
// and what is posted tops it. The language is the toolbar's locale (ja, else en).
//
// The server is the fake of @my/e2e, which answers from the scenario and keeps what the reader writes; this file stages
// it for the composer: a post takes a moment, and one whose text has "#fail" is refused with the server's words, so
// that a failed post shows; an upload is processed for a moment, and a file whose name has "fail" is refused, so that
// the retry shows. Leaving the composer: close and Escape discard a written post, with Undo; the backdrop saves it as a
// draft, which New post opens again. The drafts' menu in the head saves it as a draft, or opens the drafts and
// scheduled posts, from which one opens in the composer.

import { AppDataProvider, createAppData, defaultComposeSettings, type PostSource } from "@my/app-data";
import {
  ComposeLayer,
  emptyComposeDraft,
  type ComposeDraft,
  type ComposeSpecimen,
  type ComposeTarget,
} from "@my/app-features";
import { instantAt, MenuSheet, useLocale, wallTime } from "@my/design-system";
import { images } from "@my/design-system/samples";
import { createFakeApi, fakeHost, fakeOrigin, type FakeApi } from "@my/e2e/fake";
import { createClient } from "@mycroblog/mykit";
import type { Meta, StoryContext, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { AppRoot } from "./app";
import { createAppRouter } from "./router";
import { useNarrowViewport } from "./shell/viewport";

const problem = (status: number, title: string, more: Record<string, string> = {}) =>
  new Response(JSON.stringify({ type: "about:blank", title, status, ...more }), {
    status,
    headers: { "Content-Type": "application/problem+json" },
  });

/** A file's metadata, centered. */
const centered = { focus: { x: 0, y: 0 } };

/** How long a post takes, and an upload's processing, so that each shows. */
const postMs = 800;
const processingMs = 1500;

/** The fake of @my/e2e as the composer's stories stage it: its posts late or refused, its uploads processed late. */
function staged(fake: FakeApi): (request: Request) => Promise<Response> {
  /** The uploads still processed, by their IDs: until when, and what the server answers of each meanwhile. */
  const uploads = new Map<string, { until: number; body: string }>();
  // The limits as the composer's states are written for: a post's length alone.
  fake.script("GET /instance", {
    change: (instance) => ({ ...instance, limits: { max_post_characters: 500, require_media_description: false } }),
  });
  return async (request) => {
    const path = new URL(request.url).pathname.replace("/api/v1", "");
    if (path === "/statuses" && request.method === "POST") {
      const { text = "" } = (await request.clone().json()) as { text?: string };
      await new Promise((resolve) => setTimeout(resolve, postMs));
      if (text.includes("#fail"))
        return problem(422, "Unprocessable Content", {
          type: "/api/v1/problems/status-rejected",
          detail: document.documentElement.lang.startsWith("ja") ? "投稿が断られました。" : "The post was refused.",
        });
      return fake.serve(request);
    }
    if (path === "/media" && request.method === "POST") {
      const file = (await request.clone().formData()).get("file");
      if (!(file instanceof File) || file.name.includes("fail")) return problem(415, "Unsupported Media Type");
      const response = await fake.serve(request);
      const body = await response.clone().text();
      uploads.set((JSON.parse(body) as { id: string }).id, { until: Date.now() + processingMs, body });
      return response;
    }
    const upload = uploads.get(/^\/media\/([^/]+)$/.exec(path)?.[1] ?? "");
    if (upload !== undefined && request.method === "GET" && Date.now() < upload.until)
      return new Response(upload.body, { headers: { "Content-Type": "application/json" } });
    return fake.serve(request);
  };
}

/** A new app on a new fake server, as the composer's stories use it; with posts that approve by hand, `approvals`. */
function createComposeApp(approvals = false) {
  const fake = createFakeApi({ approvals, uploads: "processing" });
  const data = createAppData(createClient({ server: fakeOrigin, retry: { disabled: true }, fetch: staged(fake) }));
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
  const today = wallTime(new Date(), undefined);
  return instantAt({ ...today, day: today.day + days, hour, minute: 0 }, undefined).toISOString();
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
/** Anyone replies, followers boost and others ask for approval, the author alone quotes. */
const limited: ComposeDraft["policy"] = {
  reply: { audience: "everyone", approval: false },
  announce: { audience: "followers", approval: true },
  quote: { audience: "nobody", approval: false },
};
const limitedTyped = (d: ComposeDraft, w: (typeof words)["en"]) => ({ ...typed(d, w), policy: limited });
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
    draft: (d, w) => ({ ...typed(d, w), policy: limited }),
  },
  // Who may reply, boost and quote, with approval (ADR-0155): the audience menu, its permissions submenu with each
  // kind's value, and the panel of one kind with whether others may ask.
  "permissions-menu": { name: "Audience menu", target: nw, draft: limitedTyped, specimen: { opened: "audience" } },
  "permissions-kinds": {
    name: "Permissions submenu",
    target: nw,
    draft: limitedTyped,
    specimen: { opened: "permissions" },
  },
  "permissions-kind": {
    name: "Who can boost, others with approval",
    target: nw,
    draft: limitedTyped,
    specimen: { opened: "announce" },
  },
  // A reply to a post whose author approves the reader's replies by hand (the fake's `approvals`).
  "reply-waits": {
    name: "Reply waiting for approval",
    target: (fake) => ({ kind: "reply", id: idOf(fake, "layla-asks") }),
    draft: (d) => d,
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
 * One state of the composer in the flow of the page; `text`, when given, is the text written in it. Its menus and its
 * emoji picker open in a sheet on a phone's viewport, as the shell decides, else in a popover.
 */
function Specimen({ id, text }: { id: StateId; text: string }) {
  const { locale } = useLocale();
  const narrow = useNarrowViewport();
  const [{ fake, data }] = useState(() => createComposeApp(id === "reply-waits"));
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
      <MenuSheet sheet={narrow}>
        <div style={{ maxInlineSize: "37.5rem", marginInline: "auto" }}>
          <ComposeLayer
            target={target}
            initial={initial}
            inline
            pickerSheet={narrow}
            specimen={specimen}
            onOpen={() => undefined}
            onClose={() => undefined}
          />
        </div>
      </MenuSheet>
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

/** The state of the composer a story shows, from its parameters. */
const composeOf = (parameters: StoryContext["parameters"]): StateId =>
  ((parameters as { compose?: StateId }).compose ?? "empty") satisfies StateId;

/** A story of the composer in `id`. */
const shows = (id: StateId): Story => ({ parameters: { compose: id } });

// The composer's state is the story's (`parameters.compose`); the one knob is the text written in it, which replaces
// the state's own while given. Tagged `themes`, the check of every story shows each in dark too. The suggestions under the caret
// and the emoji picker open are the parts' own (the design system's Composer and Emoji picker).
const meta = {
  title: "Compose",
  tags: ["themes"],
  parameters: { screen: true, layout: "fullscreen" },
  args: { text: "" },
  argTypes: { text: { control: "text", description: "The text written in the composer, in place of the state's" } },
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
/** The audience menu: the visibilities, then the row of who can reply, boost and quote. */
export const PermissionsMenu: Story = shows("permissions-menu");
/** The permissions submenu: a row per kind with who can do it now, others with approval. */
export const PermissionsByKind: Story = shows("permissions-kinds");
/** The panel of one kind (boost): its audiences, and under them whether others may ask for approval. */
export const PermissionOfAKind: Story = shows("permissions-kind");
/** Replying to a post whose author approves the reader's replies: a line over the row says so. */
export const ReplyWaits: Story = shows("reply-waits");
export const Direct: Story = shows("visibility-direct");
export const ContentWarning: Story = shows("cw");
/**
 * What the server will read, marked behind the text as it is typed: mentions, hashtags, a URL and a custom emoji; in
 * the content warning, its custom emoji alone.
 */
export const Marks: Story = shows("marks");
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
export const WarningRefused: Story = shows("refused-cw");
export const PollOptionRefused: Story = shows("refused-option");
export const DescriptionRefused: Story = shows("refused-description");
/** The composer's view could not be fetched: its frame as tall as one line, that it could not be loaded, and Try again. */
export const PostFormNotLoaded: Story = shows("unloaded-composer");
/**
 * The editors could not be fetched: once, in the text's place, that it could not be loaded, and Try again; the content
 * warning's band empty, the count away and Post disabled.
 */
export const EditorNotLoaded: Story = shows("unloaded-editor");
/** The app itself: New post (or `n`), a post's Reply and the boost menu's Quote open the composer. */
export const LiveFlow: Story = { render: () => <Flow /> };
