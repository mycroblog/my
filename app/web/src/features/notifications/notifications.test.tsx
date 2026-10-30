// SPDX-License-Identifier: AGPL-3.0-only
// The notifications page and the unread badge through the whole app (web-client/README.md T4): the real router
// and QueryClient, with MSW as the server (FR-205).
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { afterEach, describe, expect, test } from "vitest";
import { findLoadingRegion } from "../../test/loading";
import { apiBase, problem, resumableSession } from "../../test/msw/http";
import { notificationsFixture } from "./test/handlers";
import {
  accountFixture,
  otherAccountFixture,
  relationshipFixture,
  type Schemas,
  statusFixture,
} from "../../test/fixtures";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import { expectSignInStarted } from "../../test/signIn";
import { expectSelectedTabControlsPanel } from "../../test/tabs";
import type { Notification, NotificationMarker, NotificationType } from "./api/notifications";

/** An announcement as the API answers it, published at 2026-10-21T00:00:00Z and unread, with `overrides`. */
function announcementFixture(id: string, overrides: Partial<Schemas["Announcement"]> = {}): Schemas["Announcement"] {
  return {
    id,
    title: "Maintenance on Sunday",
    content: "<p>Maintenance on Sunday.</p>",
    published_at: "2026-10-21T00:00:00Z",
    updated_at: "2026-10-21T00:00:00Z",
    read: false,
    ended: false,
    ...overrides,
  };
}

/** The server's read marker as a variable, with the updates it received. */
function markerServer(initial: NotificationMarker) {
  let marker = initial;
  const updates: string[] = [];
  server.use(
    http.get(`${apiBase}/notifications/marker`, () => HttpResponse.json(marker)),
    http.put(`${apiBase}/notifications/marker`, async ({ request }) => {
      const { last_read_id: id } = (await request.json()) as { last_read_id: string };
      updates.push(id);
      marker = { last_read_id: id, unread_count: 0 };
      return HttpResponse.json(marker);
    }),
  );
  return { updates };
}

/** The link of the notifications page in the main navigation. */
function notificationsLink() {
  return within(screen.getByRole("navigation", { name: "Main" })).getByRole("link", { name: /^Notifications/ });
}

describe("the unread badge", () => {
  test("reads the unread count as part of the navigation link", async () => {
    server.use(resumableSession());
    markerServer({ last_read_id: "0192f0c4-7a8e-7000-8000-000000000001", unread_count: 3 });

    await renderApp({ path: "/" });

    expect(await screen.findByRole("link", { name: "Notifications, 3 unread" })).toBeInTheDocument();
  });

  test("is absent when everything is read", async () => {
    server.use(resumableSession());
    markerServer({ last_read_id: "0192f0c4-7a8e-7000-8000-000000000001", unread_count: 0 });

    await renderApp({ path: "/home" });

    expect(await screen.findByRole("heading", { level: 1, name: "Home" })).toBeInTheDocument();
    expect(notificationsLink()).toHaveAccessibleName("Notifications");
  });

  test("is absent while signed out, as is the notifications item", async () => {
    markerServer({ last_read_id: "0192f0c4-7a8e-7000-8000-000000000001", unread_count: 3 });

    await renderApp({ path: "/local" });

    expect(await screen.findByRole("heading", { level: 1, name: "Local timeline" })).toBeInTheDocument();
    const nav = screen.getByRole("navigation", { name: "Main" });
    expect(await within(nav).findByRole("link", { name: "Sign in" })).toBeInTheDocument();
    expect(within(nav).queryByRole("link", { name: /^Notifications/ })).not.toBeInTheDocument();
  });
});

