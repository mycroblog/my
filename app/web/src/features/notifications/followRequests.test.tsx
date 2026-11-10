// SPDX-License-Identifier: AGPL-3.0-only
// Answering a follow request from its notification (FR-302) through the whole app (web-client/README.md T4): the real
// router and QueryClient, with MSW as the server.
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { describe, expect, test } from "vitest";
import {
  otherAccountFixture,
  relationshipFixture,
  remoteAccountFixture,
  type Schemas,
  viewerFixture,
} from "../../test/fixtures";
import { apiBase, problem, resumableSession } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";

const bob = { ...otherAccountFixture, locked: false };

/** Bob's follow request to the viewer, the only notification. */
const requestNotification: Schemas["Notification"] = {
  id: "0192f0c4-7a8e-7000-8000-0000000000c1",
  type: "follow_request",
  created_at: "2026-10-16T09:00:00Z",
  account: bob,
};

/**
 * A server where Bob's follow request waits while `waiting` says so: the notification, the relationship, the
 * viewer's count of the requests and the list of them all follow it. Returns the answers it received.
 */
function requestServer({ waiting = true }: { waiting?: boolean } = {}) {
  const state = { waiting };
  const answers: string[] = [];
  server.use(
    resumableSession(),
    http.get(`${apiBase}/notifications`, () => HttpResponse.json({ items: [requestNotification] })),
    http.get(`${apiBase}/notifications/marker`, () =>
      HttpResponse.json({ last_read_id: requestNotification.id, unread_count: 0 }),
    ),
    http.get(`${apiBase}/accounts/relationships`, ({ request }) =>
      HttpResponse.json({
        items: new URL(request.url).searchParams
          .getAll("id")
          .map((id) => relationshipFixture(id, { requested_by: id === bob.id && state.waiting })),
      }),
    ),
    http.get(`${apiBase}/accounts/verify_credentials`, () =>
      HttpResponse.json({ ...viewerFixture, locked: true, follow_requests_count: state.waiting ? 1 : 0 }),
    ),
    http.get(`${apiBase}/follow_requests`, () => HttpResponse.json({ items: state.waiting ? [bob] : [] })),
    http.post(`${apiBase}/follow_requests/:id/accept`, ({ params }) => {
      answers.push(`accept ${String(params["id"])}`);
      state.waiting = false;
      return HttpResponse.json(relationshipFixture(String(params["id"]), { followed_by: true }));
    }),
    http.post(`${apiBase}/follow_requests/:id/reject`, ({ params }) => {
      answers.push(`reject ${String(params["id"])}`);
      state.waiting = false;
      return HttpResponse.json(relationshipFixture(String(params["id"])));
    }),
  );
  return { answers, state };
}

/** The row of Bob's follow request. */
function requestRow() {
  return screen.findByRole("article", { name: /Bob requested to follow you/ });
}

