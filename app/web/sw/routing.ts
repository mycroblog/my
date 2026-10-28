// SPDX-License-Identifier: AGPL-3.0-only
// Which requests the Service Worker answers, and how (ADR-0028). A pure function, so that the rules are
// unit-tested apart from the worker (routing.test.ts).

/**
 * Paths whose navigations the worker leaves to the browser: the server's other endpoints, including the
 * server-rendered sign-in and consent pages (/auth, /oauth), which need the network and must never turn
 * into the SPA's shell. They are the server's reserved prefixes (internal/interface/web/handler.go) and
 * the Web App Manifest.
 */
const passThroughPaths = [
  "/api",
  "/oauth",
  "/auth",
  "/.well-known",
  "/nodeinfo",
  "/ap",
  "/media",
  "/uploads",
  "/theme",
  "/brand",
  "/licenses",
  "/manifest.webmanifest",
] as const;

/**
 * - `navigation`: network only with a timeout, falling back to the precached shell;
 * - `media`: cache first, bounded;
 * - `theme`: cache first, bounded: the instance theme's stylesheets (ADR-0059), whose URL names their revision;
 * - `brand`: cache first, bounded: the instance's own icon and logos (ADR-0116), whose URL names their revision;
 * - `emoji-font`: cache first, bounded: the Noto Color Emoji splits of the build (ADR-0095 §3), whose URL carries the
 *   hash of their content and which the precache leaves out (vite.config.ts), fetched only when a page shows an emoji;
 * - `network`: not answered by the worker at all (the client API, every non-GET, other origins).
 */
export type Handling = "navigation" | "media" | "theme" | "brand" | "emoji-font" | "network";

/** The emoji fonts of the build: assets/noto-emoji-<format>-<split>-<hash>.woff2 (@my/ui/emoji.css). */
const emojiFontPath = /^\/assets\/noto-emoji-[\w-]+\.woff2$/;

/** The parts of a fetch event's Request that decide its handling. */
export type RequestFacts = {
  url: string;
  method: string;
  mode: string;
  destination: string;
  /** Whether the request carries an Authorization header. */
  authorized: boolean;
};

/** Decides how the worker answers a request; `origin` is the worker's own origin. */
export function classify(request: RequestFacts, origin: string): Handling {
  if (request.method !== "GET") return "network";
  const url = new URL(request.url);
  if (url.origin !== origin) return "network";
  if (request.mode === "navigate") return underAny(url.pathname, passThroughPaths) ? "network" : "navigation";
  // Public media (overview.md: blob keys under media/ are public and served at /media/). Only images: a
  // video's range requests get partial responses, which a cache cannot replay. A request with credentials
  // in a header is never cached.
  if (underAny(url.pathname, ["/media"]) && request.destination === "image" && !request.authorized) return "media";
  // The instance theme's stylesheet (ADR-0059): its content never changes, so the offline shell gets the theme
  // it was last shown with.
  if (underAny(url.pathname, ["/theme"]) && request.destination === "style" && !request.authorized) return "theme";
  // The instance's icon and logos (ADR-0116): their content never changes either, so the offline shell shows the logo
  // it was last shown with.
  if (underAny(url.pathname, ["/brand"]) && request.destination === "image" && !request.authorized) return "brand";
  // The emoji fonts (ADR-0095 §3): too large to precache, and named by the hash of their content.
  if (emojiFontPath.test(url.pathname) && request.destination === "font") return "emoji-font";
  return "network";
}

function underAny(path: string, prefixes: readonly string[]): boolean {
  return prefixes.some((prefix) => path === prefix || path.startsWith(`${prefix}/`));
}
