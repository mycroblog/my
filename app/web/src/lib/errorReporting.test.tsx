// SPDX-License-Identifier: AGPL-3.0-only
// The reports of the web client's uncaught errors (ADR-0103 §6) against MSW: what each place of capture sends, what
// is never sent, the one report per error and page view, the beacon while the page is left, and the size of a body.
import { notFound, redirect } from "@tanstack/react-router";
import { render, screen, waitFor } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { ErrorBoundary } from "react-error-boundary";
import { afterEach, expect, test, vi } from "vitest";
import { apiBase, oauthPaths, tokenResponse } from "../test/msw/http";
import { server } from "../test/msw/server";
import { createApi } from "./api/client";
import { Session } from "./auth/session";
import { ApiError, NetworkError } from "./errors";
import {
  ErrorReporter,
  type ErrorReporterOptions,
  ErrorReporterProvider,
  maxReportsPerPage,
  useBoundaryErrorReport,
} from "./errorReporting";
import { loadI18n } from "./i18n";
import { AppLocale } from "./locale";
import { Maintenance } from "./maintenance";

const apiOrigin = window.location.origin;
const cleanups: (() => void)[] = [];

afterEach(() => {
  for (const cleanup of cleanups.splice(0)) cleanup();
});

type Received = { body: Record<string, unknown>; authorization: string | null; contentType: string | null };

/** Records every report the server receives. */
function receiveReports(): Received[] {
  const received: Received[] = [];
  server.use(
    http.post(`${apiBase}/client_errors`, async ({ request }) => {
      received.push({
        body: (await request.json()) as Record<string, unknown>,
        authorization: request.headers.get("Authorization"),
        contentType: request.headers.get("Content-Type"),
      });
      return new HttpResponse(null, { status: 204 });
    }),
  );
  return received;
}

/** An ErrorReporter on a real API client, whose stacks all count as the app's own unless `options` says otherwise. */
async function setUp(options: Partial<ErrorReporterOptions> = {}) {
  const maintenance = new Maintenance({ reload: () => undefined });
  const session = new Session({ apiOrigin, maintenance });
  cleanups.push(() => {
    session.dispose();
  });
  const i18n = await loadI18n("en");
  const api = createApi({ apiOrigin, session, locale: new AppLocale(i18n), maintenance });
  const reporter = new ErrorReporter({ api, apiOrigin, release: "v0.1.0", isOwnStack: () => true, ...options });
  return { reporter, session, api };
}

/** An error with a stack of the app's own script. */
function ownError(message = "boom", name = "TypeError"): Error {
  const error = new Error(message);
  error.name = name;
  error.stack = `${name}: ${message}\n    at render (https://my.example/assets/StatusPage-Dk2l_9a3.js:1:23)`;
  return error;
}

test("sends an error with its name, message, stack, release, and kind as JSON", async () => {
  const received = receiveReports();
  const { reporter } = await setUp();
  const error = ownError();

  reporter.report(error, "boundary_error");

  await waitFor(() => {
    expect(received).toHaveLength(1);
  });
  expect(received[0]?.contentType).toMatch(/^application\/json/);
  expect(received[0]?.body).toEqual({
    name: "TypeError",
    message: "boom",
    stack: error.stack,
    release: "v0.1.0",
    kind: "boundary_error",
  });
});

test("sends the access token of a signed-in user", async () => {
  server.use(http.post(oauthPaths.refresh, () => tokenResponse("access-1")));
  const received = receiveReports();
  const { reporter, session } = await setUp();
  await session.restore();

  reporter.report(ownError(), "uncaught");

  await waitFor(() => {
    expect(received).toHaveLength(1);
  });
  expect(received[0]?.authorization).toBe("Bearer access-1");
});

test("sends the same error once per page view, and at most maxReportsPerPage errors", async () => {
  const received = receiveReports();
  const { reporter } = await setUp();

  // The same error caught again (a page rendered again), and another error object with the same stack.
  const error = ownError("same");
  reporter.report(error, "route_error");
  reporter.report(error, "route_error");
  reporter.report(ownError("same"), "uncaught");
  for (let i = 0; i < maxReportsPerPage + 5; i++) reporter.report(ownError(`error ${i}`), "uncaught");

  await waitFor(() => {
    expect(received).toHaveLength(maxReportsPerPage);
  });
  // Let any extra request arrive before counting.
  await new Promise((resolve) => setTimeout(resolve, 50));
  expect(received).toHaveLength(maxReportsPerPage);
  expect(received.filter(({ body }) => body["message"] === "same")).toHaveLength(1);
});

