// SPDX-License-Identifier: AGPL-3.0-only
import { render, screen, within } from "@testing-library/react";
import { userEvent } from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { Section } from "./Section";
import { SectionProgress } from "./SectionProgress";

describe("SectionProgress", () => {
  beforeEach(() => {
    class FakeObserver {
      observe() {}
      disconnect() {}
      unobserve() {}
    }
    vi.stubGlobal("IntersectionObserver", FakeObserver);
    Element.prototype.scrollIntoView = vi.fn();
    window.matchMedia = vi.fn().mockReturnValue({ matches: false });
  });

  it("links to each part in order, moves focus to the chosen heading and marks it and the parts before it", async () => {
    const user = userEvent.setup();
    const parts = [
      { id: "a", title: "Alpha" },
      { id: "b", title: "Beta" },
      { id: "c", title: "Gamma" },
    ];
    render(
      <SectionProgress label="Parts of this page" parts={parts}>
        {parts.map((part) => (
          <Section key={part.id} title={part.title} headingId={part.id}>
            <p>Text</p>
          </Section>
        ))}
      </SectionProgress>,
    );
    const nav = screen.getByRole("navigation", { name: "Parts of this page" });
    const links = within(nav).getAllByRole("link");
    expect(links.map((link) => link.getAttribute("href"))).toEqual(["#a", "#b", "#c"]);
    expect(links.map((link) => link.textContent)).toEqual(["1Alpha", "2Beta", "3Gamma"]);

    const beta = within(nav).getByRole("link", { name: "Beta" });
    await user.click(beta);
    expect(screen.getByRole("heading", { level: 2, name: "Beta" })).toHaveFocus();
    expect(beta).toHaveAttribute("aria-current", "location");
    expect(links.map((link) => link.hasAttribute("data-passed"))).toEqual([true, false, false]);
  });

  it("writes the line's height to --section-progress-block-size while it is mounted, for the headings' scroll margin", () => {
    const { unmount } = render(
      <SectionProgress label="Parts of this page" parts={[{ id: "a", title: "Alpha" }]}>
        <Section title="Alpha" headingId="a">
          <p>Text</p>
        </Section>
      </SectionProgress>,
    );
    expect(document.documentElement.style.getPropertyValue("--section-progress-block-size")).toBe("0px");
    unmount();
    expect(document.documentElement.style.getPropertyValue("--section-progress-block-size")).toBe("");
  });
});
