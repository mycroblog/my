// SPDX-License-Identifier: AGPL-3.0-only
// The report of the web client's uncaught errors to its own server (ADR-0103 §6, POST /client_errors), for the
// operators. Four places catch them: the router's defaultOnCatch for a page that fails (router.ts; a route's own
// `onCatch` replaces the default, so it calls `report` too), the `onError` of every react-error-boundary outside the
// routes (useBoundaryErrorReport), React's onUncaughtError for what no boundary catches (main.tsx), and the window's
// `error` and `unhandledrejection` events (listen). onUncaughtError replaces React's default `reportError`, so an
// error React did not catch never reaches the window's `error` as well; the same error is still sent only once.
//
// Not reported: the errors of the API client (an ApiError, a NetworkError: the server reports its own failures and
// the network is the user's), notFound() and redirect() of the router, the aborts of requests, a chunk that a deploy
// removed (`vite:preloadError`, which lib/pwa reloads the page for), and errors whose stack has no script of the
// app's own origin (a browser extension, a script of another site).
//
// Each error is sent once per page view, at most `maxReportsPerPage` of them, with `fetch(..., { keepalive: true })`
// through the API client, which adds the access token, so that the server can tell the reports of signed-in users.
// While the page is being left (`pagehide`) the report goes with `navigator.sendBeacon` instead, which cannot carry
// the token. The keepalive requests of a page share 64 KiB with the leave request of a voice call, so a body stays
// under 16 KiB.
import { isNotFound, isRedirect } from "@tanstack/react-router";
import type { components } from "@mycroblog/mykit";
import { createContext, type ReactNode, use } from "react";
import type { Api } from "./api/client";
import { errorKind } from "./errors";

/** How an error was caught (ClientErrorReport.kind). */
type ClientErrorReport = components["schemas"]["ClientErrorReport"];

export type ClientErrorKind = ClientErrorReport["kind"];

/** The most reports one page view sends; the rest are dropped. */
export const maxReportsPerPage = 20;

/** The largest body in bytes, the server's limit. */
const maxBodyBytes = 16 * 1024;

/** The bounds of the fields (ClientErrorReport). */
const maxNameLength = 256;
const maxMessageLength = 4096;
const maxStackLength = 16384;
const maxReleaseLength = 64;
const maxRouteLength = 256;

/**
 * The messages of a dynamic import that failed: Chromium, Firefox, and Safari. The chunk of a version that a deploy
 * removed, which `vite:preloadError` reloads the page for (lib/pwa).
 */
const chunkLoadFailure = /Failed to fetch dynamically imported module|error loading dynamically imported module|Importing a module script failed/;

/** The version of the build (VITE_MY_RELEASE, which the image build sets from its VERSION), if it has one. */
const buildRelease: string | undefined = import.meta.env.VITE_MY_RELEASE || undefined;

declare global {
  interface ImportMetaEnv {
    /** The version of the build, which the image build sets (Dockerfile). */
    readonly VITE_MY_RELEASE?: string;
  }
}

/** The origin the app's own scripts are loaded from: this module's. */
const ownScriptOrigin = new URL(import.meta.url).origin;

export type ErrorReporterOptions = {
  api: Pick<Api, "fetch">;
  /** The origin of the client API, where a beacon goes. */
  apiOrigin: string;
  /** The version of the build; `buildRelease` by default. */
  release?: string | undefined;
  /**
   * Whether a stack has a frame of the app's own scripts; by default whether it names a URL of the origin the app's
   * scripts are loaded from.
   */
  isOwnStack?: ((stack: string) => boolean) | undefined;
};

/** The error reports of one page view. */
export class ErrorReporter {
  readonly #api: Pick<Api, "fetch">;
  readonly #beaconURL: string;
  readonly #release: string | undefined;
  readonly #isOwnStack: (stack: string) => boolean;
  /** The errors sent, by their name, message and stack. */
  readonly #sent = new Set<string>();
  /** The errors of a chunk that failed to load (`vite:preloadError`). */
  readonly #preloadErrors = new WeakSet<object>();
  /** Returns the ID of the route shown; none until a router is attached. */
  #routeId: () => string | undefined = () => undefined;
  /** Whether the page is being left (between `pagehide` and `pageshow`). */
  #leaving = false;

  constructor({ api, apiOrigin, release = buildRelease, isOwnStack }: ErrorReporterOptions) {
    this.#api = api;
    this.#beaconURL = new URL("/api/v1/client_errors", apiOrigin).href;
    this.#release = release;
    this.#isOwnStack = isOwnStack ?? ((stack) => stack.includes(`${ownScriptOrigin}/`));
  }

  /** Reads the route of each report from `router`: the ID of its deepest match, a pattern rather than the URL. */
  attachRouter(router: { state: { matches: readonly { routeId: string }[] } }): void {
    this.#routeId = () => router.state.matches.at(-1)?.routeId;
  }

