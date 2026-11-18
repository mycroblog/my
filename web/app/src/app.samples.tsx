// SPDX-License-Identifier: AGPL-3.0-only

// The app as the screens' stories show it (ADR-0165 §1): the router and the shell of the app, its features, signed in
// to the fake server of @my/e2e, which answers from the scenario as its reader (yuki) reads it, at a screen, in one of
// the states every screen is shown in: normal, empty (every list answers no item), loading (the screen's requests never
// answer), error (they fail) and long (every name and text runs long); and a thread folded (more ancestors and replies
// than its page shows at once). A story names its screen and state in `parameters.fake`, no argument of the reader's;
// right-to-left and the minimum width are the toolbar's locale and viewport, which no story pins. The router keeps
// its own history, so a link inside the app moves the app alone. The host's Stream panel stands in for the stream
// (`parameters.stream` names what it offers): its buttons hand new posts or a notification to the app as the stream
// does, or report a reconnection, through Storybook's channel; the canvas shows the app alone.

import {
  createAppData,
  receiveHomeGap,
  receiveHomeStatuses,
  receiveNotification,
  watchServer,
  type AppData,
} from "@my/app-data";
import { enterCall, leaveCall, type Call } from "@my/app-features";
import { createFakeApi, fakeHost, fakeOrigin, type FakeApi, type FakeOptions } from "@my/e2e/fake";
import { createClient } from "@mycroblog/mykit";
import type { StoryContext } from "@storybook/react-vite";
import { useEffect, useState } from "react";
import { addons } from "storybook/preview-api";
import { AppRoot } from "./app";
import { createAppRouter } from "./router";

type ScreenState = "normal" | "empty" | "loading" | "error" | "long" | "folded";

/** Where a screen's story opens the app, and the state the fake server answers in. */
export type Fake = {
  /**
   * The app's path, `announcement:<key>` of an announcement, `list:<index>` of the reader's list at that index and
   * `members:<index>` of its members,
   * `conversation:<index>` of the reader's conversation of direct messages at that index (newest first),
   * `reactions:<key>:<emoji>` of who reacted to a scenario post with an emoji, or the key of a scenario post whose page
   * it opens.
   */
  at: string;
  state?: ScreenState;
  /** Fails the home timeline's first page during the first second; its retry succeeds. */
  failFirst?: boolean;
  /** Opens the app without a session, as a visitor. */
  signedOut?: boolean;
  /** The scenario's person who reads; yuki by default (hana is the administrator, marco a moderator). */
  viewer?: string;
  /** What the server is and says across the screens: its timelines, its closure, its maintenance, being down… */
  server?: Omit<FakeOptions, "viewer" | "visitor" | "failFirst" | "lateMs">;
  /** The reader is in a call, which the shell's bar shows. */
  call?: boolean;
  /** A newer build of the app waits (the service worker's), which a toast offers. */
  updateReady?: boolean;
};

/** What the Stream panel offers a story: new posts and reconnections of the home timeline, or notifications. */
type Stream = "home" | "notifications";

/**
 * The event the host's Stream panel sends (web/storybook/.storybook/manager.tsx names it too): new posts, a
 * reconnection, or a notification, as the stream would bring them.
 */
const streamEvent = "my/stream:event";
type StreamEvent = { kind: "posts"; count: number } | { kind: "reconnect" } | { kind: "notification" };

/**
 * What the shell asks for on every screen, which every state answers as normal: the reader, the instance and its
 * health, the reader's preferences and rights, and what is around them.
 */
const shell =
  /^\/api\/v1\/(?:accounts\/verify_credentials|instance|instance\/health|preferences|announcements|voice\/rooms|events|admin\/counts|notifications\/marker)$/;

const longName = "with a display name long enough to run past any column it is given, in every locale";
const longText =
  "A paragraph long enough to wrap many times on a phone: it keeps going past the width of any column, with " +
  "a_word_that_has_no_place_to_break_in_it_at_all_until_its_very_end and a link https://example.org/a/very/long/path/that/never/ends";

