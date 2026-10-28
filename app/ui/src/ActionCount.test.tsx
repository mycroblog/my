// SPDX-License-Identifier: AGPL-3.0-only
import { render, screen } from "@testing-library/react";
import { I18nProvider } from "react-aria-components";
import { describe, expect, it } from "vitest";
import { ActionCount } from "./ActionCount";

describe("ActionCount", () => {
  it("shows nothing for zero and a compact number otherwise", () => {
    const { container, rerender } = render(
      <I18nProvider locale="en-US">
        <ActionCount value={0} />
      </I18nProvider>,
    );
    expect(container).toBeEmptyDOMElement();
    rerender(
      <I18nProvider locale="en-US">
        <ActionCount value={1234} />
      </I18nProvider>,
    );
    expect(screen.getByText("1.2K")).toBeInTheDocument();
  });

  // The count a card opens with stays still; a change after that enters as a new element (P2).
  it("marks a count that changed after it first showed, and only then", () => {
    const { rerender } = render(<ActionCount value={2} id="count" />);
    expect(screen.getByText("2")).not.toHaveAttribute("data-changed");
    const before = screen.getByText("2");

    rerender(<ActionCount value={3} id="count" />);
    const after = screen.getByText("3");
    expect(after).toHaveAttribute("data-changed");
    expect(after).not.toBe(before);
    expect(after).toHaveAttribute("id", "count");
  });
});
