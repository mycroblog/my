// SPDX-License-Identifier: AGPL-3.0-only

// The app's screens as the app shows them, on the catalog's dev server: the router and the shell of @my/app, its
// features from @my/app-features, signed in to the fake server of server.ts instead of a live one. Each page of the
// catalog opens the app at a screen, in the state `?state=` names: /app/timeline the home timeline, /app/post a post in
// its thread or alone, /app/notifications the notifications, /app/profile a profile in each state the reader meets it
// in. The router keeps its own history, so the catalog's address stays the page's, and a link inside the app moves the
// app alone. The catalog's own controls, over the app, stand in for the stream: they hand new posts or a notification
// to the app as the stream does, or report a reconnection.

import { AppRoot, createAppRouter } from "@my/app";
import {
  createAppData,
  receiveHomeGap,
  receiveHomeStatuses,
  receiveNotification,
  type NotificationItem,
} from "@my/app-data";
import { Button } from "@my/design-system";
import { createClient } from "@mycroblog/mykit";
import type { CSSProperties, ReactNode } from "react";
import { newNotification, newPosts, serve, threadPaths } from "./server";

/** A page of the catalog: where it opens the app, in each of its states. */
type Screen = { readonly states: readonly { id: string; name: string; at: string }[] };

const screens: Readonly<Record<string, Screen>> = {
  "/app/timeline": { states: [{ id: "home", name: "Home", at: "/" }] },
  "/app/post": {
    states: [
      { id: "thread", name: "Thread", at: threadPaths.thread },
      { id: "lone", name: "Alone, no replies", at: threadPaths.lone },
    ],
  },
  "/app/notifications": { states: [{ id: "all", name: "All", at: "/notifications" }] },
  "/app/profile": {
    states: [
      { id: "own", name: "Own", at: "/profile" },
      { id: "local", name: "Local, follows you", at: "/@hana" },
      { id: "remote", name: "Remote, following", at: "/@alice@example.social" },
      { id: "locked", name: "Locked", at: "/@takuya@example.jp" },
      { id: "blocked", name: "Blocked", at: "/@ravi@fedi.example.net" },
      { id: "muted", name: "Muted", at: "/@lena@toot.example.eu" },
    ],
  },
};

/** Whether the catalog previews the app at `path`. */
export const previewsApp = (path: string) => path in screens;

const page = location.pathname.replace(/\/+$/, "");
const screen = screens[page] ?? { states: [{ id: "home", name: "Home", at: "/" }] };
const asked = new URLSearchParams(location.search).get("state");
const state = screen.states.find((s) => s.id === asked) ?? screen.states[0] ?? { id: "home", name: "Home", at: "/" };

const server = "https://my.example";
const data = createAppData(createClient({ server, retry: { disabled: true }, fetch: serve }));
const router = createAppRouter({
  data,
  config: { apiOrigin: server, instanceName: "my.example", defaultLocale: "en", clientId: "my-web" },
  at: state.at,
});

// Down the end of the page, halfway, away from the new posts' button at the top and the overlay at the bottom.
const control: CSSProperties = {
  position: "fixed",
  insetBlockStart: "40%",
  insetInlineEnd: "0.5rem",
  zIndex: 10,
  display: "flex",
  flexDirection: "column",
  gap: "0.5rem",
};

const strip: CSSProperties = {
  display: "flex",
  flexWrap: "wrap",
  gap: "0.25rem 0.75rem",
  padding: "0.375rem 1rem",
  borderBlockEnd: "1px dashed currentColor",
  fontFamily: "ui-monospace, monospace",
  fontSize: "0.75rem",
};

/** The catalog's strip of the screen's states: not a part of the app. */
function States() {
  if (screen.states.length < 2) return null;
  return (
    <nav aria-label="Catalog: states" style={strip}>
      {screen.states.map(({ id, name }) => {
        const next = new URLSearchParams(location.search);
        next.set("state", id);
        return (
          <a key={id} href={`?${next.toString()}`} aria-current={id === state.id ? "page" : undefined}>
            {name}
          </a>
        );
      })}
    </nav>
  );
}

/** The catalog's controls over the app, which stand in for the stream: not a part of it. */
function Controls({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div role="group" aria-label={`Catalog: ${label}`} style={control}>
      {children}
    </div>
  );
}

/** Posts `count` new samples on the fake server and hands them to the timeline as the stream does, as the API's. */
async function post(count: number) {
  newPosts(count);
  const { data: answer } = await data.api.GET("/timelines/home", { params: { query: {} } });
  receiveHomeStatuses(data, answer?.items.slice(0, count) ?? []);
}

function TimelineControls() {
  return (
    <Controls label="new posts">
      <Button
        size="timeline"
        onPress={() => {
          void post(1);
        }}
      >
        Add a post
      </Button>
      <Button
        size="timeline"
        onPress={() => {
          void post(3);
        }}
      >
        Add 3
      </Button>
      <Button
        size="timeline"
        onPress={() => {
          receiveHomeGap(data);
        }}
      >
        Reconnect
      </Button>
    </Controls>
  );
}

function NotificationControls() {
  return (
    <Controls label="notifications">
      <Button
        size="timeline"
        onPress={() => {
          receiveNotification(data, newNotification() as NotificationItem);
        }}
      >
        Receive one
      </Button>
    </Controls>
  );
}

/** The app at the screen of the catalog's page, under the catalog's I18nProvider, with the catalog's controls. */
export function AppPreview() {
  return (
    <>
      <States />
      <AppRoot router={router} />
      {page === "/app/timeline" && <TimelineControls />}
      {page === "/app/notifications" && <NotificationControls />}
    </>
  );
}
