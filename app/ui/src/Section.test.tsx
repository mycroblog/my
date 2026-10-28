// SPDX-License-Identifier: AGPL-3.0-only
import { render, screen } from "@testing-library/react";
import { createRef } from "react";
import { describe, expect, it } from "vitest";
import { Section } from "./Section";

describe("Section", () => {
  it("names the section by its h2 in the look of a region title", () => {
    render(<Section title="Password">content</Section>);
    const region = screen.getByRole("region", { name: "Password" });
    const heading = screen.getByRole("heading", { level: 2, name: "Password" });
    expect(region.tagName).toBe("SECTION");
    expect(region).toContainElement(heading);
    expect(heading).toHaveClass("text-lg");
  });

  it("describes the section by its description", () => {
    render(
      <Section title="Fields" description="Up to four.">
        content
      </Section>,
    );
    expect(screen.getByRole("region", { name: "Fields" })).toHaveAccessibleDescription("Up to four.");
  });

  it("keeps a given heading id, so that a control in the region can refer to it", () => {
    render(
      <Section title="Avatar" headingId="avatar-heading">
        <button type="button" aria-describedby="avatar-heading">
          Choose
        </button>
      </Section>,
    );
    expect(screen.getByRole("button", { name: "Choose" })).toHaveAccessibleDescription("Avatar");
  });

  it("lets the heading or the section take focus from script", () => {
    const headingRef = createRef<HTMLHeadingElement>();
    const ref = createRef<HTMLElement>();
    render(
      <>
        <Section title="Status" headingRef={headingRef}>
          content
        </Section>
        <Section title="Secret" ref={ref} focusable surface="card">
          content
        </Section>
      </>,
    );
    headingRef.current?.focus();
    expect(screen.getByRole("heading", { name: "Status" })).toHaveFocus();
    ref.current?.focus();
    expect(screen.getByRole("region", { name: "Secret" })).toHaveFocus();
  });

  it("marks the section busy", () => {
    render(
      <Section title="Replies" surface="rows" busy>
        content
      </Section>,
    );
    expect(screen.getByRole("region", { name: "Replies" })).toHaveAttribute("aria-busy", "true");
  });
});
