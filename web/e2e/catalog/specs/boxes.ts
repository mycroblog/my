// SPDX-License-Identifier: AGPL-3.0-only

// What a layout test reads of a page and asserts of it: where the boxes of its parts are, whether the page scrolls
// sideways, on how many lines a text stands, and what is painted on top at a point. Each assertion retries until the
// page is at rest (restrict/e2e-flaky); a box read to compare with another page's (the same story in the other
// direction) is read once it stopped moving.

import { expect, type Locator, type Page } from "@playwright/test";

/** A box in the viewport, in CSS px. */
export type Box = { left: number; top: number; right: number; bottom: number; width: number; height: number };

/** How far two edges may be apart and still be the same edge: the rounding of a layout to device pixels. */
const slack = 0.75;

/** The box of the element in the viewport. */
export const boxOf = (part: Locator): Promise<Box> =>
  part.evaluate((element) => {
    const { left, top, right, bottom, width, height } = element.getBoundingClientRect();
    return { left, top, right, bottom, width, height };
  });

/** The boxes of the named parts. */
export async function boxesOf<K extends string>(parts: Record<K, Locator>): Promise<Record<K, Box>> {
  const names = Object.keys(parts) as K[];
  const boxes = await Promise.all(names.map((name) => boxOf(parts[name])));
  return Object.fromEntries(names.map((name, i) => [name, boxes[i]])) as Record<K, Box>;
}

/**
 * What `read` reads of the page once two reads in a row agree: the page at rest. A test of a known defect asserts on
 * it once, so that it fails at once and not after a retrying assertion gave up.
 */
export async function atRest<T>(read: () => Promise<T>): Promise<T> {
  let last = await read();
  await expect
    .poll(async () => {
      const now = await read();
      const same = JSON.stringify(now) === JSON.stringify(last);
      last = now;
      return same;
    })
    .toBe(true);
  return last;
}

/** The boxes of the named parts once two reads in a row agree: the page at rest. */
export const boxesAtRest = <K extends string>(parts: Record<K, Locator>): Promise<Record<K, Box>> =>
  atRest(() => boxesOf(parts));

/** The width of the page's viewport without its scrollbar, in CSS px. */
export const pageWidth = (page: Page): Promise<number> => page.evaluate(() => document.documentElement.clientWidth);

/**
 * How the page scrolls sideways: by how much it is wider than its viewport, then the innermost elements that reach
 * past an edge; empty when it does not.
 */
export const sidewaysScroll = (page: Page): Promise<string[]> =>
  page.evaluate(() => {
    const root = document.documentElement;
    const over = root.scrollWidth - root.clientWidth;
    if (over <= 0) return [];
    const reaches = (element: Element) => {
      const box = element.getBoundingClientRect();
      return box.width > 0 && (box.right > root.clientWidth + 0.5 || box.left < -0.5);
    };
    // What an element inside the viewport clips does not widen the page.
    const clipped = (element: Element) => {
      for (let el = element.parentElement; el !== null && el !== document.body; el = el.parentElement)
        if (getComputedStyle(el).overflowX !== "visible" && !reaches(el)) return true;
      return false;
    };
    const past = [...document.body.querySelectorAll("*")].filter((element) => reaches(element) && !clipped(element));
    const innermost = past.filter((element) => ![...element.children].some((child) => past.includes(child)));
    const said = innermost.slice(0, 5).map((element) => {
      const box = element.getBoundingClientRect();
      const text = element.textContent.trim().replaceAll(/\s+/g, " ").slice(0, 60);
      return `<${element.tagName.toLowerCase()}> "${text}" from ${String(Math.round(box.left))} to ${String(Math.round(box.right))}`;
    });
    return [`the page is ${String(over)} px wider than its ${String(root.clientWidth)} px viewport`, ...said];
  });

/** The page does not scroll sideways (ADR-0110 §10): it is no wider than its viewport. */
export async function expectNoSidewaysScroll(page: Page): Promise<void> {
  await expect.poll(() => sidewaysScroll(page)).toEqual([]);
}

/**
 * The texts of the element that a reader sees, in the order of the page: those that are painted, in more than a pixel
 * of what clips them. Words kept for assistive technology alone take no room, and are not among them.
 */
