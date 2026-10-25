// SPDX-License-Identifier: AGPL-3.0-only
// The errors of the API client. Every failed response becomes an ApiError that carries its RFC 9457
// problem document, as @mycroblog/mykit reads it; a request that never got a response becomes a NetworkError. Both
// have a localized `message` that the UI can show as it is (web-client/README.md E2): the server localizes `title`
// and `detail` from the Accept-Language the client sends, and the client's own catalog covers the rest.
import { problemFromResponse, type components } from "@mycroblog/mykit";
import type { Translate } from "./i18n";

/** An RFC 9457 problem document (the `Problem` schema of api/openapi.yaml). */
export type Problem = components["schemas"]["Problem"];

/**
 * A response with an error status. It is a Problem itself, so it matches the error type that the
 * generated types give every operation; branch on `type` and `status`, never on the text.
 */
export class ApiError extends Error implements Problem {
  override name = "ApiError";
  readonly type: string;
  readonly title: string;
  readonly status: number;
  readonly detail?: string;
  readonly request_id?: string;
  /**
   * When the server takes the request again (ms since the epoch), from the response's Retry-After: a 429 of the
   * rate limit says when the limit lets a request through.
   */
  readonly retryAt?: number;
  /**
   * The problem document as the server sent it, with the members its type adds (`theme_errors` of
   * `/api/v1/problems/invalid-theme`); read them only after branching on `type`.
   */
  readonly document: Readonly<Record<string, unknown>>;

  constructor(problem: Problem, { retryAt }: { retryAt?: number | undefined } = {}) {
    super(problem.detail ?? problem.title);
    this.document = { ...problem };
    this.type = problem.type;
    this.title = problem.title;
    this.status = problem.status;
    if (problem.detail !== undefined) this.detail = problem.detail;
    if (problem.request_id !== undefined) this.request_id = problem.request_id;
    if (retryAt !== undefined) this.retryAt = retryAt;
  }
}

/** The problem type of the 403 for every operation that the closed server refuses (ADR-0088 §2). */
const serviceClosedType = "/api/v1/problems/service-closed";

/** The problem type of the server's maintenance mode (ADR-0084 §3): a 503 to every operation, with Retry-After. */
const maintenanceProblemType = "/api/v1/problems/maintenance";

/**
 * The server is under maintenance (ADR-0084 §3): it answers every operation, and the OAuth endpoints, with a 503 of
 * this problem type. The session is still there, so nothing discards a token or signs out for it; the app shell shows
 * the maintenance in place of the app (lib/maintenance.ts). The members that the problem adds are kept only when they
 * are well formed, since the app shows them.
 */
export class MaintenanceError extends ApiError {
  override name = "MaintenanceError";
  /** The administrators' notice (the problem's `message`), in plain text and in no known language. */
  readonly notice?: string;
  /** When the maintenance is expected to end (the problem's `until`), in ms since the epoch. */
  readonly until?: number;
  /** The instance's status page (the problem's `status_page_url`), an https URL. */
  readonly statusPageUrl?: string;

  constructor(problem: Problem, options: { retryAt?: number | undefined } = {}) {
    super(problem, options);
    const { message, until, status_page_url: statusPageUrl } = this.document;
    if (typeof message === "string" && message.trim() !== "") this.notice = message;
    const end = typeof until === "string" ? Date.parse(until) : Number.NaN;
    if (!Number.isNaN(end)) this.until = end;
    if (typeof statusPageUrl === "string" && URL.parse(statusPageUrl)?.protocol === "https:") this.statusPageUrl = statusPageUrl;
  }
}

/** Whether `problem`, the body of a response with `status`, is the maintenance problem. */
function isMaintenance(problem: Problem, status: number): boolean {
  return status === 503 && problem.type === maintenanceProblemType;
}

/** The media type of `response`'s body, lower-cased and without its parameters. */
function mediaType(response: Response): string | undefined {
  return response.headers.get("Content-Type")?.split(";")[0]?.trim().toLowerCase();
}

/**
 * The MaintenanceError of `response` when it is the maintenance problem, or null for any other response. It reads a
 * copy of the body, so the caller can still read the response.
 */
