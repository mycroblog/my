// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import {
  avatar,
  breakpoint,
  chip,
  color,
  contrast,
  content,
  badgeColor,
  badgeContrastPairs,
  contrastPairs,
  control,
  dialog,
  disclosure,
  embed,
  entry,
  feed,
  fontFamily,
  fontWeight,
  focusRing,
  media,
  icon,
  iconButton,
  kbd,
  latinFamily,
  menu,
  mix,
  motion,
  press,
  properties,
  radius,
  shadow,
  shell,
  spinner,
  text,
  voice,
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

describe("badgeContrastPairs", () => {
  for (const { fg, bg, minimum } of badgeContrastPairs) {
    for (const scheme of ["light", "dark"] as const) {
      test(`${fg} on ${bg} reaches ${minimum}:1 in ${scheme}`, () => {
        expect(contrast(badgeColor[fg][scheme], badgeColor[bg][scheme])).toBeGreaterThanOrEqual(minimum);
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

test("the sans stack starts with the self-hosted emoji and ends with the system's emoji fonts (ADR-0091 §10)", () => {
  const families = fontFamily.sans.split(",").map((f) => f.trim());
  expect(families[0]).toBe('"my Noto Color Emoji"');
  expect(families.slice(-3)).toEqual(['"Apple Color Emoji"', '"Segoe UI Emoji"', '"Noto Color Emoji"']);
});

test("the sans stack puts Noto Sans before Noto Sans CJK, and the mono stack has no Noto Sans (ADR-0009 §5)", () => {
  const sans = fontFamily.sans.split(",").map((f) => f.trim());
  expect(sans.slice(1, 4)).toEqual(["var(--font-sans-override)", "var(--font-latin)", "var(--font-cjk)"]);
  expect(fontFamily.mono).not.toContain("--font-latin");
  expect(properties["--font-latin"]).toBe(`"${latinFamily.family}"`);
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
  expect(properties["--icon-button-padding-inline"]).toBe(`${iconButton.paddingInline}rem`);
  expect(properties["--icon-button-gap"]).toBe(`${iconButton.gap}rem`);
  expect(properties["--icon-button-touch"]).toBe(`${iconButton.touch}rem`);
  expect(properties["--focus-ring-width"]).toBe(`${focusRing.width}px`);
  expect(properties["--press-shade"]).toBe(press.shade);
  expect(properties["--press-pressed"]).toBe(`${press.pressed * 100}%`);
  expect(properties["--duration-spin"]).toBe(`${motion.spin.duration}ms`);
  expect(properties["--spinner-size"]).toBe(`${spinner.size}em`);
  expect(properties["--icon-action"]).toBe(`${icon.action}rem`);
  expect(properties["--icon-caption"]).toBe(`${icon.caption}rem`);
  expect(properties["--icon-tile-glyph"]).toBe(`${icon.tile.glyph * 100}%`);
  for (const place of ["inline", "row", "post", "preview"] as const) {
    expect(properties[`--avatar-${place}`]).toBe(`${avatar[place]}rem`);
  }
  expect(properties["--avatar-initial"]).toBe(`${avatar.initial}`);
  expect(properties["--radius-overlay"]).toBe(`${radius.overlay}rem`);
  expect(properties["--shadow-overlay"]).toBe(
    `0 ${shadow.y}px ${shadow.blur}px light-dark(rgb(0 0 0 / ${shadow.share.light * 100}%), rgb(0 0 0 / ${shadow.share.dark * 100}%))`,
  );
  for (const [name, key] of [
    ["padding", "padding"],
    ["group-gap", "groupGap"],
    ["icon-gap", "iconGap"],
    ["min-width", "minWidth"],
  ] as const) {
    expect(properties[`--menu-${name}`]).toBe(`${menu[key]}rem`);
  }
  expect(properties["--disclosure-gap"]).toBe(`${disclosure.gap}rem`);
  expect(properties["--disclosure-padding"]).toBe(`${disclosure.padding}rem`);
  expect(properties["--disclosure-padding-start"]).toBe(`${disclosure.paddingStart}rem`);
  expect(properties["--content-paragraph-gap"]).toBe(`${content.paragraphGap}rem`);
  expect(properties["--content-indent"]).toBe(`${content.indent}rem`);
  expect(properties["--content-quote-bar"]).toBe(`${content.quoteBar}rem`);
  expect(properties["--content-quote-indent"]).toBe(`${content.quoteIndent}rem`);
  expect(properties["--content-code-padding-block"]).toBe(`${content.codePaddingBlock}rem`);
  expect(properties["--content-code-padding-inline"]).toBe(`${content.codePaddingInline}rem`);
  expect(properties["--content-code-hang"]).toBe(`${content.codeHang}ch`);
  expect(properties["--content-code-inline"]).toBe(`${content.codeInline}em`);
  expect(properties["--content-emoji"]).toBe(`${content.emoji}em`);
  for (const [name, key] of [
    ["gap", "gap"],
    ["line-gap", "lineGap"],
    ["reach", "reach"],
    ["radius", "radius"],
    ["avatar-gap", "avatarGap"],
    ["name-gap", "nameGap"],
    ["caption-gap", "captionGap"],
    ["action-gap", "actionGap"],
    ["scroll-margin", "scrollMargin"],
  ] as const) {
    expect(properties[`--entry-${name}`]).toBe(`${entry[key]}rem`);
  }
  expect(properties["--entry-leading"]).toBe(`${entry.leading}`);
  expect(properties["--feed-gap"]).toBe(`${feed.gap}rem`);
  expect(properties["--feed-scroll-margin"]).toBe(`${feed.scrollMargin}rem`);
  expect(properties["--embed-padding"]).toBe(`${embed.padding}rem`);
  expect(properties["--embed-gap"]).toBe(`${embed.gap}rem`);
  expect(properties["--media-gap"]).toBe(`${media.gap}rem`);
  expect(properties["--media-ratio"]).toBe(`${media.ratio}`);
  expect(properties["--media-max-height"]).toBe(`${media.maxHeight}rem`);
  expect(properties["--media-preview-width"]).toBe(`${media.previewWidth}rem`);
  expect(properties["--media-preview-height"]).toBe(`${media.previewHeight}rem`);
  for (const [name, key] of [
    ["height", "height"],
    ["padding-inline", "paddingInline"],
    ["gap", "gap"],
    ["spacing", "spacing"],
  ] as const) {
    expect(properties[`--chip-${name}`]).toBe(`${chip[key]}rem`);
  }
  expect(properties["--chip-glyph"]).toBe(`${chip.glyph}em`);
  for (const [name, key] of [
    ["padding", "padding"],
    ["gap", "gap"],
    ["row-gap", "rowGap"],
    ["max-width", "maxWidth"],
    ["margin", "margin"],
  ] as const) {
    expect(properties[`--dialog-${name}`]).toBe(`${dialog[key]}rem`);
  }
  expect(properties["--dialog-scrim"]).toBe("light-dark(rgb(0 0 0 / 40%), rgb(0 0 0 / 60%))");
  for (const [name, key] of [
    ["min-width", "minWidth"],
    ["height", "height"],
    ["padding-inline", "paddingInline"],
    ["gap", "gap"],
  ] as const) {
    expect(properties[`--kbd-${name}`]).toBe(`${kbd[key]}rem`);
  }
  expect(properties["--font-weight-strong"]).toBe(`${fontWeight.strong}`);
  expect(properties["--icon-navigation"]).toBe(`${icon.navigation}rem`);
  expect(properties["--shell-gap-wide"]).toBe(`${shell.gapWide}rem`);
  for (const [name, value] of Object.entries(shell)) {
    if (!/[A-Z]/.test(name)) expect(properties[`--shell-${name}`]).toBe(`${value}rem`);
  }
});

test("a voice chat room's tiles hold their avatars and its lanes their reactions, declared as properties", () => {
  expect(voice.stage - 2 * voice.tilePadding).toBeGreaterThanOrEqual(avatar.preview);
  expect(voice.listener - 2 * voice.tilePadding).toBeGreaterThanOrEqual(avatar.post);
  expect(voice.lane).toBeGreaterThanOrEqual(voice.reaction);
  expect(properties["--voice-stage"]).toBe(`${voice.stage}rem`);
  expect(properties["--voice-halo-share"]).toBe(`${voice.haloShare * 100}%`);
  expect(properties["--voice-reaction-duration"]).toBe(`${voice.duration.reaction}ms`);
  expect(properties["--voice-speaking-duration"]).toBe(`${voice.duration.speaking}ms`);
});

test("the app's frame changes at widths that hold its columns, in order", () => {
  // From tablet, the rail and a main column at least a phone's width (320 px) fit within the edges.
  expect(breakpoint.tablet).toBeGreaterThanOrEqual(2 * shell.edge + shell.rail + shell.gap + 320 / 16);
  // From wide, the sidebar, the main column and the column beside it fit, the main column a little narrower at first.
  expect(breakpoint.wide).toBeGreaterThanOrEqual(shell.sidebar + shell.aside + 2 * shell.gapWide + 320 / 16);
  expect(breakpoint.tablet).toBeLessThan(breakpoint.desktop);
  expect(breakpoint.desktop).toBeLessThan(breakpoint.wide);
});

test("an entry's surface reaches past an action's tile and its focus ring", () => {
  const ring = (focusRing.gap + focusRing.width + focusRing.halo) / 16;
  expect(entry.reach).toBeGreaterThanOrEqual(iconButton.paddingInline + ring);
});

test("a feed's entries never overlap, and an entry brought to the top keeps its focus ring in view", () => {
  const ring = (focusRing.gap + focusRing.width + focusRing.halo) / 16;
  expect(feed.gap).toBeGreaterThanOrEqual(2 * entry.reach);
  expect(feed.scrollMargin).toBeGreaterThanOrEqual(ring);
});
