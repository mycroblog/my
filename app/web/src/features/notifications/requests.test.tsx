// SPDX-License-Identifier: AGPL-3.0-only
// The notification requests through the whole app (web-client/README.md T4, ADR-0158 §7): the real router and
// QueryClient, MSW for the client API and the event stream. The notifications page shows how many people's
// notifications the policy held, and the requests are accepted or dismissed from their list or their own page, which
// updates the counts and the notifications without a reload.
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import { accountFixture, type Schemas, statusFixture } from "../../test/fixtures";
import { apiBase, problem, resumableSession } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { streamingServer } from "../../test/msw/streaming";
import { renderApp } from "../../test/render";
import { notificationsFixture } from "./test/handlers";

const carol = accountFixture(3, "carol");
const dave = accountFixture(4, "dave");

/** A notification held in the request of `account`. */
function held(n: number, account: Schemas["Account"]): Schemas["Notification"] {
  return {
    id: `0192f0c4-7a8e-7000-8000-0000000001${String(n).padStart(2, "0")}`,
    type: "mention",
    created_at: "2026-10-16T10:00:00Z",
    account,
    status: { ...statusFixture(`0192f0c4-7a8e-7000-8000-0000000002${String(n).padStart(2, "0")}`), account },
  };
}

/**
 * A server with a request of Carol (two mentions) and one of Dave (one mention). An accepted request's notifications
 * join the list of notifications; an answered request leaves the requests. `fail` refuses every answer.
 */
function requestsServer({ fail = false }: { fail?: boolean } = {}) {
  const heldBy = new Map<string, Schemas["Notification"][]>([
    [carol.id, [held(2, carol), held(1, carol)]],
    [dave.id, [held(3, dave)]],
  ]);
  const accounts = new Map([
    [carol.id, carol],
    [dave.id, dave],
  ]);
  let listed = [...notificationsFixture];
  const answers: string[] = [];
  const requestOf = (id: string): Schemas["NotificationRequest"] | undefined => {
    const items = heldBy.get(id);
    const account = accounts.get(id);
    if (items === undefined || account === undefined) return undefined;
    const [last] = items;
    return {
      id,
      account,
      notifications_count: items.length,
      ...(last === undefined ? {} : { last_notification: last }),
    };
  };
  const summary = (): Schemas["NotificationRequestSummary"] => ({
    accounts_count: heldBy.size,
    notifications_count: [...heldBy.values()].reduce((sum, items) => sum + items.length, 0),
  });
  const notFound = () => problem(404, "/api/v1/problems/not-found", "Not Found");
  server.use(
    resumableSession(),
    http.get(`${apiBase}/notifications`, () => HttpResponse.json({ items: listed })),
    http.get(`${apiBase}/notifications/requests/summary`, () => HttpResponse.json(summary())),
    http.get(`${apiBase}/notifications/requests`, () =>
      HttpResponse.json({ items: [...heldBy.keys()].flatMap((id) => requestOf(id) ?? []) }),
    ),
    http.get(`${apiBase}/notifications/requests/:id`, ({ params }) => {
      const request = requestOf(String(params["id"]));
      return request === undefined ? notFound() : HttpResponse.json(request);
    }),
    http.get(`${apiBase}/notifications/requests/:id/notifications`, ({ params }) => {
      const items = heldBy.get(String(params["id"]));
      return items === undefined ? notFound() : HttpResponse.json({ items });
    }),
    ...(["accept", "dismiss"] as const).map((answer) =>
      http.post(`${apiBase}/notifications/requests/:id/${answer}`, ({ params }) => {
        const id = String(params["id"]);
        answers.push(`${answer} ${id}`);
        if (fail) return problem(503, "/api/v1/problems/unavailable", "Service Unavailable");
        const items = heldBy.get(id);
        if (items === undefined) return notFound();
        heldBy.delete(id);
        if (answer === "accept") listed = [...items, ...listed];
        return new HttpResponse(null, { status: 204 });
      }),
    ),
  );
  return { answers };
}

function requestsList() {
  return screen.findByRole("list", { name: "Notification requests" });
}

/** Finds a visible toast by its text (the announcer's live regions say it too). */
async function findToast(text: string) {
  return within(await screen.findByRole("region", { name: "Messages" })).findByText(text);
}

beforeEach(() => {
  sessionStorage.clear();
});

describe("the row of the requests on the notifications page", () => {
  test("says how many people's notifications wait, and leads to the requests", async () => {
    requestsServer();
    const { user } = await renderApp({ path: "/notifications" });

    const link = await screen.findByRole("link", { name: "Notification requests from 2 people" });
    expect(
      screen.getByText(
        "Your notification policy held 3 notifications. They stay out of this list until you accept or dismiss them.",
      ),
    ).toBeInTheDocument();
    await user.click(link);

    expect(await screen.findByRole("heading", { level: 1, name: "Notification requests" })).toBeInTheDocument();
    expect(within(await requestsList()).getAllByRole("listitem")).toHaveLength(2);
  });

  test("is not shown while no request waits", async () => {
    server.use(resumableSession());
    await renderApp({ path: "/notifications" });
    expect(await screen.findByRole("feed")).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: /^Notification requests from/ })).not.toBeInTheDocument();
  });

  test("follows the counts from the stream, without a reload", async () => {
    const streams = streamingServer();
    server.use(resumableSession(), streams.handler);
    await renderApp({ path: "/notifications" });
    expect(await screen.findByRole("feed")).toBeInTheDocument();
    await waitFor(() => {
      expect(streams.connections()).toBe(1);
    });

    streams.current().send("notification_requests", { accounts_count: 1, notifications_count: 1 });
    expect(await screen.findByRole("link", { name: "Notification requests from 1 person" })).toBeInTheDocument();

    streams.current().send("notification_requests", { accounts_count: 0, notifications_count: 0 });
    await waitFor(() => {
      expect(screen.queryByRole("link", { name: /^Notification requests from/ })).not.toBeInTheDocument();
    });
  });
});

