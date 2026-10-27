// SPDX-License-Identifier: AGPL-3.0-only
// A move without a write, back in the history or to a tab seen before, renders from the cache at once
// (web-client/README.md E4, ADR-0045): the loaders return what is cached and not marked stale without asking the
// server, so the router's blocking reload (defaultStaleReloadMode) has nothing to wait for. The server answers
// every list slowly, so that a page that waited for it would be seen.
import type { RouterHistory } from "@tanstack/react-router";
import { act, screen, within } from "@testing-library/react";
import { delay, http, HttpResponse } from "msw";
import { describe, expect, test, vi } from "vitest";
import { statusPage, viewerFixture, viewerStatusFixture } from "./test/fixtures";
import { apiBase, resumableSession } from "./test/msw/http";
import { server } from "./test/msw/server";
import { streamingServer } from "./test/msw/streaming";
import { createAppRouter } from "./router";
import { renderApp } from "./test/render";

/** How long the server takes to answer a list, in ms; longer than a test waits for a page shown from the cache. */
const latency = 1000;

/**
 * A server whose lists of posts are slow, counting the requests for them. Its event stream stays open: the
 * default one ends at once, and the reconnect after it reads the home timeline again (useLiveUpdates).
 */
function slowListsServer() {
  const requests: string[] = [];
  const post = viewerStatusFixture({ content: "<p>Cached and current</p>" });
  server.use(
    resumableSession(),
    streamingServer().handler,
    http.get(`${apiBase}/accounts/:id/statuses`, async ({ request }) => {
      // No pinned posts: the posts tab lists the post once.
      if (new URL(request.url).searchParams.get("pinned") === "true") return HttpResponse.json(statusPage([]));
      requests.push(`statuses?${new URL(request.url).searchParams.toString()}`);
      await delay(latency);
      return HttpResponse.json(statusPage([post]));
    }),
    http.get(`${apiBase}/timelines/home`, async () => {
      requests.push("home");
      await delay(latency);
      return HttpResponse.json(statusPage([post]));
    }),
  );
  return requests;
}

/** Long enough for a render from the cache, too short for a list from the server. */
const fromCache = { timeout: latency / 2 };

describe("a move without a write", () => {
  test("back in the history shows the page from the cache at once, without asking the server again", async () => {
    const requests = slowListsServer();
    let history: RouterHistory | undefined;
    const { user } = await renderApp({
      path: "/home",
      createRouter: (options) => {
        history = options.history;
        return createAppRouter(options);
      },
    });
    expect(await within(await screen.findByRole("feed", { name: "Home" }, { timeout: latency * 3 })).findByText("Cached and current")).toBeInTheDocument();
    await user.click(within(screen.getByRole("navigation", { name: "Main" })).getByRole("link", { name: "Profile" }));
    expect(await screen.findByRole("heading", { level: 1, name: viewerFixture.display_name }, { timeout: latency * 3 })).toBeInTheDocument();
    const asked = requests.length;

    history?.back();
    expect(await screen.findByRole("heading", { level: 1, name: "Home" }, fromCache)).toBeInTheDocument();
    expect(within(screen.getByRole("feed", { name: "Home" })).getByText("Cached and current")).toBeInTheDocument();
    history?.forward();
    expect(await screen.findByRole("heading", { level: 1, name: viewerFixture.display_name }, fromCache)).toBeInTheDocument();
    expect(within(screen.getByRole("feed", { name: "Posts" })).getByText("Cached and current")).toBeInTheDocument();
    expect(requests.slice(asked)).toEqual([]);
  });

  test("a profile's tab seen before shows from the cache at once, without asking the server again", async () => {
    const requests = slowListsServer();
    const { user } = await renderApp({ path: "/profile" });
    expect(await screen.findByRole("heading", { level: 1, name: viewerFixture.display_name }, { timeout: latency * 3 })).toBeInTheDocument();
    await user.click(screen.getByRole("tab", { name: "Posts and replies" }));
    expect(await screen.findByRole("tab", { name: "Posts and replies", selected: true }, { timeout: latency * 3 })).toBeInTheDocument();
    expect(await within(screen.getByRole("feed", { name: "Posts and replies" })).findByText("Cached and current", {}, { timeout: latency * 3 })).toBeInTheDocument();
    const asked = requests.length;

    await user.click(screen.getByRole("tab", { name: "Posts" }));
    expect(await screen.findByRole("tab", { name: "Posts", selected: true }, fromCache)).toBeInTheDocument();
    expect(within(screen.getByRole("feed", { name: "Posts" })).getByText("Cached and current")).toBeInTheDocument();
    expect(requests.slice(asked)).toEqual([]);
  });
});

describe("a move after the clock went back", () => {
  // The wall clock can step back (an NTP correction, a virtual machine syncing with its host); a page entered again
  // still runs its loader, so it never shows a list that a write marked stale.
  test("reads again the list that a write marked stale", async () => {
    let content = "<p>Before the write</p>";
    server.use(
      resumableSession(),
      // Open: the default stream ends at once, and the reconnect after it reads the home timeline again.
      streamingServer().handler,
      // Slow, so that a page shown from the stale cache would be seen before a refetch replaced it.
      http.get(`${apiBase}/timelines/home`, async () => {
        const answer = statusPage([viewerStatusFixture({ content })]);
        await delay(latency);
        return HttpResponse.json(answer);
      }),
    );
    const { router, queryClient } = await renderApp({ path: "/home" });
    expect(await within(await screen.findByRole("feed", { name: "Home" }, { timeout: latency * 3 })).findByText("Before the write")).toBeInTheDocument();
    await act(() => router.navigate({ href: "/local" }));
    expect(await screen.findByRole("feed", { name: "Local timeline" })).toBeInTheDocument();

    content = "<p>After the write</p>";
    await act(() => queryClient.invalidateQueries({ queryKey: ["get", "/timelines/home"], refetchType: "none" }));
    const now = Date.now.bind(Date);
    vi.spyOn(Date, "now").mockImplementation(() => now() - 60_000);
    await act(() => router.navigate({ href: "/home" }));

    expect(within(await screen.findByRole("feed", { name: "Home" })).getByText("After the write")).toBeInTheDocument();
  });
});
