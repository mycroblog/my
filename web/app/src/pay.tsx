// SPDX-License-Identifier: AGPL-3.0-only

// The payment entry (ADR-0074 §11): the checkout and portal pages of a provider without hosted ones, each a full page
// load of the Go server's shell (internal/interface/payments) with the page's data in its data block and the locale and
// direction on its <html>, under the pages' own Content-Security-Policy. It renders the page and moves the focus to what
// the server refused, and nothing else: no router, no session, no Service Worker; its forms post to the server. Its
// scripts are listed with every release (scripts/payment-scripts.ts), and the only other script it loads is payjp.js.

import "@my/design-system/styles.css";
import "@my/design-system/noto-sans.css";
import "@my/design-system/cjk.css";
import { parsePayPage, PayScreen, type PayPage } from "@my/app-features/pay";
import { readEveryLocale } from "@my/app-features/strings-every";
import { claimPressableStyle, I18nProvider } from "@my/design-system";
import { StrictMode, useEffect } from "react";
import { createRoot } from "react-dom/client";
import wordmarkDark from "../../brand/Wordmark-DarkBg.svg";
import wordmarkLight from "../../brand/Wordmark-Primary-Color.svg";
import { focusOnLoad } from "./focus";

const page = parsePayPage(document.getElementById("pay")?.textContent ?? null);
// Every locale's strings, imported with the page: it may load no script on demand (scripts/payment-scripts.ts).
readEveryLocale();

/** The page, which takes the focus to the server's refusal once it is on the page. */
function Entry({ page }: { page: PayPage }) {
  useEffect(() => {
    focusOnLoad(document);
  }, []);
  return <PayScreen page={page} wordmark={{ light: wordmarkLight, dark: wordmarkDark }} />;
}

const root = document.getElementById("root");
if (!root) throw new Error("the shell has no #root");
claimPressableStyle();
createRoot(root).render(
  <StrictMode>
    <I18nProvider locale={document.documentElement.lang || "en"}>
      <Entry page={page} />
    </I18nProvider>
  </StrictMode>,
);
