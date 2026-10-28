// SPDX-License-Identifier: AGPL-3.0-only
// The service closure screen through the whole app (web-client/README.md T4, FR-808, FR-809, ADR-0088): an
// administrator with manage_service_closure announces the closure and is warned of a short notice, sees the check
// before closing, withdraws the announcement after an alert dialog, closes the server only after typing its domain,
// and follows the progress of the deliveries; a viewer without the permission sees why; with MSW playing the API.
import { screen, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import type { Schemas } from "../../test/fixtures";
import { apiBase, operatorHandler, signedInHandler } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";

beforeEach(() => {
  document.documentElement.lang = "en";
  document.documentElement.dir = "ltr";
  server.use(signedInHandler, operatorHandler(["manage_service_closure"]));
});

type Closure = Schemas["AdminServiceClosure"];

const check: Schemas["AdminServiceClosureCheck"] = {
  local_accounts: 12,
  delivery_inboxes: 340,
  estimated_deletes: 4080,
  live_supporter_subscriptions: 3,
  pending_migrations: 2,
  retention_holds: 1,
};

const pastDate = "2026-10-01T00:00:00Z";
const farDate = "2099-01-01T00:00:00Z";

/** Plays the closure of the server; records the writes, and each write moves the state like the server. */
function closureServer(initial: Closure) {
  let closure = initial;
  const writes: { method: string; body: unknown }[] = [];
  const record = async (request: Request) => {
    const text = await request.clone().text();
    writes.push({ method: request.method, body: text === "" ? undefined : JSON.parse(text) });
  };
  server.use(
    http.get(`${apiBase}/admin/closure`, () => HttpResponse.json(closure)),
    http.get(`${apiBase}/admin/closure/check`, () => HttpResponse.json(check)),
    http.put(`${apiBase}/admin/closure/announcement`, async ({ request }) => {
      await record(request);
      const body = (await request.json()) as Schemas["AnnounceServiceClosureRequest"];
      closure = {
        state: "announced",
        closes_at: body.closes_at,
        export_until: body.export_until ?? body.closes_at,
        ...(body.message === undefined ? {} : { message: body.message }),
        announced_at: "2026-10-25T00:00:00Z",
        short_notice: Date.parse(body.closes_at) - Date.now() < 30 * 86_400_000,
      };
      return HttpResponse.json(closure);
    }),
    http.delete(`${apiBase}/admin/closure/announcement`, async ({ request }) => {
      await record(request);
      closure = { state: "none", short_notice: false };
      return HttpResponse.json(closure);
    }),
    http.post(`${apiBase}/admin/closure/close`, async ({ request }) => {
      await record(request);
      closure = {
        ...closure,
        state: "closed",
        progress: {
          accounts: 0,
          fanouts_pending: 12,
          deliveries: { pending: 0, succeeded: 0, permanent_failure: 0, skipped: 0, dead: 0 },
        },
      };
      return HttpResponse.json(closure);
    }),
  );
  return { writes };
}

const announced = (closesAt: string, short = false): Closure => ({
  state: "announced",
  closes_at: closesAt,
  export_until: "2099-02-01T00:00:00Z",
  message: "Thank you for the years.",
  announced_at: "2026-10-20T00:00:00Z",
  short_notice: short,
});

describe("the service closure screen", () => {
  test("announces the closure and warns when the notice is short", async () => {
    const closure = closureServer({ state: "none", short_notice: false });
    const { user } = await renderApp({ path: "/admin/closure" });
    expect(await screen.findByRole("heading", { level: 1, name: "Service closure" })).toBeInTheDocument();
    const form = await screen.findByRole("region", { name: "Announce the closure" });

    // Without a closing date, the form says what is missing and sends nothing.
    await user.click(within(form).getByRole("button", { name: "Announce the closure" }));
    expect(await within(form).findByText("Enter the closing date.")).toBeInTheDocument();
    expect(closure.writes).toHaveLength(0);

    const soon = new Date(Date.now() + 5 * 86_400_000);
    const local = `${String(soon.getFullYear())}-${String(soon.getMonth() + 1).padStart(2, "0")}-${String(soon.getDate()).padStart(2, "0")}T12:00`;
    await user.type(within(form).getByLabelText("Closing date"), local);
    expect(await within(form).findByText(/less than 30 days/u)).toBeInTheDocument();
    await user.type(within(form).getByRole("textbox", { name: "Message to the users" }), "See you");
    await user.click(within(form).getByRole("button", { name: "Announce the closure" }));

    expect(await screen.findByText("The closure is announced. The users see the notice now.")).toBeInTheDocument();
    expect(closure.writes).toHaveLength(1);
    expect(closure.writes[0]?.body).toMatchObject({ message: "See you" });
    expect(await screen.findByRole("region", { name: "Announcement in effect" })).toBeInTheDocument();
  });

  test("shows the check before the closing, and closes only once the domain is typed", async () => {
    const closure = closureServer(announced(pastDate));
    const { user } = await renderApp({ path: "/admin/closure" });
    const section = await screen.findByRole("region", { name: "Close the server" });

    expect(within(section).getByText("About 4,080 (12 accounts, 340 inboxes)")).toBeInTheDocument();
    expect(
      within(section).getByText("Let the migrations end before closing; the closure sends its Deletes after them."),
    ).toBeInTheDocument();
    expect(
      within(section).getByText("Closing cancels these subscriptions at once, without a refund."),
    ).toBeInTheDocument();
    expect(within(section).getByText(/Export their records with the CLI/u)).toBeInTheDocument();

    await user.click(within(section).getByRole("button", { name: "Close the server" }));
    const dialog = await screen.findByRole("alertdialog", { name: "Close the server for good?" });
    const field = within(dialog).getByRole("textbox", { name: "Server's domain" });

    // A wrong domain does not close.
    await user.type(field, "other.example");
    await user.click(within(dialog).getByRole("button", { name: "Close the server" }));
    expect(closure.writes).toHaveLength(0);
    expect(screen.getByRole("alertdialog")).toBeInTheDocument();
    expect(await within(dialog).findByText("This is not the server's domain.")).toBeInTheDocument();
    expect(field).toHaveAttribute("aria-invalid", "true");
    expect(field).toHaveFocus();

    await user.clear(field);
    await user.type(field, "MY.example");
    await user.click(within(dialog).getByRole("button", { name: "Close the server" }));
    expect(await screen.findByText("The server is closed. Its deliveries start now.")).toBeInTheDocument();
    expect(closure.writes).toEqual([{ method: "POST", body: { confirm_domain: "MY.example" } }]);
    expect(await screen.findByRole("region", { name: "Progress of the deliveries" })).toBeInTheDocument();
  });

  test("cannot close before the closing date", async () => {
    closureServer(announced(farDate, false));
    await renderApp({ path: "/admin/closure" });
    const section = await screen.findByRole("region", { name: "Close the server" });
    expect(within(section).getByRole("button", { name: "Close the server" })).toBeDisabled();
    expect(within(section).getByText(/The server can be closed from/u)).toBeInTheDocument();
  });

  test("warns of the short notice of an announcement", async () => {
    closureServer(announced(farDate, true));
    await renderApp({ path: "/admin/closure" });
    const section = await screen.findByRole("region", { name: "Announcement in effect" });
    expect(within(section).getByText(/less than 30 days/u)).toBeInTheDocument();
  });

  test("withdraws the announcement after an alert dialog", async () => {
    const closure = closureServer(announced(farDate));
    const { user } = await renderApp({ path: "/admin/closure" });
    const section = await screen.findByRole("region", { name: "Announcement in effect" });

    await user.click(within(section).getByRole("button", { name: "Withdraw the announcement" }));
    const dialog = await screen.findByRole("alertdialog", { name: "Withdraw the announcement?" });
    await user.click(within(dialog).getByRole("button", { name: "Withdraw the announcement" }));

    expect(await screen.findByText("The announcement is withdrawn. The server goes on as before.")).toBeInTheDocument();
    expect(closure.writes).toEqual([{ method: "DELETE", body: undefined }]);
    expect(await screen.findByRole("region", { name: "Announce the closure" })).toBeInTheDocument();
  });

  test("shows the progress of the deliveries of a closed server", async () => {
    closureServer({
      state: "closed",
      closes_at: pastDate,
      export_until: farDate,
      short_notice: false,
      progress: {
        accounts: 12,
        fanouts_pending: 1,
        deliveries: { pending: 300, succeeded: 3000, permanent_failure: 20, skipped: 5, dead: 2 },
      },
    });
    await renderApp({ path: "/admin/closure" });
    const progress = await screen.findByRole("region", { name: "Progress of the deliveries" });
    expect(within(progress).getByText(/Keep the workers running/u)).toBeInTheDocument();
    expect(within(progress).getByText("3,000")).toBeInTheDocument();
    expect(within(progress).getByText("Out of attempts")).toBeInTheDocument();
  });

  test("says why it shows nothing without the permission", async () => {
    server.use(operatorHandler(["manage_rules"]));
    await renderApp({ path: "/admin/closure" });
    expect(await screen.findByRole("heading", { level: 1, name: "Service closure" })).toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Announce the closure" })).not.toBeInTheDocument();
  });
});
