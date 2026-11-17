// SPDX-License-Identifier: AGPL-3.0-only

// What a test scripts on the fake API: the requests of an operation counted and kept, its answer held until the test
// lets it go, failed, or changed. The E2E tests do the same to the real server by intercepting the page's requests.

import { apiPath, createMatcher, jsonOf, type OkBody, type RequestBody, type Route, type Schema } from "./spec";

/** How a script fails a request: with a problem of a status, with the problem given, or without an answer. */
export type Failure = number | "network" | Schema<"Problem">;

/** What a script does to the requests of its operation; nothing but count and keep them when it names nothing. */
export type Script<R extends Route> = {
  /** How many of the next requests it acts on, every one by default; the later ones are answered as without it. */
  times?: number;
  /**
   * Holds the answer until `release()`: at the `request`, which the fake has not read yet, so that nothing happened; or
   * at the `response`, after the fake did what the request asks, as a server whose answer comes late.
   */
  hold?: "request" | "response";
  /**
   * Fails the request: with a problem of this status, with the problem given (its type, its detail, the member it
   * names), or as a request that got no answer (`network`). The fake does not do what the request asks, unless the
   * answer is held at the `response`.
   */
  fail?: Failure;
  /** Changes the JSON body of the fake's answer, where it answers 200. */
  change?: (body: OkBody<R>) => OkBody<R>;
};

/** A request of a scripted operation, as the fake received it: its address, its headers and its JSON body. */
export type Seen<R extends Route> = { url: URL; headers: Headers; body: RequestBody<R> };

/** A script at work. */
export type Scripted<R extends Route> = {
  /** The requests of the operation since the script began, in their order. */
  readonly seen: readonly Seen<R>[];
  /** Resolves with the `n`th request of the operation, the first by default, once it arrived. */
  arrived: (n?: number) => Promise<Seen<R>>;
  /**
   * Resolves with the `n`th request of the operation, the first by default, once the fake did what it asks: its answer
   * is made, though a hold at the `response` still keeps it. It does not resolve for a request failed unserved.
   */
  served: (n?: number) => Promise<Seen<R>>;
  /** Lets the held answers go; the later requests are not held. */
  release: () => void;
  /** Ends the script, letting go of what it holds: the later requests are answered as without it. */
  end: () => void;
};

type Running = {
  route: Route;
  script: Script<Route>;
  left: number;
  seen: Seen<Route>[];
  waiting: { n: number; resolve: (seen: Seen<Route>) => void }[];
  /** The requests the fake served, by their place in `seen`, and who waits for one. */
  served: Set<number>;
  servedWaiting: { n: number; resolve: (seen: Seen<Route>) => void }[];
  released: Promise<void>;
  release: () => void;
  ended: boolean;
};

/** The answer of a request that `fail` fails; a request without an answer throws as `fetch` does. */
function failure(fail: Failure): Response {
  if (fail === "network") throw new TypeError("Failed to fetch");
  const problem: Schema<"Problem"> =
    typeof fail === "number" ? { type: "about:blank", title: "The request failed", status: fail } : fail;
  return new Response(JSON.stringify(problem), {
    status: problem.status,
    headers: { "Content-Type": "application/problem+json" },
  });
}

/**
 * Puts scripts before `serve`, which answers the operations of `implemented`: `script` begins one, and `serve` of what
 * it returns answers a request as the scripts of its operation say: every one of them counts and keeps it, and the
 * first that holds, fails or changes and still acts on requests does so.
 */
export function createScripts(
  implemented: readonly Route[],
  serve: (request: Request) => Promise<Response>,
): {
  script: <R extends Route>(route: R, script?: Script<R>) => Scripted<R>;
  serve: (request: Request) => Promise<Response>;
} {
  const running: Running[] = [];
  let find = createMatcher(implemented);

  function script<R extends Route>(route: R, script: Script<R> = {}): Scripted<R> {
    const { promise: released, resolve: release } = Promise.withResolvers<undefined>();
    const own: Running = {
      route,
      // A script of a route reads the requests of that route alone, whose bodies its type names.
      script,
      left: script.times ?? Infinity,
      seen: [],
      waiting: [],
      served: new Set(),
      servedWaiting: [],
      released,
      release: () => {
        release(undefined);
      },
      ended: false,
    };
    running.push(own);
    find = createMatcher([...new Set([...implemented, ...running.map((r) => r.route)])]);
    return {
      seen: own.seen,
      arrived: (n = 1) => {
        const seen = own.seen[n - 1];
        if (seen !== undefined) return Promise.resolve(seen);
        return new Promise<Seen<R>>((resolve) => {
          own.waiting.push({ n, resolve });
        });
      },
      served: (n = 1) => {
        const seen = own.seen[n - 1];
        if (seen !== undefined && own.served.has(n)) return Promise.resolve(seen);
        return new Promise<Seen<R>>((resolve) => {
          own.servedWaiting.push({ n, resolve });
        });
      },
      release: own.release,
      end: () => {
        own.ended = true;
        own.release();
      },
    };
  }

  async function scripted(request: Request): Promise<Response> {
    const route = find(request.method, apiPath(request))?.route;
    const own = running.filter((r) => !r.ended && r.route === route);
    if (own.length === 0) return serve(request);
    const seen = {
      url: new URL(request.url),
      headers: new Headers(request.headers),
      body: await jsonOf(request),
    } as Seen<Route>;
    let acting: Running | undefined;
    const places: [Running, number][] = [];
    for (const r of own) {
      r.seen.push(seen);
      places.push([r, r.seen.length]);
      for (const w of r.waiting.filter((w) => w.n === r.seen.length)) w.resolve(seen);
      // A script that names nothing only counts and keeps: it leaves the request to the next one.
      const acts = r.script.hold !== undefined || r.script.fail !== undefined || r.script.change !== undefined;
      if (acting === undefined && acts && r.left > 0) {
        r.left -= 1;
        acting = r;
      }
    }
    /** The fake's own answer, which every script of the operation is told of. */
    const served = async () => {
      const response = await serve(request);
      for (const [r, n] of places) {
        r.served.add(n);
        for (const w of r.servedWaiting.filter((w) => w.n === n)) w.resolve(seen);
      }
      return response;
    };
    if (acting === undefined) return served();
    const { hold, fail, change } = acting.script;
    if (hold === "request") {
      await acting.released;
      // A request its sender gave up on meanwhile is gone, as the browser's is.
      if (request.signal.aborted) throw request.signal.reason;
    }
    if (fail !== undefined && hold !== "response") return failure(fail);
    const response = await served();
    if (hold === "response") await acting.released;
    if (fail !== undefined) return failure(fail);
    if (change === undefined || response.status !== 200 || response.headers.get("Content-Type") !== "application/json")
      return response;
    return new Response(JSON.stringify(change((await response.json()) as OkBody<Route>)), {
      status: response.status,
      headers: response.headers,
    });
  }

  return { script, serve: scripted };
}
