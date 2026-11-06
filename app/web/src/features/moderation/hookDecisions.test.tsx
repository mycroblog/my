// SPDX-License-Identifier: AGPL-3.0-only
// The automated reviews screen through the whole app (web-client/README.md T4): a moderator with manage_reports reads what
// the external reviews said on the first page and a decision's page and cancels a decision, and on the appeals page and an
// appeal's page upholds or overturns the authors' appeals of refused posts, with MSW playing the admin API.
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import { remoteAccountFixture, type Schemas } from "../../test/fixtures";
import { apiBase, operatorHandler, problem, signedInHandler } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import {
  decidedHookAppeal,
  hookAppealFixture,
  hookDecisionFixture,
  hookSummaryFixture,
  reportFixture,
} from "./test/handlers";

beforeEach(() => {
  server.use(signedInHandler, operatorHandler(["manage_reports"]));
  document.documentElement.lang = "en";
  document.documentElement.dir = "ltr";
});

type Decision = Schemas["AdminHookDecision"];

const dropped = hookDecisionFixture({
  id: "0192f0c4-7a8e-7000-8000-0000000002a2",
  subject_key: "https://remote.example/activities/1",
  account_id: remoteAccountFixture.id,
  note: "Looks like a campaign",
  reason: "spam",
});
const observed = hookDecisionFixture({
  id: "0192f0c4-7a8e-7000-8000-0000000002a1",
  subject_key: "https://remote.example/activities/2",
  mode: "observe",
  outcome: "timeout",
  overridable: false,
});

/** Plays the decisions of the server; records the requests. */
function decisionsServer(initial: Decision[]) {
  let decisions = initial;
  const lists: string[] = [];
  server.use(
    http.get(`${apiBase}/admin/hook_decisions/summary`, () => HttpResponse.json(hookSummaryFixture())),
    http.get(`${apiBase}/admin/hook_decisions/:id`, ({ params }) => {
      const found = decisions.find((d) => d.id === params["id"]);
      return found === undefined ? problem(404, "about:blank", "Not Found") : HttpResponse.json(found);
    }),
    http.get(`${apiBase}/admin/hook_decisions`, ({ request }) => {
      const query = new URL(request.url).searchParams;
      lists.push(query.toString());
      return HttpResponse.json({ items: decisions.filter((d) => !query.has("hook") || d.hook === query.get("hook")) });
    }),
    http.post(`${apiBase}/admin/hook_decisions/:id/override`, ({ params }) => {
      decisions = decisions.map((d) =>
        d.id === params.id ? { ...d, overridable: false, overridden_at: "2026-11-06T01:00:00Z" } : d,
      );
      return HttpResponse.json(decisions.find((d) => d.id === params.id));
    }),
  );
  return { lists };
}

