// SPDX-License-Identifier: AGPL-3.0-only
// The finances page through the whole app (web-client/README.md T4, ADR-0070 §5): a user whose roles have view_finance, and
// no staff permission, has Finances in the navigation and neither Moderation nor Administration, and reads the
// totals of the costs with their breakdown always and the lines of the items as far as the server settings show them;
// a holder of manage_finance reads every line. The income comes with the balance always, and with the members of each
// plan when the server shows them, never with anyone's payments. A user without either permission is told why and
// sends nothing.
// jsdom applies no styles, so the rail's and the tab bar's items are all in the document.
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import type { Schemas } from "../../test/fixtures";
import { apiBase, operatorHandler, signedInHandler } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import { deviceToday } from "./api/costs";
import { financeTotalsFixture, incomeTotalsFixture } from "./test/handlers";

beforeEach(() => {
  document.documentElement.lang = "en";
  document.documentElement.dir = "ltr";
  server.use(signedInHandler);
});

function navigation() {
  return screen.getByRole("navigation", { name: "Main" });
}

/** Plays the finances totals as a server with `viewer` answers them; records the queries and the admin requests. */
function serveFinance(
  viewer: Schemas["FinanceViewerSettings"] = { item_names: false, item_memos: false, income: "totals" },
) {
  const queries: Record<string, string>[] = [];
  const adminRequests: string[] = [];
  server.events.on("request:start", ({ request }) => {
    const path = new URL(request.url).pathname;
    if (path.startsWith(`${apiBase}/admin/`)) adminRequests.push(path);
  });
  server.use(
    http.get(`${apiBase}/finance/cost_totals`, ({ request }) => {
      const url = new URL(request.url);
      queries.push(Object.fromEntries(url.searchParams));
      return HttpResponse.json(financeTotalsFixture(url, viewer));
    }),
  );
  return { queries, adminRequests };
}

describe("a user with view_finance", () => {
  beforeEach(() => {
    server.use(operatorHandler(["view_finance"]));
  });

  test("has Finances after the user's items and no staff entry, and is asked for no admin data", async () => {
    const finance = serveFinance();
    const { user } = await renderApp({ path: "/home" });
    await screen.findByRole("heading", { level: 1, name: "Home" });

    const link = await within(navigation()).findByRole("link", { name: "Finances" });
    expect(link).toHaveAttribute("href", "/finance");
    expect(within(navigation()).queryByRole("link", { name: /^Moderation/u })).not.toBeInTheDocument();
    expect(within(navigation()).queryByRole("link", { name: /^Administration/u })).not.toBeInTheDocument();
    await user.click(within(navigation()).getByRole("button", { name: "Menu" }));
    const sheet = await screen.findByRole("dialog", { name: "Menu" });
    expect(
      within(sheet)
        .getAllByRole("link")
        .map((item) => item.getAttribute("href")),
    ).toEqual(["/profile", "/bookmarks", "/settings", "/finance", "/", "/about"]);

    await user.click(within(sheet).getByRole("link", { name: "Finances" }));
    const heading = await screen.findByRole("heading", { level: 1, name: "Finances" });
    await waitFor(() => {
      expect(heading).toHaveFocus();
    });
    expect(within(navigation()).getByRole("link", { name: "Finances" })).toHaveAttribute("aria-current", "page");
    expect(finance.adminRequests).toEqual([]);
  });

  test("reads the totals and their breakdown, without the items by default", async () => {
    const finance = serveFinance();
    const { user, router } = await renderApp({ path: "/finance" });

    expect(await screen.findByRole("heading", { level: 1, name: "Finances" })).toBeInTheDocument();
    const costs = screen.getByRole("region", { name: "Costs" });
    const table = await within(costs).findByRole("table", {
      name: "Totals by Months, Spread over the period paid for",
    });
    expect(within(table).getAllByRole("row")[1]).toHaveTextContent("¥1,650 + $12.34");
    expect(within(costs).getByRole("region", { name: /^Breakdown of/u })).toHaveTextContent("RegistrarDomain¥1,650");
    expect(within(costs).queryByRole("region", { name: /^Items of/u })).not.toBeInTheDocument();
    expect(screen.getByText("This server does not show the names or memos of the items.")).toBeInTheDocument();
    expect(screen.queryByText("Main VM")).not.toBeInTheDocument();
    expect(finance.queries[0]).toEqual({
      granularity: "month",
      basis: "amortized",
      from: expect.stringMatching(/-01$/u) as string,
      to: deviceToday(),
    });

    await user.click(screen.getByRole("tab", { name: "Weeks" }));
    expect(
      await within(screen.getByRole("region", { name: "Costs" })).findByRole("table", {
        name: "Totals by Weeks, Spread over the period paid for",
      }),
    ).toBeInTheDocument();
    expect(router.state.location.search).toEqual({ granularity: "week" });
    expect(finance.adminRequests).toEqual([]);
  });

  test("reads the lines of the items with their names and memos when the server shows them", async () => {
    serveFinance({ item_names: true, item_memos: true, income: "totals" });
    await renderApp({ path: "/finance" });

    const items = await screen.findByRole("region", { name: /^Items of/u });
    const lines = within(items).getAllByRole("listitem");
    expect(lines).toHaveLength(2);
    expect(lines[0]).toHaveTextContent("Main VMCloudServers$12.34the main server");
    expect(lines[1]).toHaveTextContent("example.comRegistrarDomain¥1,650");
    expect(within(lines[0] as HTMLElement).getByText("Main VM")).toHaveAttribute("dir", "auto");
    expect(within(lines[0] as HTMLElement).getByText("the main server")).toHaveAttribute("dir", "auto");
    expect(screen.getByText("This server shows the names and memos of the items.")).toBeInTheDocument();
  });

  test("reads the memos without the names when the server shows the memos only", async () => {
    serveFinance({ item_names: false, item_memos: true, income: "totals" });
    await renderApp({ path: "/finance" });

    const items = await screen.findByRole("region", { name: /^Items of/u });
    expect(within(items).getAllByRole("listitem")[0]).toHaveTextContent("CloudServers$12.34the main server");
    expect(within(items).queryByText("Main VM")).not.toBeInTheDocument();
    expect(within(items).queryByText("example.com")).not.toBeInTheDocument();
    expect(screen.getByText("This server shows the memos of the items, not their names.")).toBeInTheDocument();
  });
});

