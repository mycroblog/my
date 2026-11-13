// SPDX-License-Identifier: AGPL-3.0-only

import "@my/design-system/styles.css";
import { I18nProvider, useLocale } from "@my/design-system";
import { StrictMode, useEffect, type ReactNode } from "react";
import { createRoot } from "react-dom/client";
import { Catalog } from "./Catalog";
import { Timeline } from "./timeline/Timeline";

// The catalog page at /, and the rough home timeline screen at /timeline.
const Page = location.pathname.replace(/\/+$/, "") === "/timeline" ? Timeline : Catalog;

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
        <Page />
      </Html>
    </I18nProvider>
  </StrictMode>,
);