export const shownTexts = (part: Locator): Promise<string[]> =>
  part.evaluate((element) => {
    const shown: string[] = [];
    const walker = document.createTreeWalker(element, NodeFilter.SHOW_TEXT);
    const range = document.createRange();
    for (let node = walker.nextNode(); node !== null; node = walker.nextNode()) {
      const text = (node.textContent ?? "").trim();
      if (text === "") continue;
      range.selectNodeContents(node);
      let { left, top, right, bottom } = range.getBoundingClientRect();
      let painted = true;
      for (let el = node.parentElement; el !== null; el = el === element ? null : el.parentElement) {
        const style = getComputedStyle(el);
        if (style.visibility === "hidden" || style.opacity === "0") painted = false;
        if (style.overflowX === "visible" && style.overflowY === "visible") continue;
        const box = el.getBoundingClientRect();
        left = Math.max(left, box.left);
        top = Math.max(top, box.top);
        right = Math.min(right, box.right);
        bottom = Math.min(bottom, box.bottom);
      }
      if (painted && right - left > 1 && bottom - top > 1) shown.push(text);
    }
    return shown;
  });

/** The length a token of the page says (`--icon-button-touch`), in CSS px. */
export const lengthOf = (page: Page, token: `--${string}`): Promise<number> =>
  page.evaluate((name) => {
    const probe = document.createElement("div");
    probe.style.cssText = `position: absolute; visibility: hidden; inline-size: var(${name})`;
    document.body.append(probe);
    const { width } = probe.getBoundingClientRect();
    probe.remove();
    return width;
  }, token);

/** How `inner` reaches out of `outer`, by edge; empty when it is inside. */
export function outside(inner: Box, outer: Box): string[] {
  const out: string[] = [];
  if (inner.left < outer.left - slack) out.push(`left ${String(inner.left)} < ${String(outer.left)}`);
  if (inner.right > outer.right + slack) out.push(`right ${String(inner.right)} > ${String(outer.right)}`);
  if (inner.top < outer.top - slack) out.push(`top ${String(inner.top)} < ${String(outer.top)}`);
  if (inner.bottom > outer.bottom + slack) out.push(`bottom ${String(inner.bottom)} > ${String(outer.bottom)}`);
  return out;
}

/** The viewport of the page as a box. */
export const viewportBox = (page: Page): Promise<Box> =>
  page.evaluate(() => {
    const { clientWidth: width, clientHeight: height } = document.documentElement;
    return { left: 0, top: 0, right: width, bottom: height, width, height };
  });

/** The box of `inner` is inside that of `outer`, or between the sides of the page, which scrolls down. */
export async function expectInside(inner: Locator, outer: Locator | Page): Promise<void> {
  await expect
    .poll(async () => {
      if (!("goto" in outer)) return outside(await boxOf(inner), await boxOf(outer));
      return outside(await boxOf(inner), { ...(await viewportBox(outer)), top: -Infinity, bottom: Infinity });
    })
    .toEqual([]);
}

/** The box of `part` is inside the viewport, where the reader sees all of it. */
export async function expectInViewport(page: Page, part: Locator): Promise<void> {
  await expect.poll(async () => outside(await boxOf(part), await viewportBox(page))).toEqual([]);
}

/** The same box seen from the other side of a page `width` wide. */
export const mirrorOf = (box: Box, width: number): Box => ({
  ...box,
  left: width - box.right,
  right: width - box.left,
});

/**
 * The parts on `page`, which reads in the other direction, are where a mirror puts those of `before` (all of them,
 * or the ones named in `parts`): each as far from the page's end as it was from its start, at the same height and of
 * the same size.
 */
export async function expectMirrored<K extends string>(
  page: Page,
  before: Record<K, Box>,
  parts: Partial<Record<K, Locator>>,
): Promise<void> {
  const there = parts as Record<K, Locator>;
  await expect
    .poll(async () => {
      const width = await pageWidth(page);
      const now = await boxesOf(there);
      return (Object.keys(there) as K[]).flatMap((name) => {
        const want = mirrorOf(before[name], width);
        const got = now[name];
        const off = (["left", "right", "top", "bottom"] as const).filter(
          (edge) => Math.abs(want[edge] - got[edge]) > slack,
        );
        return off.map((edge) => `${name}: ${edge} ${String(got[edge])}, mirrored ${String(want[edge])}`);
      });
    })
    .toEqual([]);
}

