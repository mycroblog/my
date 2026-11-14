// SPDX-License-Identifier: AGPL-3.0-only

// The entry of the web client (ADR-0028 §1, §4): the Go server's HTML shell loads it, with the instance's runtime
// configuration in its data block and the locale and direction on its <html>. It opens the browser's session with the
// server (ADR-0090 §16) and starts the router. A server that cannot say whether there is a session leaves the app
// signed out, as when there is none.

import "@my/design-system/styles.css";
import { openSession } from "@my/app-data";
import { I18nProvider } from "@my/design-system";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { AppRoot } from "./app";
import { parseConfig } from "./config";
import { createAppRouter } from "./router";

const config = parseConfig(document.getElementById("config")?.textContent ?? null, location);
const locale = document.documentElement.lang || config.defaultLocale;
const data = await openSession({ server: config.apiOrigin, acceptLanguage: () => locale }).catch(() => null);
const router = createAppRouter({ data, config });

const root = document.getElementById("root");
if (!root) throw new Error("the shell has no #root");
createRoot(root).render(
  <StrictMode>
    <I18nProvider locale={locale}>
      <AppRoot router={router} />
    </I18nProvider>
  </StrictMode>,
);
