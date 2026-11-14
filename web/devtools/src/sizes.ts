// SPDX-License-Identifier: AGPL-3.0-only

// The device viewports a page is judged at (ADR-0169 §1), in their natural orientation: the Storybook host's viewports
// offer them, and web/storybook/shot.ts shoots a story at them.

export const sizes = {
  mobile: { width: 375, height: 812 },
  tablet: { width: 768, height: 1024 },
  pc: { width: 1280, height: 800 },
} as const;