/** A response's body with every list emptied: the items of a page, the ancestors and descendants of a thread. */
function emptied(body: unknown): unknown {
  if (Array.isArray(body)) return body.map(emptied);
  if (body === null || typeof body !== "object") return body;
  return Object.fromEntries(
    Object.entries(body).map(([key, value]) => [
      key,
      ["items", "ancestors", "descendants"].includes(key) && Array.isArray(value) ? [] : value,
    ]),
  );
}

/** A response's body with every display name and every text run long. */
function lengthened(body: unknown): unknown {
  if (Array.isArray(body)) return body.map(lengthened);
  if (body === null || typeof body !== "object") return body;
  return Object.fromEntries(
    Object.entries(body).map(([key, value]) => {
      if (key === "display_name" && typeof value === "string") return [key, `${value} ${longName}`];
      if (key === "content" && typeof value === "string" && value !== "") return [key, `${value}<p>${longText}</p>`];
      return [key, lengthened(value)];
    }),
  );
}

/** `items` repeated, each copy with an id of its own, until there are `least` of them. */
function grown(items: unknown[], least: number): unknown[] {
  const out = [...items];
  for (let n = 1; items.length > 0 && out.length < least; n++)
    for (const item of items)
      if (out.length < least && item !== null && typeof item === "object")
        out.push({ ...item, id: `${String((item as { id?: unknown }).id)}-${String(n)}` });
  return out;
}

/** A thread's context with more ancestors and replies than its page shows at once, so that both its folds show. */
function folded(body: unknown): unknown {
  if (body === null || typeof body !== "object" || Array.isArray(body)) return body;
  const context = body as { ancestors?: unknown; descendants?: unknown };
  if (!Array.isArray(context.ancestors) || !Array.isArray(context.descendants)) return body;
  return { ...body, ancestors: grown(context.ancestors, 5), descendants: grown(context.descendants, 25) };
}

/** The fake server's answers as a screen in `state` gets them. */
function staged(serve: FakeApi["serve"], state: ScreenState): FakeApi["serve"] {
  if (state === "normal") return serve;
  return async (request) => {
    if (request.method !== "GET" || shell.test(new URL(request.url).pathname)) return serve(request);
    if (state === "loading") return new Promise<Response>(() => undefined);
    if (state === "error")
      return Response.json(
        { type: "about:blank", title: "Internal Server Error", status: 500 },
        { status: 500, headers: { "Content-Type": "application/problem+json" } },
      );
    const response = await serve(request);
    if (!response.ok) return response;
    const body: unknown = await response.json();
    const answer = state === "empty" ? emptied(body) : state === "folded" ? folded(body) : lengthened(body);
    return Response.json(answer, {
      status: response.status,
      headers: response.headers,
    });
  };
}

/**
 * The app's path of `at`: a path as it is, the page of the announcement of `announcement:<key>`, the list of who
 * reacted to a scenario post with an emoji of `reactions:<key>:<emoji>` (a reaction's name), the article editor of the
 * reader's draft of `article-draft:<key>` or of their article of `article-edit:<key>`, the held-back notifications of the
 * person of `held:<username>`, the page of the reader's newest notification of `notification:<type>`, else the page of
 * the scenario post of that key.
 */
function pathOf(fake: FakeApi, at: string): string {
  if (at.startsWith("/")) return at;
  if (at.startsWith("article-draft:")) return `/articles/new?draft=${fake.idOf(at.slice("article-draft:".length))}`;
  if (at.startsWith("article-edit:")) return `/articles/${fake.idOf(at.slice("article-edit:".length))}/edit`;
  if (at.startsWith("announcement:")) return fake.announcementPath(at.slice("announcement:".length));
  if (at.startsWith("list:")) return fake.listPath(Number(at.slice("list:".length)));
  if (at.startsWith("members:")) return `${fake.listPath(Number(at.slice("members:".length)))}/members`;
  if (at.startsWith("conversation:")) return fake.conversationPath(Number(at.slice("conversation:".length)));
  if (at.startsWith("held:")) return fake.heldPath(at.slice("held:".length));
  if (at.startsWith("notification:")) return fake.notificationPath(at.slice("notification:".length));
  if (at.startsWith("reactions:")) {
    const [, key = "", emoji = ""] = at.split(":");
    return `${fake.postPath(key)}/reactions/${encodeURIComponent(emoji)}`;
  }
  return fake.postPath(at);
}

