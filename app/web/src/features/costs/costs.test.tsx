// SPDX-License-Identifier: AGPL-3.0-only
// The public running costs and income through the whole app (web-client/README.md T4, FR-903, FR-917, ADR-0067 §8,
// ADR-0073 §7): the section of the information page and the page at /about/costs show what the instance settings
// finance_visibility and income_visibility publish, for everyone, signed in or not: of the costs, nothing, the totals
// only, or the totals with their breakdown by provider and category; of the income, nothing, the totals, or the totals
// with the members of each plan; and the balance only when both are published. No individual payment is ever shown.
import { screen, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import { instanceFixture, type Schemas } from "../../test/fixtures";
import { apiBase, problem } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import { deviceToday } from "./api/costs";
import { incomeTotalsFixture, publicTotalsFixture } from "./test/handlers";

beforeEach(() => {
  document.documentElement.lang = "en";
  document.documentElement.dir = "ltr";
});

/**
 * Plays the instance with `visibility` and `income`, and the public totals of the costs and of the income as an
 * instance with them answers them; records the queries of the costs and every path asked for.
 */
function serveCosts(
  visibility: Schemas["Instance"]["finance_visibility"],
  income: Schemas["Instance"]["income_visibility"] = "private",
) {
  const queries: Record<string, string>[] = [];
  const incomeQueries: Record<string, string>[] = [];
  const paths: string[] = [];
  server.events.on("request:start", ({ request }) => {
    paths.push(new URL(request.url).pathname);
  });
  server.use(
    http.get(`${apiBase}/instance`, () =>
      HttpResponse.json({ ...instanceFixture, finance_visibility: visibility, income_visibility: income }),
    ),
    http.get(`${apiBase}/instance/income_totals`, ({ request }) => {
      const url = new URL(request.url);
      incomeQueries.push(Object.fromEntries(url.searchParams));
      if (income === "private") return problem(404, "about:blank", "Not Found");
      return HttpResponse.json(
        incomeTotalsFixture(url, { balance: visibility !== "private", plans: income === "plans" }),
      );
    }),
    http.get(`${apiBase}/instance/cost_totals`, ({ request }) => {
      const url = new URL(request.url);
      queries.push(Object.fromEntries(url.searchParams));
      if (visibility === "private") return problem(404, "about:blank", "Not Found");
      const totals = publicTotalsFixture(url);
      if (visibility === "breakdown") return HttpResponse.json(totals);
      return HttpResponse.json({ ...totals, periods: totals.periods.map((period) => ({ ...period, breakdown: [] })) });
    }),
  );
  return { queries, incomeQueries, paths };
}

describe("a server that publishes nothing", () => {
  test("has no costs on its information page", async () => {
    serveCosts("private");
    await renderApp({ path: "/about" });

    await screen.findByRole("region", { name: "Search engines and AI" });
    expect(screen.queryByRole("region", { name: "Running costs" })).not.toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "See the running costs" })).not.toBeInTheDocument();
  });

  test("says so on the costs page, without any amount", async () => {
    serveCosts("private");
    await renderApp({ path: "/about/costs" });

    expect(await screen.findByRole("heading", { level: 1, name: "Running costs of my example" })).toBeInTheDocument();
    expect(
      await screen.findByRole("heading", {
        level: 2,
        name: "This server does not publish its running costs or its income.",
      }),
    ).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "About this server" })).toHaveAttribute("href", "/about");
    expect(screen.queryByRole("table")).not.toBeInTheDocument();
  });
});

