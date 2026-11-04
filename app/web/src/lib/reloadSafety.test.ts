// SPDX-License-Identifier: AGPL-3.0-only
// The safe moment of ADR-0028 §13, one condition at a time, on a document of its own.
import { describe, expect, test } from "vitest";
import { isSafeToReload, registerReloadBlocker, reportCallActive } from "./reloadSafety";

function page(html: string): Document {
  const doc = document.implementation.createHTMLDocument();
  doc.body.innerHTML = html;
  return doc;
}

describe("isSafeToReload", () => {
  test("is safe on a page with nothing to lose", () => {
    expect(isSafeToReload(page("<main><button>Open</button></main>"))).toBe(true);
  });

  test("is not safe while a registered feature says so, until it leaves", () => {
    let busy = true;
    const remove = registerReloadBlocker(() => busy);
    expect(isSafeToReload(page(""))).toBe(false);
    busy = false;
    expect(isSafeToReload(page(""))).toBe(true);
    busy = true;
    remove();
    expect(isSafeToReload(page(""))).toBe(true);
  });

  test("is not safe during a call", () => {
    reportCallActive(true);
    expect(isSafeToReload(page(""))).toBe(false);
    reportCallActive(false);
    expect(isSafeToReload(page(""))).toBe(true);
  });

  test.each(["dialog", "alertdialog"])("is not safe while a %s is open", (role) => {
    expect(isSafeToReload(page(`<div role="${role}">Sure?</div>`))).toBe(false);
  });

  test.each([
    ["an input", "<input id=x>"],
    ["a text area", "<textarea id=x></textarea>"],
    ["a select", "<select id=x></select>"],
    ["an editable element", '<div id=x contenteditable="true" tabindex="0"></div>'],
  ])("is not safe while focus is in %s", (_name, html) => {
    const doc = page(html);
    // A document without a browsing context has no focus of its own.
    const element = doc.getElementById("x");
    Object.defineProperty(doc, "activeElement", { value: element });
    expect(isSafeToReload(doc)).toBe(false);
  });

  test("is safe while focus is on a button", () => {
    const doc = page("<button id=x>Open</button>");
    Object.defineProperty(doc, "activeElement", { value: doc.getElementById("x") });
    expect(isSafeToReload(doc)).toBe(true);
  });

  function withMedia(state: Partial<Pick<HTMLMediaElement, "paused" | "ended" | "muted" | "loop">>): Document {
    const doc = page("<video></video>");
    const video = doc.querySelector("video");
    for (const [key, value] of Object.entries(state)) Object.defineProperty(video, key, { value });
    return doc;
  }

  test("is not safe while media plays", () => {
    expect(isSafeToReload(withMedia({ paused: false, ended: false, muted: false, loop: false }))).toBe(false);
  });

  test("is safe with paused or ended media, and with a muted loop such as an animated GIF", () => {
    expect(isSafeToReload(withMedia({ paused: true }))).toBe(true);
    expect(isSafeToReload(withMedia({ paused: false, ended: true }))).toBe(true);
    expect(isSafeToReload(withMedia({ paused: false, ended: false, muted: true, loop: true }))).toBe(true);
  });
});
