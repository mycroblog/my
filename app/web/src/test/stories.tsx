// SPDX-License-Identifier: AGPL-3.0-only
// The screen stories of app/web (ADR-0112 §1): a story renders the whole app at a route, as an integration test does
// (render.tsx), with MSW's worker in the browser playing the server with the default handlers of every feature
// (msw/browser.ts) and the story's own overrides. The locale is Storybook's `locale` global (../../.storybook/preview.tsx):
// the page starts in it, and the signed-in viewer's preferences name it, so the app stays in it after signing in.
import type { Loader, StoryContext } from "@storybook/react-vite";
import { createMemoryHistory, isNotFound, isRedirect } from "@tanstack/react-router";
import { delay, http, HttpResponse, type RequestHandler } from "msw";
import { type ComponentType, useEffect, useState } from "react";
import { expect, waitFor } from "storybook/test";
import { destroyAnnouncer } from "react-aria/private/live-announcer/LiveAnnouncer";
import { App } from "../App";
import type { RuntimeConfig } from "../lib/config";
import { ApiError, NetworkError } from "../lib/errors";
import { applyDocumentLocale, type I18n, loadI18n } from "../lib/i18n";
import { createAppServices } from "../lib/services";
import { signInKey, type StoredSignIn } from "../lib/storage";
import { createAppRouter } from "../router";
import { preferencesFixture, type Schemas, viewerFixture } from "./fixtures";
import { worker } from "./msw/browser";
import { apiBase, problem, signedInHandler, withViewer } from "./msw/http";

/** The parameters of a screen story: where the app opens, whether the viewer is signed in, and MSW's overrides. */
export type ScreenParameters = {
  /** The address the app opens at. */
  path: string;
  /** Whether the session resumes as the default handlers' viewer; signed out by default. */
  signedIn?: boolean;
  /** Handlers that answer before the default ones, for the story's state (empty, loading, error, long text). */
  handlers?: RequestHandler[];
  /** The signed-in viewer's preferences besides the defaults; their locale is always the story's. */
  preferences?: Partial<Schemas["Preferences"]>;
  /**
   * Whether the story shows an error or not-found page on purpose; `settled` fails the story on one otherwise.
   * `"request"`, which `failingOn` sets, allows the page of a request that failed (an ApiError, a NetworkError, a
   * not-found); `true` allows any, a fault of the app's own code included.
   */
  errorPage?: boolean | "request";
  /** A sign-in this tab started, waiting for the callback (lib/auth/signIn); none by default. */
  pendingSignIn?: StoredSignIn;
};

/** The stories' runtime config: the API is MSW's worker on the page's own origin. */
function storyConfig(): RuntimeConfig {
  return {
    apiOrigin: window.location.origin,
    instanceName: "my example",
    accountDomain: "my.example",
    defaultLocale: "en",
  };
}

let started: Promise<unknown> | undefined;

/** An error the screen showed: a route's error or not-found page, or an error boundary's fallback. */
type ShownError = { where: string; error: unknown };

/** The errors the current story's screen showed; `settled` fails the story on them unless it opts in. */
const shown: ShownError[] = [];

/** The requests of the API that no handler answered in the current story; `settled` fails the story on them. */
const unhandled: string[] = [];

/** Starts MSW's worker once per page; later stories reuse it. */
function startWorker(): Promise<unknown> {
  started ??= worker.start({
    quiet: true,
    // A request of the API that no handler answers is an error, as in the tests; the page's own modules pass.
    onUnhandledFrame({ frame, defaults }) {
      const { data } = frame as { data: { request?: Request } };
      const pathname = data.request === undefined ? "" : new URL(data.request.url).pathname;
      if (!pathname.startsWith(apiBase) && !pathname.startsWith("/oauth/")) return;
      unhandled.push(`${data.request?.method ?? ""} ${pathname}`);
      defaults.error();
    },
  });
  return started;
}

function screenParameters(context: StoryContext): ScreenParameters {
  const screen: unknown = context.parameters["screen"];
  if (typeof screen !== "object" || screen === null || !("path" in screen)) {
    throw new Error(`story ${context.id} has no parameters.screen.path`);
  }
  return screen as ScreenParameters;
}

/** The locale of Storybook's `locale` global. */
function storyLocale(context: StoryContext): string {
  const locale: unknown = context.globals["locale"];
  return typeof locale === "string" ? locale : "en";
}

/**
 * Prepares a screen story: the worker answers with the story's handlers first, the page keeps nothing of an earlier
 * story (lib/storage, the live announcer), and the catalog of the story's locale is loaded.
 */