describe("a server that publishes its totals", () => {
  test("links to them from its information page", async () => {
    serveCosts("totals");
    await renderApp({ path: "/about" });

    const section = await screen.findByRole("region", { name: "Running costs" });
    expect(section).toHaveTextContent(
      "This server publishes the totals of what it costs to run by week, month, and year.",
    );
    expect(within(section).getByRole("link", { name: "See the running costs" })).toHaveAttribute(
      "href",
      "/about/costs",
    );
    expect(section).not.toHaveTextContent("income");
  });

  test("shows the totals by period without the breakdown, and switches to weeks and cash", async () => {
    const { queries } = serveCosts("totals");
    const { user, router } = await renderApp({ path: "/about/costs" });

    const table = await screen.findByRole("table", { name: "Totals by Months, Spread over the period paid for" });
    expect(within(table).getAllByRole("row")[1]).toHaveTextContent("¥1,650 + $12.34");
    expect(screen.getByText("This server publishes the totals only, not what they are spent on.")).toBeInTheDocument();
    expect(screen.queryByRole("region", { name: /^Breakdown of/u })).not.toBeInTheDocument();
    expect(screen.getByRole("region", { name: "How the periods are counted" })).toHaveTextContent(
      "Each currency is totaled on its own and never converted.",
    );
    expect(queries[0]).toEqual({
      granularity: "month",
      basis: "amortized",
      from: expect.stringMatching(/-01$/u) as string,
      to: deviceToday(),
    });

    await user.click(screen.getByRole("tab", { name: "Weeks" }));
    expect(
      await screen.findByRole("table", { name: "Totals by Weeks, Spread over the period paid for" }),
    ).toBeInTheDocument();
    // The panel of the selected tab is the one its aria-controls names, labeled by it.
    const weeks = screen.getByRole("tab", { name: "Weeks" });
    const panel = screen.getByRole("tabpanel");
    expect(weeks).toHaveAttribute("aria-controls", panel.id);
    expect(panel).toHaveAttribute("aria-labelledby", weeks.id);
    await user.click(screen.getByRole("radio", { name: /On the payment date/u }));
    expect(await screen.findByRole("table", { name: "Totals by Weeks, On the payment date" })).toBeInTheDocument();
    expect(router.state.location.search).toEqual({ granularity: "week", basis: "cash" });
    expect(queries.at(-1)).toMatchObject({ granularity: "week", basis: "cash" });
  });
});

describe("a server that publishes the breakdown", () => {
  test("shows the breakdown of the current period by provider and category", async () => {
    serveCosts("breakdown");
    await renderApp({ path: "/about/costs" });

    const breakdown = await screen.findByRole("region", { name: /^Breakdown of/u });
    const lines = within(breakdown).getAllByRole("listitem");
    expect(lines.map((line) => line.textContent)).toEqual(["RegistrarDomain¥1,650", "CloudServers$12.34"]);
    expect(within(breakdown).getByText("Cloud")).toHaveAttribute("dir", "auto");
    expect(
      screen.queryByText("This server publishes the totals only, not what they are spent on."),
    ).not.toBeInTheDocument();
  });

  test("shows each provider as the server spells it, as entered (ADR-0098)", async () => {
    serveCosts("breakdown");
    server.use(
      http.get(`${apiBase}/instance/cost_totals`, ({ request }) => {
        const totals = publicTotalsFixture(new URL(request.url));
        const breakdown: Schemas["CostBreakdownLine"][] = [
          { provider: "ConoHa", category: "compute", amount: { amount: "1650", currency: "JPY" } },
          { provider: "Amazon SES", category: "email", amount: { amount: "0.10", currency: "USD" } },
          { provider: "Cloudflare R2", category: "storage", amount: { amount: "1.50", currency: "USD" } },
        ];
        return HttpResponse.json({ ...totals, periods: totals.periods.map((period) => ({ ...period, breakdown })) });
      }),
    );
    await renderApp({ path: "/about/costs" });

    const breakdown = await screen.findByRole("region", { name: /^Breakdown of/u });
    for (const provider of ["ConoHa", "Amazon SES", "Cloudflare R2"]) {
      expect(within(breakdown).getByText(provider)).toHaveAttribute("dir", "auto");
    }
    expect(within(breakdown).queryByText("conoha")).not.toBeInTheDocument();
  });

  test("says what it publishes on its information page", async () => {
    serveCosts("breakdown");
    await renderApp({ path: "/about" });

    const section = await screen.findByRole("region", { name: "Running costs" });
    expect(section).toHaveTextContent("with a breakdown by provider and category");
  });
});