describe("automated reviews", () => {
  test("shows each review with its counts, and failures apart from verdicts", async () => {
    decisionsServer([dropped, observed]);
    await renderApp({ path: "/moderation/hook-decisions" });
    const reviews = await screen.findByRole("table", { name: "Reviews" });
    const row = within(reviews).getByRole("row", { name: /Posts from other servers/u });
    expect(within(row).getByText("Enforce")).toBeVisible();
    expect(within(row).getByText("Dropped (Enforce)")).toBeVisible();
    expect(within(row).getByText(/3 decisions, 25 ms on average, 40 ms at most/u)).toBeVisible();
    expect(within(reviews).getByRole("row", { name: /Sign-ups/u })).toHaveTextContent("Not set up");

    // A row names its subject, which links to the decision's page, and says where it stands; the rest is on that page.
    const list = screen.getByRole("table", { name: "Decisions" });
    expect(within(list).getByRole("link", { name: dropped.subject_key })).toHaveAttribute(
      "href",
      `/moderation/hook-decisions/${dropped.id}`,
    );
    const observedRow = within(list).getByRole("row", { name: /activities\/2/u });
    expect(within(observedRow).getByText("Timed out (Observe)")).toBeVisible();
    expect(within(list).queryByText("Looks like a campaign")).not.toBeInTheDocument();
    // The screen's pages are links, the one on screen marked as the current page.
    const pages = screen.getByRole("navigation", { name: "Automated review pages" });
    expect(within(pages).getByRole("link", { name: "Decisions" })).toHaveAttribute("aria-current", "page");
    expect(within(pages).getByRole("link", { name: "Appeals of refused posts" })).not.toHaveAttribute("aria-current");
  });

  test("a decision's page shows every fact of it, the account and the note in its own direction", async () => {
    decisionsServer([dropped, observed]);
    await renderApp({ path: `/moderation/hook-decisions/${dropped.id}` });
    expect(await screen.findByRole("heading", { level: 1, name: "Decision of an automated review" })).toBeVisible();
    expect(screen.getByText(dropped.subject_key)).toHaveAttribute("dir", "ltr");
    expect(screen.getByText("Looks like a campaign")).toHaveAttribute("dir", "auto");
    expect(await screen.findByText("Bob")).toBeVisible();
    expect(screen.getByText("Spam")).toBeVisible();
    expect(screen.getByRole("link", { name: "Back to automated reviews" })).toHaveAttribute(
      "href",
      "/moderation/hook-decisions",
    );
  });

  test("a decision's page cancels it, says so and moves focus to the cancellation", async () => {
    decisionsServer([dropped]);
    const { user } = await renderApp({ path: `/moderation/hook-decisions/${dropped.id}` });
    await user.click(await screen.findByRole("button", { name: "Cancel decision" }));
    expect(await screen.findByText("Cancelled the decision of Posts from other servers: Dropped.")).toBeVisible();
    expect(await screen.findByText("A moderator cancelled this decision.")).toBeVisible();
    expect(screen.queryByRole("button", { name: "Cancel decision" })).not.toBeInTheDocument();
    await waitFor(() => {
      expect(screen.getByRole("heading", { name: "Cancellation" })).toHaveFocus();
    });
  });

  test("a decision that had no effect has nothing to cancel, and says so", async () => {
    decisionsServer([observed]);
    await renderApp({ path: `/moderation/hook-decisions/${observed.id}` });
    expect(await screen.findByText("This decision had no effect, so there is nothing to cancel.")).toBeVisible();
    expect(screen.queryByRole("button", { name: "Cancel decision" })).not.toBeInTheDocument();
  });

  test("a refused local post is cancelled through its appeal, and its page says so", async () => {
    const refused = hookAppealFixture().decision;
    decisionsServer([refused]);
    await renderApp({ path: `/moderation/hook-decisions/${refused.id}` });
    expect(await screen.findByText(/A refused local post is not cancelled here/u)).toBeVisible();
  });

  test("cancels a decision and says so", async () => {
    decisionsServer([dropped, observed]);
    const { user } = await renderApp({ path: "/moderation/hook-decisions" });
    const button = await screen.findByRole("button", {
      name: /Cancel decision of Posts from other servers, Dropped/u,
    });
    // The one that had no effect has nothing to cancel.
    expect(screen.getAllByRole("button", { name: /Cancel decision of/u })).toHaveLength(1);
    await user.click(button);
    expect(await screen.findByText("Cancelled the decision of Posts from other servers: Dropped.")).toBeVisible();
    await waitFor(() => {
      expect(screen.queryByRole("button", { name: /Cancel decision of/u })).not.toBeInTheDocument();
    });
  });

  test("a drop's page says until when cancelling brings the post back", async () => {
    const restorable = hookDecisionFixture({
      id: "0192f0c4-7a8e-7000-8000-00000000d101",
      subject_key: "https://remote.example/activities/restorable",
      outcome: "drop",
      mode: "enforce",
      overridable: true,
      restorable_until: "2999-01-01T00:00:00Z",
    });
    decisionsServer([restorable]);
    await renderApp({ path: `/moderation/hook-decisions/${restorable.id}` });
    expect(await screen.findByText("Can be brought back until")).toBeVisible();
    expect(
      screen.queryByText("Its received copy is gone, so cancelling records the cancellation and brings nothing back."),
    ).not.toBeInTheDocument();
  });

  test("a drop's page says when cancelling no longer brings the post back", async () => {
    const gone = hookDecisionFixture({
      id: "0192f0c4-7a8e-7000-8000-00000000d102",
      subject_key: "https://remote.example/activities/gone",
      outcome: "drop",
      mode: "enforce",
      overridable: true,
      restorable_until: "2001-01-01T00:00:00Z",
    });
    decisionsServer([gone]);
    await renderApp({ path: `/moderation/hook-decisions/${gone.id}` });
    expect(
      await screen.findByText(
        "Its received copy is gone, so cancelling records the cancellation and brings nothing back.",
      ),
    ).toBeVisible();
  });

  test("a report that a review filed says so", async () => {
    const id = "0192f0c4-7a8e-7000-8000-00000000d103";
    server.use(
      http.get(`${apiBase}/admin/reports/:id`, () =>
        HttpResponse.json(
          reportFixture({
            id,
            hook_origin: {
              hook: "inbound.status",
              decision_id: "0192f0c4-7a8e-7000-8000-00000000d104",
              reason: "spam",
            },
          }),
        ),
      ),
    );
    await renderApp({ path: `/moderation/reports/${id}` });
    expect(await screen.findByText("Filed by the review of Posts from other servers, not by a person.")).toBeVisible();
    expect(screen.getByText("Reason of the review: Spam")).toBeVisible();
  });

  test("a failed cancellation leaves the row and says why", async () => {
    decisionsServer([dropped]);
    server.use(
      http.post(`${apiBase}/admin/hook_decisions/:id/override`, () =>
        problem(409, "/api/v1/problems/hook-decision-not-overridable", "This decision cannot be cancelled."),
      ),
    );
    const { user } = await renderApp({ path: "/moderation/hook-decisions" });
    await user.click(await screen.findByRole("button", { name: /Cancel decision of/u }));
    expect(await screen.findByText(/Could not cancel the decision/u)).toBeVisible();
    expect(screen.getByRole("button", { name: /Cancel decision of/u })).toBeVisible();
  });

  test("the decision filter asks for one review and stays in the URL", async () => {
    const { lists } = decisionsServer([dropped]);
    await renderApp({ path: "/moderation/hook-decisions?hook=signup.check" });
    expect(await screen.findByText("Nothing was stopped")).toBeVisible();
    expect(lists.at(-1)).toContain("hook=signup.check");
  });
});

