// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import type { ReactNode } from "react";
import { I18nProvider } from "react-aria-components";
import { renderToStaticMarkup } from "react-dom/server";
import type { MediaItem } from "./media";
import { MediaViewer } from "./viewer";

/** The markup without classes and glyphs, to read the structure. */
const bare = (node: ReactNode, locale = "en") =>
  renderToStaticMarkup(<I18nProvider locale={locale}>{node}</I18nProvider>)
    .replace(/ class="[^"]*"/g, "")
    .replace(/<svg .*?<\/svg>/g, "");

const items: MediaItem[] = [
  { id: "a", src: "https://m.example/a-small.webp", href: "https://m.example/a.webp", alt: "A ridge at dawn" },
  { id: "b", src: "https://m.example/b-small.webp", href: "https://m.example/b.webp", alt: "" },
  {
    id: "v",
    kind: "video",
    src: "https://m.example/v-poster.webp",
    href: "https://m.example/v.webm",
    alt: "Waves at dusk",
    duration: 92,
  },
];

const at = (index: number, media = items, locale?: string) =>
  bare(
    <MediaViewer items={media} index={index} onIndexChange={() => undefined} onClose={() => undefined} layer={false} />,
    locale,
  );

describe("the media viewer", () => {
  test("a dialog named Media that says where it is among the post's media", () => {
    const html = at(0);
    expect(html).toMatch(/^(<link [^>]*\/>)?<div role="dialog" aria-label="Media">/);
    expect(html).toContain("<span>1 of 3</span>");
    expect(at(1)).toContain("<span>2 of 3</span>");
  });

  test("the picture whole, from its largest file, its description read once, as a caption", () => {
    const html = at(0);
    expect(html).toMatch(/<img src="https:\/\/m.example\/a.webp" alt=""/);
    expect(html).toContain(">A ridge at dawn</p>");
    expect(html.match(/A ridge at dawn/g)).toHaveLength(1);
    expect(at(1)).not.toContain("</p>");
  });

  test("an image zooms; a video plays with its controls and has no zoom", () => {
    expect(at(0)).toMatch(/<button [^>]*aria-label="Zoom in"/);
    const video = at(2);
    expect(video).toMatch(/<video [^>]*src="https:\/\/m.example\/v.webm"[^>]*controls=""/);
    expect(video).not.toContain('aria-label="Zoom in"');
    expect(video).toContain(">Waves at dusk</p>");
  });

  test("Close always; Previous and Next where there is one", () => {
    const first = at(0);
    expect(first).toMatch(/aria-label="Close"/);
    expect(first).toMatch(/aria-label="Next"/);
    expect(first).not.toMatch(/aria-label="Previous"/);
    expect(at(1)).toMatch(/aria-label="Previous".*aria-label="Next"/);
    expect(at(2)).not.toMatch(/aria-label="Next"/);
  });

  test("a single picture says no position and has nowhere to move", () => {
    const html = at(0, items.slice(0, 1));
    expect(html).not.toContain(" of ");
    expect(html).not.toMatch(/aria-label="(Previous|Next)"/);
  });

  test("it announces nothing as it opens: a move is what it announces", () => {
    expect(at(1)).toContain('<span aria-live="polite"></span>');
  });

  test("it reads in the reader's locale", () => {
    const html = at(1, items, "ja");
    expect(html).toContain('<div role="dialog" aria-label="メディア">');
    expect(html).toContain("<span>3 件中 2 件目</span>");
    expect(html).toMatch(/aria-label="閉じる".*aria-label="前へ".*aria-label="次へ"/s);
  });
});
