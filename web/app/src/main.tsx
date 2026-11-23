// SPDX-License-Identifier: AGPL-3.0-only

// The entry of the web client (ADR-0028 §1, §4): the Go server's HTML shell loads it, with the instance's runtime
// configuration in its data block. It resolves the browser's locale (locale.ts), which its requests name and under
// which it places the app; the app shows the reader's own language and time zone, and puts the locale and its
// direction on the <html>, in place of the shell's, which are those of what a page previews (reader.tsx). It opens the
// browser's session with the server (ADR-0090 §16) as it loads the strings of the reader's locale (strings.ts), with
// a session its stream (ADR-0028 §18), and starts the router. The colour scheme the device chose shows before all of it.
// A server that cannot say whether there is a session leaves the app signed out, as when there is none, but for what
// the device keeps of the reader, which stays (reader.tsx). Signing out in another tab makes this one leave. Every request
// goes through one watch of the server, which tells its maintenance, an outage and a too old build (ADR-0088, ADR-0028
// §12). The build registers its Service Worker (ADR-0028 §3). Once the instance is read, the app measures the session if
// the instance measures it (measure.ts, ADR-0167 §3).

import "@my/design-system/styles.css";
import "@my/design-system/emoji.css";
import "@my/design-system/noto-sans.css";
import "@my/design-system/cjk.css";
import { instanceQuery, onSignedOutElsewhere, openSession, startStream, visitorData, watchServer } from "@my/app-data";
import { inCall } from "@my/app-features";
import { claimPressableStyle, I18nProvider } from "@my/design-system";
import {
  browserLanguages,
  followThemeChoice,
  readDeviceValue,
  registerServiceWorker,
  writeDeviceValue,
} from "@my/platform";

/** The device's key of when the page last loaded again for a chunk of an older build. */
const reloadedKey = "my:reloaded-for-chunk";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { AppRoot } from "./app";
import { parseConfig } from "./config";
import { readerLocale } from "./locale";
import { startMeasuring } from "./measure";
import { keptLocale } from "./reader";
import { createAppRouter } from "./router";
import { followSignOut } from "./sign-out";
import { loadStrings } from "./strings";

// The colour scheme this device chose for the app, from the first paint (the shell's is the system's).
followThemeChoice();
const config = parseConfig(document.getElementById("config")?.textContent ?? null, location);
// The browser's locale: what the requests name (Accept-Language), and what the app shows a reader whose account
// chose no language.
const locale = readerLocale(browserLanguages(), config.defaultLocale);
const server = {
  server: config.apiOrigin,
  acceptLanguage: () => locale,
  watch: watchServer({ server: config.apiOrigin }),
};
// The strings of the browser's locale, and of the one the account chose when this device last read its preferences,
// before the first paint (reader.tsx); the other locales' only when the reader changes to one. One that fails to load
// leaves its locale in en.
const kept = keptLocale();
const strings = Promise.all(
  [locale, ...(kept === undefined ? [] : [kept])].map((tag) => loadStrings(tag).catch(() => undefined)),
);
// The session's data; null when the server said the browser has none, undefined when it could not say (it did not
// answer, or failed): the app then shows as signed out, and keeps what the device holds of the reader.
const [opened] = await Promise.all([openSession(server).catch(() => undefined), strings]);
const data = opened ?? null;
if (data !== null) startStream(data);
// The Service Worker of the build (sw/sw.ts); the dev server has none.
if (import.meta.env.PROD) registerServiceWorker("/sw.js");
// A chunk of an older build that a newer one replaced (its worker took over and dropped the old precache, and the
// server serves the new build alone) fails to load: the page loads again on the build the server serves, once a
// minute at most, so that a chunk that fails for another reason never loops (ADR-0028 §12). Never while the reader is
// in a call, which a load would end: the navigation that needed the chunk fails instead, and the call goes on.
addEventListener("vite:preloadError", (event) => {
  if (inCall()) return;
  const last = Number(readDeviceValue(reloadedKey) ?? "0");
  if (Date.now() - last < 60_000) return;
  event.preventDefault();
  writeDeviceValue(reloadedKey, String(Date.now()));
  location.reload();
});
const visitor = visitorData(server);
const router = createAppRouter({ data, config });
// Signing out in one tab signs every tab of the browser out.
if (data !== null)
  onSignedOutElsewhere(data, () => {
    followSignOut(router.options.context);
  });

const root = document.getElementById("root");
if (!root) throw new Error("the shell has no #root");
// The style React Aria would put in the head is refused by the shell's policy; styles.css holds its rule.
claimPressableStyle();
createRoot(root).render(
  <StrictMode>
    <I18nProvider locale={locale}>
      <AppRoot router={router} visitor={visitor} unasked={opened === undefined} />
    </I18nProvider>
  </StrictMode>,
);
// The UX measurements of this browser session, if the instance takes them and drew it.
const read = data ?? visitor;
read.queryClient.query({ ...instanceQuery(read), staleTime: Number.POSITIVE_INFINITY }).then(
  (instance) => {
    startMeasuring({ router, instance, server: config.apiOrigin });
  },
  () => undefined,
);
