// SPDX-License-Identifier: AGPL-3.0-only
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { HomeIcon, LocalIcon } from "./Icons";
import { RailItem } from "./RailItem";

/** The element that holds the item's icon: the pill of the tab bar. */
function pillOf(name: string): HTMLElement {
  const icon = screen.getByRole("link", { name }).querySelector("svg");
  if (icon?.parentElement == null) throw new Error(`no icon in ${name}`);
  return icon.parentElement;
}

describe("RailItem", () => {
  // The place the user is in shows by shape, not by the label's color alone (WCAG 1.4.1, web-client §21 P3).
  it("fills the pill behind the current tab's icon, and only the current one's", () => {
    render(
      <ul>
        <li>
          <RailItem href="/home" icon={<HomeIcon />} label="Home" current />
        </li>
        <li>
          <RailItem href="/local" icon={<LocalIcon />} label="Local" />
        </li>
      </ul>,
    );
    expect(screen.getByRole("link", { name: "Home" })).toHaveAttribute("aria-current", "page");
    expect(screen.getByRole("link", { name: "Local" })).not.toHaveAttribute("aria-current");
    expect(pillOf("Home")).toHaveClass("bg-soft");
    expect(pillOf("Local")).not.toHaveClass("bg-soft");
  });
});
