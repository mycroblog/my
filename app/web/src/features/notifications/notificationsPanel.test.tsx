// SPDX-License-Identifier: AGPL-3.0-only
// The latest notifications in the third column on the timeline pages (web-client/09-app-shell.md §9): at most ten, kept
// up to date by the event stream with no request of their own, read again after a reconnect, marking nothing read;
// only from xl, signed in, on a timeline page, with no request anywhere else; a failure stays inside the panel.
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { afterEach, describe, expect, test, vi } from "vitest";
import { otherAccountFixture, type Schemas } from "../../test/fixtures";
import { apiBase, problem, signedInHandler } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { streamingServer } from "../../test/msw/streaming";
import { renderApp } from "../../test/render";
import type { Notification } from "./api/notifications";

afterEach(() => {
  vi.unstubAllGlobals();
});

/** Makes the screen as wide as xl (80rem), where the layout has the third column; jsdom has no matchMedia otherwise. */
function stubWideScreen() {
  vi.stubGlobal("matchMedia", (query: string) => ({
    matches: query === "(min-width: 80rem)" || query === "(min-width: 48rem)",
    media: query,
    addEventListener: () => undefined,
    removeEventListener: () => undefined,
  }));
}

/** A follow by an account named `name` numbered `n`; a higher number is newer. */
function follow(n: number): Notification {
  const name = `Person ${String(n)}`;
  return {
    id: `0192f0c4-7a8e-7000-8000-0000000d${String(n).padStart(4, "0")}`,
    type: "follow",
    created_at: "2026-10-16T09:00:00Z",
    account: { ...otherAccountFixture, id: `account-${String(n)}`, display_name: name },
  };
}

/** Numbers `from` down to `to`, the newest first. */
function follows(from: number, to: number): Notification[] {
  return Array.from({ length: from - to + 1 }, (_, index) => follow(from - index));
}

/** GET /notifications answering from `read()` as the server does, up to its `limit`, counting the panel's requests. */
function notificationsServer(read: () => readonly Notification[]) {
  const requests = { panel: 0 };
  return {
    requests,
    handler: http.get(`${apiBase}/notifications`, ({ request }) => {
      const limit = new URL(request.url).searchParams.get("limit");
      if (limit !== null) requests.panel += 1;
      return HttpResponse.json({ items: read().slice(0, limit === null ? 20 : Number(limit)) });
    }),
  };
}

const panelName = "Notifications";

const announcement: Schemas["Announcement"] = {
  id: "0199f0e0-0000-7000-8000-00000000b001",
  title: "Maintenance on Sunday",
  content: "<p>Maintenance on Sunday.</p>",
  published_at: "2026-10-21T00:00:00Z",
  updated_at: "2026-10-21T00:00:00Z",
  read: false,
  ended: false,
};

/** The names of the rows of the panel, in their order. */
function rowNames(panel: HTMLElement): (string | undefined)[] {
  return within(panel)
    .getAllByRole("article")
    .map((row) => /Person \d+/u.exec(row.textContent)?.[0]);
}

