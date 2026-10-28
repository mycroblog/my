// SPDX-License-Identifier: AGPL-3.0-only
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { AnimationsPlayContext, AutoplayContext, type ImageSources, Picture } from "./Picture";

const still = [
  { url: "/s.webp", width: 96 },
  { url: "/m.webp", width: 192 },
  { url: "/l.jpg", width: 400 },
];
const animated = [
  { url: "/s-a.webp", width: 96 },
  { url: "/m-a.webp", width: 192 },
  { url: "/l-a.gif", width: 400 },
];

describe("Picture", () => {
  it("offers the still renditions by width", () => {
    render(<Picture sources={{ still }} sizes="48px" alt="A cat" />);
    const img = screen.getByRole("img", { name: "A cat" });
    expect(img).toHaveAttribute("src", "/s.webp");
    expect(img).toHaveAttribute("srcset", "/s.webp 96w, /m.webp 192w, /l.jpg 400w");
    expect(img).toHaveAttribute("sizes", "48px");
    // eslint-disable-next-line testing-library/no-node-access -- <picture> and <source> have no role.
    expect(img.closest("picture")).toBeNull();
  });

  it("loads the first rendition when a width is unknown, as for a remote file", () => {
    const sources: ImageSources = {
      still: [{ url: "/proxy/small" }, { url: "/proxy/full", width: 640 }, { url: "/proxy/full", width: 640 }],
    };
    render(<Picture sources={sources} sizes="48px" alt="" />);
    const img = screen.getByRole("presentation");
    expect(img).toHaveAttribute("src", "/proxy/small");
    expect(img).not.toHaveAttribute("srcset");
  });

  it("moves an animation only without a reduced-motion preference when animations play by themselves", () => {
    render(<Picture sources={{ still, animated }} sizes="48px" alt="A dance" />);
    // eslint-disable-next-line testing-library/no-node-access -- <picture> and <source> have no role.
    const source = screen.getByRole("img", { name: "A dance" }).closest("picture")?.querySelector("source");
    expect(source).toHaveAttribute("media", "(prefers-reduced-motion: no-preference)");
    expect(source).toHaveAttribute("srcset", "/s-a.webp 96w, /m-a.webp 192w, /l-a.gif 400w");
    expect(screen.getByRole("img", { name: "A dance" })).toHaveAttribute("src", "/s.webp");
  });

  it("shows the still rendition until hovered when animations do not play by themselves", async () => {
    const user = userEvent.setup();
    render(
      <AutoplayContext value={false}>
        <Picture sources={{ still, animated }} sizes="48px" alt="A dance" />
      </AutoplayContext>,
    );
    const img = screen.getByRole("img", { name: "A dance" });
    // eslint-disable-next-line testing-library/no-node-access -- <picture> and <source> have no role.
    const source = () => img.closest("picture")?.querySelector("source");
    expect(source()).toBeNull();
    expect(img).toHaveAttribute("srcset", "/s.webp 96w, /m.webp 192w, /l.jpg 400w");
    await user.hover(img);
    // The moving rendition comes in behind the reduced-motion query, and the <img> stays the same element.
    expect(source()).toHaveAttribute("media", "(prefers-reduced-motion: no-preference)");
    expect(source()).toHaveAttribute("srcset", "/s-a.webp 96w, /m-a.webp 192w, /l-a.gif 400w");
    expect(screen.getByRole("img", { name: "A dance" })).toBe(img);
    await user.unhover(img);
    expect(source()).toBeNull();
  });

  it("moves while the scope it is in plays", () => {
    render(
      <AutoplayContext value={false}>
        <AnimationsPlayContext value>
          <Picture sources={{ still, animated }} sizes="48px" alt="A dance" />
        </AnimationsPlayContext>
      </AutoplayContext>,
    );
    // eslint-disable-next-line testing-library/no-node-access -- <picture> and <source> have no role.
    const source = screen.getByRole("img", { name: "A dance" }).closest("picture")?.querySelector("source");
    expect(source).toHaveAttribute("media", "(prefers-reduced-motion: no-preference)");
  });
});
