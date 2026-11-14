// SPDX-License-Identifier: AGPL-3.0-only

// The entry of the web client (ADR-0028 §1, §4): the Go server's HTML shell loads it, with the instance's runtime
// configuration in its data block. It renders in the reader's locale (locale.ts), and puts that locale and its
// direction on the <html>, in place of the shell's, which are those of what a page previews. It opens the browser's
// session with the server (ADR-0090 §16), with a session its stream (ADR-0028 §18), and starts the router. A server
// that cannot say whether there is a session leaves the app signed out, as when there is none. Every request goes
// through one watch of the server, which tells its maintenance, an outage and a too old build (ADR-0088, ADR-0028 §12).

import "@my/design-system/styles.css";
import { openSession, startStream, visitorData, watchServer } from "@my/app-data";
import { I18nProvider, useLocale } from "@my/design-system";
import { browserLanguages } from "@my/platform";
import { StrictMode, useLayoutEffect, type ReactNode } from "react";
import { createRoot } from "react-dom/client";
import { AppRoot } from "./app";
import { parseConfig } from "./config";
import { readerLocale } from "./locale";
import { createAppRouter } from "./router";

const config = parseConfig(document.getElementById("config")?.textContent ?? null, location);
const locale = readerLocale(browserLanguages(), config.defaultLocale);
const server = {
  server: config.apiOrigin,
  acceptLanguage: () => locale,
  watch: watchServer({ server: config.apiOrigin }),
};
const data = await openSession(server).catch(() => null);
if (data !== null) startStream(data);
const visitor = visitorData(server);
const router = createAppRouter({ data, config });

/** The page's <html> in the I18nProvider's locale and direction, before the first paint. */
function Html({ children }: { children: ReactNode }) {
  const { locale, direction } = useLocale();
  useLayoutEffect(() => {
    document.documentElement.lang = locale;
    document.documentElement.dir = direction;
  }, [locale, direction]);
  return children;
}

const root = document.getElementById("root");
if (!root) throw new Error("the shell has no #root");
createRoot(root).render(
  <StrictMode>
    <I18nProvider locale={locale}>
      <Html>
        <AppRoot router={router} visitor={visitor} />
      </Html>
    </I18nProvider>
  </StrictMode>,
);
