// SPDX-License-Identifier: AGPL-3.0-only
// The moderators' writes that change what everyone sees, and a report filed by a moderator, are in the lists that
// no page shows when they are made (web-client/README.md S1, ADR-0045): the timeline or the queue opened afterwards shows
// the change from its first render. The server answers every list slowly, as over a real network, so that a list
// shown from a stale cache would be seen.
import { screen, waitFor, within } from "@testing-library/react";
import { delay, http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import {
  otherAccountFixture,
  remoteStatusFixture,
  type Schemas,
  statusPage,
  timelineStatusFixture,
} from "../../test/fixtures";
import { apiBase, operatorHandler, signedInHandler } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import {
  adminAccountFixture,
  newAccountAction,
  newDomainAction,
  registrationFixture,
  reportFixture,
} from "./test/handlers";

/**
 * How long the server takes to answer a list, in ms. The waits keep the setup's asyncUtilTimeout
 * (web-client/README.md T7): a loaded machine stretches this latency well beyond any multiple of it.
 */
const latency = 300;

beforeEach(() => {
  server.use(signedInHandler, operatorHandler(["manage_accounts", "manage_reports", "manage_domains"]));
});

/** U+2066 LEFT-TO-RIGHT ISOLATE … U+2069 POP DIRECTIONAL ISOLATE: a handle inside a sentence. */
function isolated(handle: string): string {
  return `⁦${handle}⁩`;
}

describe("a suspension of an account", () => {
  test("is on the local timeline at once when it opens again", async () => {
    let suspended = false;
    server.use(
      http.post(`${apiBase}/admin/accounts/:id/actions`, async ({ request, params }) => {
        const body = (await request.json()) as Schemas["CreateAccountActionRequest"];
        suspended = body.kind === "suspend";
        return HttpResponse.json(newAccountAction(String(params["id"]), body));
      }),
      http.get(`${apiBase}/timelines/local`, async () => {
        await delay(latency);
        return HttpResponse.json(statusPage(suspended ? [] : [timelineStatusFixture]));
      }),
    );
    const { user, router } = await renderApp({ path: "/local" });
    expect(
      await within(await screen.findByRole("feed", { name: "Local timeline" })).findByRole("article", {
        description: "Hello from the local timeline!",
      }),
    ).toBeInTheDocument();

    await router.navigate({ to: "/moderation/accounts/$id", params: { id: otherAccountFixture.id } });
    const form = await screen.findByRole("region", { name: "Take action" });
    await user.click(within(form).getByRole("radio", { name: "Suspend" }));
    await user.click(within(form).getByRole("button", { name: "Take action" }));
    // The action is taken once the lists it changes have been fetched again.
    expect(await within(form).findByText("Action taken (Suspend).", {})).toBeInTheDocument();

    await router.navigate({ to: "/local" });
    expect(await screen.findByRole("heading", { level: 1, name: "Local timeline" })).toBeInTheDocument();
    // The timeline leaves out Bob's post from its first render.
    expect(screen.getByRole("heading", { level: 2, name: "No posts yet" })).toBeInTheDocument();
  });
});

describe("a limit of a domain", () => {
  test("is on the federated timeline at once when it opens again", async () => {
    let limited = false;
    server.use(
      http.post(`${apiBase}/admin/domain_actions`, async ({ request }) => {
        const body = (await request.json()) as Schemas["CreateDomainActionRequest"];
        limited = body.kind === "limit";
        return HttpResponse.json(newDomainAction(body));
      }),
      http.get(`${apiBase}/timelines/public`, async () => {
        await delay(latency);
        return HttpResponse.json(
          statusPage(limited ? [timelineStatusFixture] : [timelineStatusFixture, remoteStatusFixture]),
        );
      }),
    );
    const { user, router } = await renderApp({ path: "/public" });
    const feed = await screen.findByRole("feed", { name: "Federated timeline" });
    expect(await within(feed).findByText("Cats")).toBeInTheDocument();

    await router.navigate({ to: "/moderation/domain-actions" });
    await user.type(await screen.findByRole("textbox", { name: "Domain" }), "remote.example");
    await user.click(screen.getByRole("radio", { name: "Limit" }));
    await user.click(screen.getByRole("button", { name: "Take action" }));
    // The action is taken once the lists it changes have been fetched again.
    expect(await screen.findByText(`Limit is in effect for ${isolated("remote.example")}.`, {})).toBeInTheDocument();

    await router.navigate({ to: "/public" });
    expect(await screen.findByRole("heading", { level: 1, name: "Federated timeline" })).toBeInTheDocument();
    // The timeline leaves out Carol's post from its first render.
    const again = screen.getByRole("feed", { name: "Federated timeline" });
    expect(within(again).getAllByRole("article")).toHaveLength(1);
    expect(within(again).queryByText("Cats")).not.toBeInTheDocument();
  });
});

describe("an approved registration", () => {
  test("is in the account search at once when it opens again", async () => {
    let approved = false;
    server.use(
      http.get(`${apiBase}/admin/registrations`, () =>
        HttpResponse.json({ items: approved ? [] : [registrationFixture()] }),
      ),
      http.post(`${apiBase}/admin/registrations/:userId/approve`, () => {
        approved = true;
        return HttpResponse.json(registrationFixture({ state: "approved" }));
      }),
      http.get(`${apiBase}/admin/accounts`, async () => {
        await delay(latency);
        const state = approved
          ? { state: "active", registration_state: "approved" }
          : { state: "pending", registration_state: "pending" };
        return HttpResponse.json({
          items: [adminAccountFixture(otherAccountFixture, state as Partial<Schemas["AdminAccount"]>)],
        });
      }),
    );
    const { user, router } = await renderApp({ path: "/moderation/accounts" });
    const results = await screen.findByRole("region", { name: "Results" });
    expect((await within(results).findAllByText("Awaiting approval", {})).length).toBeGreaterThan(0);

    await router.navigate({ to: "/moderation/registrations" });
    await user.click(await screen.findByRole("button", { name: `Approve ${isolated("@bob")}` }));
    await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Approve" }));
    // The decision is done once the searches have been fetched again.
    expect(await screen.findByText(`${isolated("@bob")} is approved and can sign in.`, {})).toBeInTheDocument();

    await router.navigate({ to: "/moderation/accounts" });
    expect(await screen.findByRole("heading", { level: 1, name: "Accounts" })).toBeInTheDocument();
    // The search shows Bob approved from its first render, not pending as before the decision.
    const again = screen.getByRole("region", { name: "Results" });
    expect(within(again).getByText("Active")).toBeInTheDocument();
    expect(within(again).queryAllByText("Awaiting approval")).toHaveLength(0);
  });
});

describe("a report filed by a moderator", () => {
  test("is in the report queue at once when it opens again", async () => {
    const reports: Schemas["AdminReportDetail"][] = [];
    server.use(
      http.get(`${apiBase}/admin/reports`, async () => {
        await delay(latency);
        return HttpResponse.json({ items: reports });
      }),
    );
    // The default handler answers the report; the queue has it from then on.
    server.events.on("request:start", ({ request }) => {
      if (request.method === "POST" && new URL(request.url).pathname === `${apiBase}/reports`)
        reports.push(reportFixture());
    });
    const { user, router } = await renderApp({ path: "/moderation/reports" });
    expect(await screen.findByRole("heading", { level: 2, name: "No open reports" })).toBeInTheDocument();

    await router.navigate({ to: "/local" });
    const article = await within(await screen.findByRole("feed", { name: "Local timeline" })).findByRole("article", {
      description: "Hello from the local timeline!",
    });
    await user.click(await within(article).findByRole("button", { name: "More actions" }));
    await user.click(await screen.findByRole("menuitem", { name: "Report" }));
    const dialog = await screen.findByRole("dialog", { name: "Report ⁨Bob⁩" });
    await user.click(within(dialog).getByRole("radio", { name: "Spam" }));
    await user.click(within(dialog).getByRole("button", { name: "Next" }));
    await user.click(await within(dialog).findByRole("button", { name: "Next" }));
    await user.click(await within(dialog).findByRole("button", { name: "Send report" }));
    // The report is sent once the queue has been fetched again.
    const sent = await screen.findByRole("heading", { name: "Report sent" });
    await waitFor(() => {
      expect(sent).toHaveFocus();
    });
    await user.click(within(dialog).getByRole("button", { name: "Close" }));

    await router.navigate({ to: "/moderation/reports" });
    expect(await screen.findByRole("heading", { level: 1, name: "Reports" })).toBeInTheDocument();
    // The queue shows the new report from its first render, not the empty queue from before it.
    expect(screen.getByRole("link", { name: "Report on ⁨@bob⁩" })).toBeInTheDocument();
  });
});
