// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { I18nProvider } from "react-aria-components";
import { LinkCard, type LinkCardProps } from "./card";

const page: LinkCardProps = {
  href: "https://blog.example/posts/rye",
  title: "Baking rye at home",
  description: "What three weeks of starters taught me.",
  site: "Field Notes",
  image: {
    src: "https://my.example/proxy/rye.webp",
    width: 1200,
    height: 600,
    alt: "A dark loaf on a board",
    blurhash: "LEHV6nWB2yk8pyo0adR*.7kCMdnj",
  },
  author: { shown: <b>Ann Lee</b>, name: "Ann Lee" },
};

const { alt: _, ...undescribed } = page.image ?? { src: "", width: 0, height: 0 };

const inLocale = (locale: string, node: ReactNode) => render(<I18nProvider locale={locale}>{node}</I18nProvider>);

/** Every address the document would have the browser fetch through the media proxy. */
const asked = () =>
  Array.from(document.querySelectorAll("*"))
    .flatMap((element) => element.getAttributeNames().map((name) => element.getAttribute(name) ?? ""))
    .filter((value) => value.includes("https://my.example/proxy/"));

describe("the link card", () => {
  test("it is one link to the page, in a new tab, that never passes on rank or the reader's page", () => {
    render(<LinkCard {...page} />);

    const links = screen.getAllByRole<HTMLAnchorElement>("link");
    expect(links).toHaveLength(1);
    expect(links[0]?.getAttribute("href")).toBe("https://blog.example/posts/rye");
    expect(links[0]?.target).toBe("_blank");
    expect(new Set(links[0]?.rel.split(" "))).toEqual(new Set(["noopener", "noreferrer", "nofollow"]));
  });

  test("it is named by its image, the site, the title, the description, by the author, and that it opens a new tab", () => {
    render(<LinkCard {...page} />);

    expect(
      screen.getByRole("link", {
        name: "A dark loaf on a board Field Notes Baking rye at home What three weeks of starters taught me. by Ann Lee (opens in a new tab)",
      }),
    ).not.toBeNull();
  });

  test("the author shows as given, once, and reads as by and the name in the reader's locale", () => {
    const { unmount } = render(<LinkCard {...page} image={undefined} description="" />);
    expect(screen.getByText("Ann Lee").textContent).toBe("Ann Lee");
    expect(
      screen.getByRole("link", { name: "Field Notes Baking rye at home by Ann Lee (opens in a new tab)" }),
    ).not.toBeNull();
    unmount();

    inLocale("ja", <LinkCard {...page} image={undefined} description="" />);
    expect(screen.getByRole("link", { name: /^Field Notes Baking rye at home 著者: Ann Lee/ })).not.toBeNull();
  });

  test("its image comes from where it is given with its own description, and says nothing without one", () => {
    const { unmount } = render(<LinkCard {...page} />);
    expect(screen.getByRole("img", { name: "A dark loaf on a board" }).getAttribute("src")).toBe(
      "https://my.example/proxy/rye.webp",
    );
    unmount();

    render(<LinkCard {...page} image={undescribed} />);
    expect(screen.queryByRole("img")).toBeNull();
    expect(screen.getByRole("presentation").getAttribute("src")).toBe("https://my.example/proxy/rye.webp");
    expect(screen.getByRole("link", { name: /^Field Notes Baking rye at home/ })).not.toBeNull();
  });

  test("small, and without an image, a description or an author, it is the same link", () => {
    const { unmount } = render(<LinkCard {...page} size="small" />);
    expect(screen.getByRole("link", { name: /Field Notes Baking rye at home/ }).getAttribute("href")).toBe(page.href);
    expect(screen.getByRole("img", { name: "A dark loaf on a board" })).not.toBeNull();
    unmount();

    render(<LinkCard {...page} image={undefined} description="" author={undefined} />);
    expect(
      screen.getByRole("link", { name: "Field Notes Baking rye at home (opens in a new tab)" }).getAttribute("href"),
    ).toBe(page.href);
    expect(screen.queryByRole("img")).toBeNull();
    expect(screen.queryByRole("presentation")).toBeNull();
  });

  test("the language of the page marks its words", () => {
    render(<LinkCard {...page} lang="de" />);

    expect(screen.getByText("Baking rye at home").closest("[lang]")?.getAttribute("lang")).toBe("de");
  });
});

describe("a sensitive card", () => {
  test("its image is covered: no image is read and none is asked for", () => {
    render(<LinkCard {...page} sensitive />);

    expect(screen.queryByRole("img")).toBeNull();
    expect(screen.queryByRole("presentation")).toBeNull();
    expect(asked()).toEqual([]);
    expect(screen.getByRole("link", { name: /^Field Notes Baking rye at home/ })).not.toBeNull();
  });

  test("the line that shows it is outside the link and before it, and Show shows the image and becomes Hide", async () => {
    const user = userEvent.setup();
    render(<LinkCard {...page} sensitive />);
    const link = screen.getByRole("link");
    const show = screen.getByRole("button", { name: "Show", description: "Sensitive image", expanded: false });
    expect(link.contains(show)).toBe(false);
    expect(show.getAttribute("aria-controls")).toBe(link.id);

    await user.tab();
    expect(document.activeElement).toBe(show);
    await user.keyboard("{Enter}");

    expect(screen.getByRole("button", { name: "Hide", description: "Sensitive image", expanded: true })).toBe(show);
    expect(document.activeElement).toBe(show);
    expect(screen.getByRole("img", { name: "A dark loaf on a board" }).getAttribute("src")).toBe(
      "https://my.example/proxy/rye.webp",
    );
    await user.tab();
    expect(document.activeElement).toBe(link);
  });

  test("the line reads in the reader's locale", () => {
    inLocale("ja", <LinkCard {...page} sensitive />);

    expect(
      screen.getByRole("button", { name: "表示", description: "センシティブな画像", expanded: false }),
    ).not.toBeNull();
  });

  test("a sensitive card without an image has nothing to cover", () => {
    render(<LinkCard {...page} image={undefined} sensitive />);

    expect(screen.queryByRole("button")).toBeNull();
    expect(screen.getByRole("link", { name: /^Field Notes Baking rye at home/ })).not.toBeNull();
  });
});
