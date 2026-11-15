// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import type { ReactNode } from "react";
import { I18nProvider } from "react-aria-components";
import { renderToStaticMarkup } from "react-dom/server";
import { AudioRow, FileLink, MediaGrid, playTime, type MediaItem } from "./media";

const items = (n: number): MediaItem[] =>
  Array.from({ length: n }, (_, i) => ({ id: `m${i}`, src: `https://m.example/${i}.webp`, alt: `Image ${i}` }));

test("no images draw nothing", () => {
  expect(renderToStaticMarkup(<MediaGrid items={[]} />)).toBe("");
});

test("one image alone, two side by side, three or four in two rows, in a box 16:9 of its width up to a height", () => {
  const grid = (n: number) => renderToStaticMarkup(<MediaGrid items={items(n)} />);
  expect(grid(1)).toContain("grid-cols-[minmax(0,1fr)] grid-rows-[minmax(0,1fr)]");
  expect(grid(2)).toContain("grid-cols-[minmax(0,1fr)_minmax(0,1fr)] grid-rows-[minmax(0,1fr)]");
  expect(grid(4)).toContain("grid-cols-[minmax(0,1fr)_minmax(0,1fr)] grid-rows-[minmax(0,1fr)_minmax(0,1fr)]");
  expect(grid(1)).toContain("h-[min(calc(100cqw*var(--media-ratio)),var(--media-max-height))]");
  expect(grid(1)).toContain('<div class="@container"><div class="grid');
});

test("each image has its description as its text, and no border", () => {
  const html = renderToStaticMarkup(<MediaGrid items={items(2)} />);
  expect(html).toContain('src="https://m.example/1.webp" alt="Image 1"');
  expect(html).not.toContain("border");
});

test("as a preview, each image is a small tile in a row", () => {
  const html = renderToStaticMarkup(<MediaGrid items={items(2)} size="preview" />);
  expect(html).toContain('<div class="flex flex-wrap gap-media"><span');
  expect(html).toContain("h-media-preview-height w-media-preview-width");
  expect(html).not.toContain("@container");
});

/** The markup without classes and glyphs, to read the structure. */
const bare = (node: ReactNode, locale = "en") =>
  renderToStaticMarkup(<I18nProvider locale={locale}>{node}</I18nProvider>)
    .replace(/ class="[^"]*"/g, "")
    .replace(/<svg .*?<\/svg>/g, "");

const open = () => undefined;
const picture: MediaItem = {
  id: "p",
  src: "https://m.example/p-small.webp",
  alt: "A ridge at dawn",
  href: "https://m.example/p.webp",
  blurhash: "LEHV6nWB2yk8pyo0adR*.7kCMdnj",
};
const video: MediaItem = {
  id: "v",
  kind: "video",
  src: "https://m.example/v-poster.webp",
  alt: "Waves at dusk",
  href: "https://m.example/v.webm",
  duration: 92,
  blurhash: "LEHV6nWB2yk8pyo0adR*.7kCMdnj",
};
const gif: MediaItem = {
  id: "g",
  kind: "gifv",
  src: "https://m.example/g-still.webp",
  alt: "",
  href: "https://m.example/g.mp4",
  blurhash: "LEHV6nWB2yk8pyo0adR*.7kCMdnj",
};
const sound: MediaItem = {
  id: "s",
  kind: "audio",
  src: "",
  alt: "Rain on a tin roof",
  href: "https://m.example/s.mp3",
  duration: 192,
};
const file: MediaItem = { id: "f", kind: "unknown", src: "", alt: "", href: "https://files.example.org/slides.key" };

