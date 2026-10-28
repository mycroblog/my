// SPDX-License-Identifier: AGPL-3.0-only
import { render, screen } from "@testing-library/react";
import { userEvent } from "@testing-library/user-event";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";
import { ariaSnapshot } from "../test/aria-snapshot";
import { ContentWarning } from "./ContentWarning";

const ignore = () => undefined;

function Warning({ onExpandedChange = ignore }: { onExpandedChange?: (expanded: boolean) => void }) {
  return (
    <ContentWarning
      summary="Spoilers for the finale"
      summaryLang="en"
      toggleLabel="Show content"
      onExpandedChange={onExpandedChange}
    >
      <p>The butler did it.</p>
      <a href="#more">Read more</a>
    </ContentWarning>
  );
}

describe("ContentWarning", () => {
  it("is a disclosure whose closed content is hidden and out of the tab order", async () => {
    const user = userEvent.setup();
    const { container } = render(<Warning />);
    expect(ariaSnapshot(container)).toBe(
      [
        `- generic [lang=en] [dir=auto]:`,
        `  - text: "Spoilers for the finale"`,
        `- button "Show content" [expanded=false]`,
      ].join("\n"),
    );
    const button = screen.getByRole("button", { name: "Show content" });
    expect(button).toHaveAttribute("aria-controls", expect.any(String));
    expect(screen.getByText("The butler did it.")).not.toBeVisible();
    expect(screen.queryByRole("link", { name: "Read more" })).not.toBeInTheDocument();

    await user.tab();
    expect(button).toHaveFocus();
    await user.tab();
    expect(document.body).toHaveFocus();
  });

  it("opens and closes with the button, keeping focus on it", async () => {
    const user = userEvent.setup();
    const onExpandedChange = vi.fn();
    render(<Warning onExpandedChange={onExpandedChange} />);
    const button = screen.getByRole("button", { name: "Show content" });
    await user.tab();
    await user.keyboard("{Enter}");
    expect(button).toHaveAttribute("aria-expanded", "true");
    expect(button).toHaveFocus();
    expect(screen.getByText("The butler did it.")).toBeVisible();
    await user.tab();
    expect(screen.getByRole("link", { name: "Read more" })).toHaveFocus();

    await user.click(button);
    expect(button).toHaveAttribute("aria-expanded", "false");
    expect(onExpandedChange.mock.calls).toEqual([[true], [false]]);
  });

  it("can be controlled", async () => {
    const user = userEvent.setup();
    function Controlled() {
      const [expanded, setExpanded] = useState(true);
      return (
        <ContentWarning summary="CW" toggleLabel="Show content" isExpanded={expanded} onExpandedChange={setExpanded}>
          <p>Behind the warning</p>
        </ContentWarning>
      );
    }
    render(<Controlled />);
    expect(screen.getByText("Behind the warning")).toBeVisible();
    await user.click(screen.getByRole("button", { name: "Show content" }));
    expect(screen.getByText("Behind the warning")).not.toBeVisible();
  });
});
