// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { I18nProvider } from "react-aria-components";
import { Button } from "./button";
import { AudioRow, FileLink, MediaGrid, SensitiveLine, type MediaItem } from "./media";

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

const many = (item: MediaItem, n: number) =>
  Array.from({ length: n }, (_, i) => ({ ...item, id: `${item.id}${String(i)}` }));

const inLocale = (locale: string, node: ReactNode) => render(<I18nProvider locale={locale}>{node}</I18nProvider>);

/** A video or a sound has no role to find it by: the players of the document, in its order. */
const players = (kind: "video" | "audio") => Array.from(document.getElementsByTagName(kind));

/** Every address the document would have the browser fetch from the media's server. */
const asked = () =>
  Array.from(document.querySelectorAll("*"))
    .flatMap((element) => element.getAttributeNames().map((name) => element.getAttribute(name) ?? ""))
    .filter((value) => value.includes("https://m.example/"));

test("no media draw nothing", () => {
  const { container } = render(<MediaGrid items={[]} />);

  expect(container.childElementCount).toBe(0);
});

describe("each kind of media", () => {
  test("each image has its description as its alternative text, and is no button where nothing opens it", () => {
    render(
      <MediaGrid
        items={[
          { id: "a", src: "https://m.example/a.webp", alt: "A ridge at dawn" },
          { id: "b", src: "https://m.example/b.webp", alt: "A harbour in fog" },
        ]}
      />,
    );

    expect(screen.getByRole("img", { name: "A ridge at dawn" }).getAttribute("src")).toBe("https://m.example/a.webp");
    expect(screen.getByRole("img", { name: "A harbour in fog" }).getAttribute("src")).toBe("https://m.example/b.webp");
    expect(screen.queryByRole("button")).toBeNull();
  });

  test("an image is a button that opens it, named by its kind and its description, with the ALT mark", async () => {
    const user = userEvent.setup();
    const opened: number[] = [];
    render(
      <MediaGrid
        items={[picture, { ...picture, id: "undescribed", alt: "" }]}
        onOpen={(index) => opened.push(index)}
      />,
    );

    expect(screen.getByRole("img", { name: "A ridge at dawn" }).getAttribute("src")).toBe(picture.src);
    // The mark shows that there is a description; the one image without one has none.
    expect(screen.getAllByText("ALT")).toHaveLength(1);
    await user.tab();
    expect(document.activeElement).toBe(screen.getByRole("button", { name: "Open image: A ridge at dawn" }));
    await user.keyboard("{Enter}");
    await user.tab();
    expect(document.activeElement).toBe(screen.getByRole("button", { name: "Open image" }));
    await user.keyboard(" ");

    expect(opened).toEqual([0, 1]);
  });

  test("a GIF loops silently in place from its file, a button that opens it, with the GIF mark", async () => {
    const user = userEvent.setup();
    const opened: number[] = [];
    render(
      <MediaGrid items={[gif, { ...gif, id: "described", alt: "Gulls" }]} onOpen={(index) => opened.push(index)} />,
    );

    const [first, second] = players("video");
    expect(first?.getAttribute("src")).toBe("https://m.example/g.mp4");
    expect(first?.getAttribute("poster")).toBe("https://m.example/g-still.webp");
    expect(["muted", "loop", "autoplay", "controls"].map((name) => first?.hasAttribute(name))).toEqual([
      true,
      true,
      true,
      false,
    ]);
    expect(second?.getAttribute("aria-label")).toBe("Gulls");
    expect(screen.getAllByText("GIF")).toHaveLength(2);
    await user.click(screen.getByRole("button", { name: "Open GIF" }));
    await user.click(screen.getByRole("button", { name: "Open GIF: Gulls" }));

    expect(opened).toEqual([0, 1]);
  });

  test("a video shows its poster and its length, and loads nothing of its file before it plays", () => {
    render(<MediaGrid items={[video, { ...video, id: "undescribed", alt: "" }]} onOpen={() => undefined} />);

    expect(screen.getByRole("img", { name: "Waves at dusk" }).getAttribute("src")).toBe(video.src);
    expect(screen.getByRole("button", { name: "Play video: Waves at dusk" })).not.toBeNull();
    expect(screen.getByRole("button", { name: "Play video" })).not.toBeNull();
    expect(screen.getAllByText("1:32")).toHaveLength(2);
    expect(players("video")).toEqual([]);
    expect(asked()).not.toContain("https://m.example/v.webm");
  });

  test("a press on a video's play button plays it in place, with the player's controls, named by its description", async () => {
    const user = userEvent.setup();
    render(<MediaGrid items={[video]} onOpen={() => undefined} />);

    await user.click(screen.getByRole("button", { name: "Play video: Waves at dusk" }));

    const [player] = players("video");
    expect(player?.getAttribute("src")).toBe("https://m.example/v.webm");
    expect(player?.getAttribute("aria-label")).toBe("Waves at dusk");
    expect(["controls", "autoplay"].map((name) => player?.hasAttribute(name))).toEqual([true, true]);
    expect(screen.queryByRole("button", { name: /^Play video/ })).toBeNull();
  });

  test("Enter on a video's play button leaves the focus in the video's place", async () => {
    const user = userEvent.setup();
    render(
      <>
        <Button>Before</Button>
        <MediaGrid items={[video]} onOpen={() => undefined} />
      </>,
    );
    await user.tab();
    await user.tab();
    expect(document.activeElement).toBe(screen.getByRole("button", { name: "Play video: Waves at dusk" }));

    await user.keyboard("{Enter}");

    expect(players("video")).toHaveLength(1);
    expect(document.activeElement === document.body).toBe(false);
  });

  test("a video opens whole from its own button, by its place among the pictures", async () => {
    const user = userEvent.setup();
    const opened: number[] = [];
    render(<MediaGrid items={[picture, sound, video]} onOpen={(index) => opened.push(index)} />);

    await user.click(screen.getByRole("button", { name: "Open video" }));

    // A sound is no picture: the video is the second of them.
    expect(opened).toEqual([1]);
  });

  test("a sound is a row: its play button, its description, and its time read in words; nothing loads before it plays", () => {
    render(<MediaGrid items={[sound]} />);

    const play = screen.getByRole("button", { name: "Play audio" });
    // It says what a press does by its name, not by a state.
    expect(play.hasAttribute("aria-pressed")).toBe(false);
    expect(screen.getByText("Rain on a tin roof")).not.toBeNull();
    expect(screen.getByText("0:00 of 3:12")).not.toBeNull();
    const [player] = players("audio");
    expect(player?.getAttribute("src")).toBe("https://m.example/s.mp3");
    expect(player?.preload).toBe("none");
  });

  test("a press on a sound's play button plays it, and the button then pauses it", async () => {
    const user = userEvent.setup();
    render(<AudioRow item={sound} />);

    await user.tab();
    expect(document.activeElement).toBe(screen.getByRole("button", { name: "Play audio" }));
    await user.keyboard("{Enter}");

    expect(document.activeElement).toBe(screen.getByRole("button", { name: "Pause audio" }));
    expect(screen.queryByRole("button", { name: "Play audio" })).toBeNull();
  });

  test("a sound without a description says its kind", () => {
    render(<AudioRow item={{ ...sound, alt: "" }} />);

    expect(screen.getByText("Audio")).not.toBeNull();
  });

  test("a file of an unknown kind is a link that opens in a new tab and says so, by its file's name, with its host", () => {
    render(<MediaGrid items={[file]} />);

    const link = screen.getByRole<HTMLAnchorElement>("link", {
      name: "slides.key files.example.org (opens in a new tab)",
    });
    expect(link.getAttribute("href")).toBe("https://files.example.org/slides.key");
    expect(link.target).toBe("_blank");
    expect(link.rel.split(" ")).toContain("noopener");
    // A host reads left to right in either direction.
    expect(within(link).getByText("files.example.org").dir).toBe("ltr");
  });

  test("a file is named by its description, else by its file's name as it is written, else as an attached file", () => {
    render(
      <>
        <FileLink item={{ ...file, alt: "Slides of the talk" }} />
        <FileLink item={{ ...file, href: "https://files.example.org/a/%E8%B3%87%E6%96%99.pdf/" }} />
        <FileLink item={{ ...file, href: "https://files.example.org/" }} />
      </>,
    );

    expect(screen.getByRole("link", { name: /^Slides of the talk files\.example\.org/ })).not.toBeNull();
    expect(screen.getByRole("link", { name: /^資料\.pdf files\.example\.org/ })).not.toBeNull();
    expect(screen.getByRole("link", { name: /^Open attached file files\.example\.org/ })).not.toBeNull();
  });

  test("the names read in the reader's locale", () => {
    inLocale(
      "ja",
      <MediaGrid
        items={[picture, video, sound, { ...file, href: "https://files.example.org/" }]}
        onOpen={() => undefined}
      />,
    );

    expect(screen.getByRole("button", { name: "画像を開く: A ridge at dawn" })).not.toBeNull();
    expect(screen.getByRole("button", { name: "動画を再生: Waves at dusk" })).not.toBeNull();
    expect(screen.getByRole("button", { name: "音声を再生" })).not.toBeNull();
    expect(screen.getByText("3:12 中 0:00")).not.toBeNull();
    expect(screen.getByRole("link", { name: /^添付ファイルを開く/ })).not.toBeNull();
  });

  test("a locale of its own names the media, whatever the provider's", () => {
    inLocale("ja", <MediaGrid items={[picture]} onOpen={() => undefined} locale="en" />);

    expect(screen.getByRole("button", { name: "Open image: A ridge at dawn" })).not.toBeNull();
  });
});

