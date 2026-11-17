// SPDX-License-Identifier: AGPL-3.0-only

// The media viewer, a dialog named "Media" over the page that shows a post's pictures and videos whole, one at a time:
// where it is among them ("2 of 3"), and its buttons. A swipe is a touch on the page (the browser's context has
// touch), from the picture sideways.

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

  button(name: "Close" | "Previous" | "Next" | "Zoom in" | "Zoom out"): Locator {
    return this.dialog.getByRole("button", { name, exact: true });
  }

  /** The picture shown, drawn once it has loaded; it has no name of its own, its caption describes it. */
  get picture(): Locator {
    return this.dialog.getByRole("presentation");
  }

  /**
   * Moves by a swipe that starts on the shown picture, as a reader's does, once it is drawn: until then the description
   * may sit where the picture will be, and a touch on the description scrolls it rather than swipes. "left" shows the
   * next in a left-to-right page.
   */
  async swipe(direction: "left" | "right"): Promise<void> {
    await this.picture.waitFor({ state: "visible" });
    const [box, picture] = await Promise.all([this.dialog.boundingBox(), this.picture.boundingBox()]);
    if (box === null || picture === null) throw new Error("e2e: the media viewer is not shown");
    const x = picture.x + picture.width / 2;
    const y = picture.y + picture.height / 2;
    const steps = [0, 0.1, 0.2, 0.3, 0.4].map((f) => x + box.width * (direction === "left" ? -f : f));
    const cdp = await this.page.context().newCDPSession(this.page);
    const [start = 0, ...moves] = steps;
    await cdp.send("Input.dispatchTouchEvent", { type: "touchStart", touchPoints: [{ x: start, y }] });
    for (const x of moves) await cdp.send("Input.dispatchTouchEvent", { type: "touchMove", touchPoints: [{ x, y }] });
    await cdp.send("Input.dispatchTouchEvent", { type: "touchEnd", touchPoints: [] });
    await cdp.detach();
  }
}
