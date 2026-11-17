// SPDX-License-Identifier: AGPL-3.0-only

// A feature for its tests (ADR-0171 §2, §5): the app's data on a fake API of its own (@my/e2e/fake), read as a person of
// its seed or as a visitor, and the feature under the providers the app places, in a locale: rendered on the DOM the
// test file's kind registered (`renderFeature`), or as its markup where the file has none (`markupOf`). A test loads
// what the screen's route loads, finds what the feature shows by role and name, presses and types as a reader does,
// scripts the fake's answers, and reads what the server was asked.

import { AppDataProvider, createAppData, type AppData } from "@my/app-data";
import { I18nProvider } from "@my/design-system";
import { createFakeApi, fakeOrigin, type FakeApi, type FakeOptions } from "@my/e2e/fake";
import { createClient } from "@mycroblog/mykit";
import { render } from "@testing-library/react";
import userEvent, { type UserEvent } from "@testing-library/user-event";
import type { ReactNode } from "react";
import { renderToStaticMarkup } from "react-dom/server";

/** The app's data on a fake API, and what it asked of it. */
export type FakeData = {
  /** The fake API: `fake.script(route, …)` holds, fails or changes its answers, `fake.send` pushes a stream event. */
  fake: FakeApi;
  /** The app's data on the fake, without retries. */
  data: AppData;
  /** The requests the app's data sent, in order, each as `METHOD /path?query` under the client API's path. */
  requests: string[];
};

/**
 * The app's data on a fake API of its own, as `options` names its reader (`viewer`, yuki by default, or `visitor`) and
 * what its server holds (`seed`, the scenario by default).
 */
export function fakeData(options: FakeOptions = {}): FakeData {
  const fake = createFakeApi(options);
  const requests: string[] = [];
  const api = createClient({
    server: fakeOrigin,
    retry: { disabled: true },
    fetch: (request) => {
      const url = new URL(request.url);
      requests.push(`${request.method} ${url.pathname.replace("/api/v1", "")}${url.search}`);
      return fake.serve(request);
    },
  });
  return { fake, data: createAppData(api), requests };
}

/** `children` on `data` under the providers the app places, in `locale`. */
const provided = (data: AppData, children: ReactNode, locale: string) => (
  <AppDataProvider data={data}>
    <I18nProvider locale={locale}>{children}</I18nProvider>
  </AppDataProvider>
);

/**
 * Renders `children` on `data` under the providers the app places, in `locale`, and returns the reader's hands:
 * `user.click`, `user.keyboard`, `user.type`, `user.tab`.
 */
export function renderFeature(data: AppData, children: ReactNode, locale = "en"): UserEvent {
  render(provided(data, children, locale));
  return userEvent.setup();
}

/** The markup of `children` on `data` under the providers the app places, in `locale`: one render, without a DOM. */
export const markupOf = (data: AppData, children: ReactNode, locale = "en"): string =>
  renderToStaticMarkup(provided(data, children, locale));