  /**
   * Listens to `target` for the errors nothing caught (`error`), the promise rejections nothing handled
   * (`unhandledrejection`), the chunks that failed to load (`vite:preloadError`), and the page being left. Returns
   * the function that stops listening.
   */
  listen(target: Window): () => void {
    const onError = (event: ErrorEvent) => {
      // A cross-origin script's error has no error object ("Script error."), and nothing to report.
      if (event.error !== null && event.error !== undefined) this.report(event.error, "uncaught");
    };
    const onRejection = (event: PromiseRejectionEvent) => {
      this.report(event.reason, "unhandledrejection");
    };
    const onPreloadError = (event: Event) => {
      const payload = (event as Event & { payload?: unknown }).payload;
      if (typeof payload === "object" && payload !== null) this.#preloadErrors.add(payload);
    };
    const onPageHide = () => {
      this.#leaving = true;
    };
    const onPageShow = () => {
      this.#leaving = false;
    };
    target.addEventListener("error", onError);
    target.addEventListener("unhandledrejection", onRejection);
    target.addEventListener("vite:preloadError", onPreloadError);
    target.addEventListener("pagehide", onPageHide);
    target.addEventListener("pageshow", onPageShow);
    return () => {
      target.removeEventListener("error", onError);
      target.removeEventListener("unhandledrejection", onRejection);
      target.removeEventListener("vite:preloadError", onPreloadError);
      target.removeEventListener("pagehide", onPageHide);
      target.removeEventListener("pageshow", onPageShow);
    };
  }

  /** Sends `error`, caught as `kind`, unless it is not reported or was sent already in this page view. */
  report(error: unknown, kind: ClientErrorKind): void {
    const report = this.#reportOf(error, kind);
    if (report === null) return;
    const key = `${report.name}\n${report.message}\n${report.stack}`;
    if (this.#sent.has(key) || this.#sent.size >= maxReportsPerPage) return;
    this.#sent.add(key);
    if (this.#leaving && typeof navigator.sendBeacon === "function") {
      navigator.sendBeacon(this.#beaconURL, new Blob([JSON.stringify(report)], { type: "application/json" }));
      return;
    }
    // A report that fails is not reported, nor sent again.
    this.#api.fetch.POST("/client_errors", { body: report, keepalive: true }).catch(() => undefined);
  }

  /** The report of `error`, or null when it is not reported. */
  #reportOf(error: unknown, kind: ClientErrorKind): ClientErrorReport | null {
    if (!(error instanceof Error)) return null;
    if (isNotFound(error) || isRedirect(error) || errorKind(error) !== "unexpected" || error.name === "AbortError") return null;
    if (this.#preloadErrors.has(error) || chunkLoadFailure.test(error.message)) return null;
    const stack = error.stack ?? "";
    if (!this.#isOwnStack(stack)) return null;
    const report: ClientErrorReport = {
      name: error.name.slice(0, maxNameLength),
      message: error.message.slice(0, maxMessageLength),
      stack: stack.slice(0, maxStackLength),
      kind,
    };
    if (this.#release !== undefined) report.release = this.#release.slice(0, maxReleaseLength);
    const route = this.#routeId();
    if (route !== undefined) report.route = route.slice(0, maxRouteLength);
    return fitBody(report);
  }
}

/** `report` with its stack, and then its message, cut so that its JSON is at most maxBodyBytes of UTF-8. */
function fitBody(report: ClientErrorReport): ClientErrorReport {
  const encoder = new TextEncoder();
  let fitted = report;
  for (const field of ["stack", "message"] as const) {
    let over = encoder.encode(JSON.stringify(fitted)).length - maxBodyBytes;
    while (over > 0 && fitted[field].length > 0) {
      // A character is at least one byte of UTF-8, and its escape in JSON at most six.
      fitted = { ...fitted, [field]: fitted[field].slice(0, Math.max(0, fitted[field].length - over)) };
      over = encoder.encode(JSON.stringify(fitted)).length - maxBodyBytes;
    }
  }
  return fitted;
}

const ErrorReporterContext = createContext<ErrorReporter | null>(null);

export type ErrorReporterProviderProps = { reporter: ErrorReporter; children: ReactNode };

/** Gives `useBoundaryErrorReport()` to its children. */
export function ErrorReporterProvider({ reporter, children }: ErrorReporterProviderProps) {
  return <ErrorReporterContext value={reporter}>{children}</ErrorReporterContext>;
}

/**
 * The `onError` of a react-error-boundary: reports what the boundary caught as a `boundary_error`. Outside an
 * ErrorReporterProvider (a component rendered without the app, in a test) it reports nothing.
 */
export function useBoundaryErrorReport(): (error: unknown) => void {
  const reporter = use(ErrorReporterContext);
  return (error) => {
    reporter?.report(error, "boundary_error");
  };
}
