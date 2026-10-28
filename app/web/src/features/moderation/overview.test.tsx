// SPDX-License-Identifier: AGPL-3.0-only
// The moderation overview (screens/moderation.md): a tile for each queue of the viewer's roles, each a link to it, how
// long the oldest open report has waited, and the open reports to handle first (the oldest legal ones first, then the
// longest waiting of the whole queue, as the server orders them) with their category, their wait and who handles them.
import { screen, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import { otherAccountFixture, remoteAccountFixture, viewerFixture } from "../../test/fixtures";
import { apiBase, operatorHandler, signedInHandler } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import { countsHandler, reportFixture } from "./test/handlers";

beforeEach(() => {
  server.use(signedInHandler);
});

const day = 24 * 60 * 60 * 1000;

describe("the moderation overview", () => {
  test("lists the reports to handle first: legal ones, then the longest waiting, with their wait and assignee", async () => {
    const now = Date.now();
    const ago = (ms: number) => new Date(now - ms).toISOString();
    const legal = reportFixture({ id: "r-legal", category: "legal", filed_at: ago(30 * 60 * 1000) });
    const old = reportFixture({
      id: "r-old",
      filed_at: ago(4 * day),
      target_account: remoteAccountFixture,
      assignee_account_id: viewerFixture.id,
    });
    const recent = reportFixture({ id: "r-new", filed_at: ago(60 * 60 * 1000), target_account: otherAccountFixture });
    const queries: string[] = [];
    server.use(
      operatorHandler(["manage_reports"]),
      countsHandler({ open_reports: 3, oldest_open_report_filed_at: old.filed_at }),
      // The server orders the queue: oldest first, and only the legal ones when asked.
      http.get(`${apiBase}/admin/reports`, ({ request }) => {
        const query = new URL(request.url).searchParams;
        queries.push(query.toString());
        const oldestFirst = [old, legal, recent];
        return HttpResponse.json({
          items: query.get("category") === "legal" ? [legal] : oldestFirst,
        });
      }),
    );
    await renderApp({ path: "/moderation" });

    const waiting = within(await screen.findByRole("region", { name: "Waiting for you" }));
    expect(await waiting.findByRole("link", { name: "Open reports" })).toHaveAttribute("href", "/moderation/reports");
    expect(waiting.getByText("Waiting")).toBeInTheDocument();
    expect(waiting.getByRole("link", { name: "Longest report wait" })).toHaveAttribute(
      "href",
      "/moderation/reports?order=oldest",
    );
    expect(waiting.getByText("4 days")).toBeInTheDocument();
    expect(waiting.getByText("Waiting long")).toBeInTheDocument();
    // A viewer without manage_accounts has no tiles for registrations and appeals.
    expect(waiting.queryByRole("link", { name: "Registrations awaiting approval" })).not.toBeInTheDocument();

    const first = within(screen.getByRole("region", { name: "Reports to handle first" }));
    const items = await first.findAllByRole("listitem");
    expect(items.map((item) => within(item).getAllByRole("link")[0]?.getAttribute("href"))).toEqual([
      "/moderation/reports/r-legal",
      "/moderation/reports/r-old",
      "/moderation/reports/r-new",
    ]);
    const [legalItem, oldItem] = items as [HTMLElement, HTMLElement, HTMLElement];
    expect(within(legalItem).getByText("Illegal content")).toBeInTheDocument();
    expect(within(legalItem).getByText("Unassigned")).toBeInTheDocument();
    expect(within(oldItem).getByText("Waiting 4 days")).toHaveClass("bg-danger-bg");
    expect(within(oldItem).getByText("You")).toBeInTheDocument();
    expect(first.getByRole("link", { name: "Open the report queue" })).toHaveAttribute("href", "/moderation/reports");
    expect(queries.toSorted()).toEqual([
      "limit=40&state=open&category=legal&order=oldest",
      "limit=40&state=open&order=oldest",
    ]);
  });

  test("says so when no report is open", async () => {
    server.use(operatorHandler(["manage_reports"]), countsHandler({ open_reports: 0 }));
    await renderApp({ path: "/moderation" });

    const first = within(await screen.findByRole("region", { name: "Reports to handle first" }));
    expect(await first.findByRole("heading", { level: 3, name: "No open reports" })).toBeInTheDocument();
    const waiting = within(screen.getByRole("region", { name: "Waiting for you" }));
    await waiting.findByRole("link", { name: "Open reports" });
    expect(waiting.queryByText("Waiting")).not.toBeInTheDocument();
    expect(waiting.getByText("No wait")).toBeInTheDocument();
  });
});
