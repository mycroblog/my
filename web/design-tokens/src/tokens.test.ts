// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { color, contrast, contrastPairs, css, text } from "./tokens";

describe("contrastPairs", () => {
  for (const { fg, bg, minimum } of contrastPairs) {
    for (const scheme of ["light", "dark"] as const) {
      test(`${fg} on ${bg} reaches ${minimum}:1 in ${scheme}`, () => {
        expect(contrast(color[fg][scheme], color[bg][scheme])).toBeGreaterThanOrEqual(minimum);
      });
    }
  }
});

test("contrast follows WCAG 2", () => {
  expect(contrast("#000000", "#ffffff")).toBeCloseTo(21, 5);
  expect(contrast("#ffffff", "#ffffff")).toBe(1);
  expect(contrast("#ff6b6b", "#ffffff")).toBeCloseTo(2.78, 2);
});

test("every color is #rrggbb", () => {
  for (const c of Object.values(color)) {
    expect(c.light).toMatch(/^#[0-9a-f]{6}$/);
    expect(c.dark).toMatch(/^#[0-9a-f]{6}$/);
  }
});

test("css declares every token", () => {
  for (const [name, c] of Object.entries(color)) {
    expect(css).toContain(`--color-${name}: light-dark(${c.light}, ${c.dark});`);
  }
  for (const [name, step] of Object.entries(text)) {
    expect(css).toContain(`--text-${name}: ${step.size}rem;`);
    expect(css).toContain(`--text-${name}--line-height: ${step.lineHeight};`);
  }
  expect(css).toContain("--font-sans: ");
});
