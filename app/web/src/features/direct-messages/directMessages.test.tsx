// SPDX-License-Identifier: AGPL-3.0-only
// The direct messages through the whole app (web-client/README.md T4; web-client/screens/direct-messages.md): the real
// router and QueryClient, with MSW as the server. The list shows each conversation's people, newest post and unread
// mark from its page alone; opening one marks it read; the navigation counts the unread ones, and the stream's
// conversations change the count and the list without reading the list again.
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { describe, expect, test } from "vitest";
import { accountFixture, otherAccountFixture, remoteAccountFixture, type Schemas } from "../../test/fixtures";
import { apiBase, problem, resumableSession } from "../../test/msw/http";
import { streamingServer } from "../../test/msw/streaming";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import { conversationFixture, directStatusFixture } from "./test/handlers";

const fromBob = conversationFixture(
  "0192f0c4-7a8e-7000-8000-00000000d002",
  directStatusFixture("0192f0c4-7a8e-7000-8000-0000000000e2", "are you coming on Saturday?", "2026-10-16T10:00:00Z"),
  { unread: true },
);

const withGroup = conversationFixture(
  "0192f0c4-7a8e-7000-8000-00000000d001",
  {
    ...directStatusFixture("0192f0c4-7a8e-7000-8000-0000000000e1", "", "2026-10-16T09:00:00Z"),
    spoiler_text: "Plans for the party",
  },
  { accounts: [otherAccountFixture, remoteAccountFixture, accountFixture(4, "Dave"), accountFixture(5, "Erin")] },
);

/** GET /conversations answering `items` on one page, counting the requests. */
function conversations(items: Schemas["Conversation"][]) {
  const reads = { count: 0 };
  server.use(
    http.get(`${apiBase}/conversations`, () => {
      reads.count += 1;
      return HttpResponse.json({ items });
    }),
  );
  return reads;
}

describe("the direct messages page", () => {
  test("shows each conversation's people, newest post and unread mark, and the unread count in the navigation", async () => {
    server.use(resumableSession());
    conversations([fromBob, withGroup]);
    await renderApp({ path: "/direct-messages" });

    const feed = await screen.findByRole("feed", { name: "Direct messages" });
    const [first, second] = within(feed).getAllByRole("article");
    if (first === undefined || second === undefined) throw new Error("two conversations expected");
    expect(first).toHaveAccessibleName(expect.stringMatching(/^Unread: Bob/));
    expect(within(first).getByRole("link", { name: /are you coming on Saturday\?/ })).toHaveAttribute(
      "href",
      expect.stringContaining("/status/0192f0c4-7a8e-7000-8000-0000000000e2"),
    );
    // Four people: three names, then how many more; the newest post has a content warning, which alone shows.
    expect(second).toHaveAccessibleName(expect.stringMatching(/^Bob, Carol, Dave and 1 other/));
    expect(within(second).getByRole("link", { name: "Content warning: Plans for the party" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Direct messages, 1 unread" })).toHaveAttribute("aria-current", "page");
    // On phones the item is in the menu's sheet by default: the Menu button counts it, as no moderation count.
    expect(screen.getByRole("button", { name: "Menu, 1 new" })).toBeInTheDocument();
  });

  test("marks a conversation read as its thread opens", async () => {
    server.use(resumableSession());
    conversations([fromBob]);
    const marked: string[] = [];
    server.use(
      http.post(`${apiBase}/conversations/:id/read`, ({ params }) => {
        marked.push(String(params["id"]));
        return HttpResponse.json({ ...fromBob, unread: false });
      }),
    );
    const { user } = await renderApp({ path: "/direct-messages" });
    const feed = await screen.findByRole("feed", { name: "Direct messages" });

    await user.click(within(feed).getByRole("link", { name: /are you coming on Saturday\?/ }));

    expect(await screen.findByRole("link", { name: "Direct messages" })).toBeInTheDocument();
    expect(marked).toEqual([fromBob.id]);
    expect(screen.queryByRole("link", { name: /^Direct messages, / })).not.toBeInTheDocument();
  });

  test("says when there is no direct message yet", async () => {
    server.use(resumableSession());
    conversations([]);
    await renderApp({ path: "/direct-messages" });

    expect(await screen.findByRole("heading", { level: 2, name: "No direct messages yet" })).toBeInTheDocument();
  });

  test("says that the list could not be loaded, and loads it again", async () => {
    server.use(resumableSession());
    let fail = true;
    server.use(
      http.get(`${apiBase}/conversations`, () =>
        fail ? problem(503, "about:blank", "Service Unavailable") : HttpResponse.json({ items: [fromBob] }),
      ),
    );
    const { user } = await renderApp({ path: "/direct-messages" });

    expect(
      await screen.findByRole("heading", { level: 2, name: "Could not load the direct messages" }),
    ).toBeInTheDocument();
    fail = false;
    await user.click(screen.getByRole("button", { name: "Try again" }));

    expect(await screen.findByRole("feed", { name: "Direct messages" })).toBeInTheDocument();
  });
});

describe("a conversation from the stream", () => {
  test("updates the unread count and the list without reading the list again", async () => {
    const streams = streamingServer();
    server.use(resumableSession(), streams.handler);
    const reads = conversations([fromBob, withGroup]);
    await renderApp({ path: "/direct-messages" });
    const feed = await screen.findByRole("feed", { name: "Direct messages" });
    expect(screen.getByRole("link", { name: "Direct messages, 1 unread" })).toBeInTheDocument();
    await waitFor(() => {
      expect(streams.connections()).toBe(1);
    });
    const readsBefore = reads.count;

    // A new post in the group's conversation: it turns unread and goes to the top.
    streams.current().send("conversation", {
      ...withGroup,
      last_status: directStatusFixture(
        "0192f0c4-7a8e-7000-8000-0000000000e3",
        "the cake is ready",
        "2026-10-16T11:00:00Z",
      ),
      unread: true,
    } satisfies Schemas["Conversation"]);

    expect(await screen.findByRole("link", { name: "Direct messages, 2 unread" })).toBeInTheDocument();
    expect(within(feed).getAllByRole("article")[0]).toHaveAccessibleName(expect.stringMatching(/^Unread: Bob, Carol/));

    // Bob's conversation, read on another device: it stays in its place.
    streams.current().send("conversation", { ...fromBob, unread: false } satisfies Schemas["Conversation"]);

    expect(await screen.findByRole("link", { name: "Direct messages, 1 unread" })).toBeInTheDocument();
    expect(within(feed).getAllByRole("article")[1]).toHaveAccessibleName(expect.stringMatching(/^Bob/));
    expect(reads.count).toBe(readsBefore);
  });
});
