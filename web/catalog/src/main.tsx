// SPDX-License-Identifier: AGPL-3.0-only

import "@my/design-system/styles.css";
import { keymap } from "@my/app";
import { I18nProvider, ShortcutProvider, useLocale } from "@my/design-system";
import { namesCommandKey } from "@my/platform";
import { StrictMode, useEffect, type ReactNode } from "react";
import { createRoot } from "react-dom/client";
import { Catalog } from "./Catalog";
import { AuthPreview } from "./auth/AuthPreview";
import { Notifications } from "./screens/Notifications";
import { PostDetail } from "./screens/PostDetail";
import { Profile } from "./screens/Profile";
import { AppPreview } from "./timeline/AppPreview";

// The catalog's pages (Catalog.tsx), under the shortcuts of the app's keymap; at /app/timeline the app's home screen,
// which places its own; at /app/auth the auth entry's screens, which are full pages outside the app; at /app/post,
// /app/notifications and /app/profile the rough screens in the app's shell (screens/), which place their own.
// /timeline, the timeline's former address, which bookmarks and older notes name, leads there.
const path = location.pathname.replace(/\/+$/, "");
if (path === "/timeline") location.replace(`/app/timeline${location.search}${location.hash}`);
const timeline = path === "/app/timeline";
const auth = path === "/app/auth";
const roughs: Readonly<Record<string, () => ReactNode>> = {
  "/app/post": PostDetail,
  "/app/notifications": Notifications,
  "/app/profile": Profile,
};
const Rough = roughs[path];

// The page's locale: `?lang=`, which the overlay's locale switch sets (en, ja, ar-XB), else <html lang>. The parts
// read it, and its direction, from the I18nProvider; the page's <html> carries both.
const locale = (() => {
  const asked = new URLSearchParams(location.search).get("lang");
  try {
    return Intl.getCanonicalLocales(asked ?? [])[0] ?? document.documentElement.lang;
  } catch {
    return document.documentElement.lang;
  }
})();

function Html({ children }: { children: ReactNode }) {
  const { locale, direction } = useLocale();
  useEffect(() => {
    document.documentElement.lang = locale;
    document.documentElement.dir = direction;
  }, [locale, direction]);
  return children;
}

const root = document.getElementById("root");
if (!root) throw new Error("index.html has no #root");
createRoot(root).render(
  <StrictMode>
    <I18nProvider locale={locale}>
      <Html>
        {auth ? (
          <AuthPreview />
        ) : Rough ? (
          <Rough />
        ) : timeline ? (
          <AppPreview />
        ) : (
          <ShortcutProvider keymap={keymap} mac={namesCommandKey()}>
            <Catalog path={location.pathname} />
          </ShortcutProvider>
        )}
      </Html>
    </I18nProvider>
  </StrictMode>,
);
