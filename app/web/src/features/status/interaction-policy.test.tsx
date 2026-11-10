// SPDX-License-Identifier: AGPL-3.0-only
// The interaction policy of a post as its viewer meets it (web-client/README.md T4, ADR-0155 §7, §10): the reply and
// boost a post's policy refuses, a boost and a reply that wait for the author's approval, and the composer telling a
// reply will wait. The real router and QueryClient, MSW for the client API.
import { screen, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { apiBase, oauthPaths, tokenResponse } from "../../test/msw/http";
import { permissionsFixture, type Schemas, statusFixture, viewerStatusFixture } from "../../test/fixtures";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import type { Status } from "./api/status";

const bobsId = "0192f0c4-7a8e-7000-8000-0000000000f1";
const replyId = "0192f0c4-7a8e-7000-8000-0000000000f2";

function signedIn() {
  server.use(http.post(oauthPaths.refresh, () => tokenResponse("test-access-token")));
}

/** Answers GET /statuses/{id} with `statuses` by ID. */
function serve(...statuses: Status[]) {
  server.use(
    http.get(`${apiBase}/statuses/:id`, ({ params }) => {
      const found = statuses.find((s) => s.id === params["id"]);
      return HttpResponse.json(found ?? statusFixture(String(params["id"])));
    }),
  );
}

/** Bob's post, with what the viewer may do with it. */
function bobs(permissions: Partial<Schemas["InteractionPermissions"]>): Status {
  return {
    ...statusFixture(bobsId),
    content: "<p>Bob's thought</p>",
    permissions: { ...permissionsFixture, ...permissions },
  };
}

/** The action bar of the post on its own page. */
async function actionsOf() {
  const article = await screen.findByRole("article", { name: /^Opened post/ });
  await within(article).findByRole("button", { name: "More actions" });
  return article;
}

beforeEach(() => {
  // Only Date is faked: timers, MSW and user-event run as usual.
  vi.useFakeTimers({ toFake: ["Date"], now: new Date("2026-10-16T12:00:00Z") });
  sessionStorage.clear();
});

afterEach(() => {
  vi.useRealTimers();
});

describe("a post whose policy refuses the viewer", () => {
  test("has no reply button, and a boost button that cannot be pressed", async () => {
    signedIn();
    serve(bobs({ reply: "denied", announce: "denied" }));
    await renderApp({ path: `/@bob/status/${bobsId}` });

    const article = await actionsOf();
    expect(within(article).queryByRole("button", { name: "Reply" })).not.toBeInTheDocument();
    expect(within(article).getByRole("button", { name: "Boost" })).toBeDisabled();
    expect(within(article).getByRole("button", { name: "Favourite" })).toBeEnabled();
  });

  test("keeps a boost the viewer has, which can be taken back", async () => {
    signedIn();
    serve({ ...bobs({ announce: "denied" }), boosted: true, boosts_count: 1 });
    await renderApp({ path: `/@bob/status/${bobsId}` });

    const article = await actionsOf();
    expect(within(article).getByRole("button", { name: /^Boost/ })).toBeEnabled();
  });
});

describe("a post whose author approves the interactions", () => {
  test("says that the viewer's boost waits for the approval", async () => {
    signedIn();
    const post = bobs({ announce: "pending" });
    serve(post);
    server.use(
      http.post(`${apiBase}/statuses/:id/boost`, () =>
        HttpResponse.json({ ...post, boosted: false, boost_approval: "pending" } satisfies Status),
      ),
    );
    const { user } = await renderApp({ path: `/@bob/status/${bobsId}` });

    const article = await actionsOf();
    await user.click(within(article).getByRole("button", { name: "Boost" }));

    expect(await within(article).findByText("Your boost waits for the author's approval.")).toBeInTheDocument();
    const boost = within(article).getByRole("button", { name: "Boost" });
    expect(boost).toHaveAttribute("aria-pressed", "false");
    expect(boost).toBeDisabled();
    expect(boost).toHaveAccessibleDescription("Your boost waits for the author's approval.");
  });

  test("tells before the reply is sent that it waits for the approval", async () => {
    signedIn();
    serve(bobs({ reply: "pending" }));
    const { user } = await renderApp({ path: `/@bob/status/${bobsId}` });

    const article = await actionsOf();
    await user.click(within(article).getByRole("button", { name: "Reply" }));
    const sheet = await screen.findByRole("dialog", { name: "New post" });
    expect(
      await within(sheet).findByText("Bob approves replies to this post: yours shows to others once they approve it."),
    ).toBeInTheDocument();
  });

  test("marks the viewer's own reply that waits, which only they see", async () => {
    signedIn();
    serve({
      ...viewerStatusFixture({ id: replyId, content: "<p>May I?</p>" }),
      in_reply_to_id: bobsId,
      approval: "pending",
    });
    await renderApp({ path: `/@alice/status/${replyId}` });

    const article = await screen.findByRole("article", { name: /Alice/ });
    expect(await within(article).findByText("Waiting for approval: only you can see this reply")).toBeInTheDocument();
  });
});