export async function maintenanceFromResponse(response: Response): Promise<MaintenanceError | null> {
  if (response.status !== 503 || mediaType(response) !== "application/problem+json") return null;
  try {
    const body: unknown = await response.clone().json();
    if (isProblem(body) && isMaintenance(body, response.status)) {
      return new MaintenanceError(body, { retryAt: retryAfterTime(response.headers.get("Retry-After"), Date.now()) });
    }
  } catch {
    // Not JSON after all: some other 503.
  }
  return null;
}

/**
 * When the request that failed with `error` may be sent again, if it failed because the server limits how many
 * requests the user sends (429 Too Many Requests with Retry-After); undefined for any other error.
 */
export function rateLimitedUntil(error: unknown): number | undefined {
  return error instanceof ApiError && error.status === 429 ? error.retryAt : undefined;
}

/**
 * The time a Retry-After value (RFC 9110 §10.2.3) names, in ms since the epoch: a number of seconds after `now`,
 * or an HTTP-date. Undefined when the value is neither.
 */
function retryAfterTime(value: string | null, now: number): number | undefined {
  if (value === null) return undefined;
  const trimmed = value.trim();
  if (/^\d+$/.test(trimmed)) return now + Number(trimmed) * 1000;
  const date = Date.parse(trimmed);
  return Number.isNaN(date) ? undefined : date;
}

/**
 * The message to show for an error of the API client. The generated types call every error a Problem, but
 * the middleware throws an ApiError (a Problem with a message) or a NetworkError.
 */
export function errorMessage(error: unknown): string {
  if (error instanceof Error) return error.message;
  if (isProblem(error)) return error.detail ?? error.title;
  return String(error);
}

/** A request that got no response: the network is down, or the server is unreachable. */
export class NetworkError extends Error {
  override name = "NetworkError";
}

/**
 * What kind of failure an error is, for the words that describe it (web-client/16-look-states-and-wording.md G16): no response
 * (`network`), the server refused the viewer (`forbidden`, 403) or found nothing (`not-found`, 404), the
 * server failed otherwise (`server`), or the error did not come from the API at all (`unexpected`).
 */
export type ErrorKind = "network" | "server" | "not-found" | "forbidden" | "closed" | "unexpected";

/** Classifies an error of the API client by what the user can make of it. */
export function errorKind(error: unknown): ErrorKind {
  if (error instanceof NetworkError) return "network";
  if (!(error instanceof ApiError)) return "unexpected";
  if (error.type === serviceClosedType) return "closed";
  if (error.status === 403) return "forbidden";
  if (error.status === 404) return "not-found";
  return "server";
}

/** Whether `value` has the required members of a Problem. */
function isProblem(value: unknown): value is Problem {
  if (typeof value !== "object" || value === null) return false;
  const { type, title, status, detail } = value as Record<string, unknown>;
  return (
    typeof type === "string" &&
    typeof title === "string" &&
    typeof status === "number" &&
    (detail === undefined || typeof detail === "string")
  );
}

/**
 * The ApiError of a response with an error status, whose problem document @mycroblog/mykit reads. A body that is not a
 * problem document (a proxy's HTML page, say) becomes an `about:blank` problem titled from the client's catalog, and
 * so does a problem without a title. The maintenance problem becomes a MaintenanceError.
 */
export async function apiErrorFromResponse(response: Response, t: Translate): Promise<ApiError> {
  const options = { retryAt: retryAfterTime(response.headers.get("Retry-After"), Date.now()) };
  const fallbackTitle = () => t("client:api.error.status", { status: String(response.status) });
  if (mediaType(response) !== "application/problem+json") {
    await response.body?.cancel().catch(() => undefined);
    return new ApiError({ type: "about:blank", title: fallbackTitle(), status: response.status }, options);
  }
  const { problem } = await problemFromResponse(response);
  const document = problem.title === "" ? { ...problem, title: fallbackTitle() } : problem;
  return isMaintenance(document, response.status) ? new MaintenanceError(document, options) : new ApiError(document, options);
}
