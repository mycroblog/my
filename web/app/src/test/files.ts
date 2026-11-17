// SPDX-License-Identifier: AGPL-3.0-only

// The browser's file chooser, for the component tests of the app's routes: a press that opens it, and the files a
// reader picks in it. No simulation opens a chooser, so the test takes the input the press asked to open and gives it
// the files, as a browser does once the reader chose them.

import { spyOn } from "bun:test";
import type { TestApp } from "./app";

/** Presses `button`, which opens the file chooser, and picks `files` in it. */
export async function chooseFiles(app: TestApp, button: HTMLElement, files: File | File[]): Promise<void> {
  const opened: HTMLInputElement[] = [];
  const click = spyOn(HTMLInputElement.prototype, "click").mockImplementation(function (this: HTMLInputElement) {
    if (this.type === "file") opened.push(this);
  });
  try {
    await app.user.click(button);
  } finally {
    click.mockRestore();
  }
  const [chooser] = opened;
  if (chooser === undefined) throw new Error("test: the press opened no file chooser");
  await app.user.upload(chooser, files);
}

/** A file of an image to pick: a PNG by its name, whose bytes no test reads. */
export const imageFile = (name: string): File =>
  new File([new Uint8Array([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a])], name, { type: "image/png" });
