// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import {
  color,
  contrast,
  contrastPairs,
  control,
  fontFamily,
  focusRing,
  icon,
  mix,
  motion,
  press,
  properties,
  radius,
  spinner,
  text,
} from "./tokens";

describe("contrastPairs", () => {
  for (const { fg, bg, minimum } of contrastPairs) {
    for (const scheme of ["light", "dark"] as const) {
      test(`${fg} on ${bg} reaches ${minimum}:1 in ${scheme}`, () => {
        expect(contrast(color[fg][scheme], color[bg][scheme])).toBeGreaterThanOrEqual(minimum);
      });
    }
  }
});

describe("press", () => {
  for (const scheme of ["light", "dark"] as const) {
    for (const step of ["hover", "pressed"] as const) {
      test(`on-accent on the accent ${step} reaches 4.5:1 in ${scheme}`, () => {
        const fill = mix(press.shade, color.accent[scheme], press[step]);
        expect(contrast(color["on-accent"][scheme], fill)).toBeGreaterThanOrEqual(4.5);
      });
    }
  }
});

test("a control without a surface stays lighter on hover than one with a surface at rest", () => {
  for (const scheme of ["light", "dark"] as const) {
    const surface = color.surface[scheme];
    expect(contrast(color["surface-hover"][scheme], surface)).toBeLessThan(
      contrast(color["surface-raised"][scheme], surface),
    );
  }
});

test("mix blends in sRGB as color-mix does", () => {
  expect(mix("#000000", "#ffffff", 0.5)).toBe("#808080");
  expect(mix("#ff6b6b", "#111111", 1)).toBe("#ff6b6b");
});

test("contrast follows WCAG 2", () => {
  expect(contrast("#000000", "#ffffff")).toBeCloseTo(21, 5);
  expect(contrast("#ffffff", "#ffffff")).toBe(1);
  expect(contrast("#ff6b6b", "#ffffff")).toBeCloseTo(2.78, 2);
});

test("every color is #rrggbb", () => {
  for (const c of [...Object.values(color).flatMap((c) => [c.light, c.dark]), press.shade]) {
    expect(c).toMatch(/^#[0-9a-f]{6}$/);
  }
});

test("properties declare every token", () => {
  for (const [name, c] of Object.entries(color)) {
    expect(properties[`--color-${name}`]).toBe(`light-dark(${c.light}, ${c.dark})`);
  }
  for (const [name, step] of Object.entries(text)) {
    expect(properties[`--text-${name}`]).toBe(`${step.size}rem`);
    expect(properties[`--text-${name}--line-height`]).toBe(`${step.lineHeight}`);
  }
  expect(properties["--font-sans"]).toBe(fontFamily.sans);
  expect(properties["--radius-control"]).toBe(`${radius.control}rem`);
  for (const [size, c] of Object.entries(control)) {
    expect(properties[`--control-${size}-height`]).toBe(`${c.height}rem`);
    expect(properties[`--control-${size}-padding-inline`]).toBe(`${c.paddingInline}rem`);
  }
  expect(properties["--focus-ring-width"]).toBe(`${focusRing.width}px`);
  expect(properties["--press-shade"]).toBe(press.shade);
  expect(properties["--press-pressed"]).toBe(`${press.pressed * 100}%`);
  expect(properties["--duration-spin"]).toBe(`${motion.spin.duration}ms`);
  expect(properties["--spinner-size"]).toBe(`${spinner.size}em`);
  expect(properties["--icon-action"]).toBe(`${icon.action}rem`);
  expect(properties["--icon-caption"]).toBe(`${icon.caption}rem`);
  expect(properties["--icon-tile-glyph"]).toBe(`${icon.tile.glyph * 100}%`);
});
