// SPDX-License-Identifier: AGPL-3.0-only

// The app for the component tests of its routes (ADR-0171 §2, §5): the router and the shell, signed in to a fake API
// of its own (@my/e2e/fake) as a person of its seed, or as a visitor, in a locale, on the DOM the test file's kind
// registered, with the stream of a session as the app's entry starts it. A test finds what the screen shows by role and
// name, presses and types as a reader does, scripts the fake's answers and pushes its stream's events, and reads where
// the app is, the document's title and the focus.

import { startStream } from "@my/app-data";
import { I18nProvider } from "@my/design-system";
import { fakeHost, fakeOrigin, type FakeOptions } from "@my/e2e/fake";
import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent, { type UserEvent } from "@testing-library/user-event";
import { onTestFinished } from "bun:test";
// What the features load on first use is loaded before a test renders: no test waits for a chunk.
import "@my/app-features/test/chunks";
import { AppRoot } from "../app";
import { createAppRouter } from "../router";
import { fakeApp, type FakeApp } from "./server";

/** Who reads, on what server, in what locale: the fake's options, and the reader's locale (`en` by default). */
export type AppOptions = FakeOptions & {
  locale?: string;
  /**
   * The server to render on, in place of a new one of the fake's options: one a test made with `fakeApp(options)` to
   * script its fake before the app's first request.
   */
  on?: FakeApp;
  /**
   * The app could not ask whether the browser has a session when it started (the server did not answer): it runs as
   * for a visitor (`visitor: true`), without knowing that there is none.
   */
  unasked?: boolean;
};

/** The app a test rendered: its fake API (to script and to push events), its data, and what a reader does and reads. */
export type TestApp = Pick<FakeApp, "fake" | "data"> & {
  /** The reader's hands: `user.click`, `user.keyboard`, `user.type`, `user.tab`. */
  user: UserEvent;
  /** Where the app is: its path, with the query and the fragment. */
  path: () => string;
  /** The document's title. */
  title: () => string;
  /** The element that has the focus; null when it is the page itself. */
  focused: () => Element | null;
  /** The addresses the app left for by a load of the page, which no simulation makes, in their order. */
  left: () => readonly string[];
  /** Goes to `to` as a link of the app does, and returns once the screen is in place. */
  navigate: (to: string) => Promise<void>;
  /** Goes back, as the browser's button does, and returns once the screen is in place. */
  back: () => Promise<void>;
  /**
   * Returns once the app is at `path` with its screen loaded and drawn, which a press of the reader led to: it waits
   * for the router's end of the navigation, not for a span of the wall's clock that a busy machine outlasts.
   */
  arrived: (path: string) => Promise<void>;
};

/**
 * The address of the page the app is served from, as the DOM of the process gave it before any test of the app ran.
 * A press the app leaves to the browser (a download's link, the sign-in's page, a page of another server) makes
 * happy-dom follow it and move the page's address for every later test of the process, in whichever file: at
 * `about:blank` or a `blob:` address no link of the app is of the page's origin, and every link of a later test then
 * does nothing.
 */
const page = document.URL;

/** Puts the page back at the app's address without loading it, as happy-dom lets a test do. */
function atThePage(): void {
  const dom = Reflect.get(globalThis, "happyDOM") as { setURL?: (url: string) => void } | undefined;
  if (dom?.setURL === undefined) throw new Error("test: renderApp runs on happy-dom, which can set the page's address");
  if (document.URL !== page) dom.setURL(page);
}

/**
 * Renders the app at the path `at`, on a fake API of its own, and returns once the first screen is in place. The
 * reader is the fake's (`viewer`, yuki by default), or a visitor (`visitor: true`).
 */
