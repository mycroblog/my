// SPDX-License-Identifier: AGPL-3.0-only
// The administration overview (screens/admin.md): whether the server runs well, as tiles that lead to their screens:
// the failed jobs, the longest wait of the job queue (a length, not a date), and the open error groups.
import { screen, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import { apiBase, operatorHandler, signedInHandler } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import { errorGroupFixture } from "./test/handlers";

beforeEach(() => {
  server.use(signedInHandler);
});

describe("the administration overview", () => {
  test("warns of failed jobs, a backed-up queue and open errors, each tile a link to its screen", async () => {
    server.use(
      operatorHandler(["manage_jobs", "manage_errors"]),
      http.get(`${apiBase}/admin/queue`, () =>
        HttpResponse.json({
          adapter: "db",
          pending_listing: true,
          counts: [
            { state: "queued", type: "federation.deliver", count: 12 },
            { state: "dead", type: "federation.deliver", count: 2 },
            { state: "dead", type: "kv.cleanup", count: 1 },
          ],
          oldest_ready_at: new Date(Date.now() - 20 * 60 * 1000).toISOString(),
        }),
      ),
      http.get(`${apiBase}/admin/error_groups`, () =>
        HttpResponse.json({ items: [errorGroupFixture("e1"), errorGroupFixture("e2")], next_cursor: "e2" }),
      ),
    );
    await renderApp({ path: "/admin" });

    const health = within(await screen.findByRole("region", { name: "Server health" }));
    // Each tile replaces its loading one once its number loads.
    await health.findByText("Needs a retry");
    await health.findByText("2+");
    const tiles = health.getAllByRole("listitem");
    expect(tiles).toHaveLength(3);
    const [failed, wait, errors] = tiles as [HTMLElement, HTMLElement, HTMLElement];
    expect(within(failed).getByText("3")).toBeInTheDocument();
    expect(within(failed).getByText("Needs a retry")).toBeInTheDocument();
    expect(within(failed).getByRole("link", { name: "Failed jobs" })).toHaveAttribute("href", "/admin/jobs?state=dead");
    expect(within(wait).getByText(/^(19|20) min\.$/u)).toBeInTheDocument();
    expect(within(wait).getByText("Backed up")).toBeInTheDocument();
    expect(within(errors).getByText("2+")).toBeInTheDocument();
    expect(within(errors).getByRole("link", { name: "Open errors" })).toHaveAttribute("href", "/admin/errors");
  });

  test("has nothing to warn of when the queue is idle", async () => {
    server.use(
      operatorHandler(["manage_jobs"]),
      http.get(`${apiBase}/admin/queue`, () => HttpResponse.json({ adapter: "db", pending_listing: true, counts: [] })),
    );
    await renderApp({ path: "/admin" });

    const health = within(await screen.findByRole("region", { name: "Server health" }));
    expect(await health.findByText("No wait")).toBeInTheDocument();
    expect(health.getByText("0")).toBeInTheDocument();
    expect(health.queryByText("Needs a retry")).not.toBeInTheDocument();
  });

  test("points to the menu when the roles watch nothing the overview shows", async () => {
    server.use(operatorHandler(["manage_settings"]));
    await renderApp({ path: "/admin" });

    expect(
      await screen.findByText("Your roles watch nothing that this overview shows. Open a screen from the menu."),
    ).toBeInTheDocument();
  });
});
