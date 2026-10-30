// SPDX-License-Identifier: AGPL-3.0-only
// The server's announcements through the whole app (web-client/README.md T4, FR-726, ADR-0125): above the home
// timeline below xl, the unread ones say so in words and in the heading's badge, marking one read sends it and moves
// focus to its heading, and the region goes once none is unread; from xl they are in the third column instead, by their
// titles; the list page has the ended ones, opens one from its notification and marks it read, and loads more; and the
// event stream adds, changes and removes announcements; with MSW playing the API.
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import type { Schemas } from "../../test/fixtures";
import { apiBase, resumableSession, signedInHandler } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { streamingServer } from "../../test/msw/streaming";
import { renderApp } from "../../test/render";
import { announcementFixture } from "./test/handlers";

beforeEach(() => {
  document.documentElement.lang = "en";
  document.documentElement.dir = "ltr";
});

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

const maintenance = announcementFixture("0199f0e0-0000-7000-8000-00000000b001", {
  title: "Maintenance on Sunday",
  content: '<p>Maintenance on <strong>Sunday</strong>. <a href="https://status.example/">Status</a></p>',
  published_at: "2026-10-21T00:00:00Z",
  ends_at: "2026-10-28T00:00:00Z",
});
const welcome = announcementFixture("0199f0e0-0000-7000-8000-00000000b002", {
  title: "Welcome",
  content: "<p>Welcome!</p>",
  published_at: "2026-10-20T00:00:00Z",
  read: true,
});
const rules = announcementFixture("0199f0e0-0000-7000-8000-00000000b003", {
  title: "New rules",
  content: "<p>We updated the rules.</p>",
  published_at: "2026-09-01T00:00:00Z",
  ends_at: "2026-09-08T00:00:00Z",
  ended: true,
});

describe("the announcements above the home timeline", () => {
  test("shows nothing when none is unread", async () => {
    server.use(
      resumableSession(),
      http.get(`${apiBase}/announcements`, () => HttpResponse.json({ items: [welcome] })),
    );
    await renderApp({ path: "/home" });

    expect(await screen.findByRole("heading", { level: 1, name: "Home" })).toBeInTheDocument();
    expect(screen.queryByRole("region", { name: /Announcements/u })).not.toBeInTheDocument();
  });

  test("marks the unread ones in words, marks one read, and goes once none is unread", async () => {
    const reads: string[] = [];
    // The list reads as the server keeps it: the default event stream ends at once, and the app reads the list
    // again on every reconnect, which can come after the click.
    server.use(
      resumableSession(),
      http.get(`${apiBase}/announcements`, () =>
        HttpResponse.json({ items: [{ ...maintenance, read: reads.includes(maintenance.id) }, welcome] }),
      ),
      http.post(`${apiBase}/announcements/:id/read`, ({ params }) => {
        reads.push(String(params["id"]));
        return HttpResponse.json({ ...maintenance, read: true });
      }),
    );
    const { user } = await renderApp({ path: "/home" });

    const region = await screen.findByRole("region", { name: /^Announcements\s*1 unread$/u });
    const [first, second] = within(region).getAllByRole("article");
    expect(within(first ?? region).getByRole("heading", { level: 3 })).toHaveTextContent(
      /^Maintenance on SundayUnread$/u,
    );
    expect(within(first ?? region).getByText("Oct 21, 2026")).toBeInTheDocument();
    expect(within(first ?? region).getByText("Sunday")).toBeInTheDocument();
    expect(within(first ?? region).getByText(/^Shown until/u)).toBeInTheDocument();
    expect(within(second ?? region).getByRole("heading", { level: 3 })).toHaveTextContent(/^Welcome$/u);
    expect(within(second ?? region).queryByRole("button", { name: "Mark as read" })).not.toBeInTheDocument();
    expect(within(region).getByRole("link", { name: "All announcements" })).toHaveAttribute("href", "/announcements");

    await user.click(within(first ?? region).getByRole("button", { name: "Mark as read" }));

    // The last unread one read: the region goes, and focus moves to the page's heading.
    await waitFor(() => {
      expect(screen.queryByRole("region", { name: /Announcements/u })).not.toBeInTheDocument();
    });
    expect(screen.getByRole("heading", { level: 1, name: "Home" })).toHaveFocus();
    expect(reads).toEqual([maintenance.id]);
  });

  test("moves focus to the heading of the one marked read while others stay unread", async () => {
    const second = announcementFixture("0199f0e0-0000-7000-8000-00000000b004", {
      title: "Voice chat is here",
      published_at: "2026-10-19T00:00:00Z",
    });
    let items = [maintenance, second];
    server.use(
      resumableSession(),
      http.get(`${apiBase}/announcements`, () => HttpResponse.json({ items })),
      http.post(`${apiBase}/announcements/:id/read`, () => {
        items = [{ ...maintenance, read: true }, second];
        return HttpResponse.json({ ...maintenance, read: true });
      }),
    );
    const { user } = await renderApp({ path: "/home" });
    const region = await screen.findByRole("region", { name: /^Announcements\s*2 unread$/u });
    const [first] = within(region).getAllByRole("article");

    await user.click(within(first ?? region).getByRole("button", { name: "Mark as read" }));

    await waitFor(() => {
      expect(within(first ?? region).getByRole("heading", { level: 3 })).toHaveTextContent(/^Maintenance on Sunday$/u);
    });
    await waitFor(() => {
      expect(within(first ?? region).getByRole("heading", { level: 3 })).toHaveFocus();
    });
    expect(screen.getByRole("region", { name: /^Announcements\s*1 unread$/u })).toBeInTheDocument();
  });

  test("follows the announcements that the event stream sends", async () => {
    const streams = streamingServer();
    server.use(
      resumableSession(),
      streams.handler,
      http.get(`${apiBase}/announcements`, () => HttpResponse.json({ items: [welcome] })),
    );
    await renderApp({ path: "/home" });
    await screen.findByRole("heading", { level: 1, name: "Home" });
    await waitFor(() => {
      expect(streams.connections()).toBe(1);
    });

    streams.current().send("announcement", maintenance);

    const region = await screen.findByRole("region", { name: /^Announcements\s*1 unread$/u });
    expect(within(region).getByText("Sunday")).toBeInTheDocument();
    expect(within(region).getAllByRole("article")).toHaveLength(2);

    streams.current().send("announcement_removed", { id: maintenance.id });

    await waitFor(() => {
      expect(screen.queryByRole("region", { name: /Announcements/u })).not.toBeInTheDocument();
    });
  });
});

