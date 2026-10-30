// SPDX-License-Identifier: AGPL-3.0-only
// The client API client (ADR-0029, ADR-0093 §3): the client of @mycroblog/mykit, typed by the types it generates from
// api/openapi.yaml, and openapi-react-query on top of it for the `queryOptions` of features/<x>/api
// (web-client/README.md S1). mykit sends the session's access token as a Bearer token, and after a 401 refreshes it
// once through the session (a TokenSource) and sends the request again; it sends the locale the app is displayed in
// as Accept-Language, read for every request, as the user can change it while the page is open, so that the server
// localizes its problems too. The app does not let it retry a 429 or a 503: the app shows them (the rate limit, the
// maintenance) instead.
//
// The middleware of the app on top of it
//
// - turns every response with an error status into an ApiError and every network failure into a NetworkError, with a
//   localized message (errors.ts);
// - tells lib/network whether the server answered, which decides whether the app is offline (web-client/16-look-states-and-wording.md G17);
// - tells lib/maintenance when the server answers with its maintenance problem (a MaintenanceError, ADR-0084 §3), and
//   when it answers again after that. The 503 of the maintenance is no failure of the session: it never discards the
//   token, counts as a failed refresh, or signs out, on a request or on the refresh after a 401.
//
// `stream` opens the event stream (lib/sse) with the same origin, token, and locale.
import {
  createClient,
  type Api as MykitApi,
  NetworkError as MykitNetworkError,
  type paths,
  runStream,
  type StreamOptions,
  type TokenSource,
} from "@mycroblog/mykit";
import type { Middleware } from "openapi-fetch";
import createQueryClient, { type OpenapiQueryClient } from "openapi-react-query";
import type { Session } from "../auth/session";
import type { AppLocale } from "../locale";
import { apiErrorFromResponse, MaintenanceError, maintenanceFromResponse, NetworkError } from "../errors";
import type { Maintenance } from "../maintenance";
import { isBrowserOffline, reportNetworkFailure, reportResponse } from "../network";

/** What the app's stream gives runStream: everything but the connection, which the Api holds. */
type EventStreamOptions = Pick<StreamOptions, "signal" | "timeline" | "onOpen" | "onEvent" | "onError">;

export type Api = {
  /** The openapi-fetch client of @mycroblog/mykit, for loaders and mutations. */
  fetch: MykitApi;
  /** The openapi-react-query client, for `queryOptions` and `useMutation`. */
  query: OpenapiQueryClient<paths>;
  /**
   * Keeps GET /streaming (or, with `timeline`, GET /streaming/timelines/{timeline}) open until `signal` aborts,
   * reconnecting after every end or failure (mykit's runStream). It rejects only when the server refuses the stream for
   * good (a 4xx other than 408 and 429).
   */
  stream: (options: EventStreamOptions) => Promise<void>;
};

export type CreateApiOptions = {
  /** The origin of the client API (the runtime config's apiOrigin). */
  apiOrigin: string;
  session: Session;
  /** The locale the app is displayed in, sent as Accept-Language, and its messages for errors the server did not describe. */
  locale: Pick<AppLocale, "locale" | "t">;
  /** Whether the server is under maintenance, which the client reports to. */
  maintenance: Maintenance;
};

export function createApi({ apiOrigin, session, locale, maintenance }: CreateApiOptions): Api {
  const acceptLanguage = () => locale.locale;
  const fetch = createClient({ server: apiOrigin, tokenSource: session, acceptLanguage, retry: { disabled: true } });
  fetch.use(errorMiddleware(locale, maintenance));
  return {
    fetch,
    query: createQueryClient(fetch),
    stream: (options) =>
      runStream({
        ...options,
        server: apiOrigin,
        tokenSource: streamTokenSource(session, apiOrigin),
        acceptLanguage,
        fetch: reportingFetch(maintenance),
      }),
  };
}

/** Whether `error` is the abort of a request (TanStack Query cancelling a query, or a stream that was closed). */
function isAbort(error: unknown): boolean {
  return error instanceof DOMException && error.name === "AbortError";
}

/**
 * Throws an ApiError for every response with an error status, and turns network failures into a NetworkError, whose
 * message says that the app is offline when the browser has no network. The maintenance problem is reported to
 * `maintenance`, and so is the first answer after it. It sees the answer after mykit's refresh, and throws before
 * mykit's own ProblemError.
 */
function errorMiddleware(locale: Pick<AppLocale, "t">, maintenance: Maintenance): Middleware {
  return {
    async onResponse({ response }) {
      reportResponse();
      if (response.ok) {
        maintenance.answered();
        return undefined;
      }
      const error = await apiErrorFromResponse(response, locale.t);
      if (error instanceof MaintenanceError) maintenance.report(error);
      throw error;
    },
    onError({ error }) {
      // An aborted request (TanStack Query cancelling a query) stays an AbortError.
      if (isAbort(error)) return undefined;
      // The refresh after a 401 met the maintenance: the token stays.
      if (error instanceof MaintenanceError) {
        maintenance.report(error);
        return error;
      }
      reportNetworkFailure();
      return new NetworkError(locale.t(isBrowserOffline() ? "client:api.error.offline" : "client:api.error.network"), {
        cause: error,
      });
    },
  };
}

/**
 * The session as the TokenSource of the stream. A refresh that cannot reach the server, or meets the maintenance, is a
 * mykit NetworkError there, which the stream recovers from by connecting again later, as from any other network failure.
 */
function streamTokenSource(session: Session, apiOrigin: string): TokenSource {
  return {
    getAccessToken: () => session.getAccessToken(),
    async refresh(refused) {
      try {
        return await session.refresh(refused);
      } catch (error) {
        if (error instanceof NetworkError || error instanceof MaintenanceError)
          throw new MykitNetworkError("POST", new URL("/oauth/token/refresh", apiOrigin).href, error);
        throw error;
      }
    },
  };
}

/**
 * The fetch of the stream, which tells lib/network and lib/maintenance what it met, as errorMiddleware does for the
 * other requests.
 */
function reportingFetch(maintenance: Maintenance): (request: Request) => Promise<Response> {
  return async (request) => {
    let response: Response;
    try {
      response = await globalThis.fetch(request);
    } catch (error) {
      if (!isAbort(error) && !request.signal.aborted) reportNetworkFailure();
      throw error;
    }
    reportResponse();
    if (response.ok) {
      maintenance.answered();
    } else {
      const underMaintenance = await maintenanceFromResponse(response);
      if (underMaintenance !== null) maintenance.report(underMaintenance);
    }
    return response;
  };
}
