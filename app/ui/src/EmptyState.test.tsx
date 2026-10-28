// SPDX-License-Identifier: AGPL-3.0-only
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { EmptyState } from "./EmptyState";

describe("EmptyState", () => {
  it("has an h2 heading by default", () => {
    render(<EmptyState heading="No posts yet" />);
    expect(screen.getByRole("heading", { level: 2, name: "No posts yet" })).toBeInTheDocument();
  });

  it("takes the heading level, description and action from props", () => {
    render(
      <EmptyState
        heading="No results"
        headingLevel={4}
        description="Try another word."
        action={<a href="#x">Explore</a>}
      />,
    );
    expect(screen.getByRole("heading", { level: 4, name: "No results" })).toBeInTheDocument();
    expect(screen.getByText("Try another word.")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Explore" })).toBeInTheDocument();
  });

  it("hides the illustration from assistive technology", () => {
    render(<EmptyState heading="Empty" illustration={<svg role="img" aria-label="box" />} />);
    expect(screen.queryByRole("img")).not.toBeInTheDocument();
  });
});
