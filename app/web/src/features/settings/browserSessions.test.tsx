// SPDX-License-Identifier: AGPL-3.0-only
// The signed-in browsers page through the whole app (web-client/README.md T4): the list with this browser marked and
// without a button, what each row says, signing out one browser and the others after their dialogs, and a failure that
// keeps the dialog open; with MSW playing the server.
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import type { Schemas } from "../../test/fixtures";
import { apiBase, problem, signedInHandler } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import { describeUserAgent } from "./lib/describeUserAgent";

beforeEach(() => {
  server.use(signedInHandler);
  document.documentElement.lang = "en";
  document.documentElement.dir = "ltr";
});

const chromeOnWindows =
  "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36";
const safariOnIPhone =
  "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1";

const current: Schemas["BrowserSession"] = {
  id: "019a0000-0000-7000-8000-0000000000d1",
  current: true,
  ip: "203.0.113.1",
  user_agent: chromeOnWindows,
  signed_in_at: "2026-10-01T09:00:00Z",
  last_used_at: "2026-10-21T09:00:00Z",
};
const phone: Schemas["BrowserSession"] = {
  id: "019a0000-0000-7000-8000-0000000000d2",
  current: false,
  ip: "198.51.100.2",
  user_agent: safariOnIPhone,
  signed_in_at: "2026-10-10T12:00:00Z",
  last_used_at: "2026-10-20T08:00:00Z",
};
const erased: Schemas["BrowserSession"] = {
  id: "019a0000-0000-7000-8000-0000000000d3",
  current: false,
  signed_in_at: "2026-04-01T12:00:00Z",
};
const tool: Schemas["BrowserSession"] = {
  id: "019a0000-0000-7000-8000-0000000000d4",
  current: false,
  user_agent: "my-tool/1.0",
  signed_in_at: "2026-10-12T12:00:00Z",
};

/** The server's browsers of the viewer, which signing out changes, with what it was asked to do. */
function sessionServer(initial: Schemas["BrowserSession"][], fail = false) {
  let items = initial;
  const revoked: string[] = [];
  let others = 0;
  server.use(
    http.get(`${apiBase}/sessions`, () => HttpResponse.json({ items })),
    http.delete(`${apiBase}/sessions/:id`, ({ params }) => {
      if (fail) return problem(500, "about:blank", "Internal Server Error");
      revoked.push(String(params["id"]));
      items = items.filter((session) => session.id !== params["id"]);
      return new HttpResponse(null, { status: 204 });
    }),
    http.post(`${apiBase}/sessions/revoke_others`, () => {
      if (fail) return problem(500, "about:blank", "Internal Server Error");
      others++;
      items = items.filter((session) => session.current);
      return new HttpResponse(null, { status: 204 });
    }),
  );
  return { revoked, others: () => others };
}

function row(name: string): HTMLElement {
  const item = screen.getAllByRole("listitem").find((li) => within(li).queryByRole("heading", { name }) !== null);
  if (item === undefined) throw new Error(`no row of ${name}`);
  return item;
}

describe("signed-in browsers", () => {
  test("marks this browser, offers it no sign-out, and says what is known of the others", async () => {
    sessionServer([current, phone, erased, tool]);
    await renderApp({ path: "/settings/sessions" });
    await screen.findByRole("heading", { name: "Chrome on Windows" });

    const here = within(row("Chrome on Windows"));
    expect(here.getByText("This browser")).toBeInTheDocument();
    expect(here.getByText("203.0.113.1")).toBeInTheDocument();
    expect(here.queryByRole("button")).not.toBeInTheDocument();

    const other = within(row("Safari on iOS"));
    expect(other.getByText("198.51.100.2")).toBeInTheDocument();
    expect(other.getByRole("button", { name: /Sign out/ })).toBeInTheDocument();

    const unknown = within(row("Unknown browser"));
    expect(unknown.getByText("Unknown")).toBeInTheDocument();
    expect(unknown.getByText("No use recorded")).toBeInTheDocument();

    expect(screen.getByRole("heading", { name: "my-tool/1.0" })).toBeInTheDocument();
  });

  test("signs out one browser after its dialog", async () => {
    const api = sessionServer([current, phone]);
    const { user } = await renderApp({ path: "/settings/sessions" });
    await screen.findByRole("heading", { name: "Safari on iOS" });

    await user.click(within(row("Safari on iOS")).getByRole("button", { name: /Sign out/ }));
    const dialog = await screen.findByRole("alertdialog");
    expect(api.revoked).toEqual([]);
    await user.click(within(dialog).getByRole("button", { name: "Sign out" }));

    await waitFor(() => expect(screen.queryByRole("heading", { name: "Safari on iOS" })).not.toBeInTheDocument());
    expect(api.revoked).toEqual([phone.id]);
    expect(await screen.findByText("The browser is signed out.")).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Chrome on Windows" })).toBeInTheDocument();
  });

  test("signs out the others after its dialog, and then offers it no more", async () => {
    const api = sessionServer([current, phone, tool]);
    const { user } = await renderApp({ path: "/settings/sessions" });
    await screen.findByRole("heading", { name: "Safari on iOS" });

    await user.click(screen.getByRole("button", { name: "Sign out all other browsers" }));
    const dialog = await screen.findByRole("alertdialog");
    await user.click(within(dialog).getByRole("button", { name: "Sign out all other browsers" }));

    await waitFor(() => expect(screen.queryByRole("heading", { name: "Safari on iOS" })).not.toBeInTheDocument());
    expect(api.others()).toBe(1);
    expect(screen.getByRole("heading", { name: "Chrome on Windows" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Sign out all other browsers" })).not.toBeInTheDocument();
  });

  test("a failure keeps the dialog open and says why", async () => {
    const api = sessionServer([current, phone], true);
    const { user } = await renderApp({ path: "/settings/sessions" });
    await screen.findByRole("heading", { name: "Safari on iOS" });

    await user.click(within(row("Safari on iOS")).getByRole("button", { name: /Sign out/ }));
    const dialog = await screen.findByRole("alertdialog");
    await user.click(within(dialog).getByRole("button", { name: "Sign out" }));

    expect(await within(dialog).findByText(/Could not sign the browser out/)).toBeInTheDocument();
    expect(api.revoked).toEqual([]);
  });
});

describe("describeUserAgent", () => {
  test.each([
    [chromeOnWindows, "Chrome", "Windows"],
    [safariOnIPhone, "Safari", "iOS"],
    [
      "Mozilla/5.0 (Macintosh; Intel Mac OS X 14_0) AppleWebKit/605.1.15 Version/17.0 Safari/605.1.15",
      "Safari",
      "macOS",
    ],
    ["Mozilla/5.0 (X11; Linux x86_64; rv:130.0) Gecko/20100101 Firefox/130.0", "Firefox", "Linux"],
    [
      "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/130.0.0.0 Safari/537.36 Edg/130.0.0.0",
      "Edge",
      "Windows",
    ],
    ["Mozilla/5.0 (Linux; Android 14) AppleWebKit/537.36 Chrome/130.0.0.0 Mobile Safari/537.36", "Chrome", "Android"],
    ["my-tool/1.0", undefined, undefined],
  ])("%s", (userAgent, browser, os) => {
    expect(describeUserAgent(userAgent)).toEqual({ browser, os });
  });
});
