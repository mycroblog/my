// SPDX-License-Identifier: AGPL-3.0-only

// The server of the app's tests: a fake API of its own (@my/e2e/fake, ADR-0171 §5), read through the app's watch of
// the server, and its instance as GET /instance answers it. No test reaches a live server.

import {
  createAppData,
  instanceQuery,
  openSession,
  watchServer,
  type AppData,
  type Instance,
  type ServerWatch,
} from "@my/app-data";
import { createFakeApi, fakeOrigin, type FakeApi, type FakeOptions } from "@my/e2e/fake";
import { createClient } from "@mycroblog/mykit";
import { fakeBrowser } from "./tabs";

/** A fake API, the app's watch of it, and the app's data on it. */
export type FakeApp = {
  /** The fake API: `fake.script(route, …)` holds, fails or changes its answers, `fake.send` pushes a stream event. */
  fake: FakeApi;
  /** The app's watch of the server, which every request goes through. */
  watch: ServerWatch;
  /** The app's data on the fake, without retries; a reader with a session has what their stream connects with. */
  data: AppData;
  /** Whether the reader has no session (`visitor`). */
  visitor: boolean;
};

/**
 * The app's data on a fake API of its own, as `options` names its reader (`viewer`, yuki by default, or `visitor`)
 * and its server, through the app's watch of the server, which the test stops (`watch.stop()`) when it started it.
 */
export function fakeApp(options: FakeOptions = {}): FakeApp {
  const fake = createFakeApi(options);
  const watch = watchServer({ server: fakeOrigin, fetch: fake.serve, followDevice: false });
  const api = createClient({ server: fakeOrigin, retry: { disabled: true }, fetch: watch.fetch });
  const visitor = options.visitor === true;
  const data = createAppData(api, undefined, watch);
  return {
    fake,
    watch,
    data: visitor ? data : { ...data, connection: { server: fakeOrigin, fetch: watch.fetch } },
    visitor,
  };
}

/**
 * `app` with a session the server ended: its token source has no access token to give. Without it the reader's
 * connection has no token source at all; `fakeTabs` gives a session that has its token.
 */
export function withoutToken(app: FakeApp): FakeApp {
  return {
    ...app,
    data: {
      ...app.data,
      connection: {
        server: fakeOrigin,
        fetch: app.watch.fetch,
        tokenSource: { getAccessToken: () => Promise.resolve(null) },
      },
    },
  };
}

/**
 * A browser of several tabs signed in on a fake API of `options`, whose sign-in server trades the session's cookie
 * for an access token until one is revoked (no operation of the client API, so the browser answers it here): `on` is
 * the server with the first tab's data, to render the app on, and `tab()` opens one more tab, with the app's data
 * in it.
 */
export async function fakeTabs(options: FakeOptions = {}): Promise<{ on: FakeApp; tab: () => Promise<AppData> }> {
  const fake = createFakeApi(options);
  const serve = (request: Request): Promise<Response> =>
    new URL(request.url).pathname === "/oauth/token/refresh"
      ? Promise.resolve(
          fake.revocations.length > 0
            ? Response.json({ error: "invalid_grant" }, { status: 400 })
            : Response.json({ access_token: "a1", token_type: "Bearer", expires_in: 3600 }),
        )
      : fake.serve(request);
  // The watch is what a session's requests go through, the trade of its cookie too.
  const watch = watchServer({ server: fakeOrigin, fetch: serve, followDevice: false });
  const tabs = fakeBrowser();
  const tab = async () => {
    const data = await openSession({ server: fakeOrigin, tabs: tabs.join(), watch });
    if (data === null) throw new Error("test: the browser has no session");
    return data;
  };
  return { on: { fake, watch, data: await tab(), visitor: false }, tab };
}

/** The instance of a fake server of `options`, as the shell reads it. */
export function instanceOf(options: FakeOptions = {}): Promise<Instance> {
  const { data } = fakeApp(options);
  return data.queryClient.query(instanceQuery(data));
}