type HookAppeal = Schemas["AdminHookAppeal"];

const appeal = hookAppealFixture();
// An appeal in Arabic without a comment, which reads right to left whatever the page's direction.
const arabic: HookAppeal = {
  id: "0192f0c4-7a8e-7000-8000-0000000003a2",
  decision: appeal.decision,
  account_id: appeal.account_id,
  status_text: "حفلة في الحي يوم السبت",
  state: "pending",
  submitted_at: "2026-11-05T09:00:00Z",
};
const upheld = decidedHookAppeal(hookAppealFixture({ id: "0192f0c4-7a8e-7000-8000-0000000003a3" }), "uphold");

/** Plays the appeals of refused posts: the pending ones or every one, and decisions; records the requests. */
function appealsServer(initial: HookAppeal[]) {
  let appeals = initial;
  const lists: string[] = [];
  const decisions: string[] = [];
  decisionsServer([]);
  server.use(
    http.get(`${apiBase}/admin/hook_appeals`, ({ request }) => {
      const pending = new URL(request.url).searchParams.get("state") === "pending";
      lists.push(pending ? "pending" : "all");
      return HttpResponse.json({ items: appeals.filter((item) => !pending || item.state === "pending") });
    }),
    http.get(`${apiBase}/admin/hook_appeals/:id`, ({ params }) => {
      const found = appeals.find((item) => item.id === params["id"]);
      return found === undefined ? problem(404, "about:blank", "Not Found") : HttpResponse.json(found);
    }),
    ...(["uphold", "overturn"] as const).map((decision) =>
      http.post(`${apiBase}/admin/hook_appeals/:id/${decision}`, ({ params }) => {
        const found = appeals.find((item) => item.id === params["id"]);
        if (found === undefined) return problem(404, "about:blank", "Not Found");
        decisions.push(`${decision} ${found.id}`);
        const decided = decidedHookAppeal(found, decision);
        appeals = appeals.map((item) => (item.id === found.id ? decided : item));
        return HttpResponse.json(decided);
      }),
    ),
  );
  return { lists, decisions };
}

/** Confirms the open decision of `title` with its button `action`. */
async function confirmDecision(user: Awaited<ReturnType<typeof renderApp>>["user"], title: string, action: string) {
  await user.click(
    within(await screen.findByRole("alertdialog", { name: title })).getByRole("button", { name: action }),
  );
}

