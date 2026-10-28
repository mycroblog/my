// SPDX-License-Identifier: AGPL-3.0-only
// The appeals screens through the whole app (web-client/README.md T4): a moderator with manage_accounts lists the
// appeals awaiting review and those reviewed, opens one next to its action, and upholds or overturns it through
// its confirmation with an explanation to the user, with MSW playing the admin API.
import { screen, waitFor, within } from "@testing-library/react";
import { delay, http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import type { Schemas } from "../../test/fixtures";
import { apiBase, operatorHandler, problem, signedInHandler } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import { appealFixture, decidedAppeal } from "./test/handlers";

beforeEach(() => {
  server.use(signedInHandler, operatorHandler(["manage_accounts"]));
  document.documentElement.lang = "en";
  document.documentElement.dir = "ltr";
});

type Appeal = Schemas["AdminAppeal"];

const pending = appealFixture();
// An appeal in Arabic, which reads right to left whatever the page's direction.
const arabic = appealFixture({
  id: "0192f0c4-7a8e-7000-8000-0000000f0002",
  text: "لم أنشر إعلانات",
  submitted_at: "2026-10-16T08:00:00Z",
});
const upheld = decidedAppeal(
  appealFixture({
    id: "0192f0c4-7a8e-7000-8000-0000000f0003",
    action: { ...pending.action, id: "0192f0c4-7a8e-7000-8000-0000000d0103", kind: "limit" },
  }),
  "uphold",
  "The posts break rule 2.",
);

/** Plays the appeals of the server: lists by state, one appeal, and decisions; records the requests. */
function appealsServer(initial: Appeal[]) {
  let appeals = initial;
  const lists: string[] = [];
  const decisions: { id: string; decision: string; body: Schemas["DecideAppealRequest"] }[] = [];
  const decide = (decision: "uphold" | "overturn") =>
    http.post(`${apiBase}/admin/appeals/:id/${decision}`, async ({ request, params }) => {
      const id = String(params["id"]);
      const body = (await request.json()) as Schemas["DecideAppealRequest"];
      decisions.push({ id, decision, body });
      const appeal = appeals.find((item) => item.id === id);
      if (appeal === undefined) return problem(404, "about:blank", "Not Found");
      const decided = decidedAppeal(appeal, decision, body.public_reason);
      appeals = appeals.map((item) => (item.id === id ? decided : item));
      return HttpResponse.json(decided);
    });
  server.use(
    http.get(`${apiBase}/admin/appeals`, ({ request }) => {
      const state = new URL(request.url).searchParams.get("state");
      lists.push(state ?? "all");
      return HttpResponse.json({
        items: state === null ? appeals : appeals.filter((appeal) => appeal.state === state),
      });
    }),
    http.get(`${apiBase}/admin/appeals/:id`, ({ params }) => {
      const appeal = appeals.find((item) => item.id === String(params["id"]));
      return appeal === undefined ? problem(404, "about:blank", "Not Found") : HttpResponse.json(appeal);
    }),
    decide("uphold"),
    decide("overturn"),
  );
  return { lists, decisions };
}

describe("the list of appeals", () => {
  test("shows the appeals awaiting review, each with its action, account and text in its own direction", async () => {
    const api = appealsServer([pending, arabic, upheld]);
    await renderApp({ path: "/moderation/appeals" });

    expect(await screen.findByRole("heading", { level: 1, name: "Appeals" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Appeals by state/ })).toHaveTextContent("Awaiting review");
    const links = screen.getAllByRole("link", { name: "Appeal of Suspend" });
    expect(links).toHaveLength(2);
    expect(links[0]).toHaveAttribute("href", `/moderation/appeals/${pending.id}`);
    const [first] = within(screen.getByRole("table")).getAllByRole("row").slice(1);
    if (first === undefined) throw new Error("an appeal is missing");
    expect(await within(first).findByText("Bob")).toBeInTheDocument();
    expect(within(first).getByText(/^Oct 16, 2026/)).toBeInTheDocument();
    expect(within(first).getByText("In effect")).toBeInTheDocument();
    expect(within(first).getByText(pending.text)).toHaveAttribute("dir", "auto");
    expect(screen.getByText(arabic.text)).toHaveAttribute("dir", "auto");
    expect(api.lists).toEqual(["pending"]);
  });

  test("shows the reviewed appeals with their decision in the other tab, kept in the URL", async () => {
    appealsServer([pending, upheld]);
    const { user, router } = await renderApp({ path: "/moderation/appeals" });

    await user.click(await screen.findByRole("button", { name: /Appeals by state/ }));
    await user.click(await screen.findByRole("option", { name: "Reviewed" }));

    expect(await screen.findByRole("link", { name: "Appeal of Limit" })).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Appeal of Suspend" })).not.toBeInTheDocument();
    expect(screen.getByText("Upheld")).toBeInTheDocument();
    expect(router.state.location.search).toEqual({ state: "decided" });
  });

  test("the reviewed tab reads on past pages of pending appeals only", async () => {
    const cursors: (string | null)[] = [];
    server.use(
      http.get(`${apiBase}/admin/appeals`, ({ request }) => {
        const url = new URL(request.url);
        expect(url.searchParams.get("state")).toBeNull();
        const cursor = url.searchParams.get("cursor");
        cursors.push(cursor);
        return HttpResponse.json(cursor === null ? { items: [pending], next_cursor: "next" } : { items: [upheld] });
      }),
    );
    await renderApp({ path: "/moderation/appeals?state=decided" });

    expect(await screen.findByRole("link", { name: "Appeal of Limit" })).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Appeal of Suspend" })).not.toBeInTheDocument();
    expect(cursors).toEqual([null, "next"]);
  });

  test("says when no appeal awaits review", async () => {
    appealsServer([]);
    await renderApp({ path: "/moderation/appeals" });

    expect(await screen.findByRole("heading", { level: 2, name: "No appeals await review" })).toBeInTheDocument();
  });

  test("tells a viewer without manage_accounts why, without asking the server", async () => {
    server.use(operatorHandler(["manage_reports"]));
    const api = appealsServer([pending]);
    await renderApp({ path: "/moderation/appeals" });

    expect(await screen.findByRole("heading", { name: "You cannot use this screen" })).toBeInTheDocument();
    expect(screen.getByText(/manage_accounts/)).toBeInTheDocument();
    expect(api.lists).toEqual([]);
  });

  test("shows why when the server refuses the permission after all", async () => {
    server.use(
      http.get(`${apiBase}/admin/appeals`, () => problem(403, "/api/v1/problems/missing-permission", "Forbidden")),
    );
    await renderApp({ path: "/moderation/appeals" });

    expect(await screen.findByRole("heading", { name: "You cannot use this screen" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { level: 1, name: "Appeals" })).toBeInTheDocument();
  });
});

describe("an appeal's page", () => {
  test("shows the appeal next to its action", async () => {
    appealsServer([pending]);
    await renderApp({ path: `/moderation/appeals/${pending.id}` });

    expect(await screen.findByRole("heading", { level: 1, name: "Appeal" })).toBeInTheDocument();
    const appeal = screen.getByRole("region", { name: "Appeal" });
    expect(within(appeal).getByText("Awaiting review")).toBeInTheDocument();
    expect(await within(appeal).findByText("Bob")).toBeInTheDocument();
    expect(within(appeal).getByText(pending.text)).toHaveAttribute("dir", "auto");
    const action = screen.getByRole("region", { name: "Moderation action" });
    expect(within(action).getByText("Suspend")).toBeInTheDocument();
    expect(within(action).getByText("In effect")).toBeInTheDocument();
    expect(within(action).getByText("Repeated advertising.")).toBeInTheDocument();
    expect(within(action).getByText("Third report this week.")).toBeInTheDocument();
    const review = screen.getByRole("region", { name: "Review" });
    expect(within(review).getByRole("button", { name: "Uphold" })).toBeInTheDocument();
    expect(within(review).getByRole("button", { name: "Overturn" })).toBeInTheDocument();
  });

  test("overturns after the confirmation with an explanation: the action shows as lifted, and focus moves to the review", async () => {
    const api = appealsServer([pending]);
    const { user } = await renderApp({ path: `/moderation/appeals/${pending.id}` });

    await user.click(await screen.findByRole("button", { name: "Overturn" }));
    const dialog = await screen.findByRole("alertdialog", { name: "Overturn this appeal?" });
    expect(dialog).toHaveAccessibleDescription(
      "The action “Suspend” is lifted now. Deleted posts stay deleted. The decision cannot be changed afterwards.",
    );
    const reason = within(dialog).getByRole("textbox", { name: "Explanation to the user" });
    await waitFor(() => {
      expect(reason).toHaveFocus();
    });
    expect(api.decisions).toEqual([]);
    await user.type(reason, "  The link was to your own blog.  ");
    await user.click(within(dialog).getByRole("button", { name: "Overturn" }));

    const review = screen.getByRole("region", { name: "Review" });
    expect(await within(review).findByRole("status")).toHaveTextContent(
      "The appeal is overturned and the action is lifted.",
    );
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    const action = screen.getByRole("region", { name: "Moderation action" });
    expect(within(action).getByText("Lifted", { selector: "span" })).toBeInTheDocument();
    expect(within(action).queryByText("In effect")).not.toBeInTheDocument();
    expect(within(screen.getByRole("region", { name: "Appeal" })).getByText("Overturned")).toBeInTheDocument();
    expect(within(review).getByText("The link was to your own blog.")).toBeInTheDocument();
    expect(within(review).queryByRole("button", { name: "Overturn" })).not.toBeInTheDocument();
    await waitFor(() => {
      expect(within(review).getByRole("heading", { level: 2, name: "Review" })).toHaveFocus();
    });
    expect(api.decisions).toEqual([
      { id: pending.id, decision: "overturn", body: { public_reason: "The link was to your own blog." } },
    ]);
  });

  test("upholds without an explanation: the action stays", async () => {
    const api = appealsServer([pending]);
    const { user } = await renderApp({ path: `/moderation/appeals/${pending.id}` });

    await user.click(await screen.findByRole("button", { name: "Uphold" }));
    const dialog = await screen.findByRole("alertdialog", { name: "Uphold this appeal?" });
    await user.click(within(dialog).getByRole("button", { name: "Uphold" }));

    const review = screen.getByRole("region", { name: "Review" });
    expect(await within(review).findByRole("status")).toHaveTextContent("The appeal is upheld. The action stays.");
    expect(
      within(screen.getByRole("region", { name: "Moderation action" })).getByText("In effect"),
    ).toBeInTheDocument();
    expect(within(review).getByText("Upheld")).toBeInTheDocument();
    expect(within(review).getByText("You")).toBeInTheDocument();
    expect(api.decisions).toEqual([{ id: pending.id, decision: "uphold", body: {} }]);
  });

  // #468: the list, shown from its cache as the route loads it, must not show the appeal as it was before the
  // decision while a slow server answers the list again.
  test("back on the list after a decision, the decided appeal is gone at once, however slowly the lists come", async () => {
    const api = appealsServer([pending, arabic]);
    const { user } = await renderApp({ path: "/moderation/appeals" });
    const [link] = await screen.findAllByRole("link", { name: "Appeal of Suspend" });
    if (link === undefined) throw new Error("an appeal is missing");
    await user.click(link);
    await user.click(await screen.findByRole("button", { name: "Overturn" }));
    // From now on every list of appeals takes a while; the handler falls through to appealsServer's.
    server.use(
      http.get(`${apiBase}/admin/appeals`, async () => {
        await delay(300);
      }),
    );
    await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Overturn" }));
    const review = await screen.findByRole("region", { name: "Review" });
    expect(await within(review).findByRole("status")).toHaveTextContent(
      "The appeal is overturned and the action is lifted.",
    );
    await waitFor(() => {
      expect(within(review).getByRole("heading", { level: 2, name: "Review" })).toHaveFocus();
    });

    await user.click(screen.getByRole("link", { name: "Back to appeals" }));
    expect(await screen.findByText(arabic.text)).toBeInTheDocument();
    expect(screen.queryByText(pending.text)).not.toBeInTheDocument();
    expect(screen.getAllByRole("link", { name: "Appeal of Suspend" })).toHaveLength(1);
    expect(api.lists).toEqual(["pending", "pending"]);
  });

  test("back on the list after another moderator's decision, that appeal is gone at once, however slowly the lists come", async () => {
    const api = appealsServer([pending, arabic]);
    const { user } = await renderApp({ path: "/moderation/appeals" });
    const [link] = await screen.findAllByRole("link", { name: "Appeal of Suspend" });
    if (link === undefined) throw new Error("an appeal is missing");
    await user.click(link);
    await user.click(await screen.findByRole("button", { name: "Uphold" }));
    // Another moderator decides it the other way first; every list of appeals takes a while from now on.
    const other = decidedAppeal(pending, "overturn", undefined);
    server.use(
      http.post(`${apiBase}/admin/appeals/:id/uphold`, () =>
        problem(409, "/api/v1/problems/appeal-decided", "The appeal is already decided"),
      ),
      http.get(`${apiBase}/admin/appeals/:id`, () => HttpResponse.json(other)),
      http.get(`${apiBase}/admin/appeals`, async ({ request }) => {
        await delay(300);
        const state = new URL(request.url).searchParams.get("state");
        return HttpResponse.json({
          items: [other, arabic].filter((appeal) => state === null || appeal.state === state),
        });
      }),
    );
    await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Uphold" }));
    const review = await screen.findByRole("region", { name: "Review" });
    expect(await within(review).findByRole("status")).toHaveTextContent(
      "Another moderator already reviewed this appeal. That decision stands.",
    );

    await user.click(screen.getByRole("link", { name: "Back to appeals" }));
    expect(await screen.findByText(arabic.text)).toBeInTheDocument();
    expect(screen.queryByText(pending.text)).not.toBeInTheDocument();
    expect(api.decisions).toEqual([]);
  });

  test("cancel and Esc close the confirmation without deciding", async () => {
    const api = appealsServer([pending]);
    const { user } = await renderApp({ path: `/moderation/appeals/${pending.id}` });

    const overturn = await screen.findByRole("button", { name: "Overturn" });
    await user.click(overturn);
    await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Cancel" }));
    await waitFor(() => {
      expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    });
    await waitFor(() => {
      expect(overturn).toHaveFocus();
    });

    await user.click(screen.getByRole("button", { name: "Uphold" }));
    await screen.findByRole("alertdialog");
    await user.keyboard("{Escape}");
    await waitFor(() => {
      expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    });
    expect(api.decisions).toEqual([]);
  });

  test("a failed decision stays in the confirmation to try again", async () => {
    appealsServer([pending]);
    server.use(
      http.post(`${apiBase}/admin/appeals/:id/overturn`, () =>
        problem(422, "/api/v1/problems/not-appealable", "The account is deleted"),
      ),
    );
    const { user } = await renderApp({ path: `/moderation/appeals/${pending.id}` });

    await user.click(await screen.findByRole("button", { name: "Overturn" }));
    const dialog = await screen.findByRole("alertdialog");
    await user.click(within(dialog).getByRole("button", { name: "Overturn" }));

    expect(await within(dialog).findByRole("alert")).toHaveTextContent(/^Could not save the decision: /);
    // The modal dialog hides the page from assistive technology while it is open.
    expect(
      within(screen.getByRole("region", { name: "Moderation action", hidden: true })).getByText("In effect"),
    ).toBeInTheDocument();
  });

  test("an appeal that another moderator decided the other way first shows that decision", async () => {
    const other = decidedAppeal(pending, "uphold", undefined);
    let reads = 0;
    server.use(
      http.get(`${apiBase}/admin/appeals/:id`, () => {
        reads += 1;
        return HttpResponse.json(reads === 1 ? pending : other);
      }),
      http.post(`${apiBase}/admin/appeals/:id/overturn`, () =>
        problem(409, "/api/v1/problems/appeal-decided", "The appeal is already decided"),
      ),
    );
    const { user } = await renderApp({ path: `/moderation/appeals/${pending.id}` });

    await user.click(await screen.findByRole("button", { name: "Overturn" }));
    await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Overturn" }));

    const review = screen.getByRole("region", { name: "Review" });
    expect(await within(review).findByRole("status")).toHaveTextContent(
      "Another moderator already reviewed this appeal. That decision stands.",
    );
    expect(within(review).getByText("Upheld")).toBeInTheDocument();
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
  });

  test("an unknown appeal is not found", async () => {
    appealsServer([]);
    await renderApp({ path: "/moderation/appeals/0192f0c4-7a8e-7000-8000-0000000f00ff" });

    expect(await screen.findByRole("heading", { level: 1, name: "Appeal" })).toBeInTheDocument();
    expect(await screen.findByRole("heading", { level: 2, name: /not found/i })).toBeInTheDocument();
  });
});
