// SPDX-License-Identifier: AGPL-3.0-only
import { render, screen } from "@testing-library/react";
import { userEvent } from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { ArticleCard } from "./ArticleCard";

const cover = { sources: { still: [{ url: "/cover.png" }] }, width: 1600, height: 900, alt: "A tomato" };
const common = {
  href: "/@alice/status/1",
  untitledLabel: "Untitled article",
  kindLabel: "Article",
  revealLabel: "Show sensitive media",
};

describe("ArticleCard", () => {
  it("is one link to the article's page, with its title, summary and kind", () => {
    render(<ArticleCard {...common} title="Tomatoes" summary="On balconies." lang="en" />);
    const link = screen.getByRole("link", { name: "Tomatoes" });
    expect(link).toHaveAttribute("href", "/@alice/status/1");
    expect(link).not.toHaveAttribute("target");
    expect(screen.getAllByRole("link")).toHaveLength(1);
    expect(screen.getByText("Article")).toBeInTheDocument();
    // The title and the summary are user content (web-client/README.md Y10).
    expect(screen.getByText("Tomatoes")).toHaveAttribute("dir", "auto");
    expect(screen.getByText("Tomatoes")).toHaveAttribute("lang", "en");
    expect(screen.getByText("On balconies.")).toHaveAttribute("dir", "auto");
    expect(screen.getByText("On balconies.")).toHaveAttribute("lang", "en");
  });

  it("names an article without a title in the app's words, outside its language", () => {
    render(<ArticleCard {...common} title={null} lang="ar" />);
    expect(screen.getByRole("link", { name: "Untitled article" })).not.toHaveAttribute("lang");
    expect(screen.getByText("Untitled article")).not.toHaveAttribute("lang");
  });

  it("shows the cover named by its description, and conceals it while the article is sensitive", async () => {
    const user = userEvent.setup();
    const { rerender } = render(<ArticleCard {...common} title="Tomatoes" cover={cover} />);
    expect(screen.getByRole("img", { name: "A tomato" })).toHaveAttribute("src", "/cover.png");
    rerender(<ArticleCard {...common} title="Tomatoes" summary="Still here." cover={cover} sensitive />);
    expect(screen.queryByRole("img")).not.toBeInTheDocument();
    expect(screen.getByText("Still here.")).toBeInTheDocument();
    const toggle = screen.getByRole("button", { name: "Show sensitive media" });
    await user.click(toggle);
    expect(toggle).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByRole("img", { name: "A tomato" })).toBeInTheDocument();
  });
});
