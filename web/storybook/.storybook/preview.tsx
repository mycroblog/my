// SPDX-License-Identifier: AGPL-3.0-only

// What every story renders under: the stylesheet of @my/design-system and its emoji and CJK fonts, the page's color scheme (data-theme on <html>,
// as the server's shell sets it) on the theme's surface and text (preview.css), so that a story shows one theme, the
// toolbar's, and its locale, whose direction the parts read from the I18nProvider and <html>
// carries; ar-XB, the pseudo-locale made from en, reads from right to left. The toolbar switches both, and the viewport
// among the device sizes of @my/devtools/sizes (ADR-0169 §1) and the minimum width every page must fit (ADR-0110 §10).
// A component's story runs under the shortcuts of the app's keymap, its menus shown as the app shows them by the
// viewport (a sheet on a phone, a popover from `tablet`), in the system's sans; a screen's story
// (`parameters.screen`) is the whole app, which places its own, and shows its reader's own language where their
// account chose one. The loader asks @my/platform whether the browser runs
// view transitions, which a story of the design system cannot ask itself (ADR-0028 §17). The accessibility panel
// checks a story with the axe rules of the tags of ADR-0110 §9.

import "@my/design-system/styles.css";
import "@my/design-system/emoji.css";
import "@my/design-system/noto-sans.css";
import "@my/design-system/cjk.css";
import "./preview.css";
import { keymap } from "@my/app";
import { claimPressableStyle, I18nProvider, MenuSheet, ShortcutProvider, useLocale } from "@my/design-system";
import { breakpoint, fontFamily } from "@my/design-tokens";
import { sizes } from "@my/devtools/sizes";
import { namesCommandKey, supportsViewTransitions } from "@my/platform";
import type { Decorator, Preview } from "@storybook/react-vite";
import { useLayoutEffect, useSyncExternalStore, type ReactNode } from "react";

const locales = ["en", "ja", "ar-XB"] as const;
const themes = ["light", "dark"] as const;

/** The minimum width no page may scroll sideways at (ADR-0110 §10). */
const minimum = { width: 320, height: 640 };

const px = ({ width, height }: { width: number; height: number }) => ({ width: `${width}px`, height: `${height}px` });

const viewports = {
  minimum: { name: "Minimum width (320)", styles: px(minimum), type: "mobile" },
  mobile: { name: "Mobile", styles: px(sizes.mobile), type: "mobile" },
  tablet: { name: "Tablet", styles: px(sizes.tablet), type: "tablet" },
  pc: { name: "PC", styles: px(sizes.pc), type: "desktop" },
} as const;

/**
 * Puts the locale's language and direction on <html>, where the page's own styles and the browser read them. It stands
 * before the story, not around it, so that a story of the whole app, which writes its reader's own there, writes last.
 */
function Html() {
  const { locale, direction } = useLocale();
  useLayoutEffect(() => {
    document.documentElement.lang = locale;
    document.documentElement.dir = direction;
  }, [locale, direction]);
  return null;
}

const phone = `(width < ${String(breakpoint.tablet)}rem)`;
const subscribePhone = (listener: () => void) => {
  const list = window.matchMedia(phone);
  list.addEventListener("change", listener);
  return () => {
    list.removeEventListener("change", listener);
  };
};

/** Shows the menus as the app does: a sheet on a phone's viewport, a popover from `tablet`. */
function Menus({ children }: { children: ReactNode }) {
  const sheet = useSyncExternalStore(
    subscribePhone,
    () => window.matchMedia(phone).matches,
    () => false,
  );
  return <MenuSheet sheet={sheet}>{children}</MenuSheet>;
}

function Theme({ theme, children }: { theme: string; children: ReactNode }) {
  useLayoutEffect(() => {
    document.documentElement.setAttribute("data-theme", theme);
  }, [theme]);
  return children;
}

const page: Decorator = (Story, { globals, parameters }) => {
  const locale = typeof globals["locale"] === "string" ? globals["locale"] : "en";
  const theme = typeof globals["theme"] === "string" ? globals["theme"] : "light";
  return (
    <Theme theme={theme}>
      <I18nProvider locale={locale}>
        <Html />
        {parameters["screen"] === true ? (
          <Story />
        ) : (
          <ShortcutProvider keymap={keymap} mac={namesCommandKey()}>
            <Menus>
              <div style={{ fontFamily: fontFamily.sans }}>
                <Story />
              </div>
            </Menus>
          </ShortcutProvider>
        )}
      </I18nProvider>
    </Theme>
  );
};

// As the app does: the pressables take their rule from styles.css, not from a style React Aria puts in the head.
claimPressableStyle();

/** The tags of the axe rules a story must pass (ADR-0110 §9). */
const axeTags = ["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22aa"];

const preview: Preview = {
  decorators: [page],
  loaders: [() => ({ viewTransitions: supportsViewTransitions() })],
  globalTypes: {
    theme: {
      description: "Color scheme",
      toolbar: { title: "Theme", icon: "mirror", items: [...themes], dynamicTitle: true },
    },
    locale: {
      description: "Locale; ar-XB reads from right to left",
      toolbar: {
        title: "Locale",
        icon: "globe",
        items: locales.map((value) => ({ value, title: value === "ar-XB" ? "ar-XB (RTL)" : value })),
        dynamicTitle: true,
      },
    },
  },
  initialGlobals: { theme: "light", locale: "en" },
  parameters: {
    layout: "padded",
    viewport: { options: viewports },
    a11y: { test: "error", options: { runOnly: { type: "tag", values: axeTags } } },
  },
};

export default preview;