describe("the announcements in the third column", () => {
  beforeEach(() => {
    stubWideScreen();
  });

  test("lists the titles, the unread first, and leaves them out of the home page", async () => {
    const second = announcementFixture("0199f0e0-0000-7000-8000-00000000b004", {
      title: "Voice chat is here",
      published_at: "2026-10-19T00:00:00Z",
    });
    const third = announcementFixture("0199f0e0-0000-7000-8000-00000000b005", {
      title: "Spring cleaning",
      published_at: "2026-10-18T00:00:00Z",
    });
    server.use(
      resumableSession(),
      http.get(`${apiBase}/announcements`, () => HttpResponse.json({ items: [welcome, maintenance, second, third] })),
    );
    await renderApp({ path: "/home" });
    await screen.findByRole("heading", { level: 1, name: "Home" });

    const panel = await screen.findByRole("complementary", { name: /^Announcements\s*3 unread$/u });
    // The unread first, the latest first, and three of them.
    expect(
      within(panel)
        .getAllByRole("listitem")
        .map((item) => within(item).getByRole("link").textContent),
    ).toEqual(["Maintenance on SundayUnread", "Voice chat is hereUnread", "Spring cleaningUnread"]);
    expect(within(panel).getByRole("link", { name: /^Maintenance on Sunday/u })).toHaveAttribute(
      "href",
      `/announcements?id=${maintenance.id}`,
    );
    expect(within(panel).getByRole("link", { name: "All announcements" })).toHaveAttribute("href", "/announcements");
    // From xl the home page has no announcements of its own.
    expect(screen.queryByRole("region", { name: /Announcements/u })).not.toBeInTheDocument();
  });

  test("is not on the list page, which has them all", async () => {
    server.use(
      signedInHandler,
      http.get(`${apiBase}/announcements`, () => HttpResponse.json({ items: [maintenance] })),
      http.get(`${apiBase}/announcements/history`, () => HttpResponse.json({ items: [maintenance] })),
    );
    await renderApp({ path: "/announcements" });

    expect(await screen.findByRole("heading", { level: 1, name: "Announcements" })).toBeInTheDocument();
    expect(await screen.findByRole("heading", { level: 2, name: /^Maintenance on Sunday/u })).toBeInTheDocument();
    expect(screen.queryByRole("complementary", { name: /Announcements/u })).not.toBeInTheDocument();
  });
});

