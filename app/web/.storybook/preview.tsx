// SPDX-License-Identifier: AGPL-3.0-only
// The Storybook preview of app/web (ADR-0112 §1): the app's stylesheet, the locale and theme toolbars, the viewports of
// the screen stories, and the accessibility gate that fails a story test on any axe violation. A screen story renders
// the whole app (src/test/stories.tsx), which sets lang and dir of <html> from the locale itself.
import type { Decorator, Preview } from "@storybook/react-vite";
import { type ReactNode, useEffect } from "react";
import "../src/index.css";

/** The locales of the toolbar: the source locale, Japanese and the two pseudo-locales (ADR-0009). */
const locales = {
  en: "English",
  ja: "日本語",
  "en-XA": "en-XA (accented pseudo-locale)",
  "ar-XB": "ar-XB (right-to-left pseudo-locale)",
} as const;

const themes = ["light", "dark"] as const;
type Theme = (typeof themes)[number];

function isTheme(value: unknown): value is Theme {
  return themes.some((theme) => theme === value);
}

/** The viewports of the screen stories: the 320 CSS px of WCAG 1.4.10, a phone, and a desktop. */
const viewports = {
  narrow: { name: "320 px (WCAG 1.4.10 reflow)", styles: { width: "320px", height: "640px" }, type: "mobile" },
  phone: { name: "Phone", styles: { width: "390px", height: "844px" }, type: "mobile" },
  desktop: { name: "Desktop", styles: { width: "1280px", height: "800px" }, type: "desktop" },
} as const;

/** Applies the theme global the way lib/theme does: data-theme on <html>. */
function ThemeRoot({ theme, children }: { theme: Theme; children: ReactNode }) {
  useEffect(() => {
    document.documentElement.dataset["theme"] = theme;
  }, [theme]);
  return children;
}

const withTheme: Decorator = (Story, { globals }) => (
  <ThemeRoot theme={isTheme(globals["theme"]) ? globals["theme"] : "light"}>
    <Story />
  </ThemeRoot>
);

const preview: Preview = {
  decorators: [withTheme],
  globalTypes: {
    locale: {
      description: "Locale the app starts in and the signed-in viewer prefers",
      toolbar: {
        title: "Locale",
        icon: "globe",
        items: Object.entries(locales).map(([value, title]) => ({ value, title })),
        dynamicTitle: true,
      },
    },
    theme: {
      description: "Theme: data-theme on <html>",
      toolbar: {
        title: "Theme",
        icon: "mirror",
        items: [
          { value: "light", title: "Light", icon: "sun" },
          { value: "dark", title: "Dark", icon: "moon" },
        ],
        dynamicTitle: true,
      },
    },
  },
  initialGlobals: { locale: "en", theme: "light" },
  parameters: {
    // A screen fills the page, as the app does.
    layout: "fullscreen",
    // The theme paints the page (body uses --color-surface); Storybook's backgrounds would hide it.
    backgrounds: { disable: true },
    viewport: { options: viewports },
    a11y: {
      // Every story test fails on an axe violation (web-client/README.md Y1).
      test: "error",
      options: { runOnly: { type: "tag", values: ["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22aa"] } },
    },
  },
};

export default preview;