/** Posts `count` new samples on the fake server and hands them to the timeline as the stream does, as the API's. */
async function post(data: AppData, fake: FakeApi, count: number) {
  fake.newPosts(count);
  const { data: answer } = await data.api.GET("/timelines/home", { params: { query: {} } });
  receiveHomeStatuses(data, answer?.items.slice(0, count) ?? []);
}

/** Hands the app what the Stream panel sends, as the stream would, while `stream` names what the story offers. */
function useStream(data: AppData, fake: FakeApi, stream: Stream | undefined) {
  useEffect(() => {
    if (stream === undefined) return;
    const channel = addons.getChannel();
    const on = (event: StreamEvent) => {
      if (event.kind === "posts" && stream === "home") void post(data, fake, event.count);
      if (event.kind === "reconnect" && stream === "home") receiveHomeGap(data);
      if (event.kind === "notification" && stream === "notifications")
        receiveNotification(data, fake.newNotification());
    };
    channel.on(streamEvent, on);
    return () => {
      channel.off(streamEvent, on);
    };
  }, [data, fake, stream]);
}

/** The app at a screen in a state, on a fake server of its own, fed by the Stream panel. */
/** The call a story's reader is in. */
const sampleCall: Call = {
  roomId: "morning-coffee",
  room: "Morning coffee chat",
  host: "Sam Rivera",
  participants: 12,
  muted: false,
};

/** Puts the reader in the sample call while the story shows, when `call` says so. */
function useSampleCall(call: boolean) {
  useEffect(() => {
    if (!call) return;
    enterCall(sampleCall);
    return () => {
      leaveCall();
    };
  }, [call]);
}

function StoryApp({
  at,
  state = "normal",
  failFirst = false,
  signedOut = false,
  viewer,
  server,
  call = false,
  updateReady = false,
  stream,
}: Fake & { stream: Stream | undefined }) {
  const [{ fake, data, router, watch }] = useState(() => {
    const fake = createFakeApi({
      failFirst,
      visitor: signedOut,
      ...(viewer === undefined ? {} : { viewer }),
      ...server,
    });
    const watch = watchServer({ server: fakeOrigin, fetch: staged(fake.serve, state), followDevice: false });
    if (updateReady) watch.offerUpdate();
    const data = createAppData(
      createClient({ server: fakeOrigin, retry: { disabled: true }, fetch: watch.fetch }),
      undefined,
      watch,
    );
    const router = createAppRouter({
      data: signedOut ? null : data,
      config: { apiOrigin: fakeOrigin, instanceName: fakeHost, defaultLocale: "en", clientId: "my-web" },
      at: pathOf(fake, at),
      // A story's app has no page to load anew: what leaves it (signing out) goes there within it.
      leave: (href) => {
        void router.navigate({ href });
      },
    });
    return { fake, data, router, watch };
  });
  useEffect(
    () => () => {
      watch.stop();
    },
    [watch],
  );
  useSampleCall(call);
  useStream(data, fake, stream);
  return <AppRoot router={router} visitor={data} />;
}

/** The parameters of a screen's story: the fake server's screen and state, what the Stream panel offers. */
type ScreenParameters = { fake?: Fake; stream?: Stream };

const fakeOf = (parameters: ScreenParameters): Fake => parameters.fake ?? { at: "/" };

/** The fake server's screen and state of a story, for its meta or the story itself: `screen({ at: "/" })`. */
export const screen = (fake: Fake, stream?: Stream) => ({ fake, ...(stream ? { stream } : {}) });

/**
 * What every screen's story shares: the whole app, edge to edge, which places its own shortcuts, and a new app for
 * each story. Tagged `themes`, the check of every story shows it in dark too.
 */
export const screenMeta = {
  tags: ["themes"],
  parameters: { screen: true, layout: "fullscreen" },
  render: (_: unknown, { id, parameters }: StoryContext) => {
    const p = parameters as ScreenParameters;
    return <StoryApp key={id} {...fakeOf(p)} stream={p.stream} />;
  },
};