describe("the list page", () => {
  beforeEach(() => {
    server.use(signedInHandler);
  });

  /** GET /announcements/history answering the pages of `pages`, each cursor the index of the next one. */
  function historyServer(pages: Schemas["Announcement"][][]) {
    const reads: string[] = [];
    server.use(
      http.get(`${apiBase}/announcements/history`, ({ request }) => {
        const cursor = Number(new URL(request.url).searchParams.get("cursor") ?? "0");
        const items = (pages[cursor] ?? []).map((item) => ({ ...item, read: item.read || reads.includes(item.id) }));
        return HttpResponse.json({ items, ...(cursor + 1 < pages.length && { next_cursor: String(cursor + 1) }) });
      }),
      http.post(`${apiBase}/announcements/:id/read`, ({ params }) => {
        const id = String(params["id"]);
        reads.push(id);
        const found = pages.flat().find((item) => item.id === id) ?? announcementFixture(id);
        return HttpResponse.json({ ...found, read: true });
      }),
    );
    return { reads };
  }

  test("reads the ones that show now and the ended ones, and marks an unread one read", async () => {
    const { reads } = historyServer([[maintenance, welcome, rules]]);
    const { user } = await renderApp({ path: "/announcements" });

    expect(await screen.findByRole("heading", { level: 1, name: "Announcements" })).toBeInTheDocument();
    const articles = await screen.findAllByRole("article");
    expect(articles.map((article) => within(article).getByRole("heading", { level: 2 }).textContent)).toEqual([
      "Maintenance on SundayUnread",
      "Welcome",
      "New rulesEnded",
    ]);
    const [first, , ended] = articles;
    expect(within(ended ?? document.body).getByText(/^Ended Sep 8, 2026/u)).toBeInTheDocument();
    expect(within(ended ?? document.body).queryByRole("button", { name: "Mark as read" })).not.toBeInTheDocument();

    await user.click(within(first ?? document.body).getByRole("button", { name: "Mark as read" }));

    await waitFor(() => {
      expect(within(first ?? document.body).getByRole("heading", { level: 2 })).toHaveTextContent(
        /^Maintenance on Sunday$/u,
      );
    });
    await waitFor(() => {
      expect(within(first ?? document.body).getByRole("heading", { level: 2 })).toHaveFocus();
    });
    expect(reads).toEqual([maintenance.id]);
  });

  test("opened for an announcement on a later page, loads until it has it, focuses it, and marks it read", async () => {
    const older = announcementFixture("0199f0e0-0000-7000-8000-00000000b009", {
      title: "Older news",
      published_at: "2026-08-01T00:00:00Z",
    });
    const { reads } = historyServer([[welcome], [older]]);
    await renderApp({ path: `/announcements?id=${older.id}` });

    const heading = await screen.findByRole("heading", { level: 2, name: /^Older news/u });
    await waitFor(() => {
      expect(heading).toHaveFocus();
    });
    await waitFor(() => {
      expect(reads).toEqual([older.id]);
    });
    await waitFor(() => {
      expect(heading).toHaveTextContent(/^Older news$/u);
    });
  });

  test("opened for a withdrawn announcement, says so above the list", async () => {
    historyServer([[welcome]]);
    await renderApp({ path: "/announcements?id=0199f0e0-0000-7000-8000-00000000b0ff" });

    expect(await screen.findByText("That announcement was withdrawn, so it no longer shows.")).toBeInTheDocument();
    expect(screen.getByRole("heading", { level: 2, name: "Welcome" })).toBeInTheDocument();
  });

  test("shows more and moves focus to the first one it added", async () => {
    historyServer([[maintenance], [rules]]);
    const { user } = await renderApp({ path: "/announcements" });
    await screen.findByRole("heading", { level: 2, name: /^Maintenance on Sunday/u });

    await user.click(screen.getByRole("button", { name: "Show more" }));

    const added = await screen.findByRole("heading", { level: 2, name: /^New rules/u });
    await waitFor(() => {
      expect(added).toHaveFocus();
    });
    expect(screen.queryByRole("button", { name: "Show more" })).not.toBeInTheDocument();
  });

  test("says when there are none yet", async () => {
    historyServer([[]]);
    await renderApp({ path: "/announcements" });

    expect(await screen.findByRole("heading", { level: 2, name: "No announcements yet" })).toBeInTheDocument();
  });

  test("says why the list could not be loaded, and tries again", async () => {
    let fail = true;
    server.use(
      http.get(`${apiBase}/announcements/history`, () =>
        fail
          ? HttpResponse.json({ type: "about:blank", title: "Unavailable", status: 503 }, { status: 503 })
          : HttpResponse.json({ items: [welcome] }),
      ),
    );
    const { user } = await renderApp({ path: "/announcements" });

    expect(await screen.findByText("The announcements could not be loaded")).toBeInTheDocument();
    fail = false;
    await user.click(screen.getByRole("button", { name: "Try again" }));
    expect(await screen.findByRole("heading", { level: 2, name: "Welcome" })).toBeInTheDocument();
  });
});