describe("the notifications panel", () => {
  test("shows the latest ten, newest first, with a link to the page, between the other panels", async () => {
    stubWideScreen();
    const { handler, requests } = notificationsServer(() => follows(15, 1));
    server.use(
      signedInHandler,
      handler,
      http.get(`${apiBase}/announcements`, () => HttpResponse.json({ items: [announcement] })),
    );
    await renderApp({ path: "/home" });

    const panel = await screen.findByRole("complementary", { name: panelName });
    await within(panel).findAllByRole("article");
    expect(rowNames(panel)).toEqual(follows(15, 6).map((n) => n.account?.display_name));
    expect(within(panel).getByRole("link", { name: "All notifications" })).toHaveAttribute("href", "/notifications");
    expect(requests.panel).toBe(1);
    const announcements = await screen.findByRole("complementary", { name: /^Announcements/u });
    const about = screen.getByRole("complementary", { name: "About this server" });
    expect(announcements.compareDocumentPosition(panel) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(panel.compareDocumentPosition(about) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  test("takes a notification from the stream at the top, stays at ten, and reads nothing for it", async () => {
    stubWideScreen();
    const streams = streamingServer();
    const { handler, requests } = notificationsServer(() => follows(10, 1));
    server.use(signedInHandler, streams.handler, handler);
    await renderApp({ path: "/home" });
    const panel = await screen.findByRole("complementary", { name: panelName });
    await within(panel).findByText("Person 10");
    await waitFor(() => {
      expect(streams.connections()).toBe(1);
    });

    streams.current().send("notification", follow(11));

    expect(await within(panel).findByText("Person 11")).toBeInTheDocument();
    expect(rowNames(panel)).toEqual(follows(11, 2).map((n) => n.account?.display_name));
    expect(requests.panel).toBe(1);
  });

  test("reads them again when a notification comes during the first read, which may have missed it", async () => {
    stubWideScreen();
    const streams = streamingServer();
    let release = (): void => undefined;
    const released = new Promise<void>((resolve) => {
      release = resolve;
    });
    let reads = 0;
    server.use(
      signedInHandler,
      streams.handler,
      http.get(`${apiBase}/notifications`, async () => {
        reads += 1;
        if (reads > 1) return HttpResponse.json({ items: follows(2, 1) });
        await released;
        return HttpResponse.json({ items: follows(1, 1) });
      }),
    );
    await renderApp({ path: "/home" });
    const panel = await screen.findByRole("complementary", { name: panelName });
    await waitFor(() => {
      expect(streams.connections()).toBe(1);
    });
    await waitFor(() => {
      expect(reads).toBe(1);
    });

    streams.current().send("notification", follow(2));

    expect(await within(panel).findByText("Person 2")).toBeInTheDocument();
    release();
  });

  test("reads the notifications again after the stream reconnected", async () => {
    stubWideScreen();
    const streams = streamingServer();
    let items = follows(3, 1);
    const { handler } = notificationsServer(() => items);
    server.use(signedInHandler, streams.handler, handler);
    await renderApp({ path: "/home" });
    const panel = await screen.findByRole("complementary", { name: panelName });
    await within(panel).findByText("Person 3");
    await waitFor(() => {
      expect(streams.connections()).toBe(1);
    });

    // One arrives while the stream is down.
    items = follows(4, 1);
    streams.current().close();

    expect(await within(panel).findByText("Person 4", {}, { timeout: 3_000 })).toBeInTheDocument();
  });

  test("marks nothing read, and marks the unread rows", async () => {
    stubWideScreen();
    let markerMoves = 0;
    const { handler } = notificationsServer(() => follows(2, 1));
    server.use(
      signedInHandler,
      handler,
      http.get(`${apiBase}/notifications/marker`, () =>
        HttpResponse.json({ last_read_id: follow(1).id, unread_count: 1 }),
      ),
      http.put(`${apiBase}/notifications/marker`, () => {
        markerMoves += 1;
        return HttpResponse.json({ last_read_id: follow(2).id, unread_count: 0 });
      }),
    );
    await renderApp({ path: "/home" });

    const panel = await screen.findByRole("complementary", { name: panelName });
    expect(await within(panel).findByRole("article", { name: /^Unread:.*Person 2/u })).toBeInTheDocument();
    expect(within(panel).getByRole("article", { name: /^Person 1/u })).toBeInTheDocument();
    expect(markerMoves).toBe(0);
  });

  test("has no buttons on its rows", async () => {
    stubWideScreen();
    const { handler } = notificationsServer(() => [follow(1)]);
    server.use(signedInHandler, handler);
    await renderApp({ path: "/home" });

    const panel = await screen.findByRole("complementary", { name: panelName });
    await within(panel).findByText("Person 1");
    expect(within(panel).queryByRole("button")).not.toBeInTheDocument();
  });

  test("says so when there are none", async () => {
    stubWideScreen();
    const { handler } = notificationsServer(() => []);
    server.use(signedInHandler, handler);
    await renderApp({ path: "/home" });

    const panel = await screen.findByRole("complementary", { name: panelName });
    expect(await within(panel).findByText("No notifications yet")).toBeInTheDocument();
  });

  test("shows that it is loading", async () => {
    stubWideScreen();
    server.use(
      signedInHandler,
      http.get(`${apiBase}/notifications`, async () => {
        await new Promise(() => undefined);
        return HttpResponse.json({ items: [] });
      }),
    );
    await renderApp({ path: "/home" });

    const panel = await screen.findByRole("complementary", { name: panelName });
    expect(within(panel).getByRole("progressbar", { name: "Loading notifications" })).toBeInTheDocument();
  });

  test("keeps a failure inside the panel, with the timeline around it, and tries again", async () => {
    stubWideScreen();
    const user = userEvent.setup();
    let failing = true;
    server.use(
      signedInHandler,
      http.get(`${apiBase}/notifications`, () =>
        failing ? problem(400, "about:blank", "Bad Request") : HttpResponse.json({ items: [follow(1)] }),
      ),
    );
    await renderApp({ path: "/home" });

    const panel = await screen.findByRole("complementary", { name: panelName });
    expect(
      await within(panel).findByRole("heading", { level: 3, name: "Could not load the notifications" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("feed")).toBeInTheDocument();

    failing = false;
    await user.click(within(panel).getByRole("button", { name: "Try again" }));

    expect(await within(panel).findByText("Person 1")).toBeInTheDocument();
  });

  test("keeps the failure and the button's focus through a retry, announces one that fails again, and then focuses its heading", async () => {
    stubWideScreen();
    const user = userEvent.setup();
    let answer = (): Response | Promise<Response> => problem(400, "about:blank", "Bad Request");
    server.use(
      signedInHandler,
      http.get(`${apiBase}/notifications`, () => answer()),
    );
    await renderApp({ path: "/home" });
    const panel = await screen.findByRole("complementary", { name: panelName });
    await within(panel).findByText("Bad Request");

    let release = (): void => undefined;
    answer = async () => {
      await new Promise<void>((resolve) => {
        release = resolve;
      });
      return problem(400, "about:blank", "Still bad");
    };
    const button = within(panel).getByRole("button", { name: "Try again" });
    await user.click(button);
    await waitFor(() => {
      expect(button).toHaveAttribute("data-pending", "true");
    });
    expect(button).toHaveFocus();
    expect(within(panel).getByText("Bad Request")).toBeInTheDocument();
    expect(within(panel).queryByRole("alert")).not.toBeInTheDocument();

    release();
    expect(await within(panel).findByRole("alert")).toHaveTextContent("Still bad");
    expect(button).toHaveFocus();

    answer = () => HttpResponse.json({ items: [follow(1)] });
    await user.click(button);
    expect(await within(panel).findByText("Person 1")).toBeInTheDocument();
    expect(within(panel).getByRole("heading", { level: 2, name: panelName })).toHaveFocus();
  });

  test.each(["/settings", "/bookmarks", "/notifications", "/@bob"])("is not on %s, and reads nothing", async (path) => {
    stubWideScreen();
    const { handler, requests } = notificationsServer(() => [follow(1)]);
    server.use(signedInHandler, handler);
    await renderApp({ path });

    expect(await screen.findByRole("complementary", { name: "About this server" })).toBeInTheDocument();
    expect(await screen.findByRole("heading", { level: 1 })).toBeInTheDocument();
    expect(screen.queryByRole("complementary", { name: panelName })).not.toBeInTheDocument();
    expect(requests.panel).toBe(0);
  });

  test.each(["/local", "/public", "/tags/fediverse", "/lists/0192f0c4-7a8e-7000-8000-0000000000f1"])(
    "is on %s",
    async (path) => {
      stubWideScreen();
      const { handler } = notificationsServer(() => [follow(1)]);
      server.use(signedInHandler, handler);
      await renderApp({ path });

      const panel = await screen.findByRole("complementary", { name: panelName });
      expect(await within(panel).findByText("Person 1")).toBeInTheDocument();
    },
  );

  test("is not below xl, and reads nothing there", async () => {
    const { handler, requests } = notificationsServer(() => [follow(1)]);
    server.use(signedInHandler, handler);
    await renderApp({ path: "/home" });

    expect(await screen.findByRole("feed")).toBeInTheDocument();
    expect(screen.queryByRole("complementary", { name: panelName })).not.toBeInTheDocument();
    expect(requests.panel).toBe(0);
  });

  test("is not there signed out", async () => {
    stubWideScreen();
    const { handler, requests } = notificationsServer(() => [follow(1)]);
    server.use(handler);
    await renderApp({ path: "/local" });

    expect(await screen.findByRole("feed")).toBeInTheDocument();
    expect(screen.queryByRole("complementary", { name: panelName })).not.toBeInTheDocument();
    expect(requests.panel).toBe(0);
  });
});