export const screenLoader: Loader = async (context) => {
  const { signedIn = false, handlers = [], preferences: initial = {}, pendingSignIn } = screenParameters(context);
  const locale = storyLocale(context);
  await startWorker();
  worker.resetHandlers();
  unhandled.length = 0;
  shown.length = 0;
  if (signedIn) {
    // The server's preferences of the viewer, which PATCH changes, in the story's locale.
    let preferences: Schemas["Preferences"] = { ...preferencesFixture, ...initial, locale };
    worker.use(
      signedInHandler,
      http.get(`${apiBase}/preferences`, ({ request }) => withViewer(request, () => HttpResponse.json(preferences))),
      http.patch(`${apiBase}/preferences`, async ({ request }) => {
        const change = (await request.clone().json()) as Schemas["UpdatePreferencesRequest"];
        return withViewer(request, () => {
          preferences = { ...preferences, ...change };
          return HttpResponse.json(preferences);
        });
      }),
    );
  }
  // Last, since each use() goes before the handlers already in use: the story's handlers answer first.
  worker.use(...handlers);
  localStorage.clear();
  sessionStorage.clear();
  if (pendingSignIn !== undefined) signInKey.write(pendingSignIn);
  // React Aria's live announcer outlives the story that announced (a saved form): its messages name elements that
  // are gone with that story. The next story starts with a fresh one, as each test does (src/test/setup.ts).
  destroyAnnouncer();
  const i18n = await loadI18n(locale);
  // As the page's start does (lib/locale's loadStartI18n): <html lang dir> in the locale the app starts in.
  applyDocumentLocale(document, i18n.locale);
  return { i18n };
};

/** The components that record what they show in `shown`, so that each is wrapped once (the routes outlive a story). */
const watchers = new WeakSet<object>();

/**
 * `Component`, an error or not-found page of the router, recording what it shows in `shown`. The lazy component of a
 * code-split route keeps its `preload`, which the router calls before rendering it.
 */
function watching(Component: ComponentType<object>, where: string): ComponentType<object> {
  if (watchers.has(Component)) return Component;
  function Watched(props: object) {
    shown.push({ where, error: "error" in props ? props.error : undefined });
    return <Component {...props} />;
  }
  Object.defineProperty(Watched, "preload", { get: () => (Component as { preload?: unknown }).preload });
  watchers.add(Watched);
  return Watched;
}

type ErrorPages = { errorComponent?: unknown; notFoundComponent?: unknown };
type DefaultErrorPages = { defaultErrorComponent?: unknown; defaultNotFoundComponent?: unknown };

/** Makes every error and not-found page of `router` record what it shows in `shown`. */
function watchErrorPages(router: ReturnType<typeof createAppRouter>): void {
  for (const route of Object.values(router.routesById) as { id: string; options: unknown }[]) {
    const options = route.options as ErrorPages;
    if (typeof options.errorComponent === "function")
      options.errorComponent = watching(options.errorComponent as ComponentType<object>, `${route.id} error page`);
    if (typeof options.notFoundComponent === "function")
      options.notFoundComponent = watching(
        options.notFoundComponent as ComponentType<object>,
        `${route.id} not-found page`,
      );
  }
  const defaults = router.options as DefaultErrorPages;
  if (typeof defaults.defaultErrorComponent === "function")
    defaults.defaultErrorComponent = watching(
      defaults.defaultErrorComponent as ComponentType<object>,
      "default error page",
    );
  if (typeof defaults.defaultNotFoundComponent === "function")
    defaults.defaultNotFoundComponent = watching(
      defaults.defaultNotFoundComponent as ComponentType<object>,
      "default not-found page",
    );
}

type ScreenProps = { path: string; i18n: I18n };

/** The whole app at `path`, with services of its own, as main.tsx makes them for the page. */
function Screen({ path, i18n }: ScreenProps) {
  const [app] = useState(() => {
    const config = storyConfig();
    const services = createAppServices(config, i18n, { leaveApp: () => undefined, reload: () => undefined });
    // An error boundary of the app (a post's card) reports what it caught: the story records it too.
    const report = services.errors.report.bind(services.errors);
    services.errors.report = (error, kind) => {
      if (kind === "boundary_error") shown.push({ where: "error boundary", error });
      report(error, kind);
    };
    const router = createAppRouter({ ...services, history: createMemoryHistory({ initialEntries: [path] }) });
    watchErrorPages(router);
    return { config, services, router };
  });
  useEffect(() => {
    void app.services.session.restore();
    return () => {
      app.services.session.dispose();
    };
  }, [app]);
  return <App config={app.config} {...app.services} router={app.router} />;
}

/** The `render` of a screen story's meta: the app at the story's path, after screenLoader. */
export function renderScreen(_args: unknown, context: StoryContext) {
  const { path } = screenParameters(context);
  const { i18n } = context.loaded as { i18n: I18n };
  return <Screen key={context.id} path={path} i18n={i18n} />;
}

