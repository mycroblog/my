// SPDX-License-Identifier: AGPL-3.0-only

// The media viewer, a dialog named "Media" over the page that shows a post's pictures and videos whole, one at a time:
// where it is among them ("2 of 3"), the shown one's description as a caption, what it announces as the reader moves,
// and its buttons. A swipe is a touch on the page (the browser's context has touch), from one side toward the other.

import type { Locator, Page } from "@playwright/test";

export class MediaViewer {
  readonly page: Page;
  readonly dialog: Locator;

  constructor(page: Page) {
    this.page = page;
    this.dialog = page.getByRole("dialog", { name: "Media", exact: true });
  }

  /** Where it is among the post's media, such as "2 of 3". */
  position(text: string): Locator {
    return this.dialog.getByText(text, { exact: true });
  }

  /** The shown picture's description, its caption. */
  caption(text: string): Locator {
    return this.dialog.getByRole("paragraph").filter({ hasText: text });
  }

  /** What it announced of the last move: where it is, then the start of the description or the kind. */
  announced(text: string | RegExp): Locator {
    return this.dialog.getByText(text, { exact: true });
  }

  button(name: "Close" | "Previous" | "Next" | "Zoom in" | "Zoom out"): Locator {
    return this.dialog.getByRole("button", { name, exact: true });
  }

  /** Moves by a swipe across the middle of the viewer: "left" shows the next in a left-to-right page. */
  async swipe(direction: "left" | "right"): Promise<void> {
    const box = await this.dialog.boundingBox();
    if (box === null) throw new Error("e2e: the media viewer is not shown");
    const y = box.y + box.height / 2;
    const steps = [0.7, 0.6, 0.5, 0.4, 0.3].map((f) => box.x + box.width * (direction === "left" ? f : 1 - f));
    const cdp = await this.page.context().newCDPSession(this.page);
    const [start = 0, ...moves] = steps;
    await cdp.send("Input.dispatchTouchEvent", { type: "touchStart", touchPoints: [{ x: start, y }] });
    for (const x of moves) await cdp.send("Input.dispatchTouchEvent", { type: "touchMove", touchPoints: [{ x, y }] });
    await cdp.send("Input.dispatchTouchEvent", { type: "touchEnd", touchPoints: [] });
    await cdp.detach();
  }
}
