// SPDX-License-Identifier: AGPL-3.0-only
import { render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { describe, expect, it } from "vitest";
import { ExternalLink, ExternalLinkContext, externalRel } from "./ExternalLink";

/**
 * The name of a link followed by the hidden "(opens in a new tab)". jsdom lays nothing out, so the name it computes
 * has no space between the two; a browser's has one, as the hidden text is a block (`sr-only` is absolute).
 */
function newTab(name: string): RegExp {
  return new RegExp(`^${name.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")} ?\\(opens in a new tab\\)$`);
}

function withLabel(ui: ReactNode) {
  return (
    <ExternalLinkContext value={{ newTabLabel: "(opens in a new tab)", isAppLink: () => false }}>
      {ui}
    </ExternalLinkContext>
  );
}

describe("ExternalLink", () => {
  it("opens in a new tab without the opener or the referrer, and says so to assistive technology", () => {
    render(withLabel(<ExternalLink href="https://remote.example/@carol">Carol on remote.example</ExternalLink>));
    const link = screen.getByRole("link", { name: newTab("Carol on remote.example") });
    expect(link).toHaveAttribute("href", "https://remote.example/@carol");
    expect(link).toHaveAttribute("target", "_blank");
    expect(link).toHaveAttribute("rel", "noopener noreferrer");
    // eslint-disable-next-line testing-library/no-node-access -- a decorative icon has no role.
    expect(link.querySelector("svg")).toHaveAttribute("aria-hidden", "true");
  });

  it("keeps the tokens of rel and can leave out the icon", () => {
    render(
      withLabel(
        <ExternalLink href="https://news.example/" rel="nofollow ugc" icon={false}>
          News
        </ExternalLink>,
      ),
    );
    const link = screen.getByRole("link", { name: newTab("News") });
    expect(link).toHaveAttribute("rel", "noopener noreferrer nofollow ugc");
    // eslint-disable-next-line testing-library/no-node-access -- a decorative icon has no role.
    expect(link.querySelector("svg")).toBeNull();
  });

  it("ends a name given by aria-label with the text", () => {
    render(
      withLabel(
        <ExternalLink href="https://remote.example/x" aria-label="Open the original post on remote.example">
          remote.example
        </ExternalLink>,
      ),
    );
    expect(
      screen.getByRole("link", { name: "Open the original post on remote.example (opens in a new tab)" }),
    ).toBeInTheDocument();
  });

  it("still opens in a new tab without the app's text", () => {
    render(<ExternalLink href="https://remote.example/">remote.example</ExternalLink>);
    expect(screen.getByRole("link", { name: "remote.example" })).toHaveAttribute("target", "_blank");
  });
});

describe("externalRel", () => {
  it("adds noopener and noreferrer once to the given tokens", () => {
    expect(externalRel()).toBe("noopener noreferrer");
    expect(externalRel("nofollow noopener noreferrer")).toBe("noopener noreferrer nofollow");
    expect(externalRel(" NoFollow  ugc ")).toBe("noopener noreferrer nofollow ugc");
  });
});
