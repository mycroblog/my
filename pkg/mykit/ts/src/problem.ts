// SPDX-License-Identifier: MIT
import type { components } from "./schema.gen.js";

/** The RFC 9457 problem document of an answer with a status of 400 or more, with its extension members. */
export type Problem = components["schemas"]["Problem"] & Record<string, unknown>;

/** The type of a problem that means no more than its HTTP status (RFC 9457 §4.2.1). */
export const problemTypeAboutBlank = "about:blank";

/**
 * The error of an answer with a status of 400 or more: its RFC 9457 problem document.
 *
 * ```ts
 * try { await api.POST("/statuses", { body }); }
 * catch (e) { if (e instanceof ProblemError && e.type === "/api/v1/problems/blocking") { ... } }
 * ```
 *
 * Branch on `type` and `status`, never on `title` or `detail`, which the server localizes from Accept-Language. The
 * members beyond the standard ones (such as `reason`) are in `problem`.
 */
export class ProblemError extends Error {
  override readonly name = "ProblemError";
  /** The whole problem document, extension members included. */
  readonly problem: Problem;
  /** The headers of the answer, such as Retry-After and WWW-Authenticate. */
  readonly headers: Headers;

  constructor(problem: Problem, headers: Headers) {
    super(problemMessage(problem));
    this.problem = problem;
    this.headers = headers;
  }

  /** The URI that identifies the problem type, `about:blank` when the problem means no more than its status. */
  get type(): string {
    return this.problem.type;
  }

  /** The HTTP status of the answer. */
  get status(): number {
    return this.problem.status;
  }

  /** The localized summary of the problem type. */
  get title(): string {
    return this.problem.title;
  }

  /** The localized explanation of this occurrence. */
  get detail(): string | undefined {
    return this.problem.detail;
  }
}

function problemMessage(p: Problem): string {
  let s = `mykit: ${String(p.status)} ${p.type}`;
  if (p.title !== "") s += `: ${p.title}`;
  if (p.detail !== undefined && p.detail !== "") s += `: ${p.detail}`;
  return s;
}

/**
 * Reads the problem of an error answer. An answer that is no problem document (from a proxy, say) becomes a problem
 * of type about:blank with the answer's status.
 */
export async function problemFromResponse(response: Response): Promise<ProblemError> {
  let doc: Record<string, unknown> = {};
  if (isJSON(response.headers.get("Content-Type"))) {
    try {
      const parsed: unknown = JSON.parse(await response.text());
      if (typeof parsed === "object" && parsed !== null && !Array.isArray(parsed)) doc = parsed as Record<string, unknown>;
    } catch {
      // Not JSON after all: the fallback below.
    }
  } else {
    await response.body?.cancel();
  }
  const problem: Problem = {
    ...doc,
    type: typeof doc["type"] === "string" && doc["type"] !== "" ? doc["type"] : problemTypeAboutBlank,
    title: typeof doc["title"] === "string" && doc["title"] !== "" ? doc["title"] : response.statusText,
    // The status of the answer is the one that happened, whatever the document says.
    status: response.status,
  };
  if (typeof doc["detail"] !== "string") delete problem.detail;
  return new ProblemError(problem, response.headers);
}

/** Whether a Content-Type is JSON, application/problem+json among them. */
function isJSON(contentType: string | null): boolean {
  const mt = contentType?.split(";")[0]?.trim().toLowerCase() ?? "";
  return mt === "application/json" || mt.endsWith("+json");
}

/**
 * The failure to get an answer from the server: the connection, TLS, or the fetch implementation. Its `cause` is the
 * error of `fetch`. An aborted request is not one: it rejects with the reason of its signal (an AbortError).
 */
export class NetworkError extends Error {
  override readonly name = "NetworkError";
  readonly method: string;
  readonly url: string;

  constructor(method: string, url: string, cause: unknown) {
    super(`mykit: ${method} ${url}: ${cause instanceof Error ? cause.message : String(cause)}`, { cause });
    this.method = method;
    this.url = url;
  }
}