describe("the notifications page", () => {
  // The kind shows by shape before the sentence is read (C6), and a mention is answered from its row.
  test("marks each row's avatar with its kind, and replies to a mention from its row", async () => {
    server.use(resumableSession());
    const { user } = await renderApp({ path: "/notifications" });

    const feed = await screen.findByRole("feed", { name: "Notifications" });
    const [mention, follow] = within(feed).getAllByRole("article");
    expect(mention?.innerHTML).toMatch(/data-notification-kind="mention"/);
    expect(follow?.innerHTML).toMatch(/data-notification-kind="follow"/);
    // Only the mention has the reply; the follow has its follow back instead.
    expect(within(follow ?? feed).queryByRole("button", { name: "Reply" })).not.toBeInTheDocument();

    await user.click(await within(mention ?? feed).findByRole("button", { name: "Reply" }));
    const sheet = await screen.findByRole("dialog", { name: "New post" });
    expect(within(sheet).getByText(/^Replying to /)).toBeInTheDocument();
  });

  test("shows each type, marks the newest read, and clears the badge", async () => {
    server.use(resumableSession());
    const marker = markerServer({ last_read_id: "0192f0c4-7a8e-7000-8000-0000000000a1", unread_count: 1 });
    // The posts come embedded in the notifications; none is fetched on its own.
    const statusRequests: string[] = [];
    server.use(
      http.get(`${apiBase}/statuses/:id`, ({ request }) => {
        statusRequests.push(request.url);
        return new HttpResponse(null, { status: 404 });
      }),
    );

    await renderApp({ path: "/notifications" });

    const feed = await screen.findByRole("feed", { name: "Notifications" });
    const articles = within(feed).getAllByRole("article");
    expect(articles).toHaveLength(2);
    // The mention came after the marker: it stays marked new while the page is open.
    expect(articles[0]).toHaveAccessibleName(/^Unread: Bob mentioned you/);
    expect(articles[1]).toHaveAccessibleName(/^Bob followed you/);
    // The post the mention is about is quoted as its text, in one link to its page.
    expect(within(articles[0] ?? feed).getByRole("link", { name: "@alice hello" })).toHaveAttribute(
      "href",
      "/@bob/status/0192f0c4-7a8e-7000-8000-0000000000b1",
    );
    // The time is relative as in the timelines, with the full date and time as its tooltip.
    const time = within(articles[0] ?? feed).getByRole("time");
    expect(time).toHaveAttribute("datetime", "2026-10-16T09:00:00Z");
    expect(time.getAttribute("title")).toMatch(/2026/);
    expect(time.textContent).not.toBe(time.getAttribute("title"));

    // Showing the list moved the marker to the newest notification, once, and the badge went away.
    await waitFor(() => {
      expect(marker.updates).toEqual([notificationsFixture[0]?.id]);
    });
    await waitFor(() => {
      expect(notificationsLink()).toHaveAccessibleName("Notifications");
    });
    expect(articles[0]).toHaveAccessibleName(/^Unread: /);
    expect(statusRequests).toEqual([]);
  });

  test("does not update the marker when nothing new is shown", async () => {
    server.use(resumableSession());
    const marker = markerServer({ last_read_id: "0192f0c4-7a8e-7000-8000-0000000000a2", unread_count: 0 });

    await renderApp({ path: "/notifications" });

    const feed = await screen.findByRole("feed", { name: "Notifications" });
    expect(within(feed).getAllByRole("article")[0]).toHaveAccessibleName(/^Bob mentioned you/);
    expect(marker.updates).toEqual([]);
  });

  test("opens the post a notification is about on its page in the app", async () => {
    server.use(resumableSession());
    markerServer({ last_read_id: "0192f0c4-7a8e-7000-8000-0000000000a2", unread_count: 0 });
    const { user, router } = await renderApp({ path: "/notifications" });
    const feed = await screen.findByRole("feed", { name: "Notifications" });

    await user.click(within(feed).getByRole("link", { name: "@alice hello" }));

    expect(await screen.findByRole("heading", { level: 1, name: "Post" })).toHaveFocus();
    expect(router.state.location.pathname).toBe("/@bob/status/0192f0c4-7a8e-7000-8000-0000000000b1");
    expect(await screen.findByRole("region", { name: "Replies" })).toBeInTheDocument();
  });

  test("filters by type from the URL and the tabs, without moving the marker", async () => {
    server.use(resumableSession());
    const marker = markerServer({ last_read_id: "0192f0c4-7a8e-7000-8000-000000000001", unread_count: 2 });
    const requested: string[][] = [];
    server.use(
      http.get(`${apiBase}/notifications`, ({ request }) => {
        const types = new URL(request.url).searchParams.getAll("types");
        requested.push(types);
        return HttpResponse.json({ items: notificationsFixture.filter((n) => types.includes(n.type)) });
      }),
    );

    const { user, router } = await renderApp({ path: "/notifications?filter=follows" });

    const tabs = await screen.findByRole("tablist", { name: "Notification types" });
    expect(within(tabs).getByRole("tab", { name: "Follows" })).toHaveAttribute("aria-selected", "true");
    const feed = await screen.findByRole("feed", { name: "Notifications" });
    expect(within(feed).getAllByRole("article")).toHaveLength(1);
    expect(within(feed).getByRole("article")).toHaveAccessibleName(/Bob followed you/);
    expect(requested).toEqual([["follow", "follow_request", "follow_accepted"]]);

    await user.click(within(tabs).getByRole("tab", { name: "Mentions" }));

    await waitFor(() => {
      expect(router.state.location.search).toEqual({ filter: "mentions" });
    });
    expect(await screen.findByRole("article", { name: /Bob mentioned you/ })).toBeInTheDocument();
    expect(within(tabs).getByRole("tab", { name: "Mentions" })).toHaveAttribute("aria-selected", "true");
    expectSelectedTabControlsPanel("Mentions");
    expect(marker.updates).toEqual([]);
  });

  test("ignores an unknown filter", async () => {
    server.use(resumableSession());

    await renderApp({ path: "/notifications?filter=nonsense" });

    const tabs = await screen.findByRole("tablist", { name: "Notification types" });
    expect(within(tabs).getByRole("tab", { name: "All" })).toHaveAttribute("aria-selected", "true");
  });

  test("loads older notifications a page at a time", async () => {
    server.use(resumableSession());
    markerServer({ last_read_id: "0192f0c4-7a8e-7000-8000-0000000000a2", unread_count: 0 });
    const [mention, follow] = notificationsFixture;
    server.use(
      http.get(`${apiBase}/notifications`, ({ request }) => {
        const cursor = new URL(request.url).searchParams.get("cursor");
        return HttpResponse.json(cursor === null ? { items: [mention], next_cursor: "page-2" } : { items: [follow] });
      }),
    );

    const { user } = await renderApp({ path: "/notifications" });

    const feed = await screen.findByRole("feed", { name: "Notifications" });
    expect(within(feed).getAllByRole("article")).toHaveLength(1);
    await user.click(screen.getByRole("button", { name: "Load more notifications" }));
    expect(await within(feed).findByRole("article", { name: /Bob followed you/ })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Load more notifications" })).not.toBeInTheDocument();
  });

  test("shows the heading and a skeleton of notifications while the first page loads", async () => {
    server.use(resumableSession());
    markerServer({ last_read_id: "0192f0c4-7a8e-7000-8000-0000000000a2", unread_count: 0 });
    let release = () => {};
    const released = new Promise<void>((resolve) => {
      release = resolve;
    });
    server.use(
      http.get(`${apiBase}/notifications`, async () => {
        await released;
        return HttpResponse.json({ items: notificationsFixture });
      }),
    );
    const { user } = await renderApp({ path: "/home" });
    await screen.findByRole("heading", { level: 1, name: "Home" });
    await user.click(notificationsLink());

    // After the router's pending delay, the page has its heading and a skeleton named once.
    await findLoadingRegion("Loading notifications");
    expect(screen.getByRole("heading", { level: 1, name: "Notifications" })).toBeInTheDocument();
    expect(screen.queryByRole("progressbar")).not.toBeInTheDocument();
    release();

    expect(await screen.findByRole("feed", { name: "Notifications" })).toBeInTheDocument();
    expect(screen.queryByText("Loading notifications")).not.toBeInTheDocument();
  });

  test("says when there is nothing to show", async () => {
    server.use(
      resumableSession(),
      http.get(`${apiBase}/notifications`, () => HttpResponse.json({ items: [] })),
    );

    await renderApp({ path: "/notifications" });

    expect(await screen.findByRole("heading", { level: 2, name: "No notifications yet" })).toBeInTheDocument();
  });

  test("words the notifications of the appeals, which come from the instance", async () => {
    server.use(resumableSession());
    markerServer({ last_read_id: "0192f0c4-7a8e-7000-8000-0000000000ff", unread_count: 0 });
    const items: Notification[] = (["appeal_decided", "admin_appeal"] as const).map((type, i) => ({
      id: `0192f0c4-7a8e-7000-8000-0000000000d${String(i)}`,
      type,
      created_at: "2026-10-16T09:00:00Z",
    }));
    server.use(http.get(`${apiBase}/notifications`, () => HttpResponse.json({ items })));

    await renderApp({ path: "/notifications" });

    const feed = await screen.findByRole("feed", { name: "Notifications" });
    const decided = within(feed).getByRole("article", { name: /The moderators decided your appeal/ });
    expect(within(feed).getByRole("article", { name: /A new appeal awaits the moderators/ })).toBeInTheDocument();
    // Without the action, the decision still leads to the account status.
    expect(
      within(decided)
        .getAllByRole("link")
        .map((link) => link.getAttribute("href")),
    ).toEqual(["/settings/account-status"]);
  });

  test("tells an administrator of a webhook that the server disabled, and leads to the webhooks", async () => {
    server.use(resumableSession());
    markerServer({ last_read_id: "0192f0c4-7a8e-7000-8000-0000000000ff", unread_count: 0 });
    const items: Notification[] = [
      {
        id: "0192f0c4-7a8e-7000-8000-0000000000d9",
        type: "admin_webhook_disabled",
        created_at: "2026-10-16T09:00:00Z",
      },
    ];
    server.use(http.get(`${apiBase}/notifications`, () => HttpResponse.json({ items })));

    await renderApp({ path: "/notifications" });

    const feed = await screen.findByRole("feed", { name: "Notifications" });
    const disabled = within(feed).getByRole("article", { name: /The server disabled a webhook that kept failing/ });
    expect(within(disabled).getByRole("link", { name: "Open the webhooks" })).toHaveAttribute(
      "href",
      "/admin/webhooks",
    );
  });

  test("tells of a voice chat room that a followed account started, and leads to the room's page", async () => {
    server.use(resumableSession());
    markerServer({ last_read_id: "0192f0c4-7a8e-7000-8000-0000000000ff", unread_count: 0 });
    const roomId = "0192f0c4-7a8e-7000-8000-0000000000f1";
    const items: Notification[] = [
      {
        id: "0192f0c4-7a8e-7000-8000-0000000000da",
        type: "voice_room_started",
        created_at: "2026-10-16T09:00:00Z",
        account: otherAccountFixture,
        voice_room_id: roomId,
      },
    ];
    server.use(http.get(`${apiBase}/notifications`, () => HttpResponse.json({ items })));

    await renderApp({ path: "/notifications" });

    const feed = await screen.findByRole("feed", { name: "Notifications" });
    const started = within(feed).getByRole("article", { name: /started a voice chat room/ });
    expect(within(started).getByRole("link", { name: "Open the room" })).toHaveAttribute("href", `/voice/${roomId}`);
  });

  test("tells of an announcement by its title, which opens it on the list page and marks it read", async () => {
    server.use(resumableSession());
    markerServer({ last_read_id: "0192f0c4-7a8e-7000-8000-0000000000ff", unread_count: 0 });
    const announcement = announcementFixture("0192f0c4-7a8e-7000-8000-0000000000f2", {
      title: "Maintenance on Sunday",
    });
    const items: Notification[] = [
      {
        id: "0192f0c4-7a8e-7000-8000-0000000000da",
        type: "announcement",
        created_at: "2026-10-16T09:00:00Z",
        announcement,
      },
    ];
    const reads: string[] = [];
    server.use(
      http.get(`${apiBase}/notifications`, () => HttpResponse.json({ items })),
      http.get(`${apiBase}/announcements/history`, () =>
        HttpResponse.json({ items: [{ ...announcement, read: reads.includes(announcement.id) }] }),
      ),
      http.post(`${apiBase}/announcements/:id/read`, ({ params }) => {
        reads.push(String(params["id"]));
        return HttpResponse.json({ ...announcement, read: true });
      }),
    );

    const { user, router } = await renderApp({ path: "/notifications" });

    const feed = await screen.findByRole("feed", { name: "Notifications" });
    const row = within(feed).getByRole("article", { name: /Announcement: Maintenance on Sunday/ });
    // No one sent it: the announcement's mark stands in the avatar's place.
    expect(row.innerHTML).toMatch(/data-notification-kind="announcement"/);
    const link = within(row).getByRole("link", { name: "Maintenance on Sunday" });
    expect(link).toHaveAttribute("href", `/announcements?id=${announcement.id}`);

    await user.click(link);

    const heading = await screen.findByRole("heading", { level: 2, name: /^Maintenance on Sunday/ });
    expect(router.state.location.pathname).toBe("/announcements");
    await waitFor(() => {
      expect(reads).toEqual([announcement.id]);
    });
    await waitFor(() => {
      expect(heading).toHaveFocus();
    });
  });

  test("tells of custom emoji requests: a new one to an administrator, and a decision with its reason to the requester", async () => {
    server.use(resumableSession());
    markerServer({ last_read_id: "0192f0c4-7a8e-7000-8000-0000000000ff", unread_count: 0 });
    const request = {
      id: "0192f0c4-7a8e-7000-8000-0000000000e1",
      kind: "create",
      state: "pending",
      shortcode: "blobcat",
      created_at: "2026-10-16T08:00:00Z",
    } as const;
    const items: Notification[] = [
      {
        id: "0192f0c4-7a8e-7000-8000-0000000000da",
        type: "admin_custom_emoji_request",
        created_at: "2026-10-16T09:00:00Z",
        custom_emoji_request: request,
      },
      {
        id: "0192f0c4-7a8e-7000-8000-0000000000db",
        type: "custom_emoji_request_decided",
        created_at: "2026-10-16T10:00:00Z",
        custom_emoji_request: {
          ...request,
          kind: "image",
          state: "rejected",
          reason: "Too close to :blob:.",
          decided_at: "2026-10-16T10:00:00Z",
        },
      },
      {
        id: "0192f0c4-7a8e-7000-8000-0000000000dc",
        type: "custom_emoji_request_decided",
        created_at: "2026-10-16T11:00:00Z",
        custom_emoji_request: { ...request, state: "approved" },
      },
    ];
    server.use(http.get(`${apiBase}/notifications`, () => HttpResponse.json({ items })));

    await renderApp({ path: "/notifications" });

    const feed = await screen.findByRole("feed", { name: "Notifications" });
    const submitted = within(feed).getByRole("article", { name: /A user requested the custom emoji :blobcat:/ });
    expect(within(submitted).getByRole("link", { name: "Review the requests" })).toHaveAttribute(
      "href",
      "/admin/custom-emojis?tab=requests",
    );
    const rejected = within(feed).getByRole("article", {
      name: /Your request for a new image of :blobcat: was rejected/,
    });
    expect(within(rejected).getByText("Too close to :blob:.")).toBeInTheDocument();
    expect(within(rejected).getByRole("link", { name: "Open your custom emoji" })).toHaveAttribute(
      "href",
      "/settings/custom-emojis",
    );
    expect(within(feed).getByRole("article", { name: /Your custom emoji :blobcat: was approved/ })).toBeInTheDocument();
  });

  describe("of the supporter subscription", () => {
    const subscription = {
      id: "0192f0c4-7a8e-7000-8000-0000000000f1",
      plan_id: "0192f0c4-7a8e-7000-8000-0000000000f2",
      plan_name: "Gold",
      interval: "month",
      price: { amount: "500", currency: "JPY" },
      started_at: "2026-10-01T00:00:00Z",
    } as const;
    const items: Notification[] = [
      {
        id: "0192f0c4-7a8e-7000-8000-0000000000f3",
        type: "supporter_payment_failed",
        created_at: "2026-10-16T09:00:00Z",
        supporter_subscription: { ...subscription, state: "past_due", grace_ends_at: "2026-10-23T09:00:00Z" },
      },
      {
        id: "0192f0c4-7a8e-7000-8000-0000000000f4",
        type: "supporter_plan_retired",
        created_at: "2026-10-16T10:00:00Z",
        supporter_subscription: { ...subscription, state: "canceling" },
      },
      {
        id: "0192f0c4-7a8e-7000-8000-0000000000f5",
        type: "supporter_subscription_ended",
        created_at: "2026-10-16T11:00:00Z",
        supporter_subscription: { ...subscription, state: "revoked", end_reason: "reversed" },
      },
    ];

    afterEach(() => {
      delete (globalThis as { isTauri?: unknown }).isTauri;
    });

    test("names the plan, the end of the grace period, and why it ended, and leads to the supporter page", async () => {
      server.use(resumableSession());
      markerServer({ last_read_id: "0192f0c4-7a8e-7000-8000-0000000000ff", unread_count: 0 });
      server.use(http.get(`${apiBase}/notifications`, () => HttpResponse.json({ items })));

      await renderApp({ path: "/notifications" });

      const feed = await screen.findByRole("feed", { name: "Notifications" });
      const failed = within(feed).getByRole("article", { name: /A payment for your supporter plan failed/ });
      expect(within(failed).getByText("Gold")).toHaveAttribute("dir", "auto");
      expect(within(failed).getByText(/You keep the plan's benefits until/)).toBeInTheDocument();
      expect(within(failed).getByRole("link", { name: "Update the payment method" })).toHaveAttribute(
        "href",
        "/settings/supporter",
      );
      const retired = within(feed).getByRole("article", { name: /The administrators retired your supporter plan/ });
      expect(within(retired).getByText(/Your subscription will not renew/)).toBeInTheDocument();
      expect(within(retired).getByRole("link", { name: "Open your supporter plan" })).toHaveAttribute(
        "href",
        "/settings/supporter",
      );
      const ended = within(feed).getByRole("article", { name: /Your supporter plan ended/ });
      expect(within(ended).getByText("A payment was refunded in full or charged back.")).toBeInTheDocument();
    });

    test("lead nowhere to pay in the Tauri app", async () => {
      (globalThis as { isTauri?: unknown }).isTauri = true;
      server.use(resumableSession());
      markerServer({ last_read_id: "0192f0c4-7a8e-7000-8000-0000000000ff", unread_count: 0 });
      server.use(http.get(`${apiBase}/notifications`, () => HttpResponse.json({ items })));

      await renderApp({ path: "/notifications" });

      const feed = await screen.findByRole("feed", { name: "Notifications" });
      expect(
        within(feed).getByRole("article", { name: /A payment for your supporter plan failed/ }),
      ).toBeInTheDocument();
      expect(within(feed).queryByRole("link", { name: "Update the payment method" })).not.toBeInTheDocument();
      expect(within(feed).queryByRole("link", { name: "Open your supporter plan" })).not.toBeInTheDocument();
    });
  });

  test("names the actor of every type and hides a post behind its content warning", async () => {
    server.use(resumableSession());
    markerServer({ last_read_id: "0192f0c4-7a8e-7000-8000-0000000000ff", unread_count: 0 });
    const types: NotificationType[] = [
      "follow_request",
      "follow_accepted",
      "boost",
      "favourite",
      "poll_closed",
      "moderation_notice",
      "report_resolved",
      "admin_report",
      "admin_sign_up",
    ];
    const items: Notification[] = types.map((type, i) => ({
      id: `0192f0c4-7a8e-7000-8000-0000000000c${String(i)}`,
      type,
      created_at: "2026-10-16T09:00:00Z",
      ...(type === "moderation_notice" || type === "report_resolved" || type === "admin_report"
        ? {}
        : { account: { ...otherAccountFixture, display_name: "" } }),
      ...(type === "boost"
        ? {
            status: {
              ...statusFixture("0192f0c4-7a8e-7000-8000-0000000000b2"),
              spoiler_text: "spoilers",
              content: "<p>the ending</p>",
            },
          }
        : {}),
    }));
    server.use(http.get(`${apiBase}/notifications`, () => HttpResponse.json({ items })));

    await renderApp({ path: "/notifications" });

    const feed = await screen.findByRole("feed", { name: "Notifications" });
    // Without a display name, the username stands in.
    expect(
      within(feed)
        .getAllByRole("article")
        .map((article) => article.getAttribute("aria-posinset")),
    ).toEqual(["1", "2", "3", "4", "5", "6", "7", "8", "9"]);
    expect(within(feed).getByRole("article", { name: /bob requested to follow you/ })).toBeInTheDocument();
    expect(within(feed).getByRole("article", { name: /bob accepted your follow request/ })).toBeInTheDocument();
    expect(within(feed).getByRole("article", { name: /bob favourited your post/ })).toBeInTheDocument();
    expect(within(feed).getByRole("article", { name: /A poll you took part in has ended/ })).toBeInTheDocument();
    expect(
      within(feed).getByRole("article", { name: /The moderators of this server sent you a notice/ }),
    ).toBeInTheDocument();
    expect(within(feed).getByRole("article", { name: /A report you filed has been resolved/ })).toBeInTheDocument();
    expect(within(feed).getByRole("article", { name: /A new report awaits the moderators/ })).toBeInTheDocument();
    expect(within(feed).getByRole("article", { name: /bob signed up and awaits approval/ })).toBeInTheDocument();
    // A post behind a content warning shows the warning only; its page asks to open it.
    const boost = within(feed).getByRole("article", { name: /bob boosted your post/ });
    expect(within(boost).getByRole("link", { name: "Content warning: spoilers" })).toHaveAttribute(
      "href",
      "/@bob/status/0192f0c4-7a8e-7000-8000-0000000000b2",
    );
    expect(within(boost).queryByText("the ending", { exact: false })).not.toBeInTheDocument();
    // The types without an actor link to no profile, only to where the user follows them up.
    expect(
      within(within(feed).getByRole("article", { name: /The moderators/ }))
        .getAllByRole("link")
        .map((link) => link.textContent),
    ).toEqual(["View your account status"]);
    expect(
      within(within(feed).getByRole("article", { name: /A poll you took part in/ })).queryAllByRole("link"),
    ).toEqual([]);
  });

  test("says which emoji an account reacted with", async () => {
    server.use(resumableSession());
    markerServer({ last_read_id: "0192f0c4-7a8e-7000-8000-0000000000ff", unread_count: 0 });
    const post = statusFixture("0192f0c4-7a8e-7000-8000-0000000000b3");
    const reaction = (id: string, emoji: NonNullable<Notification["reaction"]>): Notification => ({
      id,
      type: "reaction",
      created_at: "2026-10-16T09:00:00Z",
      account: otherAccountFixture,
      status: post,
      reaction: emoji,
    });
    const items: Notification[] = [
      reaction("0192f0c4-7a8e-7000-8000-0000000000e2", { name: "👍" }),
      reaction("0192f0c4-7a8e-7000-8000-0000000000e1", {
        name: "blobcat@misskey.example",
        shortcode: "blobcat",
        domain: "misskey.example",
        emoji: {
          shortcode: "blobcat",
          url: "https://my.example/media/proxy/blobcat",
          static_url: "https://my.example/media/proxy/blobcat",
        },
      }),
    ];
    server.use(http.get(`${apiBase}/notifications`, () => HttpResponse.json({ items })));

    await renderApp({ path: "/notifications" });

    const feed = await screen.findByRole("feed", { name: "Notifications" });
    // Each reaction is a row of its own, not grouped like the favourites.
    expect(within(feed).getAllByRole("article")).toHaveLength(2);
    expect(within(feed).getByRole("article", { name: /Bob reacted to your post with 👍/ })).toBeInTheDocument();
    expect(within(feed).getByRole("article", { name: /Bob reacted to your post with :blobcat:/ })).toBeInTheDocument();
  });

  test("tells that an account the user followed moved, and what became of the follow", async () => {
    server.use(resumableSession());
    markerServer({ last_read_id: "0192f0c4-7a8e-7000-8000-0000000000ff", unread_count: 0 });
    const at = "2026-10-16T09:00:00Z";
    const carol = {
      ...otherAccountFixture,
      id: "0192f0c4-7a8e-7000-8000-0000000000c9",
      username: "carol",
      acct: "carol@new.example",
      display_name: "Carol",
    };
    const items: Notification[] = [
      {
        id: "0192f0c4-7a8e-7000-8000-0000000000e1",
        type: "account_moved",
        created_at: at,
        account: otherAccountFixture,
        account_move: { target: carol, outcome: "followed" },
      },
      {
        id: "0192f0c4-7a8e-7000-8000-0000000000e2",
        type: "account_moved",
        created_at: at,
        account: otherAccountFixture,
        account_move: { target: carol, outcome: "requested" },
      },
      {
        id: "0192f0c4-7a8e-7000-8000-0000000000e3",
        type: "account_moved",
        created_at: at,
        account: otherAccountFixture,
        account_move: { target: carol, outcome: "blocking" },
      },
      // The new account is gone: the row says only that the account moved.
      {
        id: "0192f0c4-7a8e-7000-8000-0000000000e4",
        type: "account_moved",
        created_at: at,
        account: otherAccountFixture,
        account_move: { outcome: "followed" },
      },
    ];
    server.use(http.get(`${apiBase}/notifications`, () => HttpResponse.json({ items })));

    await renderApp({ path: "/notifications" });

    const feed = await screen.findByRole("feed", { name: "Notifications" });
    const rows = within(feed).getAllByRole("article");
    expect(rows).toHaveLength(4);
    expect(rows[0]).toHaveTextContent("You follow ⁨@carol@new.example⁩ now.");
    expect(within(rows[0] as HTMLElement).getByRole("link", { name: "Open ⁨@carol@new.example⁩" })).toHaveAttribute(
      "href",
      "/@carol@new.example",
    );
    expect(rows[1]).toHaveTextContent("Your request to follow ⁨@carol@new.example⁩ awaits its approval.");
    expect(rows[2]).toHaveTextContent("You did not follow ⁨@carol@new.example⁩, because you block it.");
    expect(rows[3]).toHaveTextContent("Bob moved to another account");
    expect(within(rows[3] as HTMLElement).queryByRole("link", { name: /^Open/ })).not.toBeInTheDocument();
  });

  test("says which suspension ended follows and how many the user lost", async () => {
    server.use(resumableSession());
    markerServer({ last_read_id: "0192f0c4-7a8e-7000-8000-0000000000ff", unread_count: 0 });
    const at = "2026-10-16T09:00:00Z";
    const items: Notification[] = [
      {
        id: "0192f0c4-7a8e-7000-8000-0000000000d1",
        type: "relationships_severed",
        created_at: at,
        relationship_severance: {
          kind: "domain_suspension",
          domain: "bad.example",
          followers_lost: 2,
          following_lost: 1,
        },
      },
      {
        id: "0192f0c4-7a8e-7000-8000-0000000000d2",
        type: "relationships_severed",
        created_at: at,
        account: otherAccountFixture,
        relationship_severance: { kind: "account_suspension", followers_lost: 1, following_lost: 0 },
      },
      // A severance that is no longer there says only that follows ended.
      { id: "0192f0c4-7a8e-7000-8000-0000000000d3", type: "relationships_severed", created_at: at },
    ];
    server.use(http.get(`${apiBase}/notifications`, () => HttpResponse.json({ items })));

    await renderApp({ path: "/notifications" });

    const feed = await screen.findByRole("feed", { name: "Notifications" });
    const domain = within(feed).getByRole("article", {
      name: /The moderators suspended bad\.example\. You lost 2 followers and 1 account you followed there\./,
    });
    // A server has no profile to link to: the row leads to the user's follow lists.
    expect(
      (await within(domain).findAllByRole("link")).map((link) => [link.textContent, link.getAttribute("href")]),
    ).toEqual([
      ["View your followers", "/@alice/followers"],
      ["View the accounts you follow", "/@alice/following"],
    ]);
    const account = within(feed).getByRole("article", {
      name: /The moderators suspended Bob\. You lost 1 follower and 0 accounts you followed\./,
    });
    expect(within(account).getByRole("link", { name: "Bob" })).toHaveAttribute("href", "/@bob");
    expect(
      within(feed).getByRole("article", { name: /A suspension by the moderators ended some of your follows/ }),
    ).toBeInTheDocument();
  });

  test("tells what a moderation notice, a resolved report and a decided appeal are about, and leads to the screens that follow them up", async () => {
    server.use(resumableSession());
    markerServer({ last_read_id: "0192f0c4-7a8e-7000-8000-0000000000ff", unread_count: 0 });
    const at = "2026-10-16T09:00:00Z";
    const reportId = "0192f0c4-7a8e-7000-8000-0000000000e9";
    const items: Notification[] = [
      {
        id: "0192f0c4-7a8e-7000-8000-0000000000e1",
        type: "moderation_notice",
        created_at: at,
        moderation_action: {
          id: "0192f0c4-7a8e-7000-8000-0000000000f1",
          kind: "limit",
          kind_label: "Limit",
          reason_category: "spam",
          reason_category_label: "Spam",
          public_reason: "Repeated promotional posts.",
          status_ids: [],
          state: "active",
          issued_at: at,
          expires_at: "2026-10-23T09:00:00Z",
          appealable: true,
        },
      },
      {
        id: "0192f0c4-7a8e-7000-8000-0000000000e2",
        type: "report_resolved",
        created_at: at,
        report: {
          id: "0192f0c4-7a8e-7000-8000-0000000000f2",
          category: "spam",
          comment: "",
          forwarded: false,
          filed_at: at,
          target_account: otherAccountFixture,
          status_ids: [],
          rule_ids: [],
          state: "resolved",
        },
      },
      { id: "0192f0c4-7a8e-7000-8000-0000000000e3", type: "admin_report", created_at: at, report_id: reportId },
      {
        id: "0192f0c4-7a8e-7000-8000-0000000000e4",
        type: "admin_sign_up",
        created_at: at,
        account: otherAccountFixture,
      },
      {
        id: "0192f0c4-7a8e-7000-8000-0000000000e5",
        type: "appeal_decided",
        created_at: at,
        moderation_action: {
          id: "0192f0c4-7a8e-7000-8000-0000000000f3",
          kind: "limit",
          kind_label: "Limit",
          reason_category: "spam",
          reason_category_label: "Spam",
          status_ids: [],
          state: "lifted",
          issued_at: at,
          lifted_at: at,
          appealable: false,
          appeal: {
            id: "0192f0c4-7a8e-7000-8000-0000000000f4",
            text: "I am not a bot.",
            state: "overturned",
            submitted_at: at,
            decided_at: at,
            decision_reason: "We were wrong.",
          },
        },
      },
      {
        id: "0192f0c4-7a8e-7000-8000-0000000000e6",
        type: "admin_appeal",
        created_at: at,
        appeal_id: "0192f0c4-7a8e-7000-8000-0000000000f4",
      },
    ];
    server.use(http.get(`${apiBase}/notifications`, () => HttpResponse.json({ items })));

    const { user, router } = await renderApp({ path: "/notifications" });

    const feed = await screen.findByRole("feed", { name: "Notifications" });
    // The notice says what the action does, why, and until when; the article is described by it.
    const notice = within(feed).getByRole("article", { name: /The moderators of this server sent you a notice/ });
    expect(notice).toHaveAccessibleDescription(/Limit: Spam.*Repeated promotional posts\..*Until Oct 23, 2026/);
    expect(within(notice).getByText("Repeated promotional posts.")).toHaveAttribute("dir", "auto");
    expect(within(notice).getByRole("link", { name: "View your account status" })).toHaveAttribute(
      "href",
      "/settings/account-status",
    );
    // A resolved report names the reported account, a link to its profile, without its avatar.
    const resolved = within(feed).getByRole("article", { name: /Your report on Bob has been resolved/ });
    expect(
      within(resolved)
        .getAllByRole("link")
        .map((link) => link.getAttribute("href")),
    ).toEqual(["/@bob"]);
    // A moderator's notifications lead to the report and to the registrations.
    const report = within(feed).getByRole("article", { name: /A new report awaits the moderators/ });
    expect(within(report).getByRole("link", { name: "Open the report" })).toHaveAttribute(
      "href",
      `/moderation/reports/${reportId}`,
    );
    // The decision on an appeal says what was decided and why, and leads to the account status.
    const decided = within(feed).getByRole("article", { name: /The moderators decided your appeal/ });
    expect(decided).toHaveAccessibleDescription(
      /Limit: Spam.*Your appeal was granted, and the action was lifted\..*We were wrong\./,
    );
    expect(within(decided).getByRole("link", { name: "View your account status" })).toHaveAttribute(
      "href",
      "/settings/account-status",
    );
    const appeal = within(feed).getByRole("article", { name: /A new appeal awaits the moderators/ });
    expect(within(appeal).getByRole("link", { name: "Open the appeal" })).toHaveAttribute(
      "href",
      "/moderation/appeals/0192f0c4-7a8e-7000-8000-0000000000f4",
    );
    const signUp = within(feed).getByRole("article", { name: /Bob signed up and awaits approval/ });
    await user.click(within(signUp).getByRole("link", { name: "Review registrations" }));
    await waitFor(() => {
      expect(router.state.location.pathname).toBe("/moderation/registrations");
    });
  });

  test("links the actor to their profile and offers a follow back, one tab stop each", async () => {
    server.use(resumableSession());
    markerServer({ last_read_id: "0192f0c4-7a8e-7000-8000-0000000000a2", unread_count: 0 });
    server.use(
      http.get(`${apiBase}/accounts/relationships`, ({ request }) =>
        HttpResponse.json(
          new URL(request.url).searchParams.getAll("id").map((id) => relationshipFixture(id, { followed_by: true })),
        ),
      ),
    );
    const { user, router } = await renderApp({ path: "/notifications?filter=follows" });
    const follow = await screen.findByRole("article", { name: /Bob followed you/ });

    // The follow back is the profile's follow button, without the menu of mute and block.
    const followBack = await within(follow).findByRole("button", { name: "Follow back" });
    expect(within(follow).getAllByRole("button")).toEqual([followBack]);
    // The avatar is the same link as the name for the pointer, out of the tab order and hidden from screen readers.
    const name = within(follow).getByRole("link", { name: "Bob" });
    expect(within(follow).getAllByRole("link")).toEqual([name]);

    // From the keyboard alone: the tab stops of the row are the name, then the follow back.
    name.focus();
    await user.tab();
    expect(followBack).toHaveFocus();
    await user.click(followBack);
    expect(await within(follow).findByRole("button", { name: "Following" })).toHaveFocus();

    await user.tab({ shift: true });
    expect(name).toHaveFocus();
    await user.keyboard("{Enter}");
    await waitFor(() => {
      expect(router.state.location.pathname).toBe("/@bob");
    });
  });

  test("offers a retry when the list fails", async () => {
    let fail = true;
    server.use(
      resumableSession(),
      http.get(`${apiBase}/notifications`, () =>
        fail ? problem(503, "about:blank", "Service Unavailable") : HttpResponse.json({ items: notificationsFixture }),
      ),
    );

    const { user } = await renderApp({ path: "/notifications" });

    // The page keeps its heading; the list shows what failed and why, without interrupting as an alert (G16).
    expect(await screen.findByText("Service Unavailable")).toBeInTheDocument();
    expect(screen.getByRole("heading", { level: 2, name: "Could not load the notifications" })).toBeInTheDocument();
    expect(within(screen.getByRole("main")).queryByRole("alert")).not.toBeInTheDocument();
    expect(screen.getByRole("heading", { level: 1, name: "Notifications" })).toBeInTheDocument();
    fail = false;
    await user.click(screen.getByRole("button", { name: "Try again" }));
    expect(await screen.findByRole("feed", { name: "Notifications" })).toBeInTheDocument();
  });

  test("sends to sign in when no session resumes, and back here after it", async () => {
    const { leftTo } = await renderApp({ path: "/notifications" });

    await expectSignInStarted(leftTo, "/notifications");
  });
});

describe("the favourites and the boosts of one post", () => {
  const post = statusFixture("0192f0c4-7a8e-7000-8000-0000000000b1");
  const postPath = "/@bob/status/0192f0c4-7a8e-7000-8000-0000000000b1";
  const carol = accountFixture(3, "Carol");
  const dave = accountFixture(4, "Dave");
  /** A favourite of the post by `account`. */
  const favourite = (id: string, account: NonNullable<Notification["account"]>): Notification => ({
    id,
    type: "favourite",
    created_at: "2026-10-16T05:00:00Z",
    account,
    status: post,
  });
  /** Newest first: Carol's favourite (after the marker), a follow, Bob's favourite and boost, Dave's favourite (read). */
  const items: Notification[] = [
    favourite("0192f0c4-7a8e-7000-8000-0000000000d5", carol),
    { id: "0192f0c4-7a8e-7000-8000-0000000000d4", type: "follow", created_at: "2026-10-16T08:00:00Z", account: dave },
    favourite("0192f0c4-7a8e-7000-8000-0000000000d3", otherAccountFixture),
    {
      id: "0192f0c4-7a8e-7000-8000-0000000000d2",
      type: "boost",
      created_at: "2026-10-16T06:00:00Z",
      account: otherAccountFixture,
      status: post,
    },
    favourite("0192f0c4-7a8e-7000-8000-0000000000d1", dave),
  ];

  test("are one row each that counts the people, and the marker still moves to the newest notification", async () => {
    server.use(
      resumableSession(),
      http.get(`${apiBase}/notifications`, () => HttpResponse.json({ items })),
    );
    const marker = markerServer({ last_read_id: "0192f0c4-7a8e-7000-8000-0000000000d4", unread_count: 1 });

    await renderApp({ path: "/notifications" });

    const feed = await screen.findByRole("feed", { name: "Notifications" });
    const articles = within(feed).getAllByRole("article");
    expect(articles).toHaveLength(3);
    // The row stays where its newest notification is, and is new when that one is.
    expect(articles[0]).toHaveAccessibleName(/^Unread: Carol and 2 others favourited your post/);
    expect(articles[1]).toHaveAccessibleName(/^Dave followed you/);
    expect(articles[2]).toHaveAccessibleName(/^Bob boosted your post/);
    // The others' avatars are links for the pointer only: the row's links are the newest actor's name and the post.
    const favourites = articles[0] ?? feed;
    expect(
      within(favourites)
        .getAllByRole("link")
        .map((link) => link.getAttribute("href")),
    ).toEqual(["/@carol@remote.example", postPath]);
    const hiddenLinks = within(favourites)
      .getAllByRole("link", { hidden: true })
      .map((link) => link.getAttribute("href"));
    expect(hiddenLinks).toEqual(expect.arrayContaining(["/@bob", "/@dave@remote.example"]));

    await waitFor(() => {
      expect(marker.updates).toEqual(["0192f0c4-7a8e-7000-8000-0000000000d5"]);
    });
    expect(articles[0]).toHaveAccessibleName(/^Unread: /);
    expect(articles[2]).not.toHaveAccessibleName(/^Unread: /);
  });

  test("join the row of the newest one when an older page loads", async () => {
    server.use(
      resumableSession(),
      http.get(`${apiBase}/notifications`, ({ request }) => {
        const cursor = new URL(request.url).searchParams.get("cursor");
        return HttpResponse.json(
          cursor === null ? { items: items.slice(0, 2), next_cursor: "page-2" } : { items: items.slice(2) },
        );
      }),
    );
    markerServer({ last_read_id: "0192f0c4-7a8e-7000-8000-0000000000d5", unread_count: 0 });

    const { user } = await renderApp({ path: "/notifications" });

    const feed = await screen.findByRole("feed", { name: "Notifications" });
    expect(within(feed).getAllByRole("article")[0]).toHaveAccessibleName(/^Carol favourited your post/);
    await user.click(screen.getByRole("button", { name: "Load more notifications" }));
    expect(
      await within(feed).findByRole("article", { name: /^Carol and 2 others favourited your post/ }),
    ).toBeInTheDocument();
    expect(within(feed).getAllByRole("article")).toHaveLength(3);
  });

  test("count a person who favourited the post again once", async () => {
    const again = [
      favourite("0192f0c4-7a8e-7000-8000-0000000000d5", carol),
      favourite("0192f0c4-7a8e-7000-8000-0000000000d0", carol),
    ];
    server.use(
      resumableSession(),
      http.get(`${apiBase}/notifications`, () => HttpResponse.json({ items: again })),
    );
    markerServer({ last_read_id: "0192f0c4-7a8e-7000-8000-0000000000d5", unread_count: 0 });

    await renderApp({ path: "/notifications" });

    const feed = await screen.findByRole("feed", { name: "Notifications" });
    expect(within(feed).getByRole("article")).toHaveAccessibleName(/^Carol favourited your post/);
  });
});