type Method = "get" | "post" | "put" | "patch" | "delete";

/** A story whose server answers with `handlers` before the default ones (parameters merge with the meta's). */
export function answering(...handlers: RequestHandler[]) {
  return { parameters: { screen: { handlers } } };
}

/** Whether `error`, shown by an error page, is that of a request that failed rather than a fault of the app. */
function isFailedRequest(error: unknown): boolean {
  return error === undefined || isNotFound(error) || error instanceof ApiError || error instanceof NetworkError;
}

/** The errors in `shown` that the story does not show on purpose (ScreenParameters.errorPage). */
function unexpectedErrors(errorPage: ScreenParameters["errorPage"]): string[] {
  if (errorPage === true) return [];
  return shown
    .filter(({ error }) => !isRedirect(error) && !(errorPage === "request" && isFailedRequest(error)))
    .map(({ where, error }) => `${where}: ${error instanceof Error ? `${error.name}: ${error.message}` : "not found"}`);
}

/** What a play function of a screen story is given that `settled` reads. */
export type ScreenPlayContext = { canvasElement: HTMLElement; parameters?: Record<string, unknown> };

/**
 * Waits until the screen has loaded: the page's h1 is there (each route state has one, routes/-layout/RouteStates.tsx)
 * and no spinner is left, and every request of the API had a handler. The error and not-found pages have an h1 too:
 * the screen must have shown none, nor an error boundary's fallback, unless the story shows one on purpose
 * (ScreenParameters.errorPage). The meta of a screen story plays it, so axe checks the screen and not its loading.
 */
export async function settled({ canvasElement, parameters }: ScreenPlayContext): Promise<void> {
  await waitFor(
    () => {
      if (canvasElement.querySelector("main h1") === null) throw new Error("the page has no h1 yet");
      if (canvasElement.querySelector('[role="progressbar"]') !== null) throw new Error("a spinner is still shown");
    },
    { timeout: 10_000 },
  );
  await expect(unhandled, "requests of the API that no MSW handler answers").toEqual([]);
  const screen = parameters?.["screen"] as ScreenParameters | undefined;
  await expect(unexpectedErrors(screen?.errorPage), "error pages the story does not show on purpose").toEqual([]);
}

/** A story whose request `method path` never answers: the screen shows that it is loading. */
export function loadingOn(method: Method, path: string) {
  return {
    ...answering(
      http[method](`${apiBase}${path}`, async () => {
        await delay("infinite");
        return new HttpResponse(null, { status: 204 });
      }),
    ),
    // The page's spinner, or the page's heading over a section that shows its own loading.
    play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
      await waitFor(
        () => {
          if (canvasElement.querySelector('main h1, [role="progressbar"]') === null)
            throw new Error("nothing shown yet");
        },
        { timeout: 10_000 },
      );
    },
  };
}

/** A story whose request `method path` fails with a 500, as a server that broke. */
export function failingOn(method: Method, path: string) {
  return answeringWithErrorPage(
    http[method](`${apiBase}${path}`, () => problem(500, "about:blank", "Internal Server Error")),
  );
}

/**
 * `answering`, for a story whose screen shows its error or not-found page on purpose: a request of `handlers` fails
 * (a 404, a 401, a 500) or the server turned the page off.
 */
export function answeringWithErrorPage(...handlers: RequestHandler[]) {
  return { parameters: { screen: { handlers, errorPage: "request" as const } } };
}

/** A story displayed in `locale`: Japanese, or the pseudo-locales en-XA (long words) and ar-XB (right to left). */
export function inLocale(locale: "ja" | "en-XA" | "ar-XB") {
  return {
    globals: { locale },
    // The page is in the locale once it has loaded: the signed-in viewer's preferences did not replace it.
    play: async (context: ScreenPlayContext) => {
      await settled(context);
      await expect(document.documentElement).toHaveAttribute("lang", locale);
      await expect(document.documentElement).toHaveAttribute("dir", locale === "ar-XB" ? "rtl" : "ltr");
    },
  };
}

/** A story at the 320 CSS px wide viewport (WCAG 1.4.10 reflow; .storybook/preview.tsx). */
export const narrow = { globals: { viewport: { value: "narrow", isRotated: false } } };

/** GET /accounts/verify_credentials answering the default viewer with `changes`. */
export function viewerWith(changes: Partial<Schemas["CredentialAccount"]>): RequestHandler {
  return http.get(`${apiBase}/accounts/verify_credentials`, ({ request }) =>
    withViewer(request, () => HttpResponse.json({ ...viewerFixture, ...changes })),
  );
}
