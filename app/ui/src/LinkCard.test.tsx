// SPDX-License-Identifier: AGPL-3.0-only
import { render, screen } from "@testing-library/react";
import { userEvent } from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { LinkCard } from "./LinkCard";

const image = { src: "/card.png", width: 1200, height: 630, alt: "A sunrise" };

describe("LinkCard", () => {
  it("is one link to the page that opens in a new tab without a referrer", () => {
    render(
      <LinkCard
        href="https://news.example/a"
        title="Sunrise"
        description="It rose."
        siteName="Example News"
        lang="en"
        revealLabel="Show"
      />,
    );
    const link = screen.getByRole("link", { name: "Sunrise" });
    expect(link).toHaveAttribute("href", "https://news.example/a");
    expect(link).toHaveAttribute("target", "_blank");
    expect(link).toHaveAttribute("rel", "noopener noreferrer nofollow");
    expect(screen.getAllByRole("link")).toHaveLength(1);
    // The page's texts are user content (web-client/README.md Y10).
    expect(screen.getByText("It rose.")).toHaveAttribute("dir", "auto");
    expect(screen.getByText("It rose.")).toHaveAttribute("lang", "en");
    expect(screen.getByText("Example News")).toHaveAttribute("dir", "auto");
  });

  it("shows the image named by its description, or as decoration without one", () => {
    const { rerender } = render(
      <LinkCard href="https://a.example/" title="A" siteName="a.example" image={image} revealLabel="Show" />,
    );
    expect(screen.getByRole("img", { name: "A sunrise" })).toHaveAttribute("src", "/card.png");
    rerender(
      <LinkCard
        href="https://a.example/"
        title="A"
        siteName="a.example"
        image={{ ...image, alt: undefined }}
        revealLabel="Show"
      />,
    );
    expect(screen.queryByRole("img")).not.toBeInTheDocument();
  });

  it("conceals a sensitive image until the toggle is pressed", async () => {
    const user = userEvent.setup();
    render(
      <LinkCard
        href="https://a.example/"
        title="A"
        siteName="a.example"
        image={image}
        sensitive
        revealLabel="Show sensitive media"
      />,
    );
    expect(screen.queryByRole("img")).not.toBeInTheDocument();
    const toggle = screen.getByRole("button", { name: "Show sensitive media" });
    await user.click(toggle);
    expect(toggle).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByRole("img", { name: "A sunrise" })).toBeInTheDocument();
  });
});
