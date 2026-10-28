// SPDX-License-Identifier: AGPL-3.0-only
// The viewports of the stories of the app shell (AppShell, AppNavigation). The shell answers to the viewport, not to a
// container (web-client/README.md D2), so its stories set the viewport (Storybook's viewport global, which
// @storybook/addon-vitest applies to the test page) instead of the frame's width, and the screenshot of such a story is
// taken once, at its viewport (.storybook/vitest.setup.ts). A story at a viewport keeps its content within the
// viewport's height: the shell is as tall as the screen, and the test page paints only what fits it.

/** One viewport per layout of the shell, each inside the range of its breakpoint. */
const shellViewports = {
  phone: { name: "Phone, below md: the tab bar", styles: { width: "360px", height: "640px" }, type: "mobile" },
  short: {
    name: "Short screen (short:): the smaller tab bar",
    styles: { width: "568px", height: "320px" },
    type: "mobile",
  },
  zoomed: { name: "400% zoom (short:): 320 × 225 CSS px", styles: { width: "320px", height: "225px" }, type: "mobile" },
  tablet: { name: "Tablet, md: the rail", styles: { width: "800px", height: "640px" }, type: "tablet" },
  laptop: { name: "Laptop, lg: the sidebar", styles: { width: "1100px", height: "720px" }, type: "desktop" },
  desktop: { name: "Desktop, xl: the third column", styles: { width: "1400px", height: "800px" }, type: "desktop" },
} as const;

export type ShellViewport = keyof typeof shellViewports;

/** The parameters of a story file of the shell: no padding around the story, and the viewports above. */
export const shellParameters = { layout: "fullscreen", viewport: { options: shellViewports } } as const;

/** The globals of a story at `viewport`. */
export function atViewport(viewport: ShellViewport) {
  return { viewport: { value: viewport, isRotated: false } };
}
