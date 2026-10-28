// SPDX-License-Identifier: AGPL-3.0-only
// The screen stories of app/web (ADR-0112 §1): a story renders the whole app at a route, as an integration test does
// (render.tsx), with MSW's worker in the browser playing the server with the default handlers of every feature
// (msw/browser.ts) and the story's own overrides. The locale is Storybook's `locale` global (../../.storybook/preview.tsx):
// the page starts in it, and the signed-in viewer's preferences name it, so the app stays in it after signing in.
import type { Loader, StoryContext } from "@storybook/react-vite";
import { createMemoryHistory } from "@tanstack/react-router";
import { delay, http, HttpResponse, type RequestHandler } from "msw";
import { useEffect, useState } from "react";
import { expect, waitFor } from "storybook/test";
import { destroyAnnouncer } from "react-aria/private/live-announcer/LiveAnnouncer";
import { App } from "../App";
import type { RuntimeConfig } from "../lib/config";
import { applyDocumentLocale, type I18n, loadI18n } from "../lib/i18n";
import { createAppServices } from "../lib/services";
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
  const { signedIn = false, handlers = [], preferences: initial = {} } = screenParameters(context);
  const locale = storyLocale(context);
  await startWorker();
  worker.resetHandlers();
  unhandled.length = 0;
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
  // React Aria's live announcer outlives the story that announced (a saved form): its messages name elements that
  // are gone with that story. The next story starts with a fresh one, as each test does (src/test/setup.ts).
  destroyAnnouncer();
  const i18n = await loadI18n(locale);
  // As the page's start does (lib/locale's loadStartI18n): <html lang dir> in the locale the app starts in.
  applyDocumentLocale(document, i18n.locale);
  return { i18n };
};

type ScreenProps = { path: string; i18n: I18n };

/** The whole app at `path`, with services of its own, as main.tsx makes them for the page. */
function Screen({ path, i18n }: ScreenProps) {
  const [app] = useState(() => {
    const config = storyConfig();
    const services = createAppServices(config, i18n, { leaveApp: () => undefined, reload: () => undefined });
    const router = createAppRouter({ ...services, history: createMemoryHistory({ initialEntries: [path] }) });
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

/**
 * Waits until the screen has loaded: the page's h1 is there (each route state has one, routes/-layout/RouteStates.tsx)
 * and no spinner is left, and every request of the API had a handler. The meta of a screen story plays it, so axe
 * checks the screen and not its loading.
 */
export async function settled({ canvasElement }: { canvasElement: HTMLElement }): Promise<void> {
  await waitFor(
    () => {
      if (canvasElement.querySelector("main h1") === null) throw new Error("the page has no h1 yet");
      if (canvasElement.querySelector('[role="progressbar"]') !== null) throw new Error("a spinner is still shown");
    },
    { timeout: 10_000 },
  );
  await expect(unhandled, "requests of the API that no MSW handler answers").toEqual([]);
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
  return answering(http[method](`${apiBase}${path}`, () => problem(500, "about:blank", "Internal Server Error")));
}

/** A story displayed in `locale`: Japanese, or the pseudo-locales en-XA (long words) and ar-XB (right to left). */
export function inLocale(locale: "ja" | "en-XA" | "ar-XB") {
  return {
    globals: { locale },
    // The page is in the locale once it has loaded: the signed-in viewer's preferences did not replace it.
    play: async (context: { canvasElement: HTMLElement }) => {
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
