// SPDX-License-Identifier: AGPL-3.0-only

// The funding meter of the month (ADR-0067 §22, FR-905), as `FundingMeterView` draws what the server published: the
// share as a meter and its caption, which names the currency counted where the amounts are known and says "covered"
// once the month is; no amount where the share alone is published; and an estimate said beside it, on the about page
// and in the rail alike. Whether the server publishes the share, the amounts or nothing is the server's and the
// fake's (web/e2e/src/fake/finance.test.ts); the amounts on the page and how it lays them out are the stories'.

import { describe, expect, test } from "bun:test";
import type { components } from "@mycroblog/mykit";
import { cleanup, screen } from "@testing-library/react";
import { fakeData, renderFeature } from "../test/feature";
import { FundingMeterView } from "./meter";

type FundingMeter = components["schemas"]["FundingMeter"];

const share: FundingMeter = {
  visibility: "percent",
  label: "2026-11",
  starts_on: "2026-11-01",
  ends_on: "2026-11-30",
  percent: 72,
  covered: false,
  estimated: false,
  rail: false,
};

const amounts: FundingMeter = {
  ...share,
  visibility: "amounts",
  income: { amount: "14400", currency: "JPY" },
  target: { amount: "20000", currency: "JPY" },
  target_is_costs: false,
};

/** Draws `meter`, the about page's form (`detail`) or the rail's, and returns its text, a run of spaces as one. */
function drawn(meter: FundingMeter, detail: boolean): string {
  cleanup();
  renderFeature(fakeData().data, <FundingMeterView meter={meter} detail={detail} />);
  return document.body.textContent.replace(/\s+/g, " ").trim();
}

const meter = () => screen.getByRole("meter", { name: "Share of this month's costs covered" });

describe.each([
  ["the about page", true],
  ["the rail", false],
] as const)("in %s", (_, detail) => {
  test("the share alone is a meter and its caption, naming no currency and no amount", () => {
    const text = drawn(share, detail);
    expect([meter().getAttribute("aria-valuenow"), meter().getAttribute("aria-valuetext")]).toEqual(["72", "72%"]);
    expect(text).toContain("of November's costs covered");
    expect(text).not.toContain("counted in");
    expect(text).not.toMatch(/[¥$]/);
  });

  test("the caption names the currency the share counts in where the amounts are published", () => {
    expect(drawn(amounts, detail)).toContain("of November's costs covered, counted in JPY");
  });

  test.each([
    [100, "100"],
    [130, "100"],
  ])("at %p%% the month is covered", (percent, now) => {
    const text = drawn({ ...share, percent, covered: true }, detail);
    expect(text).toContain("November is covered");
    expect(text).not.toContain("costs covered");
    expect([meter().getAttribute("aria-valuenow"), meter().getAttribute("aria-valuetext")]).toEqual([
      now,
      `${String(percent)}%`,
    ]);
  });

  test("an estimate in the month's costs is said beside the share", () => {
    expect(drawn({ ...share, estimated: true }, detail)).toContain(
      "This month's costs include an estimate, so this may change.",
    );
    expect(drawn(share, detail)).not.toContain("estimate");
  });
});
