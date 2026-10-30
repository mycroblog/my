// SPDX-License-Identifier: AGPL-3.0-only
// Real-time updates of the home, the local and the federated timelines through the whole app (web-client/README.md S10,
// Y7, T4; FR-207; ADR-0127 §5): the
// layout's event stream, MSW as the server's GET /streaming, and the "N new posts" button.
import { act, screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { afterEach, describe, expect, test, vi } from "vitest";
import type { Status } from "./api/timelines";
import { apiBase, oauthPaths, problem, resumableSession, tokenResponse } from "../../test/msw/http";
import { eventStreamResponse, streamingServer, type TestEventStream } from "../../test/msw/streaming";
import { preferencesFixture, statusPage, timelineStatusFixture, viewerFixture } from "../../test/fixtures";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";

/** A post newer than the fixture of the home timeline. */
function newStatus(n: number): Status {
  const id = `0192f0c4-7a8e-7000-8000-0000000001${String(n).padStart(2, "0")}`;
  return {
    ...timelineStatusFixture,
    id,
    uri: `https://my.example/ap/statuses/${id}`,
    url: `https://my.example/@bob/status/${id}`,
    content: `<p>New post ${String(n)}</p>`,
  };
}

/** The reconnect waits about a second after a stream ends (@mycroblog/mykit runStream). */
const reconnectTimeout = { timeout: 3_000 };

async function openHome() {
  const view = await renderApp({ path: "/home" });
  const feed = await screen.findByRole("feed", { name: "Home" });
  return { ...view, feed };
}

describe("new posts in the home timeline", () => {
  test("wait behind the button, are not inserted on their own, and show at the top when it is pressed", async () => {
    const streams = streamingServer();
    server.use(resumableSession(), streams.handler);
    const { user, feed } = await openHome();
    await waitFor(() => {
      expect(streams.connections()).toBe(1);
    });

    streams.current().send("update", newStatus(1));
    expect(await screen.findByRole("button", { name: "1 new post" })).toBeInTheDocument();
    streams.current().send("update", newStatus(2));
    // The same entry twice counts once.
    streams.current().send("update", newStatus(2));
    const button = await screen.findByRole("button", { name: "2 new posts" });

    // The feed did not change, and the count is in a polite status region.
    expect(within(feed).getAllByRole("article")).toHaveLength(1);
    expect(
      screen
        .getAllByRole("status")
        .filter((region) => within(region).queryByRole("button", { name: "2 new posts" }) !== null),
    ).toHaveLength(1);

    await user.click(button);

    const articles = await within(feed).findAllByRole("article", {}, { timeout: 2_000 });
    expect(articles).toHaveLength(3);
    expect(articles[0]).toHaveAccessibleDescription("New post 2");
    expect(articles[1]).toHaveAccessibleDescription("New post 1");
    expect(articles[2]).toHaveAccessibleDescription("Hello from the local timeline!");
    // The button went away; focus is on the heading above the new posts.
    expect(screen.queryByRole("button", { name: /new post/ })).not.toBeInTheDocument();
    expect(screen.getByRole("heading", { level: 1, name: "Home" })).toHaveFocus();
  });

  test("do not count an entry that the timeline has loaded already", async () => {
    const streams = streamingServer();
    server.use(resumableSession(), streams.handler);
    const { feed } = await openHome();
    await waitFor(() => {
      expect(streams.connections()).toBe(1);
    });

    streams.current().send("update", timelineStatusFixture);
    streams.current().send("update", newStatus(1));

    expect(await screen.findByRole("button", { name: "1 new post" })).toBeInTheDocument();
    expect(within(feed).getAllByRole("article")).toHaveLength(1);
  });

  test("that arrive while another page is open are on the timeline when it opens again, without the button", async () => {
    const streams = streamingServer();
    server.use(resumableSession(), streams.handler);
    const { router, queryClient } = await openHome();
    await waitFor(() => {
      expect(streams.connections()).toBe(1);
    });
    await act(() => router.navigate({ to: "/local" }));
    await screen.findByRole("feed", { name: "Local timeline" });

    // The cached home timeline is not stale yet, so opening it again does not fetch it (the server would
    // not have the new post either: its handler answers the fixture only). Wait until the app has the entry, waiting or in the timeline's cache. It goes to the stream open at
    // each try: a slow runner may still be opening a new one after the navigation (the same entry counts once).
    await waitFor(
      () => {
        streams.current().send("update", newStatus(1));
        const cached = [
          queryClient.getQueryData(["stream", "home-new-statuses"]),
          queryClient.getQueryData(["get", "/timelines/home", "pages"]),
        ];
        expect(JSON.stringify(cached), `after ${String(streams.connections())} connections`).toContain(newStatus(1).id);
      },
      { timeout: 3_000 },
    );
    await act(() => router.navigate({ to: "/home" }));

    const feed = await screen.findByRole("feed", { name: "Home" });
    const articles = await within(feed).findAllByRole("article");
    expect(articles).toHaveLength(2);
    expect(articles[0]).toHaveAccessibleDescription("New post 1");
    expect(screen.queryByRole("button", { name: /new post/ })).not.toBeInTheDocument();
  });
});

describe("the event stream", () => {
  test("reconnects after the server ends it, and counts what the home timeline got meanwhile", async () => {
    let home = [timelineStatusFixture];
    const streams = streamingServer();
    server.use(
      resumableSession(),
      streams.handler,
      http.get(`${apiBase}/timelines/home`, () => HttpResponse.json(statusPage(home))),
    );
    const { feed } = await openHome();
    await waitFor(() => {
      expect(streams.connections()).toBe(1);
    });

    // A post arrives while the stream is down; nothing replays it.
    home = [newStatus(1), timelineStatusFixture];
    streams.current().close();

    await waitFor(() => {
      expect(streams.connections()).toBe(2);
    }, reconnectTimeout);
    expect(await screen.findByRole("button", { name: "1 new post" })).toBeInTheDocument();
    expect(within(feed).getAllByRole("article")).toHaveLength(1);

    // The new stream delivers again.
    streams.current().send("update", newStatus(2));
    expect(await screen.findByRole("button", { name: "2 new posts" })).toBeInTheDocument();
  });

  test("loads the timeline again when what it missed may not join what is loaded", async () => {
    let page = statusPage([timelineStatusFixture]);
    const streams = streamingServer();
    server.use(
      resumableSession(),
      streams.handler,
      http.get(`${apiBase}/timelines/home`, () => HttpResponse.json(page)),
    );
    const { user, feed } = await openHome();
    await waitFor(() => {
      expect(streams.connections()).toBe(1);
    });

    // A whole page of news, with more behind it, arrives while the stream is down.
    page = statusPage([newStatus(3), newStatus(2)], "older");
    streams.current().close();
    await user.click(await screen.findByRole("button", { name: "2 new posts" }, reconnectTimeout));

    // The feed starts again from the new first page; the older posts follow it on scrolling.
    await waitFor(() => {
      expect(
        within(feed)
          .getAllByRole("article")
          .map((article) => article.getAttribute("aria-posinset")),
      ).toEqual(["1", "2"]);
    });
    expect(within(feed).getAllByRole("article")[0]).toHaveAccessibleDescription("New post 3");
    expect(screen.getByRole("button", { name: "Load more posts" })).toBeInTheDocument();
  });

  test("tries again after a failed connection", async () => {
    let failures = 1;
    const streams = streamingServer();
    server.use(
      resumableSession(),
      http.get(`${apiBase}/streaming`, () => {
        if (failures === 0) return undefined;
        failures -= 1;
        return problem(503, "about:blank", "Service Unavailable");
      }),
      streams.handler,
    );
    await openHome();

    await waitFor(() => {
      expect(streams.connections()).toBe(1);
    }, reconnectTimeout);
    streams.current().send("update", newStatus(1));
    expect(await screen.findByRole("button", { name: "1 new post" })).toBeInTheDocument();
  });

  test("refreshes the access token when the server ended the stream because the token stopped being valid", async () => {
    let issued = 0;
    const tokens: string[] = [];
    const streams: TestEventStream[] = [];
    server.use(
      http.post(oauthPaths.refresh, () => {
        issued += 1;
        return tokenResponse(`token-${String(issued)}`);
      }),
      // The other operations of the page take any token, so that only the stream refreshes it.
      http.get(`${apiBase}/timelines/home`, () => HttpResponse.json(statusPage([timelineStatusFixture]))),
      http.get(`${apiBase}/accounts/verify_credentials`, () => HttpResponse.json(viewerFixture)),
      http.get(`${apiBase}/notifications/marker`, () => HttpResponse.json({ unread_count: 0 })),
      http.get(`${apiBase}/preferences`, () => HttpResponse.json(preferencesFixture)),
      http.get(`${apiBase}/announcements`, () => HttpResponse.json({ items: [] })),
      http.get(`${apiBase}/streaming`, ({ request }) => {
        const token = request.headers.get("Authorization");
        tokens.push(token ?? "");
        // Only the newest token is valid.
        if (token !== `Bearer token-${String(issued)}`)
          return problem(401, "/api/v1/problems/invalid-token", "Invalid token");
        const { response, stream } = eventStreamResponse();
        streams.push(stream);
        return response;
      }),
    );
    await openHome();
    await waitFor(() => {
      expect(streams).toHaveLength(1);
    });

    // The token expires: the next heartbeat ends the stream, and the old token is refused.
    issued += 1;
    streams[0]?.close();

    await waitFor(() => {
      expect(streams).toHaveLength(2);
    }, reconnectTimeout);
    expect(tokens).toEqual(["Bearer token-1", "Bearer token-1", "Bearer token-3"]);
    streams[1]?.send("update", newStatus(1));
    expect(await screen.findByRole("button", { name: "1 new post" })).toBeInTheDocument();
  });

  test("closes when the session ends", async () => {
    const streams = streamingServer();
    server.use(resumableSession(), streams.handler);
    const { session } = await openHome();
    await waitFor(() => {
      expect(streams.connections()).toBe(1);
    });

    await session.signOut();

    expect(await screen.findByRole("heading", { level: 2, name: "You are not signed in" })).toBeInTheDocument();
    // The aborted stream does not come back.
    await new Promise((resolve) => setTimeout(resolve, 1_500));
    expect(streams.connections()).toBe(1);
  });
});

/** Stands in for the browser's IntersectionObserver, which jsdom lacks: tests say whether the feed's top is in view. */
class FakeIntersectionObserver {
  static observers = new Set<FakeIntersectionObserver>();
  readonly targets = new Set<Element>();
  private readonly callback: IntersectionObserverCallback;
  constructor(callback: IntersectionObserverCallback) {
    this.callback = callback;
    FakeIntersectionObserver.observers.add(this);
  }
  observe(target: Element) {
    this.targets.add(target);
  }
  unobserve(target: Element) {
    this.targets.delete(target);
  }
  disconnect() {
    FakeIntersectionObserver.observers.delete(this);
  }
  takeRecords() {
    return [];
  }
  /** Reports whether the marker at the feed's top (the one `aria-hidden` target) is in view. */
  static setTopInView(isIntersecting: boolean) {
    for (const observer of FakeIntersectionObserver.observers) {
      for (const target of observer.targets) {
        if (target.getAttribute("aria-hidden") === "true")
          observer.callback(
            [{ target, isIntersecting } as IntersectionObserverEntry],
            observer as unknown as IntersectionObserver,
          );
      }
    }
  }
}

describe("new posts inserted on their own (ADR-0077)", () => {
  async function openHomeAutoInserting(autoInsert = true) {
    vi.stubGlobal("IntersectionObserver", FakeIntersectionObserver);
    vi.spyOn(window, "scrollTo").mockImplementation(() => undefined);
    const streams = streamingServer();
    server.use(resumableSession(), streams.handler);
    server.use(
      http.get(`${apiBase}/preferences`, () =>
        HttpResponse.json({ ...preferencesFixture, timeline_auto_insert: autoInsert }),
      ),
    );
    const view = await openHome();
    await waitFor(() => {
      expect(streams.connections()).toBe(1);
    });
    return { ...view, streams };
  }

  afterEach(() => {
    FakeIntersectionObserver.observers.clear();
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  test("insert at the top at once, without a button, a focus move or an announcement", async () => {
    const { streams, feed } = await openHomeAutoInserting();

    streams.current().send("update", newStatus(1));

    await waitFor(() => {
      expect(within(feed).getAllByRole("article")).toHaveLength(2);
    });
    expect(within(feed).getAllByRole("article")[0]).toHaveAccessibleDescription("New post 1");
    expect(screen.queryByRole("button", { name: /new post/ })).not.toBeInTheDocument();
    expect(document.body).toHaveFocus();
    expect(
      within(feed)
        .getAllByRole("article")
        .map((article) => article.getAttribute("aria-posinset")),
    ).toEqual(["1", "2"]);
  });

  test("keep waiting behind the button when the setting is off", async () => {
    const { streams, feed } = await openHomeAutoInserting(false);

    streams.current().send("update", newStatus(1));

    expect(await screen.findByRole("button", { name: "1 new post" })).toBeInTheDocument();
    expect(within(feed).getAllByRole("article")).toHaveLength(1);
  });

  test("insert away from the top and count what came in since leaving it; pressing the button goes to the top", async () => {
    const { streams, feed, user } = await openHomeAutoInserting();
    act(() => {
      FakeIntersectionObserver.setTopInView(false);
    });

    streams.current().send("update", newStatus(1));
    streams.current().send("update", newStatus(2));

    const button = await screen.findByRole("button", { name: "2 new posts" });
    expect(within(feed).getAllByRole("article")).toHaveLength(3);

    await user.click(button);
    expect(window.scrollTo).toHaveBeenCalledWith(expect.objectContaining({ top: 0 }));
    expect(screen.getByRole("heading", { level: 1, name: "Home" })).toHaveFocus();
  });

  test("reset the count when the reader is back at the top", async () => {
    const { streams } = await openHomeAutoInserting();
    act(() => {
      FakeIntersectionObserver.setTopInView(false);
    });
    streams.current().send("update", newStatus(1));
    await screen.findByRole("button", { name: "1 new post" });

    act(() => {
      FakeIntersectionObserver.setTopInView(true);
    });

    await waitFor(() => {
      expect(screen.queryByRole("button", { name: /new post/ })).not.toBeInTheDocument();
    });
  });

  test("wait while focus is in the feed, and insert when it leaves", async () => {
    const { streams, feed, user } = await openHomeAutoInserting();
    within(feed).getAllByRole("article")[0]?.focus();

    streams.current().send("update", newStatus(1));
    await screen.findByRole("button", { name: "1 new post" });
    expect(within(feed).getAllByRole("article")).toHaveLength(1);
    // The reader's place is not moved: the focused post is still the first one.
    expect(within(feed).getAllByRole("article")[0]).toHaveFocus();

    await user.tab({ shift: true });
    await waitFor(() => {
      expect(within(feed).getAllByRole("article")).toHaveLength(2);
    });
    expect(screen.queryByRole("button", { name: /new post/ })).not.toBeInTheDocument();
  });

  test("wait while text in the feed is selected, and insert when the selection ends", async () => {
    const { streams, feed } = await openHomeAutoInserting();
    const text = within(feed).getByText("Hello from the local timeline!");
    const range = document.createRange();
    range.selectNodeContents(text);
    act(() => {
      document.getSelection()?.removeAllRanges();
      document.getSelection()?.addRange(range);
      document.dispatchEvent(new Event("selectionchange"));
    });

    streams.current().send("update", newStatus(1));
    await screen.findByRole("button", { name: "1 new post" });
    expect(within(feed).getAllByRole("article")).toHaveLength(1);

    act(() => {
      document.getSelection()?.removeAllRanges();
      document.dispatchEvent(new Event("selectionchange"));
    });
    await waitFor(() => {
      expect(within(feed).getAllByRole("article")).toHaveLength(2);
    });
  });

  test("load the first page again at the top when what was missed may not join the loaded posts", async () => {
    let page = statusPage([timelineStatusFixture]);
    vi.stubGlobal("IntersectionObserver", FakeIntersectionObserver);
    const streams = streamingServer();
    server.use(resumableSession(), streams.handler);
    server.use(
      http.get(`${apiBase}/preferences`, () =>
        HttpResponse.json({ ...preferencesFixture, timeline_auto_insert: true }),
      ),
      http.get(`${apiBase}/timelines/home`, () => HttpResponse.json(page)),
    );
    const { feed } = await openHome();
    await waitFor(() => {
      expect(streams.connections()).toBe(1);
    });

    page = statusPage([newStatus(3), newStatus(2)], "older");
    streams.current().close();

    await waitFor(
      () => {
        expect(within(feed).getAllByRole("article")[0]).toHaveAccessibleDescription("New post 3");
      },
      { timeout: 4_000 },
    );
    expect(screen.queryByRole("button", { name: /new post/ })).not.toBeInTheDocument();
  });

  test("offer the reload on the button instead when the reader is away from the top", async () => {
    let page = statusPage([timelineStatusFixture]);
    vi.stubGlobal("IntersectionObserver", FakeIntersectionObserver);
    vi.spyOn(window, "scrollTo").mockImplementation(() => undefined);
    const streams = streamingServer();
    server.use(resumableSession(), streams.handler);
    server.use(
      http.get(`${apiBase}/preferences`, () =>
        HttpResponse.json({ ...preferencesFixture, timeline_auto_insert: true }),
      ),
      http.get(`${apiBase}/timelines/home`, () => HttpResponse.json(page)),
    );
    const { feed, user } = await openHome();
    await waitFor(() => {
      expect(streams.connections()).toBe(1);
    });
    act(() => {
      FakeIntersectionObserver.setTopInView(false);
    });

    page = statusPage([newStatus(3), newStatus(2)], "older");
    streams.current().close();
    await user.click(await screen.findByRole("button", { name: "2 new posts" }, reconnectTimeout));

    await waitFor(() => {
      expect(within(feed).getAllByRole("article")[0]).toHaveAccessibleDescription("New post 3");
    });
  });
});

describe("new posts in the local and the federated timelines (ADR-0127 §5)", () => {
  /** Opens the timeline's page signed in, with the auto-insert setting, once its stream is open. */
  async function openPublic(kind: "local" | "public", autoInsert: boolean) {
    vi.stubGlobal("IntersectionObserver", FakeIntersectionObserver);
    vi.spyOn(window, "scrollTo").mockImplementation(() => undefined);
    const streams = streamingServer(`/streaming/timelines/${kind === "local" ? "local" : "federated"}`);
    server.use(
      resumableSession(),
      streams.handler,
      http.get(`${apiBase}/preferences`, () =>
        HttpResponse.json({ ...preferencesFixture, timeline_auto_insert: autoInsert }),
      ),
    );
    const view = await renderApp({ path: `/${kind}` });
    const feed = await screen.findByRole("feed", { name: kind === "local" ? "Local timeline" : "Federated timeline" });
    await waitFor(() => {
      expect(streams.connections()).toBe(1);
    });
    return { ...view, feed, streams };
  }

  afterEach(() => {
    FakeIntersectionObserver.observers.clear();
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  test("wait behind the button on the local timeline, show on pressing it, and the stream closes when the page is left", async () => {
    const { streams, feed, user, router, queryClient } = await openPublic("local", false);

    streams.current().send("update", newStatus(1));
    const button = await screen.findByRole("button", { name: "1 new post" });
    expect(within(feed).getAllByRole("article")).toHaveLength(1);

    await user.click(button);
    const articles = await within(feed).findAllByRole("article", {}, { timeout: 2_000 });
    expect(articles).toHaveLength(2);
    expect(articles[0]).toHaveAccessibleDescription("New post 1");
    expect(screen.getByRole("heading", { level: 1, name: "Local timeline" })).toHaveFocus();

    await act(() => router.navigate({ to: "/home" }));
    await screen.findByRole("feed", { name: "Home" });
    // The client no longer reads the stream: what the server still sends reaches neither the waiting posts nor the
    // cached timeline, and the stream does not come back while the page is not shown.
    streams.current().send("update", newStatus(2));
    await new Promise((resolve) => setTimeout(resolve, 1_500));
    expect(
      JSON.stringify([
        queryClient.getQueryData(["stream", "local-new-statuses"]),
        queryClient.getQueryData(["get", "/timelines/local", "pages"]),
      ]),
    ).not.toContain(newStatus(2).id);
    expect(streams.connections()).toBe(1);
  });

  test("insert at the top of the federated timeline at once with the auto-insert setting", async () => {
    const { streams, feed } = await openPublic("public", true);

    streams.current().send("update", newStatus(1));

    await waitFor(() => {
      expect(within(feed).getAllByRole("article")).toHaveLength(3);
    });
    expect(within(feed).getAllByRole("article")[0]).toHaveAccessibleDescription("New post 1");
    expect(screen.queryByRole("button", { name: /new post/ })).not.toBeInTheDocument();
  });

  test("wait while focus is in the feed, and insert when it leaves", async () => {
    const { streams, feed, user } = await openPublic("local", true);
    within(feed).getAllByRole("article")[0]?.focus();

    streams.current().send("update", newStatus(1));
    await screen.findByRole("button", { name: "1 new post" });
    expect(within(feed).getAllByRole("article")).toHaveLength(1);

    await user.tab({ shift: true });
    await waitFor(() => {
      expect(within(feed).getAllByRole("article")).toHaveLength(2);
    });
  });

  test("read the first page again after the stream reconnected", async () => {
    let local = [timelineStatusFixture];
    server.use(http.get(`${apiBase}/timelines/local`, () => HttpResponse.json(statusPage(local))));
    const { streams, feed } = await openPublic("local", false);

    // A post arrives while the stream is down; nothing replays it.
    local = [newStatus(1), timelineStatusFixture];
    streams.current().close();

    await waitFor(() => {
      expect(streams.connections()).toBe(2);
    }, reconnectTimeout);
    expect(await screen.findByRole("button", { name: "1 new post" })).toBeInTheDocument();
    expect(within(feed).getAllByRole("article")).toHaveLength(1);
  });

  test("open no stream for a visitor who is not signed in", async () => {
    const streams = streamingServer("/streaming/timelines/local");
    server.use(streams.handler);
    await renderApp({ path: "/local" });
    await screen.findByRole("feed", { name: "Local timeline" });

    await new Promise((resolve) => setTimeout(resolve, 500));
    expect(streams.connections()).toBe(0);
  });
});
