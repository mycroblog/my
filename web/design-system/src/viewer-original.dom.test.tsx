// SPDX-License-Identifier: AGPL-3.0-only

// The viewer's picture before its original has come and when it never comes: the preview the post showed, in the box
// the original will take, then the original once it has decoded. happy-dom loads no image: each test says which
// originals decode, which fail and which are still on their way, through `HTMLImageElement.prototype.decode`.

import { afterEach, beforeEach, expect, test } from "bun:test";
import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { MediaViewer } from "./viewer";

/** How the decoding of an address ends: it comes, it fails, or it is still on its way. */
type Arrival = "come" | "fail" | "wait";

const decodeWas = Object.getOwnPropertyDescriptor(HTMLImageElement.prototype, "decode");
/** How each address decodes; an address not named here is still on its way. */
let arrivals = new Map<string, Arrival>();
/** The decodings on their way, which a test ends later. */
let waiting: { src: string; come: () => void; fail: (reason: Error) => void }[] = [];

beforeEach(() => {
  arrivals = new Map();
  waiting = [];
  Object.defineProperty(HTMLImageElement.prototype, "decode", {
    configurable: true,
    writable: true,
    value(this: HTMLImageElement) {
      const src = this.src;
      const arrival = arrivals.get(src) ?? "wait";
      if (arrival === "come") return Promise.resolve();
      if (arrival === "fail") return Promise.reject(new Error(`test: ${src} did not load`));
      return new Promise<void>((come, fail) => {
        waiting.push({ src, come, fail });
      });
    },
  });
});

afterEach(() => {
  if (decodeWas === undefined) Reflect.deleteProperty(HTMLImageElement.prototype, "decode");
  else Object.defineProperty(HTMLImageElement.prototype, "decode", decodeWas);
});

/** Ends every decoding of `src` still on its way, as `arrival`. */
function arrive(src: string, arrival: "come" | "fail") {
  arrivals.set(src, arrival);
  for (const one of waiting.filter((w) => w.src === src)) {
    if (arrival === "come") one.come();
    else one.fail(new Error(`test: ${src} did not load`));
  }
  waiting = waiting.filter((w) => w.src !== src);
}

const a = {
  id: "a",
  src: "https://m.example/a-small.webp",
  href: "https://m.example/a.webp",
  alt: "A ridge at dawn",
  width: 1600,
  height: 1200,
};
const b = {
  id: "b",
  src: "https://m.example/b-small.webp",
  href: "https://m.example/b.webp",
  alt: "A tall pine",
  width: 600,
  height: 900,
};
/** A picture whose size the server did not say. */
const unsized = { id: "c", src: "https://m.example/c-small.webp", href: "https://m.example/c.webp", alt: "Fog" };

/** The viewer open at `at` among `media`, moving where it asks to, and the stage that shows the picture. */
function viewer(media: Parameters<typeof MediaViewer>[0]["items"], at = 0) {
  function Open() {
    const [index, setIndex] = useState(at);
    return <MediaViewer items={media} index={index} onIndexChange={setIndex} onClose={() => undefined} locale="en" />;
  }
  render(<Open />);
  const dialog = screen.getByRole("dialog", { name: "Media" });
  return { dialog, picture: () => shown(dialog) };
}

/** What the stage shows: its picture's file, its size of its own, and what it paints behind the picture. */
function shown(dialog: HTMLElement) {
  const stage = dialog.querySelector<HTMLElement>("[data-viewer-stage]");
  if (stage === null) throw new Error("test: the viewer shows no stage");
  const img = within(stage).getByRole<HTMLImageElement>("presentation");
  const src = img.getAttribute("src") ?? "";
  return {
    src,
    size: intrinsic(img),
    backing: img.style.backgroundImage,
    fit: img.style.backgroundSize,
    /** Every address the stage would have the browser fetch. */
    asked: Array.from(stage.querySelectorAll("*")).flatMap((element) =>
      element.getAttributeNames().map((name) => element.getAttribute(name) ?? ""),
    ),
  };
}

/**
 * The size an image has of its own before any file of it has loaded, as `<width>x<height>`: its `width` and `height`,
 * or the size of the SVG it is drawn from.
 */
