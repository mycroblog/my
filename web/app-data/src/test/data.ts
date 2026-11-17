// SPDX-License-Identifier: AGPL-3.0-only

// The app's data for its unit tests (ADR-0171 §2, §5): on a fake API of its own (@my/e2e/fake), which answers from the
// scenario or the seed a test gives, as one person of it reads it. A test reads and writes through the app's data,
// scripts what one answer does (`fake.script`), and reads what the server was asked. No test reaches a live server.

import { createFakeApi, fakeOrigin, type FakeApi, type FakeOptions } from "@my/e2e/fake";
import { createClient } from "@mycroblog/mykit";
import { createAppData, type AppData } from "../app-data";

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
  return dataOn(createFakeApi(options));
}

/** The app's data on `fake`, with a cache of its own: another tab of the browser, or another load of the app. */
export function dataOn(fake: FakeApi): FakeData {
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
