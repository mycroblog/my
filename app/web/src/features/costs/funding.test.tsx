// SPDX-License-Identifier: AGPL-3.0-only
// The funding meter (FR-905, ADR-0067 §22) through the whole app: in the running costs section of the information page,
// on the supporter list, and in the third column while the instance shows it there. It reads as text (the meter's
// accessible value), says "covered" instead of a number above 100%, and marks a target with estimated costs.
import { screen, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { afterEach, describe, expect, test, vi } from "vitest";
import { instanceFixture, type Schemas } from "../../test/fixtures";
import { apiBase, signedInHandler } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";

afterEach(() => {
  vi.unstubAllGlobals();
});

/** The meter of November 2026 at 72% of a target, showing the percentage only, off the third column. */
const percentMeter: Schemas["FundingMeter"] = {
  visibility: "percent",
  label: "2026-11",
  starts_on: "2026-11-01",
  ends_on: "2026-11-30",
  percent: 72,
  covered: false,
  estimated: false,
  rail: false,
};

function serveInstance(overrides: Partial<Schemas["Instance"]>) {
  server.use(http.get(`${apiBase}/instance`, () => HttpResponse.json({ ...instanceFixture, ...overrides })));
}

/** Makes the screen as wide as xl (80rem), where the layout has the third column; jsdom has no matchMedia otherwise. */
function stubWideScreen() {
  vi.stubGlobal("matchMedia", (query: string) => ({
    matches: query === "(min-width: 80rem)" || query === "(min-width: 48rem)",
    media: query,
    addEventListener: () => undefined,
    removeEventListener: () => undefined,
  }));
}

const meterName = "Support for November 2026";

describe("the funding meter on the information page", () => {
  test("reads as text in the running costs section, even while the costs and the income are private", async () => {
    serveInstance({ finance_visibility: "private", income_visibility: "private", funding_meter: percentMeter });
    await renderApp({ path: "/about" });

    const costs = await screen.findByRole("region", { name: "Running costs" });
    const meter = within(costs).getByRole("meter", { name: meterName });
    expect(meter).toHaveAttribute("aria-valuetext", "72% covered");
    expect(meter).toHaveAttribute("aria-valuenow", "72");
    expect(within(costs).getByText("72% covered")).toBeInTheDocument();
    // Nothing else is published, so there is no page of totals to link to and no amount.
    expect(within(costs).queryByRole("link")).not.toBeInTheDocument();
    expect(within(costs).queryByText(/The goal is/u)).not.toBeInTheDocument();
  });

  test("says the month is covered instead of a number above 100%", async () => {
    serveInstance({ funding_meter: { ...percentMeter, percent: 130, covered: true } });
    await renderApp({ path: "/about" });

    const meter = await screen.findByRole("meter", { name: meterName });
    expect(meter).toHaveAttribute("aria-valuetext", "Covered this month");
    expect(meter).toHaveAttribute("aria-valuenow", "100");
    expect(screen.queryByText(/130/u)).not.toBeInTheDocument();
  });

  test("shows the amounts and the target when they are published, and marks an estimate", async () => {
    serveInstance({
      finance_visibility: "totals",
      income_visibility: "totals",
      funding_meter: {
        ...percentMeter,
        visibility: "amounts",
        estimated: true,
        income: { amount: "3600", currency: "JPY" },
        target: { amount: "5000", currency: "JPY" },
        target_is_costs: true,
      },
    });
    await renderApp({ path: "/about" });

    const costs = await screen.findByRole("region", { name: "Running costs" });
    const meter = within(costs).getByRole("meter", { name: meterName });
    expect(meter).toHaveAttribute("aria-valuetext", "¥3,600 of ¥5,000 (72%), estimated");
    expect(within(costs).getByText("The goal is this month's running costs.")).toBeInTheDocument();
    expect(
      within(costs).getByText(
        "The costs include usage charges of a month the provider has not closed yet, so this may still change.",
      ),
    ).toBeInTheDocument();
    expect(within(costs).getByRole("link", { name: "See the running costs and income" })).toHaveAttribute(
      "href",
      "/about/costs",
    );
  });

  test("names the target the operators set", async () => {
    serveInstance({
      finance_visibility: "totals",
      income_visibility: "totals",
      funding_meter: {
        ...percentMeter,
        visibility: "amounts",
        income: { amount: "12.5", currency: "USD" },
        target: { amount: "50", currency: "USD" },
        target_is_costs: false,
      },
    });
    await renderApp({ path: "/about" });

    const costs = await screen.findByRole("region", { name: "Running costs" });
    expect(within(costs).getByRole("meter", { name: meterName })).toHaveAttribute(
      "aria-valuetext",
      "$12.50 of $50.00 (72%)",
    );
    expect(within(costs).getByText("The goal is $50.00 a month, set by the operators.")).toBeInTheDocument();
  });

  test("is in Japanese in the Japanese locale", async () => {
    serveInstance({ funding_meter: { ...percentMeter, estimated: true } });
    await renderApp({ path: "/about", locale: "ja" });

    const meter = await screen.findByRole("meter", { name: "2026年11月の支援" });
    expect(meter).toHaveAttribute("aria-valuetext", "72% をまかなえています (見込み)");
  });

  test("leaves the section out while the instance publishes neither the totals nor a meter", async () => {
    serveInstance({ finance_visibility: "private", income_visibility: "private" });
    await renderApp({ path: "/about" });

    expect(await screen.findByRole("heading", { level: 1, name: "About my example" })).toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Running costs" })).not.toBeInTheDocument();
    expect(screen.queryByRole("meter")).not.toBeInTheDocument();
  });
});

describe("the funding meter on the supporter list", () => {
  test("comes before the list", async () => {
    serveInstance({ funding_meter: percentMeter });
    await renderApp({ path: "/about/supporters" });

    expect(await screen.findByRole("meter", { name: meterName })).toHaveAttribute("aria-valuetext", "72% covered");
  });
});

describe("the funding meter in the third column", () => {
  test("shows while the instance turns it on, linking to the information page", async () => {
    stubWideScreen();
    server.use(signedInHandler);
    serveInstance({ funding_meter: { ...percentMeter, rail: true } });
    await renderApp({ path: "/home" });

    const panel = await screen.findByRole("complementary", { name: "Supporting this server" });
    expect(within(panel).getByRole("meter", { name: meterName })).toHaveAttribute("aria-valuetext", "72% covered");
    expect(within(panel).getByRole("link", { name: "More about the running costs" })).toHaveAttribute("href", "/about");
  });

  test("stays out while the instance keeps it to the information page", async () => {
    stubWideScreen();
    server.use(signedInHandler);
    serveInstance({ funding_meter: percentMeter });
    await renderApp({ path: "/home" });

    expect(await screen.findByRole("complementary", { name: "About this server" })).toBeInTheDocument();
    expect(screen.queryByRole("complementary", { name: "Supporting this server" })).not.toBeInTheDocument();
  });

  test("stays out of the information page, which shows the meter itself", async () => {
    stubWideScreen();
    server.use(signedInHandler);
    serveInstance({ funding_meter: { ...percentMeter, rail: true } });
    await renderApp({ path: "/about" });

    expect(await screen.findByRole("meter", { name: meterName })).toBeInTheDocument();
    expect(screen.getAllByRole("meter")).toHaveLength(1);
    expect(screen.queryByRole("complementary", { name: "Supporting this server" })).not.toBeInTheDocument();
  });
});
