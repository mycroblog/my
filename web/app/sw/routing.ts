// SPDX-License-Identifier: AGPL-3.0-only

// Which requests the Service Worker answers, and how (ADR-0028 §3): a pure function of the request, so that the rules
// are tested apart from the worker.

/**
 * The paths whose navigations the worker leaves to the browser: the server's other endpoints (its reserved prefixes,
 * internal/interface/web/handler.go), the server-rendered sign-in and consent pages among them, which need the network
 * and must never turn into the app's shell, and the Web App Manifest.
 */
const passThroughPaths = [
  "/api",
  "/oauth",
  "/auth",
  "/mail",
  "/.well-known",
  "/nodeinfo",
  "/ap",
  "/media",
  "/uploads",
  "/theme",
  "/brand",
  "/licenses",
  "/robots.txt",
  "/manifest.webmanifest",
] as const;

/**
 * - `navigation`: the network, with a timeout, falling back to the precached shell;
 * - `media`: cache first, bounded: the public images under /media/;
 * - `theme`: cache first, bounded: the instance theme's stylesheets (ADR-0059), named by their revision;
 * - `brand`: cache first, bounded: the instance's own icon and logos (ADR-0059), named by their revision;
 * - `font`: cache first, bounded: the build's font splits (the Noto Color Emoji of ADR-0091 §10, and Noto Sans and
 *   Noto Sans CJK of ADR-0009 §5), named by the hash of their content, which the precache leaves out (tens of MB);
 * - `network`: not answered by the worker at all (the client API, every write, another origin, anything else).
 */
export type Handling = "navigation" | "media" | "theme" | "brand" | "font" | "network";

/** The fonts of the build: assets/<name>-<hash>.woff2. */
const fontPath = /^\/assets\/[^/]+\.woff2$/;

/** The parts of a fetch event's request that decide how it is answered. */
export type RequestFacts = {
  url: string;
  method: string;
  mode: string;
  destination: string;
  /** Whether the request carries an Authorization header. */
  authorized: boolean;
};

const under = (path: string, prefixes: readonly string[]) =>
  prefixes.some((prefix) => path === prefix || path.startsWith(`${prefix}/`));

/** How the worker answers a request; `origin` is its own origin. */
export function classify(request: RequestFacts, origin: string): Handling {
  if (request.method !== "GET") return "network";
  const url = new URL(request.url);
  if (url.origin !== origin) return "network";
  if (request.mode === "navigate") return under(url.pathname, passThroughPaths) ? "network" : "navigation";
  // A request with credentials in a header is never cached.
  if (request.authorized) return "network";
  // Only images: a video's range requests get partial answers, which a cache cannot replay.
  if (under(url.pathname, ["/media"]) && request.destination === "image") return "media";
  if (under(url.pathname, ["/theme"]) && request.destination === "style") return "theme";
  if (under(url.pathname, ["/brand"]) && request.destination === "image") return "brand";
  if (fontPath.test(url.pathname) && request.destination === "font") return "font";
  return "network";
}

/** Whether a request is the reader signing out (/auth/sign_out, posted or followed): every cache goes with it. */
export function signsOut(url: string, origin: string): boolean {
  const parsed = new URL(url);
  return parsed.origin === origin && parsed.pathname === "/auth/sign_out";
}
