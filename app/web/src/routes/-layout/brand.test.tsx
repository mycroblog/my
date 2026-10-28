// SPDX-License-Identifier: AGPL-3.0-only
// The server's own logo and icon in the app (ADR-0116): the logo, which includes the server's name, takes the place of
// the wordmark and the name at the start of the navigation, an image named by the server's name, with its dark variant
// for dark grounds; the icon takes the place of the built-in mark on the landing page's calling card. Without them the
// built-in wordmark and the name show. Through the whole app (web-client/README.md T4), signed out.
import { screen, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test, vi } from "vitest";
import { instanceFixture, type Schemas } from "../../test/fixtures";
import { apiBase } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";

function brandImage(kind: string, width: number, height: number): Schemas["InstanceBrandImage"] {
  return {
    revision: "0199f0e0-0000-7000-8000-00000000b001",
    svg_url: `https://my.example/brand/0199f0e0-0000-7000-8000-00000000b001/${kind}.svg`,
    width,
    height,
    renditions: [],
  };
}

function serveInstance(overrides: Partial<Schemas["Instance"]>) {
  server.use(http.get(`${apiBase}/instance`, () => HttpResponse.json({ ...instanceFixture, ...overrides })));
}

/** The `src` of the images in `root`, whatever their role (the decorative ones have none). */
function imageSources(root: HTMLElement): string[] {
  // Decorative images have no role to find them by.
  return [...root.querySelectorAll("img")].map((img) => img.getAttribute("src") ?? "");
}

beforeEach(() => {
  // The landing page's progress line marks the section in view; jsdom has no layout.
  Element.prototype.scrollIntoView = vi.fn();
});

describe("the server's own logo and icon", () => {
  test("show at the start of the navigation and on the landing page's card", async () => {
    serveInstance({
      icon: brandImage("icon", 64, 64),
      logo: brandImage("logo", 160, 40),
      logo_dark: brandImage("logo_dark", 160, 40),
    });
    await renderApp({ path: "/" });
    await screen.findByRole("heading", { level: 1, name: "my example" });
    const navigation = screen.getByRole("navigation", { name: "Main" });

    // The link to the top page is the logo, named by the server's name; the light and the dark variant are both there
    // (CSS shows the one that fits the ground).
    const link = await within(navigation).findByRole("link", { name: "my example" });
    expect(imageSources(link)).toEqual([
      "https://my.example/brand/0199f0e0-0000-7000-8000-00000000b001/logo.svg",
      "https://my.example/brand/0199f0e0-0000-7000-8000-00000000b001/logo_dark.svg",
    ]);
    // No built-in wordmark beside it, and no name as text.
    expect(imageSources(navigation).filter((src) => !src.includes("/brand/"))).toEqual([]);
    expect(within(navigation).queryByText("my example")).not.toBeInTheDocument();

    // The calling card's mark is the icon.
    expect(imageSources(screen.getByRole("main"))).toContain(
      "https://my.example/brand/0199f0e0-0000-7000-8000-00000000b001/icon.svg",
    );
  });

  test("leave the built-in wordmark and the name while the server has none", async () => {
    serveInstance({});
    await renderApp({ path: "/" });
    await screen.findByRole("heading", { level: 1, name: "my example" });
    const navigation = screen.getByRole("navigation", { name: "Main" });

    const link = within(navigation).getByRole("link", { name: "my example" });
    expect(link).toHaveTextContent("my example");
    expect(imageSources(navigation).some((src) => src.includes("/brand/"))).toBe(false);
    expect(imageSources(screen.getByRole("main")).some((src) => src.includes("/brand/"))).toBe(false);
  });
});
