// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { transition } from "@my/design-tokens";
import { modalBackdrop, modalMotion, modalStates } from "./modal";

test("no modal is drawn through ModalOverlay, whose reveal waits for a keyboard while a text field has the focus", async () => {
  for (const file of ["./modal.tsx", "./composer.tsx", "./shortcuts.tsx"]) {
    const source = await Bun.file(new URL(file, import.meta.url)).text();
    const imported = [...source.matchAll(/^import \{([^}]*)\} from "react-aria-components";$/gm)].flatMap((m) =>
      (m[1] ?? "").split(",").map((s) => s.trim()),
    );
    expect(imported).not.toContain("ModalOverlay");
    expect(imported).not.toContain("Modal");
  }
});

test("the backdrop and the panel move by one state the overlay holds, in a modal's time", () => {
  expect(modalStates(false)).toEqual({ initial: "hidden", animate: "shown", exit: "hidden" });
  for (const variants of [modalMotion.backdrop, modalMotion.panel]) {
    expect(Object.keys(variants).sort()).toEqual(["hidden", "shown"]);
    expect(variants.shown.transition.duration).toBe(transition.duration.modal / 1000);
  }
  // In: the backdrop fades its dim and blur in by opacity alone; the panel rises.
  expect(modalMotion.backdrop.hidden).toEqual({ opacity: 0, transition: modalMotion.backdrop.hidden.transition });
  expect(modalMotion.panel.hidden.y).toBe(`${String(transition.rise)}rem`);
  expect(modalMotion.panel.shown.transition.ease).toEqual([0.05, 0.7, 0.1, 1]);
  // Out: the same way back, in the easing of what leaves.
  expect(modalMotion.panel.hidden.transition.ease).toEqual([0.3, 0, 0.8, 0.15]);
});

test("with reduced motion a modal is there at once and gone at once", () => {
  expect(modalStates(true)).toEqual({ initial: false, animate: "shown" });
});

test("the backdrop dims and blurs the page, only dims with reduced transparency, and is none on a phone's screen", () => {
  for (const b of [modalBackdrop.always, modalBackdrop.tablet]) {
    expect(b).toContain("bg-scrim");
    expect(b).toContain("backdrop-blur-[var(--dialog-blur)]");
    expect(b).toContain("[@media(prefers-reduced-transparency:reduce)]:backdrop-blur-none");
  }
  expect(modalBackdrop.tablet).toContain("max-tablet:hidden");
});
