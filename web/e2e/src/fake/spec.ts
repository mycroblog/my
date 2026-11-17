// SPDX-License-Identifier: AGPL-3.0-only

// What binds the fake API to the API specification (ADR-0171 §5): the types of a handler's request and answer come
// from the operation it implements, as mykit's types generated from api/openapi.yaml give it, so that a change of an
// operation the fake implements fails the type check here. And the router that finds the handler of a request by the
// specification's path, which refuses a request of no handler by name instead of answering it.

import type { components, paths } from "@mycroblog/mykit";

/** A schema of the API specification, by its name. */
export type Schema<Name extends keyof components["schemas"]> = components["schemas"][Name];

type Method = "get" | "put" | "post" | "delete" | "patch";

/** The operation of a path and a method of the specification; never where the path has no such method. */
type OperationAt<P extends keyof paths, M extends Method> = paths[P][M] extends infer Op
  ? Op extends object
    ? Op
    : never
  : never;

/** An operation of the specification, as its method and path: `GET /statuses/{id}`. */
export type Route = {
  [P in keyof paths]: { [M in Method]: [OperationAt<P, M>] extends [never] ? never : `${Uppercase<M>} ${P}` }[Method];
}[keyof paths];

type OperationOf<R extends Route> = R extends `${infer M} ${infer P}`
  ? P extends keyof paths
    ? Lowercase<M> extends Method
      ? OperationAt<P, Lowercase<M>>
      : never
    : never
  : never;

type ParametersOf<Op> = Op extends { parameters: infer P } ? P : never;

/** The parameters of an operation's path, by name. */
type PathOf<Op> = ParametersOf<Op> extends { path: infer P } ? P : Record<string, never>;

/** The names of an operation's query parameters. */
type QueryNames<Op> =
  ParametersOf<Op> extends { query?: infer Q }
    ? [NonNullable<Q>] extends [never]
      ? never
      : keyof NonNullable<Q> & string
    : never;

/** The JSON body of an operation's request; undefined where it has none, or may come without one. */
type JsonOf<Op> = Op extends { requestBody: { content: { "application/json": infer B } } }
  ? B
  : Op extends { requestBody?: infer R }
    ? [NonNullable<R>] extends [never]
      ? undefined
      : NonNullable<R> extends { content: { "application/json": infer B } }
        ? B | undefined
        : undefined
    : undefined;

type ResponsesOf<Op> = Op extends { responses: infer R } ? R : never;

/** The statuses of an operation's answers that are no error. */
type SuccessStatus<Op> = {
  [S in keyof ResponsesOf<Op>]: S extends number ? (`${S}` extends `2${string}` ? S : never) : never;
}[keyof ResponsesOf<Op>];

type ContentOf<R> = R extends { content: infer C } ? C : never;

type Extra = { headers?: Readonly<Record<string, string>> };

/**
 * An answer of an operation that is no error, as the specification gives it for a status: its JSON `body`, the `raw`
 * body of another media `type`, or neither for a status without content.
 */
type Success<Op> = {
  [S in SuccessStatus<Op>]: [ContentOf<ResponsesOf<Op>[S]>] extends [never]
    ? { status: S } & Extra
    : {
        [T in keyof ContentOf<ResponsesOf<Op>[S]>]: T extends "application/json"
          ? { status: S; body: ContentOf<ResponsesOf<Op>[S]>[T] } & Extra
          : { status: S; type: T; raw: BodyInit } & Extra;
      }[keyof ContentOf<ResponsesOf<Op>[S]>];
}[SuccessStatus<Op>];

/** An error answer: a problem (RFC 9457) as the specification's Problem, with the status it names. */
export type Refusal = { problem: Schema<"Problem"> } & Extra;

/** The JSON body of an operation's answer of status 200. */
export type OkBody<R extends Route> =
  ResponsesOf<OperationOf<R>> extends {
    200: { content: { "application/json": infer B } };
  }
    ? B
    : never;

/** The JSON body of an operation's request. */
export type RequestBody<R extends Route> = JsonOf<OperationOf<R>>;

/** A request of an operation, as its handler reads it. */
type Call<R extends Route> = {
  /** The parameters of the path, decoded. */
  params: PathOf<OperationOf<R>>;
  /** The query, read by the names the specification gives the operation's parameters. */
  query: {
    get: (name: QueryNames<OperationOf<R>>) => string | null;
    all: (name: QueryNames<OperationOf<R>>) => string[];
  };
  /** The JSON body. */
  body: RequestBody<R>;
  /** The request itself, for its headers and a body that is no JSON. */
  request: Request;
};

/** What a handler answers: an answer of its operation, or a problem. */
export type Answer<R extends Route> = Success<OperationOf<R>> | Refusal;