/**
 * The lines the element's own boxes stand on: one for a text on one line, more once it wraps. An element that is a
 * box of its own (a paragraph) is measured by its content.
 */
export const linesOf = (text: Locator): Promise<number> =>
  text.evaluate((element) => {
    const range = document.createRange();
    range.selectNodeContents(element);
    const tops: number[] = [];
    for (const rect of range.getClientRects()) {
      if (rect.width === 0 || rect.height === 0) continue;
      const middle = (rect.top + rect.bottom) / 2;
      if (!tops.some((top) => Math.abs(top - middle) < rect.height / 2)) tops.push(middle);
    }
    return tops.length;
  });

/**
 * Whether the text is cut short with an ellipsis: the element, or the nearest box around it that clips, shows less
 * than the text is wide, and ends what it shows with an ellipsis.
 */
export const cutShort = (text: Locator): Promise<boolean> =>
  text.evaluate((element) => {
    const range = document.createRange();
    range.selectNodeContents(element);
    const width = range.getBoundingClientRect().width;
    for (let el: Element | null = element; el !== null && el !== document.body; el = el.parentElement) {
      const style = getComputedStyle(el);
      if (style.overflowX === "visible" || style.display === "inline") continue;
      return style.textOverflow === "ellipsis" && width > el.clientWidth + 0.5;
    }
    return false;
  });

/**
 * The elements among `parts` that something else is painted over at their centre (`elementFromPoint`), each with
 * what covers it. Hit testing passes over an inert element (a backdrop that takes no press is one) however it is
 * painted, so every element is hit for the probe and inert again after it: the probe sees the paint order, as the
 * reader's eye does.
 */
export const covered = (parts: Locator): Promise<string[]> =>
  parts.evaluateAll((elements) => {
    const inert = [...document.querySelectorAll<HTMLElement>("[inert]")];
    for (const el of inert) el.inert = false;
    try {
      return elements.flatMap((element) => {
        const box = element.getBoundingClientRect();
        const top = document.elementFromPoint(box.left + box.width / 2, box.top + box.height / 2);
        return top !== null && (element.contains(top) || top.contains(element))
          ? []
          : [`${element.textContent.trim().slice(0, 40)} under ${top?.outerHTML.slice(0, 120) ?? "nothing"}`];
      });
    } finally {
      for (const el of inert) el.inert = true;
    }
  });

/** The text of the element line by line, as the page breaks it. */
export const textLines = (text: Locator): Promise<string[]> =>
  text.evaluate((element) => {
    const lines: { middle: number; height: number; text: string }[] = [];
    const walker = document.createTreeWalker(element, NodeFilter.SHOW_TEXT);
    const range = document.createRange();
    for (let node = walker.nextNode(); node !== null; node = walker.nextNode()) {
      const length = node.textContent?.length ?? 0;
      for (let i = 0; i < length; i += 1) {
        range.setStart(node, i);
        range.setEnd(node, i + 1);
        const [rect] = range.getClientRects();
        if (rect === undefined || rect.width === 0) continue;
        const middle = (rect.top + rect.bottom) / 2;
        const line = lines.find((l) => Math.abs(l.middle - middle) < l.height / 2);
        const character = node.textContent?.[i] ?? "";
        if (line === undefined) lines.push({ middle, height: rect.height, text: character });
        else line.text += character;
      }
    }
    return lines.map((line) => line.text);
  });

/**
 * The glyphs drawn inside the element, in the order of the page: each one's box, and whether it is drawn mirrored. A
 * glyph has no role and no name, so it is found by what holds it.
 */
export const glyphsOf = (holder: Locator): Promise<(Box & { mirrored: boolean })[]> =>
  holder.evaluate((element) =>
    [...element.querySelectorAll("svg")].map((svg) => {
      const { left, top, right, bottom, width, height } = svg.getBoundingClientRect();
      return { left, top, right, bottom, width, height, mirrored: (svg.getScreenCTM()?.a ?? 1) < 0 };
    }),
  );