export async function renderApp(
  at: string,
  { locale = "en", on, unasked = false, ...options }: AppOptions = {},
): Promise<TestApp> {
  // Each test starts on the app's page, and leaves it there for the next, whatever its presses made the DOM follow.
  atThePage();
  onTestFinished(atThePage);
  const { fake, watch, data, visitor } = on ?? fakeApp(options);
  // What the app started ends with the test that rendered it, in whichever file it is.
  onTestFinished(() => {
    watch.stop();
  });
  // The fake's answers and its stream's events reach the app when they come, as a server's do, not inside an act()
  // of the test: React is told so until the test ends, and the test waits for what it asserts (`findBy…`, `waitFor`).
  const environment = globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean };
  const wrapped = environment.IS_REACT_ACT_ENVIRONMENT;
  environment.IS_REACT_ACT_ENVIRONMENT = false;
  onTestFinished(() => {
    if (wrapped === undefined) delete environment.IS_REACT_ACT_ENVIRONMENT;
    else environment.IS_REACT_ACT_ENVIRONMENT = wrapped;
  });
  // A stream in this tab alone: a simulation has no other tab to share it with.
  if (!visitor) onTestFinished(startStream(data, { tabs: null }));
  const left: string[] = [];
  const router = createAppRouter({
    data: visitor ? null : data,
    config: { apiOrigin: fakeOrigin, instanceName: fakeHost, defaultLocale: "en", clientId: "my-web" },
    at,
    // A load of the page is the browser's: the test reads where the app asked to go.
    leave: (href) => {
      left.push(href);
    },
  });
  render(
    <I18nProvider locale={locale}>
      <AppRoot router={router} visitor={data} unasked={unasked} />
    </I18nProvider>,
  );
  const settled = () =>
    waitFor(() => {
      if (router.state.status !== "idle")
        throw new Error(`test: the app is still on its way to ${router.state.location.href}`);
    });
  await settled();
  const isAt = (path: string) => router.state.status === "idle" && router.state.location.href === path;
  return {
    fake,
    data,
    user: userEvent.setup(),
    path: () => router.state.location.href,
    title: () => document.title,
    focused: () => (document.activeElement === document.body ? null : document.activeElement),
    left: () => left,
    navigate: async (to) => {
      await act(() => router.navigate({ to }));
      await settled();
    },
    back: async () => {
      act(() => {
        router.history.back();
      });
      await settled();
    },
    arrived: (path) =>
      new Promise<void>((resolve) => {
        if (isAt(path)) {
          resolve();
          return;
        }
        // The router says a navigation resolved once the screen it loaded is drawn; the test's timeout bounds the wait.
        const stop = router.subscribe("onResolved", () => {
          if (!isAt(path)) return;
          stop();
          resolve();
        });
      }),
  };
}

/** The queries of the screen's own part, the main landmark: the shell around it repeats its places in two navigations. */
export const main = () => within(screen.getByRole("main"));

/**
 * What `element` says as a reader reads its line: without the joiners that keep a line's last words together, each run
 * of spaces, breaking or not, as one space.
 */
export const words = (element: Element): string =>
  element.textContent.replaceAll("\u2060", "").replace(/\s+/g, " ").trim();

/**
 * Keeps the waits of exactly `ms` that the app starts from now until the test ends, for the test to end them: `pass()`
 * lets the ones that began and were not given up come at once, and returns how many they were. The test's clock in
 * place of the wall's, for one wait the app times itself (how long the settings' menu waits for what it lists);
 * every other wait, the data cache's and Testing Library's among them, keeps its time. Call it before the app starts
 * the wait.
 */
export function heldWaitsOf(ms: number): { pass: () => number } {
  const wait = globalThis.setTimeout;
  const giveUp = globalThis.clearTimeout;
  const held = new Map<unknown, () => void>();
  const holding = ((handler: TimerHandler, timeout?: number, ...rest: unknown[]) => {
    if (typeof handler !== "function" || timeout !== ms) return wait(handler, timeout, ...rest);
    // A timer that does nothing stands for the held one: what gives it up gives up the held wait.
    const timer = wait(() => undefined, 0);
    held.set(timer, () => {
      (handler as (...given: unknown[]) => void)(...rest);
    });
    return timer;
  }) as typeof setTimeout;
  globalThis.setTimeout = Object.assign(holding, wait);
  globalThis.clearTimeout = ((timer?: Parameters<typeof clearTimeout>[0]) => {
    held.delete(timer);
    giveUp(timer);
  }) as typeof clearTimeout;
  onTestFinished(() => {
    globalThis.setTimeout = wait;
    globalThis.clearTimeout = giveUp;
  });
  return {
    pass: () => {
      const over = [...held.values()];
      held.clear();
      for (const come of over) come();
      return over.length;
    },
  };
}

/**
 * Lets what the app waits `ms` or longer for (the second before it asks again whether an upload is processed) come at
 * once, until the test ends: the test's clock in place of the wall's, for a wait the app times itself. Call it before
 * the app starts the wait. What Testing Library waits for keeps its own time.
 */
export function withoutWaitsOf(ms: number): void {
  const wait = globalThis.setTimeout;
  const sooner = ((handler: TimerHandler, timeout?: number, ...rest: unknown[]) =>
    wait(handler, timeout !== undefined && timeout >= ms ? 0 : timeout, ...rest)) as typeof setTimeout;
  globalThis.setTimeout = Object.assign(sooner, wait);
  onTestFinished(() => {
    globalThis.setTimeout = wait;
  });
}
