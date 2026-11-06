// SPDX-License-Identifier: AGPL-3.0-only
import { render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { LocalNav } from "./LocalNav";

describe("LocalNav", () => {
  it("is a named navigation landmark of links, the current page marked, and no tabs", () => {
    render(
      <LocalNav
        label="Finance pages"
        items={[
          { href: "/admin/finance", label: "Overview" },
          { href: "/admin/finance/costs", label: "Costs", current: true },
          { href: "/admin/finance/income", label: "Income" },
        ]}
      />,
    );
    const nav = screen.getByRole("navigation", { name: "Finance pages" });
    const links = within(nav).getAllByRole("link");
    expect(links.map((link) => link.textContent)).toEqual(["Overview", "Costs", "Income"]);
    expect(within(nav).getByRole("link", { name: "Costs" })).toHaveAttribute("aria-current", "page");
    expect(within(nav).getByRole("link", { name: "Overview" })).not.toHaveAttribute("aria-current");
    expect(within(nav).getByRole("link", { name: "Income" })).toHaveAttribute("href", "/admin/finance/income");
    expect(screen.queryByRole("tablist")).not.toBeInTheDocument();
  });
});