describe("a server that publishes its income", () => {
  test("shows the income without the balance while the costs are private, and no payment", async () => {
    const { incomeQueries, paths } = serveCosts("private", "totals");
    await renderApp({ path: "/about/costs" });

    const income = await screen.findByRole("region", { name: "Income" });
    expect(screen.queryByRole("region", { name: "Costs" })).not.toBeInTheDocument();
    const table = await within(income).findByRole("table", {
      name: "Income by Months, Spread over the period paid for",
    });
    expect(
      within(table)
        .getAllByRole("columnheader")
        .map((cell) => cell.textContent),
    ).toEqual(["Period", "Income after fees"]);
    expect(within(table).getAllByRole("row")[1]).toHaveTextContent("¥4,800");
    expect(income).toHaveTextContent("The balance is not published, since this server does not publish its costs.");
    expect(within(income).queryByRole("region", { name: "Members of each plan" })).not.toBeInTheDocument();
    expect(within(income).getByRole("region", { name: "How the income is counted" })).toHaveTextContent(
      "stripe dates its payments in the Asia/Tokyo time zone.",
    );
    expect(incomeQueries[0]).toEqual({
      granularity: "month",
      basis: "amortized",
      from: expect.stringMatching(/-01$/u) as string,
      to: deviceToday(),
    });
    // Nothing of the individual payments: neither their screen's data nor their columns.
    expect(
      paths.filter((path) => path.startsWith(`${apiBase}/admin/`) || path.startsWith(`${apiBase}/finance/`)),
    ).toEqual([]);
    expect(screen.queryByRole("columnheader", { name: "Gross" })).not.toBeInTheDocument();
    expect(screen.queryByRole("columnheader", { name: "Fees" })).not.toBeInTheDocument();
  });

  test("says what it publishes on its information page, and links to the income", async () => {
    serveCosts("private", "totals");
    await renderApp({ path: "/about" });

    const section = await screen.findByRole("region", { name: "Running costs" });
    expect(section).toHaveTextContent("This server publishes the totals of its income from its supporters");
    expect(section).not.toHaveTextContent("It also publishes the balance");
    expect(within(section).getByRole("link", { name: "See the income" })).toHaveAttribute("href", "/about/costs");
  });

  test("shows the balance and the members of each plan with the costs, and follows the tabs of both", async () => {
    const { incomeQueries } = serveCosts("totals", "plans");
    const { user } = await renderApp({ path: "/about/costs" });

    const income = await screen.findByRole("region", { name: "Income" });
    expect(screen.getByRole("region", { name: "Costs" })).toBeInTheDocument();
    const table = await within(income).findByRole("table", {
      name: "Income by Months, Spread over the period paid for",
    });
    expect(
      within(table)
        .getAllByRole("columnheader")
        .map((cell) => cell.textContent),
    ).toEqual(["Period", "Income after fees", "Balance"]);
    const row = within(table).getAllByRole("row")[1];
    expect(row).toHaveTextContent("¥4,800");
    expect(row).toHaveTextContent("+¥3,150");
    expect(row).toHaveTextContent("$12.34");
    expect(income).not.toHaveTextContent("The balance is not published");
    const plans = within(income).getByRole("region", { name: "Members of each plan" });
    expect(plans).toHaveTextContent("Coffee3 members");
    expect(within(plans).getByText("Coffee")).toHaveAttribute("dir", "auto");
    expect(within(income).getByRole("region", { name: "How the income is counted" })).toHaveTextContent(
      "The balance is the income less the costs",
    );

    await user.click(screen.getByRole("tab", { name: "Years" }));
    expect(
      await screen.findByRole("table", { name: "Income by Years, Spread over the period paid for" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("table", { name: "Totals by Years, Spread over the period paid for" })).toBeInTheDocument();
    expect(incomeQueries.at(-1)).toMatchObject({ granularity: "year", basis: "amortized" });
  });

  test("says on its information page that the balance is published too", async () => {
    serveCosts("breakdown", "plans");
    await renderApp({ path: "/about" });

    const section = await screen.findByRole("region", { name: "Running costs" });
    expect(section).toHaveTextContent("and how many members each plan has now");
    expect(section).toHaveTextContent("It also publishes the balance: the income less the costs.");
    expect(within(section).getByRole("link", { name: "See the running costs and income" })).toHaveAttribute(
      "href",
      "/about/costs",
    );
  });
});
