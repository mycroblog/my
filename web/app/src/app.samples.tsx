// SPDX-License-Identifier: AGPL-3.0-only

// The app as the screens' stories show it (ADR-0165 §1): the router and the shell of the app, its features, signed in
// to the fake server of @my/e2e, which answers from the scenario as its reader (yuki) reads it, at a screen, in one of
// the states every screen is shown in: normal, empty (every list answers no item), loading (the screen's requests never
// answer), error (they fail) and long (every name and text runs long); and a thread folded (more ancestors and replies
// than its page shows at once). A story names its screen and state in `parameters.fake`, no argument of the reader's;
// right-to-left and the minimum width are the toolbar's locale and viewport, which a story may pin. The router keeps
// its own history, so a link inside the app moves the app alone. The host's Stream panel stands in for the stream
// (`parameters.stream` names what it offers): its buttons hand new posts or a notification to the app as the stream
// does, or report a reconnection, through Storybook's channel; the canvas shows the app alone.

import {
  createAppData,
  receiveHomeGap,
  receiveHomeStatuses,
  receiveNotification,
  type AppData,
  type NotificationItem,
} from "@my/app-data";
import { createFakeApi, fakeHost, fakeOrigin, type FakeApi } from "@my/e2e/fake";
import { createClient } from "@mycroblog/mykit";
import type { StoryContext } from "@storybook/react-vite";
import { useEffect, useState } from "react";
import { addons } from "storybook/preview-api";
import { waitFor } from "storybook/test";
import { AppRoot } from "./app";
import { createAppRouter } from "./router";

type ScreenState = "normal" | "empty" | "loading" | "error" | "long" | "folded";

/** Where a screen's story opens the app, and the state the fake server answers in. */
export type Fake = {
  /** The app's path, or the key of a scenario post whose page it opens. */
  at: string;
  state?: ScreenState;
  /** Fails the home timeline's first page during the first second; its retry succeeds. */
  failFirst?: boolean;
};

/** What the Stream panel offers a story: new posts and reconnections of the home timeline, or notifications. */
type Stream = "home" | "notifications";

/**
 * The event the host's Stream panel sends (web/storybook/.storybook/manager.tsx names it too): new posts, a
 * reconnection, or a notification, as the stream would bring them.
 */
const streamEvent = "my/stream:event";
type StreamEvent = { kind: "posts"; count: number } | { kind: "reconnect" } | { kind: "notification" };

/** What the shell asks for before any screen, which every state answers as normal. */
const shell = /^\/api\/v1\/(?:accounts\/verify_credentials|instance|preferences)$/;

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

/** The app's path of `at`: a path as it is, else the page of the scenario post of that key. */
const pathOf = (fake: FakeApi, at: string) => (at.startsWith("/") ? at : fake.postPath(at));

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
        receiveNotification(data, fake.newNotification() as NotificationItem);
    };
    channel.on(streamEvent, on);
    return () => {
      channel.off(streamEvent, on);
    };
  }, [data, fake, stream]);
}

/** The app at a screen in a state, on a fake server of its own, fed by the Stream panel. */
function StoryApp({ at, state = "normal", failFirst = false, stream }: Fake & { stream: Stream | undefined }) {
  const [{ fake, data, router }] = useState(() => {
    const fake = createFakeApi({ failFirst });
    const data = createAppData(
      createClient({ server: fakeOrigin, retry: { disabled: true }, fetch: staged(fake.serve, state) }),
    );
    const router = createAppRouter({
      data,
      config: { apiOrigin: fakeOrigin, instanceName: fakeHost, defaultLocale: "en", clientId: "my-web" },
      at: pathOf(fake, at),
    });
    return { fake, data, router };
  });
  useStream(data, fake, stream);
  return <AppRoot router={router} />;
}

/** The parameters of a screen's story: the fake server's screen and state, what the Stream panel offers. */
type ScreenParameters = { fake?: Fake; stream?: Stream };

const fakeOf = (parameters: ScreenParameters): Fake => parameters.fake ?? { at: "/" };

/**
 * Whether the screen in the canvas shows its state: what the fake server's first answers render. A first page that
 * fails once shows its failure, with the retry.
 */
function shows(canvas: HTMLElement, { state = "normal", failFirst = false }: Fake): boolean {
  const has = (selector: string) => canvas.querySelector(selector) !== null;
  const settled = !has('[aria-busy="true"]');
  if (failFirst) return has('[role="alert"]');
  switch (state) {
    case "loading":
      return true;
    case "error":
      return has('[role="alert"]');
    case "empty":
      return settled && has('article, p[role="status"]');
    case "folded":
      // Both folds, each a text button in the entry before what it holds.
      return settled && canvas.querySelectorAll("article > p > button").length >= 2;
    default:
      return settled && has("article");
  }
}

/** The fake server's screen and state of a story, for its meta or the story itself: `screen({ at: "/" })`. */
export const screen = (fake: Fake, stream?: Stream) => ({ fake, ...(stream ? { stream } : {}) });

/**
 * What every screen's story shares: the whole app, edge to edge, which places its own shortcuts; a new app for each
 * story; and a play that waits until the screen shows its state (its posts, its notice of none, its failure), so that
 * the story tests check the screen (axe, sideways scrolling) as the reader sees it. Tagged `themes`, the story tests
 * run it in dark too.
 */
export const screenMeta = {
  tags: ["themes"],
  parameters: { screen: true, layout: "fullscreen" },
  render: (_: unknown, { id, parameters }: StoryContext) => {
    const p = parameters as ScreenParameters;
    return <StoryApp key={id} {...fakeOf(p)} stream={p.stream} />;
  },
  play: async ({ canvasElement, parameters }: StoryContext) => {
    const fake = fakeOf(parameters);
    await waitFor(
      () => {
        if (!shows(canvasElement, fake))
          throw new Error(`the screen does not show its ${fake.state ?? "normal"} state yet`);
      },
      { timeout: 5000 },
    );
  },
};

/** Pins the toolbar's locale to ar-XB, which reads from right to left. */
export const rightToLeft = { locale: "ar-XB" };

/** Pins the toolbar's viewport to the minimum width (ADR-0110 §10). */
export const minimumWidth = { viewport: { value: "minimum" } };