describe("a follow request notification", () => {
  test("offers Approve and Decline, each named with the account, one tab stop each", async () => {
    requestServer();
    const { user } = await renderApp({ path: "/notifications" });
    const row = await requestRow();

    const approve = await within(row).findByRole("button", { name: "Approve: Bob" });
    const decline = within(row).getByRole("button", { name: "Decline: Bob" });
    expect(within(row).getAllByRole("button")).toEqual([approve, decline]);

    // From the keyboard alone: the name, then Approve, then Decline.
    within(row).getByRole("link", { name: "Bob" }).focus();
    await user.tab();
    expect(approve).toHaveFocus();
    await user.tab();
    expect(decline).toHaveFocus();
  });

  test("approves the request: the row says so, and the settings' list and count follow without a reload", async () => {
    const { answers } = requestServer();
    const { user, router } = await renderApp({ path: "/notifications" });
    const row = await requestRow();

    await user.click(await within(row).findByRole("button", { name: "Approve: Bob" }));

    expect(await within(row).findByText("Approved")).toBeInTheDocument();
    expect(within(row).queryByRole("button")).not.toBeInTheDocument();
    // The focus that was on the button stays in the row.
    expect(row).toHaveFocus();
    expect(await screen.findByText("⁨Bob⁩ now follows you.")).toBeInTheDocument();
    expect(answers).toEqual([`accept ${bob.id}`]);

    await router.navigate({ to: "/settings/follow-requests" });
    expect(await screen.findByRole("heading", { level: 2, name: "No follow requests" })).toBeInTheDocument();
    const menu = await screen.findByRole("navigation", { name: "Settings pages" });
    expect(within(menu).getByRole("link", { name: "Follow requests" })).not.toHaveTextContent(/\d/);
  });

  test("declines the request, both answers disabled while it is on its way", async () => {
    const { answers } = requestServer();
    let release = () => {};
    const released = new Promise<void>((resolve) => {
      release = resolve;
    });
    server.use(
      http.post(`${apiBase}/follow_requests/:id/reject`, async ({ params }) => {
        await released;
        answers.push(`reject ${String(params["id"])}`);
        return HttpResponse.json(relationshipFixture(String(params["id"])));
      }),
    );
    const { user } = await renderApp({ path: "/notifications" });
    const row = await requestRow();

    const decline = await within(row).findByRole("button", { name: "Decline: Bob" });
    await user.click(decline);
    expect(within(row).getByRole("button", { name: "Approve: Bob" })).toBeDisabled();
    // The pending button keeps its focus and ignores presses.
    expect(decline).toHaveFocus();
    await user.click(decline);
    release();

    expect(await within(row).findByText("Declined")).toBeInTheDocument();
    expect(within(row).queryByRole("button")).not.toBeInTheDocument();
    expect(await screen.findByText("You declined the follow request of ⁨Bob⁩.")).toBeInTheDocument();
    expect(answers).toEqual([`reject ${bob.id}`]);
  });

  test("has no answers once the request no longer waits", async () => {
    requestServer({ waiting: false });
    // Carol's request still waits; both relationships come in the same answer.
    const carol = { ...remoteAccountFixture, display_name: "Carol" };
    server.use(
      http.get(`${apiBase}/notifications`, () =>
        HttpResponse.json({
          items: [
            { ...requestNotification, id: "0192f0c4-7a8e-7000-8000-0000000000c2", account: carol },
            requestNotification,
          ],
        }),
      ),
      http.get(`${apiBase}/accounts/relationships`, ({ request }) =>
        HttpResponse.json({
          items: new URL(request.url).searchParams
            .getAll("id")
            .map((id) => relationshipFixture(id, { requested_by: id === carol.id })),
        }),
      ),
    );
    await renderApp({ path: "/notifications" });
    const row = await requestRow();
    const carolRow = await screen.findByRole("article", { name: /Carol requested to follow you/ });

    expect(await within(carolRow).findByRole("button", { name: "Approve: Carol" })).toBeInTheDocument();
    expect(within(row).queryByRole("button")).not.toBeInTheDocument();
  });

  test("shows a request answered elsewhere (a 404) as no longer waiting, not as an error", async () => {
    const { state } = requestServer();
    server.use(
      http.post(`${apiBase}/follow_requests/:id/accept`, () => {
        state.waiting = false;
        return problem(404, "/api/v1/problems/not-found", "Not Found");
      }),
    );
    const { user } = await renderApp({ path: "/notifications" });
    const row = await requestRow();

    await user.click(await within(row).findByRole("button", { name: "Approve: Bob" }));

    expect(await within(row).findByText("This request was already answered or withdrawn.")).toBeInTheDocument();
    expect(within(row).queryByRole("button")).not.toBeInTheDocument();
    expect(screen.queryByText(/Not Found/)).not.toBeInTheDocument();
  });

  test("says why an answer failed and keeps the answers", async () => {
    const { answers } = requestServer();
    server.use(
      http.post(`${apiBase}/follow_requests/:id/accept`, () => problem(503, "about:blank", "Service Unavailable")),
    );
    const { user } = await renderApp({ path: "/notifications" });
    const row = await requestRow();

    await user.click(await within(row).findByRole("button", { name: "Approve: Bob" }));

    expect(await screen.findAllByText("Could not answer the follow request: Service Unavailable")).not.toHaveLength(0);
    const approve = within(row).getByRole("button", { name: "Approve: Bob" });
    await waitFor(() => {
      expect(approve).not.toHaveAttribute("aria-disabled");
    });
    expect(within(row).getByRole("button", { name: "Decline: Bob" })).toBeEnabled();
    await user.click(within(row).getByRole("button", { name: "Decline: Bob" }));
    expect(await within(row).findByText("Declined")).toBeInTheDocument();
    expect(answers).toEqual([`reject ${bob.id}`]);
  });
});
