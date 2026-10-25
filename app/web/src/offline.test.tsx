// SPDX-License-Identifier: AGPL-3.0-only
// Integration tests of going offline and back (web-client/16-look-states-and-wording.md G17, web-client/README.md T4): the app's one offline bar, what a page and a
// region show when they cannot load offline, what loads again when the app is back online, and the navigation's
// timelines on a start without the network. jsdom has no network to lose, so `goOffline` makes navigator.onLine
// false, fires the `offline` event and lets every request fail as a browser offline does; `goOnline` undoes it.
import { act, screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { timelineQueryOptions } from "./features/timeline";
import { homeTimelineKey, statusPageKey } from "./lib/storage";
import { instanceFixture, statusPage, timelineStatusFixture } from "./test/fixtures";
import { newTabName } from "./test/links";
import { apiBase, resumableSession, withViewer } from "./test/msw/http";
import { server } from "./test/msw/server";
import { renderApp } from "./test/render";

const offlineMessage = "You are offline. Anything that failed to load loads again when you are back online.";
const offlineReason = "You are offline, so the server cannot be reached. Connect to the internet and try again.";

let offline = false;
/** Whether the server is out of reach while the browser still says it is online. */
let unreachable = false;

/** Offline, no request gets a response. `server.use()` puts it before the handlers already in use. */
const noResponseOffline = http.all("*", () => {
  if (offline || unreachable) return HttpResponse.error();
  return undefined;
});

beforeEach(() => {
  offline = false;
  unreachable = false;
  vi.spyOn(navigator, "onLine", "get").mockImplementation(() => !offline);
  server.use(noResponseOffline);
});

afterEach(() => {
  server.events.removeAllListeners("request:start");
});

function goOffline() {
  act(() => {
    offline = true;
    window.dispatchEvent(new Event("offline"));
  });
}

function goOnline() {
  act(() => {
    offline = false;
    window.dispatchEvent(new Event("online"));
  });
}

/** Asserts whether the app's offline bar shows its message, in its polite live region. */
function expectOfflineBar(shown: boolean) {
  if (shown) expect(screen.getAllByRole("status").filter((region) => within(region).queryByText(offlineMessage) !== null)).toHaveLength(1);
  else expect(screen.queryByText(offlineMessage)).not.toBeInTheDocument();
}

/** Counts the requests to `path` that got through, offline or not. */
function countRequests(path: string): { readonly count: number } {
  const counter = { count: 0 };
  server.events.on("request:start", ({ request }) => {
    if (new URL(request.url).pathname === `/api/v1${path}`) counter.count++;
  });
  return counter;
}

describe("the offline bar", () => {
  test("says so while the browser is offline", async () => {
    await renderApp({ path: "/local" });
    await screen.findByRole("feed", { name: "Local timeline" });
    expectOfflineBar(false);

    goOffline();
    expectOfflineBar(true);

    goOnline();
    expectOfflineBar(false);
  });

  test("appears when a request gets no response, and goes when one gets an answer again", async () => {
    unreachable = true;
    const { user } = await renderApp({ path: "/local" });

    // The browser still says it is online: the server is unreachable.
    expect(await screen.findByText("Could not reach the server. Check your connection and try again.")).toBeInTheDocument();
    expectOfflineBar(true);

    unreachable = false;
    await user.click(screen.getByRole("button", { name: "Try again" }));
    expect(await screen.findByRole("feed", { name: "Local timeline" })).toBeInTheDocument();
    expectOfflineBar(false);
  });

  /** Serves an instance with a status page; offline, no request gets a response, as noResponseOffline does. */
  function serveStatusPage() {
    server.use(
      http.get(`${apiBase}/instance`, () =>
        offline || unreachable ? HttpResponse.error() : HttpResponse.json({ ...instanceFixture, status_page_url: "https://status.example/" }),
      ),
    );
  }

  test("links to the server's status page after its message, outside the live region (FR-810)", async () => {
    serveStatusPage();
    await renderApp({ path: "/local" });
    await screen.findByRole("feed", { name: "Local timeline" });
    // GET /instance has answered.
    await waitFor(() => {
      expect(statusPageKey.read()).toBe("https://status.example/");
    });
    expect(screen.queryByRole("link", { name: newTabName("Check the server's status") })).not.toBeInTheDocument();

    goOffline();
    expectOfflineBar(true);
    const link = screen.getByRole("link", { name: newTabName("Check the server's status") });
    expect(link).toHaveAttribute("href", "https://status.example/");
    for (const region of screen.getAllByRole("status")) expect(region).not.toContainElement(link);

    goOnline();
    expect(screen.queryByRole("link", { name: newTabName("Check the server's status") })).not.toBeInTheDocument();
  });

  test("a start without the network links to the status page that the server named last time", async () => {
    serveStatusPage();
    const view = await renderApp({ path: "/local" });
    await screen.findByRole("feed", { name: "Local timeline" });
    await waitFor(() => {
      expect(statusPageKey.read()).toBe("https://status.example/");
    });
    view.unmount();

    // A reload offline: GET /instance gets no response.
    offline = true;
    await renderApp({ path: "/local" });
    await waitFor(() => {
      expectOfflineBar(true);
    });
    expect(screen.getByRole("link", { name: newTabName("Check the server's status") })).toHaveAttribute("href", "https://status.example/");
  });

  test("has no link to a status page when the server has none", async () => {
    await renderApp({ path: "/local" });
    await screen.findByRole("feed", { name: "Local timeline" });
    goOffline();
    expectOfflineBar(true);
    expect(screen.queryByRole("link", { name: newTabName("Check the server's status") })).not.toBeInTheDocument();
  });
});

describe("offline", () => {
  test("the notifications page keeps its heading and says why the list cannot load, then loads it back online", async () => {
    server.use(resumableSession());
    const { router } = await renderApp({ path: "/home" });
    await screen.findByRole("feed", { name: "Home" });

    goOffline();
    await act(() => router.navigate({ href: "/notifications" }));

    expect(await screen.findByRole("heading", { level: 1, name: "Notifications" })).toBeInTheDocument();
    expect(await screen.findByText(offlineReason)).toBeInTheDocument();
    expect(screen.queryByRole("progressbar")).not.toBeInTheDocument();

    goOnline();
    expect(await screen.findByRole("feed", { name: "Notifications" })).toBeInTheDocument();
  });

  test("the local timeline says it cannot load instead of loading forever, and loads by itself back online", async () => {
    const { router } = await renderApp({ path: "/" });
    await screen.findByRole("heading", { level: 1 });

    goOffline();
    await act(() => router.navigate({ href: "/local" }));

    expect(await screen.findByText(offlineReason)).toBeInTheDocument();
    expect(screen.queryByRole("progressbar")).not.toBeInTheDocument();

    goOnline();
    expect(await screen.findByRole("feed", { name: "Local timeline" })).toBeInTheDocument();
  });

  test("back online, only what failed loads again: a timeline already shown is not refetched", async () => {
    server.use(resumableSession());
    const local = countRequests("/timelines/local");
    const notifications = countRequests("/notifications");
    const { router } = await renderApp({ path: "/local" });
    await screen.findByRole("feed", { name: "Local timeline" });
    expect(local.count).toBe(1);

    goOffline();
    await act(() => router.navigate({ href: "/notifications" }));
    await screen.findByText(offlineReason);
    await act(() => router.navigate({ href: "/local" }));
    await screen.findByRole("feed", { name: "Local timeline" });
    const failedTries = notifications.count;

    goOnline();
    // The failed list loads when its page opens again.
    await act(() => router.navigate({ href: "/notifications" }));
    expect(await screen.findByRole("feed", { name: "Notifications" })).toBeInTheDocument();
    expect(notifications.count).toBeGreaterThan(failedTries);
    expect(local.count).toBe(1);
  });

  test("a start without the network offers the timelines the server offered last time", async () => {
    const view = await renderApp({ path: "/local" });
    const navigation = await screen.findByRole("navigation", { name: "Main" });
    expect(await within(navigation).findByRole("link", { name: "Federated" })).toBeInTheDocument();
    view.unmount();

    // A reload offline: GET /instance gets no response.
    offline = true;
    await renderApp({ path: "/local" });
    const offlineNavigation = await screen.findByRole("navigation", { name: "Main" });
    expectOfflineBar(true);
    expect(within(offlineNavigation).getByRole("link", { name: "Federated" })).toBeInTheDocument();
  });

  test("a start without the network or a stored answer does not offer the federated timeline", async () => {
    offline = true;
    await renderApp({ path: "/local" });
    const navigation = await screen.findByRole("navigation", { name: "Main" });
    await waitFor(() => {
      expectOfflineBar(true);
    });
    expect(within(navigation).queryByRole("link", { name: "Federated" })).not.toBeInTheDocument();
    expect(within(navigation).getByRole("link", { name: "Local" })).toBeInTheDocument();
    // The instance fixture serves both; the navigation does not guess.
    expect(instanceFixture.timelines.federated).toBe("everyone");
  });
});

describe("the home timeline offline (FR-604)", () => {
  const savedNotice = /^You are offline\. These are the posts as of .+\.$/;
  const newer = { ...timelineStatusFixture, id: "0192f0c4-7a8e-7000-8000-0000000000b1", content: "<p>A newer post</p>" };

  /** Asserts the posts of the home feed by their text, newest first. */
  async function expectHomePosts(...texts: string[]) {
    const feed = await screen.findByRole("feed", { name: "Home" });
    const articles = within(feed).getAllByRole("article");
    expect(articles).toHaveLength(texts.length);
    texts.forEach((text, i) => {
      expect(articles[i]).toHaveTextContent(text);
    });
  }

  test("a reload offline shows the posts loaded last, says so, and loads the timeline back online", async () => {
    server.use(resumableSession());
    const view = await renderApp({ path: "/home" });
    await expectHomePosts("Hello from the local timeline!");
    expect(homeTimelineKey.read()?.items.map((item) => item["id"])).toEqual([timelineStatusFixture.id]);
    view.unmount();

    // A reload offline: the session cannot be resumed, and the timeline cannot be fetched.
    offline = true;
    server.use(http.get(`${apiBase}/timelines/home`, ({ request }) => withViewer(request, () => HttpResponse.json(statusPage([newer, timelineStatusFixture])))));
    // Before the session's and the timeline's handlers.
    server.use(noResponseOffline);
    await renderApp({ path: "/home" });
    await expectHomePosts("Hello from the local timeline!");
    expect(screen.getByText(savedNotice)).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Sign in" })).not.toBeInTheDocument();
    expectOfflineBar(true);

    // Back online, the session resumes and the timeline loads from the server.
    goOnline();
    await waitFor(() => {
      expect(screen.queryByText(savedNotice)).not.toBeInTheDocument();
    });
    await waitFor(async () => {
      await expectHomePosts("A newer post", "Hello from the local timeline!");
    });
  });

  test("never keeps followers-only or direct posts, nor boosts of them, nor a post edited into one", async () => {
    const followers = { ...timelineStatusFixture, id: "0192f0c4-7a8e-7000-8000-0000000000b2", content: "<p>For followers</p>", visibility: "private" as const };
    const direct = { ...timelineStatusFixture, id: "0192f0c4-7a8e-7000-8000-0000000000b3", content: "<p>Direct</p>", visibility: "direct" as const };
    const boostOfFollowers = { ...timelineStatusFixture, id: "0192f0c4-7a8e-7000-8000-0000000000b4", content: "", boost: followers };
    server.use(
      resumableSession(),
      http.get(`${apiBase}/timelines/home`, ({ request }) =>
        withViewer(request, () => HttpResponse.json(statusPage([newer, followers, direct, boostOfFollowers, timelineStatusFixture]))),
      ),
    );
    const view = await renderApp({ path: "/home" });
    const feed = await screen.findByRole("feed", { name: "Home" });
    expect(within(feed).getAllByRole("article")).toHaveLength(5);
    expect(homeTimelineKey.read()?.items.map((item) => item["id"])).toEqual([newer.id, timelineStatusFixture.id]);

    // An edit narrows a post's audience: the cached post is rewritten, and the kept timeline loses it.
    act(() => {
      view.queryClient.setQueryData(timelineQueryOptions(view.api, { kind: "home" }).queryKey, (data) =>
        data === undefined
          ? data
          : { ...data, pages: data.pages.map((page) => ({ ...page, items: page.items.map((item) => (item.id === newer.id ? { ...item, visibility: "private" as const } : item)) })) },
      );
    });
    expect(homeTimelineKey.read()?.items.map((item) => item["id"])).toEqual([timelineStatusFixture.id]);
    view.unmount();

    // A reload offline shows what was kept.
    offline = true;
    server.use(noResponseOffline);
    await renderApp({ path: "/home" });
    await expectHomePosts("Hello from the local timeline!");
  });

  test("the signed-in timeline says that it shows the posts loaded last while offline", async () => {
    server.use(resumableSession());
    await renderApp({ path: "/home" });
    await screen.findByRole("feed", { name: "Home" });
    expect(screen.queryByText(savedNotice)).not.toBeInTheDocument();

    goOffline();
    expect(screen.getByText(savedNotice)).toBeInTheDocument();

    goOnline();
    expect(screen.queryByText(savedNotice)).not.toBeInTheDocument();
  });

  test("opened offline when the posts are no longer cached, it shows the ones kept on the browser", async () => {
    server.use(resumableSession());
    const { router, queryClient } = await renderApp({ path: "/home" });
    await screen.findByRole("feed", { name: "Home" });
    await act(() => router.navigate({ href: "/local" }));
    await screen.findByRole("feed", { name: "Local timeline" });

    goOffline();
    act(() => {
      queryClient.removeQueries({ queryKey: ["get", "/timelines/home"] });
    });
    await act(() => router.navigate({ href: "/home" }));
    await expectHomePosts("Hello from the local timeline!");
    expect(screen.getByText(savedNotice)).toBeInTheDocument();
  });

  test("a reload offline without kept posts offers to sign in", async () => {
    offline = true;
    await renderApp({ path: "/home" });
    expect(await screen.findByRole("link", { name: "Sign in" })).toBeInTheDocument();
    expect(screen.queryByRole("feed", { name: "Home" })).not.toBeInTheDocument();
  });

  test("the kept posts go when the session ends, and when the server says that there is no session", async () => {
    server.use(resumableSession());
    const view = await renderApp({ path: "/home" });
    await screen.findByRole("feed", { name: "Home" });
    expect(homeTimelineKey.read()).not.toBeNull();

    await act(() => view.session.signOut());
    expect(homeTimelineKey.read()).toBeNull();
    view.unmount();

    // Kept by a session that has expired since: the server answers that there is none.
    homeTimelineKey.write({ loadedAt: Date.now(), items: [timelineStatusFixture], nextCursor: null });
    server.resetHandlers();
    await renderApp({ path: "/home" });
    expect(await screen.findByRole("link", { name: "Sign in" })).toBeInTheDocument();
    expect(homeTimelineKey.read()).toBeNull();
  });
});