describe("the list of requests", () => {
  test("names each sender with how many notifications they hold, and what accepting and dismissing do", async () => {
    requestsServer();
    await renderApp({ path: "/notifications/requests" });

    const [first, second] = within(await requestsList()).getAllByRole("listitem");
    expect(first).toHaveTextContent("2 held notifications");
    expect(within(first ?? document.body).getByRole("link", { name: "View the notifications: carol" })).toHaveAttribute(
      "href",
      `/notifications/requests/${carol.id}`,
    );
    expect(second).toHaveTextContent("1 held notification");
    expect(screen.getByText(/^Notifications that your notification policy held, by who sent them\./)).toBeVisible();
    expect(screen.getByRole("link", { name: "Change the notification policy" })).toHaveAttribute(
      "href",
      "/settings/notifications",
    );
  });

  test("accepting takes the request off the list, moves the focus to the next one, and lists its notifications", async () => {
    const { answers } = requestsServer();
    const { user } = await renderApp({ path: "/notifications/requests" });
    await requestsList();

    await user.click(screen.getByRole("button", { name: "Accept: carol" }));

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Accept: dave" })).toHaveFocus();
    });
    expect(screen.queryByRole("button", { name: "Accept: carol" })).not.toBeInTheDocument();
    expect(answers).toEqual([`accept ${carol.id}`]);

    // The notifications page shows the accepted notifications and the count that is left, without a reload.
    const [navigation] = screen.getAllByRole("link", { name: /^Notifications/ });
    if (navigation === undefined) throw new Error("the navigation has no Notifications item");
    await user.click(navigation);
    expect(await screen.findByRole("link", { name: "Notification requests from 1 person" })).toBeInTheDocument();
    expect(within(screen.getByRole("feed")).getAllByRole("article", { name: /carol/ }).length).toBeGreaterThan(0);
  });

  test("dismissing the last request leaves the empty list, with the focus on the heading", async () => {
    const { answers } = requestsServer();
    const { user } = await renderApp({ path: "/notifications/requests" });
    await requestsList();

    await user.click(screen.getByRole("button", { name: "Dismiss: dave" }));
    await waitFor(() => {
      expect(screen.queryByRole("button", { name: "Dismiss: dave" })).not.toBeInTheDocument();
    });
    await user.click(screen.getByRole("button", { name: "Dismiss: carol" }));

    expect(await screen.findByRole("heading", { name: "No requests" })).toBeInTheDocument();
    await waitFor(() => {
      expect(screen.getByRole("heading", { level: 1, name: "Notification requests" })).toHaveFocus();
    });
    expect(answers).toEqual([`dismiss ${dave.id}`, `dismiss ${carol.id}`]);
  });

  test("keeps the request and its focus when the answer fails", async () => {
    requestsServer({ fail: true });
    const { user } = await renderApp({ path: "/notifications/requests" });
    await requestsList();
    const accept = screen.getByRole("button", { name: "Accept: carol" });

    await user.click(accept);

    await waitFor(() => {
      expect(accept).not.toHaveAttribute("aria-disabled");
    });
    expect(screen.getByRole("button", { name: "Accept: carol" })).toBeInTheDocument();
    expect(accept).toHaveFocus();
  });
});

describe("the page of a request", () => {
  test("shows the held notifications, and accepting goes back to the list with a toast", async () => {
    const { answers } = requestsServer();
    const { user } = await renderApp({ path: `/notifications/requests/${carol.id}` });

    expect(await screen.findByRole("heading", { level: 1, name: "Held notifications" })).toBeInTheDocument();
    expect(
      within(await screen.findByRole("feed", { name: "Held notifications" })).getAllByRole("article"),
    ).toHaveLength(2);
    await user.click(screen.getByRole("button", { name: "Accept" }));

    expect(await findToast("You accepted the notifications from carol")).toBeInTheDocument();
    expect(await screen.findByRole("heading", { level: 1, name: "Notification requests" })).toBeInTheDocument();
    expect(within(await requestsList()).getAllByRole("listitem")).toHaveLength(1);
    expect(answers).toEqual([`accept ${carol.id}`]);
  });

  test("of an account without a request says so, with the way back to the requests", async () => {
    requestsServer();
    await renderApp({ path: `/notifications/requests/${accountFixture(9, "erin").id}` });

    expect(await screen.findByRole("heading", { level: 1, name: "Request not found" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Back to the notification requests" })).toHaveAttribute(
      "href",
      "/notifications/requests",
    );
  });
});