describe("each kind of media", () => {
  test("an image is a button that opens it, named by its kind and its description, with the ALT mark", () => {
    const html = bare(<MediaGrid items={[picture]} onOpen={open} />);
    expect(html).toContain('<img src="https://m.example/p-small.webp" alt="A ridge at dawn"/>');
    expect(html).toMatch(/<button [^>]*aria-label="Open image: A ridge at dawn"/);
    expect(html).toContain('<span aria-hidden="true"><span>ALT</span></span>');
    const undescribed = bare(<MediaGrid items={[{ ...picture, alt: "" }]} onOpen={open} />);
    expect(undescribed).toMatch(/<button [^>]*aria-label="Open image"/);
    expect(undescribed).not.toContain(">ALT<");
  });

  test("a GIF loops silently in place from its file, a button that opens it, with the GIF mark", () => {
    const html = bare(<MediaGrid items={[gif]} onOpen={open} />);
    expect(html).toMatch(
      /<video src="https:\/\/m.example\/g.mp4" poster="https:\/\/m.example\/g-still.webp" muted="" loop=""/,
    );
    expect(html).toMatch(/<button [^>]*aria-label="Open GIF"/);
    expect(html).toContain("<span>GIF</span>");
    expect(bare(<MediaGrid items={[{ ...gif, alt: "Gulls" }]} onOpen={open} />)).toMatch(
      /aria-label="Open GIF: Gulls"/,
    );
  });

  test("a video shows its poster, plays in place from a button named by its description, and opens whole", () => {
    const html = bare(<MediaGrid items={[video]} onOpen={open} />);
    expect(html).toContain('<img src="https://m.example/v-poster.webp" alt="Waves at dusk"/>');
    expect(html).toMatch(/<button [^>]*aria-label="Play video: Waves at dusk"/);
    expect(html).toMatch(/<button [^>]*aria-label="Open video"/);
    expect(html).toContain("<span>1:32</span>");
    expect(html).not.toContain("https://m.example/v.webm");
    expect(bare(<MediaGrid items={[{ ...video, alt: "" }]} onOpen={open} />)).toMatch(/aria-label="Play video"/);
  });

  test("a sound is a row: Play audio, its description, and its time read as 0:00 of 3:12; nothing loads before play", () => {
    const html = bare(<MediaGrid items={[sound]} />);
    expect(html).toMatch(/<button [^>]*aria-label="Play audio"/);
    expect(html).not.toContain("aria-pressed");
    expect(html).toContain("Rain on a tin roof");
    expect(html).toContain('<span aria-hidden="true">0:00 / 3:12</span>');
    expect(html).toContain("<span>0:00 of 3:12</span>");
    expect(html).toContain('<audio src="https://m.example/s.mp3" preload="none">');
    expect(bare(<AudioRow item={{ ...sound, alt: "" }} />)).toContain("<span>Audio</span>");
  });

  test("a file of an unknown kind is a link that opens in a new tab, by its file's name, with its host", () => {
    const html = bare(<MediaGrid items={[file]} />);
    expect(html).toMatch(
      /<a [^>]*href="https:\/\/files.example.org\/slides.key" target="_blank" rel="noopener noreferrer"/,
    );
    expect(html).toContain("<span>slides.key</span>");
    expect(
      bare(<FileLink item={{ ...file, href: "https://files.example.org/a/%E8%B3%87%E6%96%99.pdf/" }} />),
    ).toContain("<span>資料.pdf</span>");
    expect(bare(<FileLink item={{ ...file, href: "https://files.example.org/" }} />)).toContain(
      "<span>Open attached file</span>",
    );
    expect(html).toContain('<bdi dir="ltr">files.example.org</bdi>');
    expect(html).toContain("<span> (opens in a new tab)</span>");
    expect(bare(<FileLink item={{ ...file, alt: "Slides of the talk" }} />)).toContain("Slides of the talk");
  });

  test("the names read in the reader's locale", () => {
    const unnamed = { ...file, href: "https://files.example.org/" };
    const html = bare(<MediaGrid items={[picture, video, sound, unnamed]} onOpen={open} />, "ja");
    expect(html).toContain('aria-label="画像を開く: A ridge at dawn"');
    expect(html).toContain('aria-label="動画を再生: Waves at dusk"');
    expect(html).toContain('aria-label="音声を再生"');
    expect(html).toContain("<span>3:12 中 0:00</span>");
    expect(html).toContain("添付ファイルを開く");
  });

  test("a length reads as minutes and seconds, with hours when it has them", () => {
    expect(playTime(7)).toBe("0:07");
    expect(playTime(192)).toBe("3:12");
    expect(playTime(3725)).toBe("1:02:05");
  });
});

describe("sensitive media", () => {
  const many = (item: MediaItem, n: number) => Array.from({ length: n }, (_, i) => ({ ...item, id: `${item.id}${i}` }));
  const line = (items: MediaItem[], locale?: string) =>
    /<span id="([^"]+)">(.*?)<\/span><button (?=[^>]*aria-expanded="false")(?=[^>]*aria-controls="[^"]+")(?=[^>]*aria-describedby="\1")[^>]*>.*?<span lang="[^"]+">(.*?)<\/span>/
      .exec(bare(<MediaGrid items={items} sensitive onOpen={open} />, locale))
      ?.slice(2, 4);

  test("one line above the media names them by kind and count, then Show", () => {
    expect(line([picture])).toEqual(["Sensitive image", "Show"]);
    expect(line(many(picture, 4))).toEqual(["4 sensitive images", "Show"]);
    expect(line([video])).toEqual(["Sensitive video", "Show"]);
    expect(line(many(gif, 2))).toEqual(["2 sensitive GIFs", "Show"]);
    expect(line([sound])).toEqual(["Sensitive audio", "Show"]);
    expect(line(many(file, 3))).toEqual(["3 sensitive files", "Show"]);
    expect(line([picture, gif, sound])).toEqual(["3 sensitive attachments", "Show"]);
  });

  test("the line reads in the reader's locale", () => {
    expect(line([picture], "ja")).toEqual(["センシティブな画像", "表示"]);
    expect(line(many(picture, 4), "ja")).toEqual(["センシティブな画像 4 枚", "表示"]);
  });

  test("covered, each picture's blurhash is drawn and no file is asked for", () => {
    const html = bare(<MediaGrid items={[picture, gif, video]} sensitive onOpen={open} />);
    expect(html.match(/<canvas [^>]*aria-hidden="true"><\/canvas>/g)).toHaveLength(3);
    expect(html).not.toContain("<img");
    expect(html).not.toContain("<video");
    expect(html).not.toContain("https://m.example/");
    expect(html).not.toContain('rel="preload"');
  });

  test("the line controls the covered media, and the covered media is a press that shows them, out of the tab order and hidden from a screen reader", () => {
    const html = bare(<MediaGrid items={many(picture, 2)} sensitive onOpen={open} />);
    const controls = /aria-controls="([^"]+)"/.exec(html)?.[1];
    expect(controls).toBeDefined();
    expect(html).toContain(`<div id="${controls ?? ""}">`);
    expect(html).toMatch(/<div [^>]*aria-hidden="true"[^>]*><button [^>]*tabindex="-1"/);
  });

  test("as a quoted post's previews, sensitive images are blurred too", () => {
    const html = bare(<MediaGrid items={many(picture, 2)} size="preview" sensitive />);
    expect(html).not.toContain("https://m.example/");
    expect(html.match(/<canvas /g)).toHaveLength(2);
  });
});
