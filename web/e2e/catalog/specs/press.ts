// SPDX-License-Identifier: AGPL-3.0-only

// What a test of a pressed control reads of it: the shades it paints (its own background, its pseudo-elements' and
// its descendants'), where its box stands, and where the nearest box around it that clips ends at the bottom. A
// pressed control takes another shade than at rest and moves down 1px; what holds it does not cut it.

import type { Locator, Page } from "@playwright/test";
import { atRest } from "./boxes";

/**
 * How a control looks: its shades, its own background's color, its top and bottom, and the bottom of the box that
 * clips it (null when none).
 */
export type Look = { shades: string; own: string; top: number; bottom: number; clip: number | null };

/** How far two edges may be apart and still be the same edge: the rounding of a layout to device pixels. */
export const slack = 0.75;

/**
 * What a test reads of a control found by its role and name: the control itself, or the label that holds it (a
 * choice's row, whose radio or checkbox lies inside it).
 */
export type Of = "itself" | "its label";

/** How `part`, or its label, looks, read once two reads in a row agree. */
export const lookOf = (part: Locator, of: Of = "itself"): Promise<Look> =>
  atRest(() =>
    part.evaluate((control, which) => {
      const element =
        which === "its label" && control instanceof HTMLInputElement ? (control.labels?.[0] ?? control) : control;
      const paints = (el: Element, pseudo?: string) => {
        const style = getComputedStyle(el, pseudo);
        return `${style.backgroundColor} ${style.backgroundImage}`;
      };
      const shades = [
        paints(element),
        paints(element, "::before"),
        paints(element, "::after"),
        ...[...element.querySelectorAll("*")].map((el) => paints(el)),
      ].join("; ");
      const { top, bottom } = element.getBoundingClientRect();
      let clip: number | null = null;
      for (let el = element.parentElement; el !== null && el !== document.body; el = el.parentElement) {
        const style = getComputedStyle(el);
        if (style.overflowX === "visible" && style.overflowY === "visible") continue;
        clip = el.getBoundingClientRect().top + el.clientTop + el.clientHeight;
        break;
      }
      return { shades, own: getComputedStyle(element).backgroundColor, top, bottom, clip };
    }, of),
  );

/** The color a token of the page says (`--color-surface-pressed`), as the page computes a background of it. */
export const colorOf = (page: Page, token: `--${string}`): Promise<string> =>
  page.evaluate((name) => {
    const probe = document.createElement("div");
    probe.style.cssText = `position: absolute; visibility: hidden; background-color: var(${name})`;
    document.body.append(probe);
    const color = getComputedStyle(probe).backgroundColor;
    probe.remove();
    return color;
  }, token);

/** Holds the main button of the mouse down at the centre of `part`, or of its label, where it is drawn. */
export async function holdDown(page: Page, part: Locator, of: Of = "itself"): Promise<void> {
  const box = await part.evaluate((control, which) => {
    const element =
      which === "its label" && control instanceof HTMLInputElement ? (control.labels?.[0] ?? control) : control;
    const { left, top, width, height } = element.getBoundingClientRect();
    return { left, top, width, height };
  }, of);
  await page.mouse.move(box.left + box.width / 2, box.top + box.height / 2);
  await page.mouse.down();
}

/** How `look` is cut at the bottom by what clips it: empty when it is whole. */
export const cutAtBottom = ({ bottom, clip }: Look): string[] =>
  clip !== null && bottom > clip + slack ? [`bottom ${String(bottom)} past the clip at ${String(clip)}`] : [];