describe("the income on the finances page", () => {
  beforeEach(() => {
    server.use(operatorHandler(["view_finance"]));
  });

  /** Plays the income totals with the members of each plan when `plans`; records the queries. */
  function serveIncome(plans: boolean) {
    const queries: Record<string, string>[] = [];
    server.use(
      http.get(`${apiBase}/finance/income_totals`, ({ request }) => {
        const url = new URL(request.url);
        queries.push(Object.fromEntries(url.searchParams));
        return HttpResponse.json(incomeTotalsFixture(url, { balance: true, plans }));
      }),
    );
    return { queries };
  }

  test("reads the net and the balance, without the members of each plan by default and without any payment", async () => {
    const finance = serveFinance();
    const income = serveIncome(false);
    const { user } = await renderApp({ path: "/finance" });

    const region = await screen.findByRole("region", { name: "Income" });
    const table = await within(region).findByRole("table", {
      name: "Income by Months, Spread over the period paid for",
    });
    expect(
      within(table)
        .getAllByRole("columnheader")
        .map((cell) => cell.textContent),
    ).toEqual(["Period", "Income after fees", "Balance"]);
    expect(within(table).getAllByRole("row")[1]).toHaveTextContent("¥4,800");
    expect(within(table).getAllByRole("row")[1]).toHaveTextContent("+¥3,150");
    expect(within(region).queryByRole("region", { name: "Members of each plan" })).not.toBeInTheDocument();
    expect(income.queries[0]).toEqual({
      granularity: "month",
      basis: "amortized",
      from: expect.stringMatching(/-01$/u) as string,
      to: deviceToday(),
    });

    await user.click(screen.getByRole("radio", { name: /On the payment date/u }));
    expect(
      await within(region).findByRole("table", { name: "Income by Months, On the payment date" }),
    ).toBeInTheDocument();
    expect(within(region).getByRole("region", { name: "How the income is counted" })).toHaveTextContent(
      "Every payment, refund, and fee counts on its date.",
    );
    expect(finance.adminRequests).toEqual([]);
  });

  test("reads the members of each plan when the server shows them", async () => {
    serveFinance({ item_names: false, item_memos: false, income: "plans" });
    serveIncome(true);
    await renderApp({ path: "/finance" });

    const region = await screen.findByRole("region", { name: "Income" });
    expect(await within(region).findByRole("region", { name: "Members of each plan" })).toHaveTextContent(
      "Coffee3 members",
    );
  });
});

describe("the finances page", () => {
  test("shows a holder of manage_finance every line, with no Finances entry of its own", async () => {
    server.use(operatorHandler(["manage_finance"]));
    serveFinance();
    await renderApp({ path: "/finance" });

    // The server answers a holder of manage_finance with every line whatever the settings; the page shows them.
    expect(await screen.findByRole("heading", { level: 1, name: "Finances" })).toBeInTheDocument();
    expect(await screen.findByRole("region", { name: /^Items of/u })).toBeInTheDocument();
    expect(screen.queryByText("This server does not show the names or memos of the items.")).not.toBeInTheDocument();
    expect(within(navigation()).queryByRole("link", { name: "Finances" })).not.toBeInTheDocument();
  });

  test("tells a user without the permission why, without asking for the totals", async () => {
    const finance = serveFinance();
    await renderApp({ path: "/finance" });

    expect(await screen.findByRole("heading", { level: 1, name: "Finances" })).toBeInTheDocument();
    expect(await screen.findByRole("heading", { level: 2, name: "You cannot use this screen" })).toBeInTheDocument();
    expect(screen.getByText(/view_finance/u)).toBeInTheDocument();
    expect(screen.queryByRole("table")).not.toBeInTheDocument();
    expect(finance.queries).toEqual([]);
    expect(within(navigation()).queryByRole("link", { name: "Finances" })).not.toBeInTheDocument();
  });
});