test("sends nothing for the errors of the API client, the router, aborts, removed chunks, and other scripts", async () => {
  const received = receiveReports();
  const { reporter } = await setUp({ isOwnStack: (stack) => stack.includes("https://my.example/") });
  const listening = reporter.listen(window);
  cleanups.push(listening);

  reporter.report(new ApiError({ type: "about:blank", title: "Internal Server Error", status: 500 }), "route_error");
  reporter.report(new NetworkError("offline"), "route_error");
  reporter.report(notFound(), "route_error");
  reporter.report(redirect({ to: "/home" }), "route_error");
  reporter.report(new DOMException("aborted", "AbortError"), "unhandledrejection");
  reporter.report("a string", "unhandledrejection");
  reporter.report(
    ownError("Failed to fetch dynamically imported module: https://my.example/assets/Page-1a2B.js"),
    "route_error",
  );
  const removed = ownError("a chunk failed");
  window.dispatchEvent(Object.assign(new Event("vite:preloadError", { cancelable: true }), { payload: removed }));
  reporter.report(removed, "route_error");
  const extension = new Error("from an extension");
  extension.stack = "Error: from an extension\n    at x (chrome-extension://abc/content.js:1:2)";
  reporter.report(extension, "uncaught");
  // One that is reported, so that the test knows the others had their chance.
  reporter.report(ownError("reported"), "uncaught");

  await waitFor(() => {
    expect(received).toHaveLength(1);
  });
  await new Promise((resolve) => setTimeout(resolve, 50));
  expect(received.map(({ body }) => body["message"])).toEqual(["reported"]);
});

test("reports the window's uncaught errors and unhandled rejections", async () => {
  const received = receiveReports();
  const { reporter } = await setUp();
  cleanups.push(reporter.listen(window));

  window.dispatchEvent(new ErrorEvent("error", { error: ownError("thrown") }));
  // A cross-origin script's error carries no error object.
  window.dispatchEvent(new ErrorEvent("error", { message: "Script error." }));
  window.dispatchEvent(Object.assign(new Event("unhandledrejection"), { reason: ownError("rejected") }));

  await waitFor(() => {
    expect(received).toHaveLength(2);
  });
  expect(received.map(({ body }) => [body["kind"], body["message"]])).toEqual([
    ["uncaught", "thrown"],
    ["unhandledrejection", "rejected"],
  ]);
});

test("sends a beacon without the token while the page is left, and fetch again once it is shown", async () => {
  const received = receiveReports();
  const beacons: { url: string; data: Blob }[] = [];
  Object.defineProperty(navigator, "sendBeacon", {
    configurable: true,
    value: (url: string, data: Blob) => {
      beacons.push({ url, data });
      return true;
    },
  });
  cleanups.push(() => {
    Reflect.deleteProperty(navigator, "sendBeacon");
  });
  const { reporter } = await setUp();
  cleanups.push(reporter.listen(window));

  window.dispatchEvent(new Event("pagehide"));
  reporter.report(ownError("while leaving"), "unhandledrejection");
  window.dispatchEvent(new Event("pageshow"));
  reporter.report(ownError("after coming back"), "uncaught");

  expect(beacons).toHaveLength(1);
  expect(beacons[0]?.url).toBe(`${apiOrigin}/api/v1/client_errors`);
  expect(beacons[0]?.data.type).toBe("application/json");
  expect(JSON.parse((await beacons[0]?.data.text()) ?? "")).toMatchObject({
    message: "while leaving",
    kind: "unhandledrejection",
  });
  await waitFor(() => {
    expect(received).toHaveLength(1);
  });
  expect(received[0]?.body["message"]).toBe("after coming back");
});

test("keeps a body under 16 KiB", async () => {
  const received = receiveReports();
  const { reporter } = await setUp();
  const error = ownError("é".repeat(4000));
  error.stack = `${error.stack ?? ""}\n${"    at frame (https://my.example/assets/index-1a2B.js:1:2) ñ\n".repeat(400)}`;

  reporter.report(error, "uncaught");

  await waitFor(() => {
    expect(received).toHaveLength(1);
  });
  const bytes = new TextEncoder().encode(JSON.stringify(received[0]?.body)).length;
  expect(bytes).toBeLessThanOrEqual(16 * 1024);
  expect(received[0]?.body["message"]).toBe("é".repeat(4000));
});

/** A component that fails while it renders. */
function Broken(): never {
  throw ownError("in a block");
}

/** A block in a react-error-boundary that reports what it catches. */
function ReportedBlock() {
  const onError = useBoundaryErrorReport();
  return (
    <ErrorBoundary fallback={<p>failed</p>} onError={onError}>
      <Broken />
    </ErrorBoundary>
  );
}

test("reports what a react-error-boundary catches as a boundary_error", async () => {
  const received = receiveReports();
  const { reporter } = await setUp();
  // React logs the caught error.
  vi.spyOn(console, "error").mockImplementation(() => undefined);

  render(
    <ErrorReporterProvider reporter={reporter}>
      <ReportedBlock />
    </ErrorReporterProvider>,
  );

  expect(await screen.findByText("failed")).toBeInTheDocument();
  await waitFor(() => {
    expect(received).toHaveLength(1);
  });
  expect(received[0]?.body).toMatchObject({ kind: "boundary_error", message: "in a block" });
});
