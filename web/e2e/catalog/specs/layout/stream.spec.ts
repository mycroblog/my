// SPDX-License-Identifier: AGPL-3.0-only

// The mark of the session's stream in the frame (FrameStream of shell.tsx): a dot at the top right of the screen, on a
// phone and on a PC, green while the stream is open, amber while it connects or reconnects, red once it stopped, with
// no visible label; a pointer that hovers it shows its label as a tooltip.

import type { Locator } from "@playwright/test";
import { boxesAtRest, shownTexts, viewportBox } from "../boxes";
import { expect, minimum, pc, test } from "../test";

type Hue = "green" | "amber" | "red";

/** The shell's stories of each state of the stream, with the story's words for it, and the hue its mark is painted in. */
const states: readonly { story: string; label: string; hue: Hue }[] = [
  { story: "design-system-shell--home", label: "Connected", hue: "green" },
  { story: "design-system-shell--stream-connecting", label: "Connecting…", hue: "amber" },
  { story: "design-system-shell--stream-reconnecting", label: "Reconnecting…", hue: "amber" },
  { story: "design-system-shell--stream-stopped", label: "Disconnected", hue: "red" },
];

/**
 * The hue of the color the mark's drawing is painted in, as the browser paints it: the color is drawn on a canvas and
 * read back as RGB, whatever notation the page computes it in.
 */
const hueOf = (mark: Locator): Promise<string> =>
  mark.evaluate((element) => {
    const drawing = element.querySelector("svg") ?? element;
    const canvas = document.createElement("canvas");
    canvas.width = 1;
    canvas.height = 1;
    const context = canvas.getContext("2d");
    if (context === null) return "no canvas";
    context.fillStyle = getComputedStyle(drawing).color;
    context.fillRect(0, 0, 1, 1);
    const [r = 0, g = 0, b = 0] = context.getImageData(0, 0, 1, 1).data;
    const max = Math.max(r, g, b);
    const min = Math.min(r, g, b);
    if (max - min < 40) return `grey rgb(${String(r)}, ${String(g)}, ${String(b)})`;
    const d = max - min;
    const h = max === r ? (((g - b) / d) % 6) * 60 : max === g ? ((b - r) / d + 2) * 60 : ((r - g) / d + 4) * 60;
    const hue = (h + 360) % 360;
    if (hue >= 70 && hue <= 170) return "green";
    if (hue >= 25 && hue < 70) return "amber";
    if (hue >= 330 || hue < 25) return "red";
    return `hue ${String(Math.round(hue))}`;
  });

test("the stream's mark is a dot at the top right in its state's color, with no label but on hover", async ({
  page,
  open,
}) => {
  const height = 640;
  for (const { story, label, hue } of states) {
    await open(story, { height });
    const mark = page.getByRole("img", { name: label, exact: true });
    for (const width of [minimum, pc]) {
      await test.step(`${story} at ${String(width)} px`, async () => {
        await page.setViewportSize({ width, height });
        await page.mouse.move(0, height / 2);
        await expect(mark).toBeVisible();
        const { box } = await boxesAtRest({ box: mark });
        const viewport = await viewportBox(page);
        // In the top right corner of the screen: its middle in the last quarter of the width and the top tenth.
        expect((box.left + box.right) / 2).toBeGreaterThan(viewport.right * 0.75);
        expect(box.right).toBeLessThanOrEqual(viewport.right);
        expect(box.top).toBeGreaterThanOrEqual(0);
        expect((box.top + box.bottom) / 2).toBeLessThan(height / 10);
        // A dot: as wide as it is high, and small.
        expect(Math.abs(box.width - box.height)).toBeLessThanOrEqual(1);
        expect(box.width).toBeLessThanOrEqual(32);
        await expect.poll(() => hueOf(mark)).toBe(hue);
        await expect.poll(() => shownTexts(mark)).toEqual([]);

        // A pointer that hovers it shows its label.
        await mark.hover();
        await expect.poll(() => shownTexts(mark)).toEqual([label]);
      });
    }
  }
});