describe("sensitive media", () => {
  /** The words of the line over `items` covered, and of its button. */
  function line(items: MediaItem[], locale = "en") {
    const { unmount } = inLocale(locale, <MediaGrid items={items} sensitive onOpen={() => undefined} />);
    const button = screen.getByRole("button", { expanded: false });
    const described = document.getElementById(button.getAttribute("aria-describedby") ?? "");
    const words = [described?.textContent, button.textContent];
    unmount();
    return words;
  }

  test("one line above the media names them by kind and count, then Show, which the line describes", () => {
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

  test("covered, no picture is read and no file is asked for", () => {
    render(<MediaGrid items={[picture, gif, video]} sensitive onOpen={() => undefined} />);

    expect(screen.queryAllByRole("img")).toEqual([]);
    expect(players("video")).toEqual([]);
    expect(asked()).toEqual([]);
    expect(screen.queryByRole("button", { name: /^Open|^Play/ })).toBeNull();
  });

  test("Show shows the media it controls and becomes Hide in the same place, which covers them again", async () => {
    const user = userEvent.setup();
    render(
      <>
        <MediaGrid items={many(picture, 2)} sensitive onOpen={() => undefined} />
        <Button>After</Button>
      </>,
    );
    const show = screen.getByRole("button", { name: "Show", description: "2 sensitive images", expanded: false });
    const covered = document.getElementById(show.getAttribute("aria-controls") ?? "");
    expect(covered).not.toBeNull();

    await user.tab();
    expect(document.activeElement).toBe(show);
    await user.keyboard("{Enter}");

    const hide = screen.getByRole("button", { name: "Hide", description: "2 sensitive images", expanded: true });
    expect(hide).toBe(show);
    expect(document.activeElement).toBe(hide);
    expect(within(covered as HTMLElement).getAllByRole("img", { name: "A ridge at dawn" })).toHaveLength(2);
    expect(within(covered as HTMLElement).getAllByRole("button", { name: "Open image: A ridge at dawn" })).toHaveLength(
      2,
    );

    await user.keyboard(" ");
    expect(screen.getByRole("button", { name: "Show", expanded: false })).toBe(show);
    expect(screen.queryAllByRole("img")).toEqual([]);
    expect(asked()).toEqual([]);
  });

  test("the covered media are a press that shows them, out of the tab order and hidden from a screen reader", async () => {
    const user = userEvent.setup();
    render(
      <>
        <MediaGrid items={many(picture, 2)} sensitive onOpen={() => undefined} />
        <Button>After</Button>
      </>,
    );
    // The line's Show is the one way in for the keyboard and a screen reader.
    expect(screen.getAllByRole("button").map((button) => button.textContent)).toEqual(["Show", "After"]);
    await user.tab();
    await user.tab();
    expect(document.activeElement).toBe(screen.getByRole("button", { name: "After" }));

    await user.click(screen.getByRole("button", { name: "2 sensitive images · Show", hidden: true }));

    expect(screen.getByRole("button", { name: "Hide", expanded: true })).not.toBeNull();
    expect(screen.getAllByRole("img", { name: "A ridge at dawn" })).toHaveLength(2);
  });

  test("a press on the covered media shows them and leaves the focus where it was", async () => {
    const user = userEvent.setup();
    render(
      <>
        <Button>Before</Button>
        <MediaGrid items={many(picture, 2)} sensitive onOpen={() => undefined} />
      </>,
    );
    const cover = () => screen.getByRole("button", { name: "2 sensitive images · Show", hidden: true });
    await user.tab();
    const before = screen.getByRole("button", { name: "Before" });
    expect(document.activeElement).toBe(before);

    await user.click(cover());
    expect(screen.getAllByRole("img", { name: "A ridge at dawn" })).toHaveLength(2);
    expect(document.activeElement).toBe(before);

    // From the line's own button too, which the press turns into Hide.
    await user.tab();
    await user.keyboard("{Enter}");
    const show = screen.getByRole("button", { name: "Show", expanded: false });
    expect(document.activeElement).toBe(show);
    await user.click(cover());
    expect(screen.getByRole("button", { name: "Hide", expanded: true })).toBe(show);
    expect(document.activeElement).toBe(show);
  });

  test("as a quoted post's previews, each image has its description, and sensitive ones are covered too", () => {
    const { unmount } = render(<MediaGrid items={many(picture, 2)} size="preview" />);
    expect(screen.getAllByRole("img", { name: "A ridge at dawn" })).toHaveLength(2);
    unmount();

    render(<MediaGrid items={many(picture, 2)} size="preview" sensitive />);
    expect(screen.queryAllByRole("img")).toEqual([]);
    expect(asked()).toEqual([]);
  });

  test("the line alone says what it covers and whether it is shown, and a press asks to toggle it", async () => {
    const user = userEvent.setup();
    let toggles = 0;
    const { rerender } = render(
      <>
        <SensitiveLine label="Sensitive image" shown={false} onToggle={() => toggles++} controls="covered" />
        <div id="covered" />
      </>,
    );
    const button = screen.getByRole("button", { name: "Show", description: "Sensitive image", expanded: false });
    expect(button.getAttribute("aria-controls")).toBe("covered");

    await user.click(button);
    expect(toggles).toBe(1);
    // Its owner keeps the state.
    expect(screen.getByRole("button", { name: "Show", expanded: false })).toBe(button);

    rerender(
      <>
        <SensitiveLine label="Sensitive image" shown onToggle={() => toggles++} controls="covered" locale="ja" />
        <div id="covered" />
      </>,
    );
    expect(screen.getByRole("button", { name: "非表示", description: "Sensitive image", expanded: true })).toBe(button);
  });
});
