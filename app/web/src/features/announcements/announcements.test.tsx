// SPDX-License-Identifier: AGPL-3.0-only
// The server's announcements above the home timeline through the whole app (web-client/README.md T4, FR-726): the
// unread ones say so in words and in the heading's badge, marking one read sends it and moves focus to its
// heading, and the event stream adds, changes and removes announcements; with MSW playing the API.
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import { apiBase, resumableSession } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { streamingServer } from "../../test/msw/streaming";
import { renderApp } from "../../test/render";
import { announcementFixture } from "./test/handlers";

beforeEach(() => {
  document.documentElement.lang = "en";
  document.documentElement.dir = "ltr";
});

const maintenance = announcementFixture("0199f0e0-0000-7000-8000-00000000b001", {
  content: '<p>Maintenance on <strong>Sunday</strong>. <a href="https://status.example/">Status</a></p>',
  published_at: "2026-10-21T00:00:00Z",
  ends_at: "2026-10-28T00:00:00Z",
});
const welcome = announcementFixture("0199f0e0-0000-7000-8000-00000000b002", {
  content: "<p>Welcome!</p>",
  published_at: "2026-10-20T00:00:00Z",
  read: true,
});

describe("the announcements above the home timeline", () => {
  test("shows nothing when there are none", async () => {
    server.use(resumableSession());
    await renderApp({ path: "/home" });

    expect(await screen.findByRole("heading", { level: 1, name: "Home" })).toBeInTheDocument();
    expect(screen.queryByRole("region", { name: /Announcements/u })).not.toBeInTheDocument();
  });

  test("marks the unread ones in words and marks one read", async () => {
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
      /^Announcement of Oct 21, 2026Unread$/u,
    );
    expect(within(first ?? region).getByText("Sunday")).toBeInTheDocument();
    expect(within(first ?? region).getByText(/^Shown until/u)).toBeInTheDocument();
    expect(within(second ?? region).getByRole("heading", { level: 3 })).toHaveTextContent(
      /^Announcement of Oct 20, 2026$/u,
    );
    expect(within(second ?? region).queryByRole("button", { name: "Mark as read" })).not.toBeInTheDocument();

    await user.click(within(first ?? region).getByRole("button", { name: "Mark as read" }));

    // The heading loses its "Unread" and takes the focus in separate renders, so wait for each.
    await waitFor(() => {
      expect(within(first ?? region).getByRole("heading", { level: 3 })).toHaveTextContent(
        /^Announcement of Oct 21, 2026$/u,
      );
    });
    await waitFor(() => {
      expect(within(first ?? region).getByRole("heading", { level: 3 })).toHaveFocus();
    });
    expect(reads).toEqual([maintenance.id]);
    expect(screen.getByRole("region", { name: "Announcements" })).toBeInTheDocument();
  });

  test("follows the announcements that the event stream sends", async () => {
    const streams = streamingServer();
    server.use(
      resumableSession(),
      streams.handler,
      http.get(`${apiBase}/announcements`, () => HttpResponse.json({ items: [welcome] })),
    );
    await renderApp({ path: "/home" });
    const region = await screen.findByRole("region", { name: "Announcements" });
    await waitFor(() => {
      expect(streams.connections()).toBe(1);
    });

    streams.current().send("announcement", maintenance);

    expect(await within(region).findByText("Sunday")).toBeInTheDocument();
    expect(within(region).getAllByRole("article")).toHaveLength(2);
    expect(screen.getByRole("region", { name: /^Announcements\s*1 unread$/u })).toBeInTheDocument();

    streams.current().send("announcement_removed", { id: maintenance.id });

    await waitFor(() => {
      expect(within(region).getAllByRole("article")).toHaveLength(1);
    });
    expect(within(region).queryByText("Sunday")).not.toBeInTheDocument();
  });
});