function intrinsic(img: HTMLImageElement): string | undefined {
  const width = img.getAttribute("width");
  const height = img.getAttribute("height");
  if (width !== null && height !== null) return `${width}x${height}`;
  const src = img.getAttribute("src") ?? "";
  if (!src.startsWith("data:image/svg+xml")) return undefined;
  const svg = decodeURIComponent(src.slice(src.indexOf(",") + 1));
  const w = /\swidth=["']?(\d+)/.exec(svg)?.[1];
  const h = /\sheight=["']?(\d+)/.exec(svg)?.[1];
  return w === undefined || h === undefined ? undefined : `${w}x${h}`;
}

/** Lets the viewer take what the decodings it asked for gave. */
const settle = () => new Promise((resolve) => setTimeout(resolve, 30));

test("with its size, the picture has the original's size from the first render, the preview filling it, until the original decodes", async () => {
  const { picture } = viewer([a]);

  expect(picture().size).toBe("1600x1200");
  expect(picture().backing).toContain(a.src);
  expect(picture().fit).toBe("contain");
  expect(picture().src).not.toBe(a.href);
  await settle();
  expect(picture().size).toBe("1600x1200");
  expect(picture().backing).toContain(a.src);

  arrive(a.href, "come");

  await waitFor(() => {
    expect(picture().src).toBe(a.href);
  });
});

test("without its size, the picture is the preview itself, whose shape the box takes, until the original decodes", async () => {
  const { picture } = viewer([unsized]);

  expect(picture().src).toBe(unsized.src);
  await settle();
  expect(picture().src).toBe(unsized.src);

  arrive(unsized.href, "come");

  await waitFor(() => {
    expect(picture().src).toBe(unsized.href);
  });
});

test("an original that fails leaves the preview in its place for good, at the original's size", async () => {
  const { picture } = viewer([a]);

  arrive(a.href, "fail");
  await settle();

  expect(picture().size).toBe("1600x1200");
  expect(picture().backing).toContain(a.src);
  expect(picture().fit).toBe("contain");
  expect(picture().src).not.toBe(a.href);
  expect(picture().src).not.toBe("");
});

test("an original of no known size that fails leaves the preview itself", async () => {
  const { picture } = viewer([unsized]);

  arrive(unsized.href, "fail");
  await settle();

  expect(picture().src).toBe(unsized.src);
});

test("a picture with no original, or whose original is its preview, shows its file at once", () => {
  const alone = { id: "d", src: "https://m.example/d.webp", alt: "Dunes", width: 800, height: 600 };
  const same = { id: "e", src: "https://m.example/e.webp", href: "https://m.example/e.webp", alt: "Ice" };

  expect(viewer([alone]).picture().src).toBe(alone.src);
  cleanup();
  expect(viewer([same]).picture().src).toBe(same.src);
});

test("moving on shows the next picture's own preview and size, never the one before's original", async () => {
  const user = userEvent.setup();
  arrivals.set(a.href, "come");
  const { dialog, picture } = viewer([a, b]);
  await waitFor(() => {
    expect(picture().src).toBe(a.href);
  });

  await user.keyboard("{ArrowRight}");

  expect(within(dialog).getByText("2 of 2")).not.toBeNull();
  expect(picture().size).toBe("600x900");
  expect(picture().backing).toContain(b.src);
  expect(picture().asked.some((value) => value.includes(a.href) || value.includes(a.src))).toBe(false);
  arrive(b.href, "come");
  await waitFor(() => {
    expect(picture().src).toBe(b.href);
  });

  await user.click(within(dialog).getByRole("button", { name: "Previous" }));

  await waitFor(() => {
    expect(picture().src).toBe(a.href);
  });
  expect(picture().asked.some((value) => value.includes(b.href) || value.includes(b.src))).toBe(false);
});

test("moving from a picture whose original failed to one still on its way gives each its own size", async () => {
  const user = userEvent.setup();
  arrivals.set(b.href, "fail");
  const { dialog, picture } = viewer([b, unsized, a]);
  await settle();
  expect(picture().size).toBe("600x900");

  await user.keyboard("{ArrowRight}");
  expect(picture().src).toBe(unsized.src);
  expect(picture().backing).not.toContain(b.src);

  await user.keyboard("{ArrowRight}");
  expect(picture().size).toBe("1600x1200");
  expect(picture().backing).toContain(a.src);

  // Previous stays where it was pressed: the focus stays in the viewer for the arrow after it.
  await user.click(within(dialog).getByRole("button", { name: "Previous" }));
  expect(picture().src).toBe(unsized.src);
  expect(picture().backing).not.toContain(a.src);

  await user.keyboard("{ArrowLeft}");
  await settle();
  expect(picture().size).toBe("600x900");
  expect(picture().backing).toContain(b.src);
  expect(picture().asked.some((value) => value.includes(a.src) || value.includes(unsized.src))).toBe(false);
});
