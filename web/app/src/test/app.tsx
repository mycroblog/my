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
import { afterEach } from "bun:test";
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
  /** Goes to `to` as a link of the app does, and returns once the screen is in place. */
  navigate: (to: string) => Promise<void>;
  /** Goes back, as the browser's button does, and returns once the screen is in place. */
  back: () => Promise<void>;
};

/** What ends with the test: the streams and the watches of the servers of the apps it rendered. */
const stops: (() => void)[] = [];

afterEach(() => {
  for (const stop of stops.splice(0)) stop();
});

/**
 * Renders the app at the path `at`, on a fake API of its own, and returns once the first screen is in place. The
 * reader is the fake's (`viewer`, yuki by default), or a visitor (`visitor: true`).
 */
export async function renderApp(at: string, { locale = "en", on, ...options }: AppOptions = {}): Promise<TestApp> {
  const { fake, watch, data, visitor } = on ?? fakeApp(options);
  stops.push(() => {
    watch.stop();
  });
  // The fake's answers and its stream's events reach the app when they come, as a server's do, not inside an act()
  // of the test: React is told so until the test ends, and the test waits for what it asserts (`findBy…`, `waitFor`).
  const environment = globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean };
  const wrapped = environment.IS_REACT_ACT_ENVIRONMENT;
  environment.IS_REACT_ACT_ENVIRONMENT = false;
  stops.push(() => {
    if (wrapped === undefined) delete environment.IS_REACT_ACT_ENVIRONMENT;
    else environment.IS_REACT_ACT_ENVIRONMENT = wrapped;
  });
  // A stream in this tab alone: a simulation has no other tab to share it with.
  if (!visitor) stops.push(startStream(data, { tabs: null }));
  const router = createAppRouter({
    data: visitor ? null : data,
    config: { apiOrigin: fakeOrigin, instanceName: fakeHost, defaultLocale: "en", clientId: "my-web" },
    at,
  });
  render(
    <I18nProvider locale={locale}>
      <AppRoot router={router} visitor={data} />
    </I18nProvider>,
  );
  const settled = () =>
    waitFor(() => {
      if (router.state.status !== "idle")
        throw new Error(`test: the app is still on its way to ${router.state.location.href}`);
    });
  await settled();
  return {
    fake,
    data,
    user: userEvent.setup(),
    path: () => router.state.location.href,
    title: () => document.title,
    focused: () => (document.activeElement === document.body ? null : document.activeElement),
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
  };
}

/** The queries of the screen's own part, the main landmark: the shell around it repeats its places in two navigations. */
export const main = () => within(screen.getByRole("main"));

/** The post by `author` (a display name) that holds `text`, in the main landmark, once the screen shows it. */
export const findPost = (author: string, text: string): Promise<HTMLElement> =>
  waitFor(() => {
    const [post, ...more] = main()
      .getAllByRole("article", { name: author })
      .filter((article) => article.textContent.includes(text));
    if (post === undefined || more.length > 0)
      throw new Error(`test: ${String(more.length + (post === undefined ? 0 : 1))} posts of ${author} hold "${text}"`);
    return post;
  });

/**
 * What `element` says as a reader reads its line: without the joiners that keep a line's last words together, each run
 * of spaces, breaking or not, as one space.
 */
export const words = (element: Element): string =>
  element.textContent.replaceAll("\u2060", "").replace(/\s+/g, " ").trim();