describe("appeals of refused posts", () => {
  test("lists those awaiting review by author, with the start of the text in its own direction", async () => {
    const api = appealsServer([appeal, arabic, upheld]);
    await renderApp({ path: "/moderation/hook-decisions/appeals" });

    const table = await screen.findByRole("table", { name: "Awaiting review" });
    const rows = within(table).getAllByRole("row").slice(1);
    expect(rows).toHaveLength(2);
    const [first] = rows;
    if (first === undefined) throw new Error("an appeal is missing");
    expect(await within(first).findByRole("link", { name: /Bob/u })).toHaveAttribute(
      "href",
      `/moderation/hook-decisions/appeals/${appeal.id}`,
    );
    expect(within(first).getByText(appeal.status_text)).toHaveAttribute("dir", "auto");
    expect(within(table).getByText(arabic.status_text)).toHaveAttribute("dir", "auto");
    // Deciding is on the appeal's page, where the appeal is read whole.
    expect(within(table).queryByRole("button")).not.toBeInTheDocument();
    expect(api.lists).toEqual(["pending"]);
    const pages = screen.getByRole("navigation", { name: "Automated review pages" });
    expect(within(pages).getByRole("link", { name: "Appeals of refused posts" })).toHaveAttribute(
      "aria-current",
      "page",
    );
  });

  test("an appeal's page shows the texts, the comment and the decision", async () => {
    appealsServer([appeal]);
    await renderApp({ path: `/moderation/hook-decisions/appeals/${appeal.id}` });

    expect(await screen.findByRole("heading", { level: 1, name: "Appeal of a refused post" })).toBeVisible();
    expect(screen.getByText(appeal.status_text)).toHaveAttribute("dir", "auto");
    expect(screen.getByText(appeal.comment ?? "")).toHaveAttribute("dir", "auto");
    expect(screen.getByText("Local posts")).toBeVisible();
    expect(screen.getByText("Spam")).toBeVisible();
    expect(screen.getByRole("link", { name: "Open the decision" })).toHaveAttribute(
      "href",
      `/moderation/hook-decisions/${appeal.decision.id}`,
    );
  });

  test("upholds an appeal after confirming, says so and moves focus to the review", async () => {
    const api = appealsServer([appeal]);
    const { user } = await renderApp({ path: `/moderation/hook-decisions/appeals/${appeal.id}` });

    await user.click(await screen.findByRole("button", { name: "Uphold" }));
    expect(await screen.findByRole("alertdialog", { name: "Uphold this appeal?" })).toHaveTextContent(
      "The post stays refused",
    );
    await confirmDecision(user, "Uphold this appeal?", "Uphold");

    expect(await screen.findByText("Upheld the appeal. The post stays refused.")).toBeVisible();
    expect(api.decisions).toEqual([`uphold ${appeal.id}`]);
    expect(await screen.findByText("Result")).toBeVisible();
    expect(screen.queryByRole("button", { name: "Uphold" })).not.toBeInTheDocument();
    await waitFor(() => {
      expect(screen.getByRole("heading", { name: "Moderator's review" })).toHaveFocus();
    });
  });

  test("overturns an appeal after confirming, which cancels the decision", async () => {
    const api = appealsServer([appeal]);
    const { user } = await renderApp({ path: `/moderation/hook-decisions/appeals/${appeal.id}` });

    await user.click(await screen.findByRole("button", { name: "Overturn" }));
    const dialog = await screen.findByRole("alertdialog", { name: "Overturn this appeal?" });
    expect(dialog).toHaveTextContent("The refused post is not restored");
    // Cancelling decides nothing.
    await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
    await waitFor(() => {
      expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    });
    expect(api.decisions).toEqual([]);

    await user.click(screen.getByRole("button", { name: "Overturn" }));
    await confirmDecision(user, "Overturn this appeal?", "Overturn");

    expect(
      await screen.findByText(
        "Overturned the appeal. The decision is cancelled, and this author's posts are no longer reviewed.",
      ),
    ).toBeVisible();
    expect(api.decisions).toEqual([`overturn ${appeal.id}`]);
  });

  test("an appeal another moderator decided first is read again", async () => {
    appealsServer([appeal]);
    server.use(
      http.post(`${apiBase}/admin/hook_appeals/:id/overturn`, () =>
        problem(409, "/api/v1/problems/appeal-decided", "Decided already"),
      ),
    );
    const { user } = await renderApp({ path: `/moderation/hook-decisions/appeals/${appeal.id}` });

    await user.click(await screen.findByRole("button", { name: "Overturn" }));
    await confirmDecision(user, "Overturn this appeal?", "Overturn");

    expect(
      await screen.findByText("Another moderator already decided this appeal. Their decision stands."),
    ).toBeVisible();
    await waitFor(() => {
      expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    });
  });

  test("the appeal filter shows every appeal and stays in the URL", async () => {
    const api = appealsServer([upheld]);
    const { user, router } = await renderApp({ path: "/moderation/hook-decisions/appeals" });

    expect(await screen.findByText("No appeals await review")).toBeVisible();
    await user.click(screen.getByRole("button", { name: /Appeals by state/u }));
    await user.click(await screen.findByRole("option", { name: "All appeals" }));

    expect(await screen.findByText("Upheld")).toBeVisible();
    expect(router.state.location.search).toEqual({ state: "all" });
    expect(api.lists.at(-1)).toBe("all");
  });
});
