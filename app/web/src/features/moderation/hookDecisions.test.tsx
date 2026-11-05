// SPDX-License-Identifier: AGPL-3.0-only
// The automated reviews screen through the whole app (web-client/README.md T4): a moderator with manage_reports reads what
// the external reviews said and cancels a decision, with MSW playing the admin API.
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import type { Schemas } from "../../test/fixtures";
import { apiBase, operatorHandler, problem, signedInHandler } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import { hookDecisionFixture, hookSummaryFixture } from "./test/handlers";

beforeEach(() => {
  server.use(signedInHandler, operatorHandler(["manage_reports"]));
  document.documentElement.lang = "en";
  document.documentElement.dir = "ltr";
});

type Decision = Schemas["AdminHookDecision"];

const dropped = hookDecisionFixture({
  id: "0192f0c4-7a8e-7000-8000-0000000002a2",
  subject_key: "https://remote.example/activities/1",
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

    const list = screen.getByRole("table", { name: "Decisions" });
    expect(within(list).getByRole("rowheader", { name: dropped.subject_key })).toBeVisible();
    expect(within(list).getByText("Looks like a campaign")).toHaveAttribute("dir", "auto");
    expect(within(list).getByText("Timed out")).toBeVisible();
  });

  test("cancels a decision and says so", async () => {
    decisionsServer([dropped, observed]);
    const { user } = await renderApp({ path: "/moderation/hook-decisions" });
    const button = await screen.findByRole("button", {
      name: /Cancel the decision of Posts from other servers, Dropped/u,
    });
    // The one that had no effect has nothing to cancel.
    expect(screen.getAllByRole("button", { name: /Cancel the decision/u })).toHaveLength(1);
    await user.click(button);
    expect(await screen.findByText("Cancelled the decision of Posts from other servers: Dropped.")).toBeVisible();
    await waitFor(() => {
      expect(screen.queryByRole("button", { name: /Cancel the decision/u })).not.toBeInTheDocument();
    });
  });

  test("a failed cancellation leaves the row and says why", async () => {
    decisionsServer([dropped]);
    server.use(
      http.post(`${apiBase}/admin/hook_decisions/:id/override`, () =>
        problem(409, "/api/v1/problems/hook-decision-not-overridable", "This decision cannot be cancelled."),
      ),
    );
    const { user } = await renderApp({ path: "/moderation/hook-decisions" });
    await user.click(await screen.findByRole("button", { name: /Cancel the decision/u }));
    expect(await screen.findByText(/Could not cancel the decision/u)).toBeVisible();
    expect(screen.getByRole("button", { name: /Cancel the decision/u })).toBeVisible();
  });

  test("the filter asks for one review and stays in the URL", async () => {
    const { lists } = decisionsServer([dropped]);
    await renderApp({ path: "/moderation/hook-decisions?hook=signup.check" });
    expect(await screen.findByText("Nothing was stopped")).toBeVisible();
    expect(lists.at(-1)).toContain("hook=signup.check");
  });
});
