// SPDX-License-Identifier: AGPL-3.0-only
// The relays screen through the whole app (web-client/README.md T4, ADR-0061): an administrator with manage_settings sees
// the relays and their state, registers and joins one, sees why the server refused one, leaves a relay, joins it
// again and deletes it after asking; with MSW playing the API.
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import type { Schemas } from "../../test/fixtures";
import { apiBase, operatorHandler, signedInHandler } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import { relayFixture } from "./test/handlers";

const relayId = "0199f0e0-0000-7000-8000-0000000000d9";
const name = "⁦relay.example⁩";

beforeEach(() => {
  document.documentElement.lang = "en";
  document.documentElement.dir = "ltr";
});

describe("the relays screen", () => {
  beforeEach(() => {
    server.use(signedInHandler, operatorHandler(["manage_settings"]));
  });

  test("tells a viewer without the permission why it is empty", async () => {
    server.use(operatorHandler(["manage_webhooks"]));
    await renderApp({ path: "/admin/relays" });

    expect(await screen.findByRole("heading", { level: 2, name: "You cannot use this screen" })).toBeInTheDocument();
  });

  test("lists the relays with their state, last day and actor, and warns of a suspended server", async () => {
    const { actor_uri: _, ...rejected } = relayFixture("0199f0e0-0000-7000-8000-0000000000da", {
      protocol: "litepub",
      url: "https://pleroma.example/relay",
      inbox_url: "https://pleroma.example/inbox",
      state: "rejected",
      receive: false,
    });
    server.use(
      http.get(`${apiBase}/admin/relays`, () =>
        HttpResponse.json({
          items: [
            relayFixture(relayId, { taken_last_day: 120, dropped_last_day: 3, domain_suspended: true }),
            rejected,
          ],
        } satisfies Schemas["AdminRelayList"]),
      ),
    );
    await renderApp({ path: "/admin/relays" });
    const list = await screen.findByRole("region", { name: "Relays of this server" });

    const [joinedRow, rejectedRow] = within(list).getAllByRole("row").slice(1);
    if (joinedRow === undefined || rejectedRow === undefined) throw new Error("two relays expected");
    expect(within(joinedRow).getByRole("rowheader", { name: /^relay\.example/u })).toBeInTheDocument();
    expect(within(joinedRow).getByText("Joined")).toBeInTheDocument();
    expect(within(joinedRow).getByText("https://relay.example/actor")).toBeInTheDocument();
    expect(within(joinedRow).getByText("120 activities")).toBeInTheDocument();
    expect(within(joinedRow).getByText("3 activities")).toBeInTheDocument();
    expect(within(joinedRow).getByText(/A domain action suspends the server of this relay/)).toBeInTheDocument();
    expect(within(joinedRow).getByRole("button", { name: `Leave ${name}` })).toBeInTheDocument();
    expect(within(joinedRow).queryByRole("button", { name: `Delete ${name}` })).not.toBeInTheDocument();

    expect(within(rejectedRow).getByText("LitePub")).toBeInTheDocument();
    expect(within(rejectedRow).getByText("Rejected by the relay")).toBeInTheDocument();
    expect(within(rejectedRow).getByText("Sends only")).toBeInTheDocument();
    expect(within(rejectedRow).getByText("https://pleroma.example/inbox")).toBeInTheDocument();
    expect(within(rejectedRow).getByRole("button", { name: "Join ⁦pleroma.example⁩ again" })).toBeInTheDocument();
    expect(within(rejectedRow).getByRole("button", { name: "Delete ⁦pleroma.example⁩" })).toBeInTheDocument();
  });

  test("registers and joins a relay", async () => {
    let sent: Schemas["CreateRelayRequest"] | undefined;
    server.use(
      http.get(`${apiBase}/admin/relays`, () =>
        HttpResponse.json({
          items:
            sent === undefined
              ? []
              : [
                  relayFixture(relayId, {
                    url: sent.url,
                    inbox_url: sent.url,
                    state: "pending",
                    receive: sent.receive ?? true,
                  }),
                ],
        }),
      ),
      http.post(`${apiBase}/admin/relays`, async ({ request }) => {
        sent = (await request.json()) as Schemas["CreateRelayRequest"];
        return HttpResponse.json(relayFixture(relayId, { url: sent.url, inbox_url: sent.url, state: "pending" }));
      }),
    );
    const { user } = await renderApp({ path: "/admin/relays" });
    const form = await screen.findByRole("region", { name: "Join a relay" });

    expect(within(form).getByRole("checkbox", { name: "Receive posts from the relay" })).toBeChecked();
    await user.type(within(form).getByRole("textbox", { name: "URL" }), "https://relay.example/inbox");
    await user.click(within(form).getByRole("checkbox", { name: "Receive posts from the relay" }));
    await user.click(within(form).getByRole("button", { name: "Join" }));

    expect(await within(form).findByRole("status")).toHaveTextContent(
      `${name} is registered, and the server asked to join it.`,
    );
    expect(sent).toEqual({ url: "https://relay.example/inbox", receive: false });
    expect(within(form).getByRole("textbox", { name: "URL" })).toHaveValue("");
    const list = screen.getByRole("region", { name: "Relays of this server" });
    expect(await within(list).findByText("Awaiting the relay's answer")).toBeInTheDocument();
  });

  test("sends the protocol chosen", async () => {
    let sent: Schemas["CreateRelayRequest"] | undefined;
    server.use(
      http.post(`${apiBase}/admin/relays`, async ({ request }) => {
        sent = (await request.json()) as Schemas["CreateRelayRequest"];
        return HttpResponse.json(relayFixture(relayId, { url: sent.url, protocol: "litepub", state: "pending" }));
      }),
    );
    const { user } = await renderApp({ path: "/admin/relays" });
    const form = await screen.findByRole("region", { name: "Join a relay" });

    await user.type(within(form).getByRole("textbox", { name: "URL" }), "https://pleroma.example/relay");
    await user.click(within(form).getByRole("button", { name: /Protocol/ }));
    await user.click(await screen.findByRole("option", { name: "LitePub" }));
    await user.click(within(form).getByRole("button", { name: "Join" }));

    await waitFor(() => {
      expect(sent).toEqual({ url: "https://pleroma.example/relay", protocol: "litepub", receive: true });
    });
  });

  test("shows why the server refused a relay on its URL", async () => {
    server.use(
      http.post(`${apiBase}/admin/relays`, () =>
        HttpResponse.json(
          {
            type: "/api/v1/problems/invalid-relay",
            title: "The relay cannot be changed that way.",
            status: 422,
            detail: "detail",
            reason: "exists",
          },
          { status: 422, headers: { "Content-Type": "application/problem+json" } },
        ),
      ),
    );
    const { user } = await renderApp({ path: "/admin/relays" });
    const form = await screen.findByRole("region", { name: "Join a relay" });
    const url = within(form).getByRole("textbox", { name: "URL" });
    await user.type(url, "https://relay.example/inbox");
    await user.click(within(form).getByRole("button", { name: "Join" }));

    await waitFor(() => {
      expect(url).toHaveAccessibleDescription(
        expect.stringContaining("A relay with this URL is already registered.") as string,
      );
    });
    expect(url).toHaveValue("https://relay.example/inbox");
  });

  test("leaves a relay and joins it again", async () => {
    let state: Schemas["RelayState"] = "accepted";
    server.use(
      http.get(`${apiBase}/admin/relays`, () => HttpResponse.json({ items: [relayFixture(relayId, { state })] })),
      http.post(`${apiBase}/admin/relays/:id/unfollow`, () => {
        state = "left";
        return HttpResponse.json(relayFixture(relayId, { state }));
      }),
      http.post(`${apiBase}/admin/relays/:id/follow`, () => {
        state = "pending";
        return HttpResponse.json(relayFixture(relayId, { state }));
      }),
    );
    const { user } = await renderApp({ path: "/admin/relays" });
    const list = await screen.findByRole("region", { name: "Relays of this server" });

    await user.click(await within(list).findByRole("button", { name: `Leave ${name}` }));
    expect(await within(list).findByRole("button", { name: `Join ${name} again` })).toBeInTheDocument();
    expect(within(list).getByText("Left")).toBeInTheDocument();
    expect(within(list).getByRole("status")).toHaveTextContent(`The server left ${name}.`);

    await user.click(within(list).getByRole("button", { name: `Join ${name} again` }));
    expect(await within(list).findByRole("button", { name: `Leave ${name}` })).toBeInTheDocument();
    expect(within(list).getByRole("status")).toHaveTextContent(`The server asked to join ${name} again.`);
  });

  test("shows why the server refused to join a relay again", async () => {
    server.use(
      http.get(`${apiBase}/admin/relays`, () =>
        HttpResponse.json({ items: [relayFixture(relayId, { state: "left" })] }),
      ),
      http.post(`${apiBase}/admin/relays/:id/follow`, () =>
        HttpResponse.json(
          {
            type: "/api/v1/problems/invalid-relay",
            title: "The relay cannot be changed that way.",
            status: 422,
            detail: "detail",
            reason: "domain_suspended",
          },
          { status: 422, headers: { "Content-Type": "application/problem+json" } },
        ),
      ),
    );
    const { user } = await renderApp({ path: "/admin/relays" });
    await user.click(await screen.findByRole("button", { name: `Join ${name} again` }));

    const list = screen.getByRole("region", { name: "Relays of this server" });
    expect(await within(list).findByRole("alert")).toHaveTextContent(
      "Could not change the relay: The server of the relay is suspended by a domain action.",
    );
  });

  test("deletes a relay after asking", async () => {
    let deleted = "";
    server.use(
      http.get(`${apiBase}/admin/relays`, () =>
        HttpResponse.json({ items: deleted === "" ? [relayFixture(relayId, { state: "left" })] : [] }),
      ),
      http.delete(`${apiBase}/admin/relays/:id`, ({ params }) => {
        deleted = String(params["id"]);
        return new HttpResponse(null, { status: 204 });
      }),
    );
    const { user } = await renderApp({ path: "/admin/relays" });
    await user.click(await screen.findByRole("button", { name: `Delete ${name}` }));
    const dialog = await screen.findByRole("alertdialog", { name: `Delete ${name}?` });
    await user.click(within(dialog).getByRole("button", { name: "Delete" }));

    expect(await screen.findByRole("heading", { level: 3, name: "No relays yet" })).toBeInTheDocument();
    expect(deleted).toBe(relayId);
  });
});
