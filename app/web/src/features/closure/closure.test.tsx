// SPDX-License-Identifier: AGPL-3.0-only
// The closure of the service through the whole app (web-client/README.md T4, FR-808, FR-809, ADR-0088 §2): the notice on every
// screen while it is announced, the sign-in page and the information page among them, and the closed screen, which is
// all a signed-in user gets after the server has closed.
import { screen, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { describe, expect, test } from "vitest";
import { ApiError, errorKind } from "../../lib/errors";
import { instanceFixture } from "../../test/fixtures";
import { apiBase, signedInHandler } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";

function serveClosure(closure: unknown) {
  server.use(http.get(`${apiBase}/instance`, () => HttpResponse.json({ ...instanceFixture, closure })));
}

const announced = {
  state: "announced",
  closes_at: "2026-12-01T00:00:00Z",
  export_until: "2027-01-15T00:00:00Z",
  message: "Thank you for the good years.",
};

describe("the notice of the closure", () => {
  test("shows on the sign-in page and the information page, with the dates, the message and the way to the export", async () => {
    serveClosure(announced);
    await renderApp({ path: "/login" });

    const notice = await screen.findByRole("region", { name: "Notice of the server's closure" });
    expect(notice).toHaveTextContent("my example will close on December 1, 2026.");
    expect(notice).toHaveTextContent("until January 15, 2027");
    expect(within(notice).getByText("Thank you for the good years.")).toHaveAttribute("dir", "auto");
    expect(within(notice).getByRole("link", { name: "Export your data" })).toHaveAttribute("href", "/settings/account");
    // Not closable, and not a live region that interrupts.
    expect(within(notice).queryByRole("button")).not.toBeInTheDocument();
  });

  test("shows on the information page too", async () => {
    serveClosure(announced);
    await renderApp({ path: "/about" });
    expect(await screen.findByRole("region", { name: "Notice of the server's closure" })).toHaveTextContent("will close on December 1, 2026");
  });

  test("is absent when no closure is announced", async () => {
    await renderApp({ path: "/login" });
    await screen.findByRole("heading", { level: 1 });
    expect(screen.queryByRole("region", { name: "Notice of the server's closure" })).not.toBeInTheDocument();
  });
});

describe("a closed server", () => {
  const closed = { ...announced, state: "closed" };

  test("tells a visitor who is not signed in that it has closed, and until when signing in takes the data out", async () => {
    serveClosure(closed);
    await renderApp({ path: "/login" });
    const notice = await screen.findByRole("region", { name: "Notice of the server's closure" });
    expect(notice).toHaveTextContent("my example has closed.");
    expect(notice).toHaveTextContent("until January 15, 2027");
  });

  test("gives a signed-in user the closed screen with the exports and sign-out, and nothing else of the app", async () => {
    serveClosure(closed);
    server.use(signedInHandler);
    await renderApp({ path: "/home" });

    expect(await screen.findByRole("heading", { level: 1, name: "This server has closed" })).toBeInTheDocument();
    expect(screen.getByText(/my example closed on December 1, 2026/u)).toBeInTheDocument();
    expect(screen.getByText("Thank you for the good years.")).toBeInTheDocument();
    expect(await screen.findByRole("button", { name: "Prepare an archive" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Sign out" })).toBeInTheDocument();
    expect(screen.queryByRole("navigation")).not.toBeInTheDocument();
    expect(document.title).toBe("This server has closed – my example");
  });
});

describe("a request the closed server refuses", () => {
  test("is a closed failure, not a missing permission", () => {
    const refused = new ApiError({ type: "/api/v1/problems/service-closed", title: "Closed", status: 403 });
    expect(errorKind(refused)).toBe("closed");
    expect(errorKind(new ApiError({ type: "about:blank", title: "Forbidden", status: 403 }))).toBe("forbidden");
  });
});
