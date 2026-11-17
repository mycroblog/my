// SPDX-License-Identifier: AGPL-3.0-only

// The server of the app's tests: a fake API of its own (@my/e2e/fake, ADR-0171 §5), read through the app's watch of
// the server, and its instance as GET /instance answers it. No test reaches a live server.

import { createAppData, instanceQuery, watchServer, type AppData, type Instance, type ServerWatch } from "@my/app-data";
import { createFakeApi, fakeOrigin, type FakeApi, type FakeOptions } from "@my/e2e/fake";
import { createClient } from "@mycroblog/mykit";

/** A fake API, the app's watch of it, and the app's data on it. */
export type FakeApp = {
  /** The fake API: `fake.script(route, …)` holds, fails or changes its answers, `fake.send` pushes a stream event. */
  fake: FakeApi;
  /** The app's watch of the server, which every request goes through. */
  watch: ServerWatch;
  /** The app's data on the fake, without retries. */
  data: AppData;
};

/**
 * The app's data on a fake API of its own, as `options` names its reader (`viewer`, yuki by default, or `visitor`)
 * and its server, through the app's watch of the server, which the test stops (`watch.stop()`) when it started it.
 */
export function fakeApp(options: FakeOptions = {}): FakeApp {
  const fake = createFakeApi(options);
  const watch = watchServer({ server: fakeOrigin, fetch: fake.serve, followDevice: false });
  const api = createClient({ server: fakeOrigin, retry: { disabled: true }, fetch: watch.fetch });
  return { fake, watch, data: createAppData(api, undefined, watch) };
}

/** The instance of a fake server of `options`, as the shell reads it. */
export function instanceOf(options: FakeOptions = {}): Promise<Instance> {
  const { data } = fakeApp(options);
  return data.queryClient.query(instanceQuery(data));
}