/** The handler of an operation. */
type Handler<R extends Route> = (call: Call<R>) => Answer<R> | Promise<Answer<R>>;

/** The handlers of a fake, by operation. */
export type Handlers = { [R in Route]?: Handler<R> };

/**
 * `value` as a `T`, refused when it has a member `T` lacks: the compiler's own check of an object literal passes over
 * the members a spread adds.
 */
export const exact =
  <T>() =>
  <V extends T>(value: V & Record<Exclude<keyof V, keyof T>, never>): T =>
    value;

/** The failure of a request for an operation the fake does not implement. */
export class UnimplementedError extends Error {
  override readonly name = "UnimplementedError";
  /** The request's method and path: `GET /api/v1/instance`. */
  readonly operation: string;

  constructor(method: string, path: string) {
    super(`fake: ${method} ${path} is no operation the fake API implements`);
    this.operation = `${method} ${path}`;
  }
}

/** The path of the client API under the server's origin. */
const basePath = "/api/v1";

/** The path of a request under the client API's. */
export const apiPath = (request: Request) => new URL(request.url).pathname.replace(basePath, "");

type Matcher = { route: Route; method: string; pattern: RegExp; names: string[] };

/** What finds the route of a request among `routes`: a path of the specification with fewer parameters first. */
export function createMatcher(routes: readonly Route[]): (method: string, path: string) => Found | undefined {
  const matchers = routes
    .map((route): Matcher => {
      const [method = "", template = ""] = route.split(" ");
      const names = [...template.matchAll(/\{(\w+)\}/g)].map((m) => m[1] ?? "");
      const pattern = new RegExp(`^${template.replace(/[.*+?^$()|[\]\\]/g, "\\$&").replace(/\{\w+\}/g, "([^/]+)")}$`);
      return { route, method, pattern, names };
    })
    .toSorted((a, b) => a.names.length - b.names.length);
  return (method, path) => {
    for (const m of matchers) {
      const match = m.method === method ? m.pattern.exec(path) : null;
      if (match === null) continue;
      const params = Object.fromEntries(m.names.map((name, i) => [name, decodeURIComponent(match[i + 1] ?? "")]));
      return { route: m.route, params };
    }
    return undefined;
  };
}

/** The route of a request, with the parameters of its path. */
type Found = { route: Route; params: Record<string, string> };

/** The JSON body of a request, or undefined for one without a JSON body. */
export async function jsonOf(request: Request): Promise<unknown> {
  if (request.headers.get("Content-Type")?.includes("json") !== true) return undefined;
  const text = await request.clone().text();
  return text === "" ? undefined : (JSON.parse(text) as unknown);
}

/** An answer of any operation, as the router reads it. */
type AnyAnswer = ({ status: number; body?: unknown; type?: string; raw?: BodyInit } & Extra) | Refusal;

/** The response of an answer. */
function respond(answer: AnyAnswer): Response {
  if ("problem" in answer)
    return new Response(JSON.stringify(answer.problem), {
      status: answer.problem.status,
      headers: { "Content-Type": "application/problem+json", ...answer.headers },
    });
  if (answer.raw !== undefined)
    return new Response(answer.raw, {
      status: answer.status,
      headers: { "Content-Type": answer.type ?? "", ...answer.headers },
    });
  if (answer.body === undefined) return new Response(null, { status: answer.status, headers: { ...answer.headers } });
  return new Response(JSON.stringify(answer.body), {
    status: answer.status,
    headers: { "Content-Type": "application/json", ...answer.headers },
  });
}

/**
 * What answers a request with the handler of its operation among `handlers`; a request of none throws an
 * UnimplementedError that names it, after `refused` was told.
 */
export function createRouter(
  handlers: Handlers,
  refused: (error: UnimplementedError) => void,
): (request: Request) => Promise<Response> {
  const find = createMatcher(Object.keys(handlers) as Route[]);
  return async (request) => {
    const url = new URL(request.url);
    const path = url.pathname.replace(basePath, "");
    const found = find(request.method, path);
    if (found === undefined) {
      const error = new UnimplementedError(request.method, url.pathname);
      refused(error);
      throw error;
    }
    // The handler of a route takes the request of that route: the table's type holds each to its operation.
    const handler = handlers[found.route] as unknown as (call: {
      params: Record<string, string>;
      query: { get: (name: string) => string | null; all: (name: string) => string[] };
      body: unknown;
      request: Request;
    }) => AnyAnswer | Promise<AnyAnswer>;
    const answer = await handler({
      params: found.params,
      query: { get: (name) => url.searchParams.get(name), all: (name) => url.searchParams.getAll(name) },
      body: await jsonOf(request),
      request,
    });
    return respond(answer);
  };
}
