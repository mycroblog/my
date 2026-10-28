// SPDX-License-Identifier: AGPL-3.0-only
import { render, screen, within } from "@testing-library/react";
import { I18nProvider } from "react-aria-components";
import { describe, expect, it } from "vitest";
import { BreakdownBar } from "./BreakdownBar";

describe("BreakdownBar", () => {
  it("lists each part's name, value and share in text", () => {
    render(
      <I18nProvider locale="en-US">
        <BreakdownBar
          label="Costs"
          items={[
            { id: "a", name: "Hosting", value: 30, valueText: "$30" },
            { id: "b", name: "Storage", value: 10, valueText: "$10" },
            { id: "c", name: "Refund", value: -5, valueText: "−$5" },
          ]}
        />
      </I18nProvider>,
    );
    const items = within(screen.getByRole("list", { name: "Costs" })).getAllByRole("listitem");
    expect(items.map((item) => item.textContent)).toEqual(["Hosting$3075%", "Storage$1025%", "Refund−$50%"]);
  });

  it("shows 0% for every part of an empty whole", () => {
    render(
      <I18nProvider locale="en-US">
        <BreakdownBar label="Costs" items={[{ id: "a", name: "Hosting", value: 0, valueText: "$0" }]} />
      </I18nProvider>,
    );
    expect(screen.getByText("0%")).toBeInTheDocument();
  });
});
