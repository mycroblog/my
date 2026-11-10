// SPDX-License-Identifier: AGPL-3.0-only
// The approval requests of the user's posts through the whole app (web-client/README.md T4, ADR-0155 §10): the list,
// approving and declining, and the thread that an approved reply joins without a reload. The real router and
// QueryClient, MSW for the client API.
import { act, screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { apiBase, oauthPaths, tokenResponse } from "../../test/msw/http";
import { type Schemas, statusFixture, viewerStatusFixture } from "../../test/fixtures";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";

type InteractionRequest = Schemas["InteractionRequest"];

const ownId = "0192f0c4-7a8e-7000-8000-0000000000e1";
const replyId = "0192f0c4-7a8e-7000-8000-0000000000e2";
const own = viewerStatusFixture({ id: ownId, content: "<p>My post</p>", replies_count: 0 });
const reply: Schemas["Status"] = {
  ...statusFixture(replyId),
  content: "<p>May I add something?</p>",
  in_reply_to_id: ownId,
};
const replyRequest: InteractionRequest = {
  id: "0192f0c4-7a8e-7000-8000-0000000000e3",
  kind: "reply",
  state: "pending",
  created_at: "2026-10-16T11:00:00Z",
  account: reply.account,
  status: own,
  interacting_status: reply,
};
const boostRequest: InteractionRequest = {
  id: "0192f0c4-7a8e-7000-8000-0000000000e4",
  kind: "announce",
  state: "pending",
  created_at: "2026-10-16T10:00:00Z",
  account: reply.account,
  status: own,
};

function signedIn() {
  server.use(http.post(oauthPaths.refresh, () => tokenResponse("test-access-token")));
}

/**
 * Serves the requests, which an answer takes off the list, and the thread of the user's post, which an approved reply
 * joins; returns the answers and how many times the thread was read.
 */
function requestsServer(requests: InteractionRequest[]) {
  let waiting = requests;
  const answers: string[] = [];
  const thread = { reads: 0, approved: false };
  server.use(
    http.get(`${apiBase}/interaction_requests`, () =>
      HttpResponse.json({ items: waiting } satisfies Schemas["InteractionRequestPage"]),
    ),
    ...(["accept", "reject"] as const).map((answer) =>
      http.post(`${apiBase}/interaction_requests/:id/${answer}`, ({ params }) => {
        const request = waiting.find((r) => r.id === params["id"]);
        if (request === undefined) return new HttpResponse(null, { status: 404 });
        waiting = waiting.filter((r) => r !== request);
        answers.push(`${answer} ${request.kind}`);
        const accepted = answer === "accept";
        if (accepted && request.kind === "reply") thread.approved = true;
        return HttpResponse.json({
          ...request,
          state: accepted ? "accepted" : "rejected",
          status: { ...own, replies_count: thread.approved ? 1 : 0 },
        } satisfies InteractionRequest);
      }),
    ),
    http.get(`${apiBase}/statuses/:id`, ({ params }) =>
      HttpResponse.json(params["id"] === ownId ? own : statusFixture(String(params["id"]))),
    ),
    http.get(`${apiBase}/statuses/:id/context`, () => {
      thread.reads++;
      return HttpResponse.json({
        ancestors: [],
        descendants: thread.approved ? [reply] : [],
      } satisfies Schemas["StatusContext"]);
    }),
  );
  return { answers, thread };
}

/** The rows of the list of requests. */
function requestRows() {
  return within(screen.getByRole("list", { name: "Approval requests" })).getAllByRole("listitem");
}

beforeEach(() => {
  // Only Date is faked: timers, MSW and user-event run as usual.
  vi.useFakeTimers({ toFake: ["Date"], now: new Date("2026-10-16T12:00:00Z") });
  sessionStorage.clear();
});

afterEach(() => {
  vi.useRealTimers();
});

describe("the approval requests", () => {
  test("list what each asks, with the posts", async () => {
    signedIn();
    requestsServer([replyRequest, boostRequest]);
    await renderApp({ path: "/settings/interaction-requests" });

    expect(await screen.findByRole("heading", { level: 1, name: "Approval requests" })).toBeInTheDocument();
    const rows = requestRows();
    expect(rows).toHaveLength(2);
    const [first, second] = rows as [HTMLElement, HTMLElement];
    expect(first).toHaveTextContent("Wants to reply to your post");
    expect(within(first).getByRole("group", { name: "Their reply" })).toHaveTextContent("May I add something?");
    expect(within(first).getByRole("group", { name: "Your post" })).toHaveTextContent("My post");
    expect(second).toHaveTextContent("Wants to boost your post");
    expect(within(second).queryByRole("group", { name: "Their reply" })).not.toBeInTheDocument();
  });

  test("approving a reply takes it off the list and puts it in the thread without a reload", async () => {
    signedIn();
    const { answers, thread } = requestsServer([replyRequest, boostRequest]);
    const { user, router } = await renderApp({ path: `/@alice/status/${ownId}` });
    expect(await screen.findByRole("article", { name: /^Opened post/ })).toHaveTextContent("My post");
    expect(screen.queryByText("May I add something?")).not.toBeInTheDocument();
    const readsBefore = thread.reads;

    await act(() => router.navigate({ href: "/settings/interaction-requests" }));
    await screen.findByRole("list", { name: "Approval requests" });
    await user.click(within(requestRows()[0] as HTMLElement).getByRole("button", { name: "Approve: Bob" }));

    await waitFor(() => {
      expect(requestRows()).toHaveLength(1);
    });
    expect(answers).toEqual(["accept reply"]);
    // The focus goes to the next request.
    expect(screen.getByRole("button", { name: "Approve: Bob" })).toHaveFocus();

    // Declining the boost leaves no request.
    await user.click(screen.getByRole("button", { name: "Decline: Bob" }));
    expect(await screen.findByRole("heading", { name: "No requests" })).toBeInTheDocument();
    expect(answers).toEqual(["accept reply", "reject announce"]);

    await act(() => router.navigate({ href: `/@alice/status/${ownId}` }));
    expect(await screen.findByText("May I add something?")).toBeInTheDocument();
    expect(thread.reads).toBeGreaterThan(readsBefore);
  });
});
