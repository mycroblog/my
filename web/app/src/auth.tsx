// SPDX-License-Identifier: AGPL-3.0-only

// The auth entry (ADR-0028 §4, ADR-0090 §16): the sign-in, registration and consent screens, each a full page load of
// the Go server's shell (internal/interface/authpages) with the screen's data in its data block and the locale and
// direction on its <html>, under the shell's own Content-Security-Policy. It renders the screen and moves the focus to
// what the server refused, and nothing else: no router, no session, no Service Worker; its forms post to the server.

import "@my/design-system/styles.css";
import { AuthScreen, parseAuthPage, type AuthPage } from "@my/app-features";
import { I18nProvider } from "@my/design-system";
import { StrictMode, useEffect } from "react";
import { createRoot } from "react-dom/client";
import wordmarkDark from "../../brand/Wordmark-DarkBg.svg";
import wordmarkLight from "../../brand/Wordmark-Primary-Color.svg";
import { focusOnLoad } from "./focus";

const page = parseAuthPage(document.getElementById("auth")?.textContent ?? null);

/** The screen, which takes the focus to the server's refusal once it is on the page. */
function Entry({ page }: { page: AuthPage }) {
  useEffect(() => {
    focusOnLoad(document);
  }, []);
  return <AuthScreen page={page} wordmark={{ light: wordmarkLight, dark: wordmarkDark }} />;
}

const root = document.getElementById("root");
if (!root) throw new Error("the shell has no #root");
createRoot(root).render(
  <StrictMode>
    <I18nProvider locale={document.documentElement.lang || "en"}>
      <Entry page={page} />
    </I18nProvider>
  </StrictMode>,
);
