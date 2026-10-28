// SPDX-License-Identifier: AGPL-3.0-only
// Refetching remote posts from their servers through the whole app (web-client/README.md T4, FR-407): a viewer with
// manage_domains asks for the posts of a remote server, of a remote account, and for one reported remote post;
// MSW plays the admin API. A viewer without manage_domains is not offered it.
import { screen, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import { otherAccountFixture, remoteAccountFixture, remoteStatusFixture, statusFixture } from "../../test/fixtures";
import { apiBase, operatorHandler, problem, signedInHandler } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import { adminAccountDetailFixture, reportFixture } from "./test/handlers";

beforeEach(() => {
  server.use(signedInHandler);
  document.documentElement.lang = "en";
  document.documentElement.dir = "ltr";
});

/** The refetch requests the app sent, as "METHOD path". */
function recordRefetches() {
  const requests: string[] = [];
  server.events.on("request:start", ({ request }) => {
    const path = new URL(request.url).pathname.slice(apiBase.length);
    if (path.includes("refetch")) requests.push(`${request.method} ${path}`);
  });
  return requests;
}

function region(name: string) {
  return screen.getByRole("region", { name });
}

describe("refetching remote posts", () => {
  test("a server's page refetches the posts of its accounts and says it is requested", async () => {
    server.use(operatorHandler(["manage_domains"]));
    const requests = recordRefetches();
    const { user } = await renderApp({ path: "/moderation/instances/remote.example" });

    expect(await screen.findByRole("region", { name: "Refetch posts" })).toBeInTheDocument();
    const refetch = within(region("Refetch posts"));
    const button = refetch.getByRole("button", { name: "Refetch posts" });
    expect(button).toHaveAccessibleDescription(/the newest 20 of each account/);
    await user.click(button);

    expect(
      await refetch.findByText(
        "The refetch is requested and recorded in the audit log. The posts change as they are fetched.",
      ),
    ).toBeInTheDocument();
    expect(requests).toEqual(["POST /admin/instances/remote.example/refetch_statuses"]);
    // The button stays, with focus on it.
    expect(button).toHaveFocus();
  });

  test("a refetch the server refuses is a toast, and the button stays", async () => {
    server.use(
      operatorHandler(["manage_domains"]),
      http.post(`${apiBase}/admin/instances/:domain/refetch_statuses`, () =>
        problem(422, "about:blank", "Not remote."),
      ),
    );
    const { user } = await renderApp({ path: "/moderation/instances/remote.example" });

    await user.click(
      await within(await screen.findByRole("region", { name: "Refetch posts" })).findByRole("button", {
        name: "Refetch posts",
      }),
    );

    expect(await screen.findByText("The refetch could not be requested: Not remote.")).toBeInTheDocument();
    expect(within(region("Refetch posts")).getByRole("button", { name: "Refetch posts" })).toBeEnabled();
  });

  test("a remote account's page refetches its posts for a viewer with manage_domains", async () => {
    server.use(
      operatorHandler(["manage_accounts", "manage_domains"]),
      http.get(`${apiBase}/admin/accounts/:id`, () =>
        HttpResponse.json(adminAccountDetailFixture(remoteAccountFixture)),
      ),
    );
    const requests = recordRefetches();
    const { user } = await renderApp({ path: `/moderation/accounts/${remoteAccountFixture.id}` });

    const refetch = within(await screen.findByRole("region", { name: "Refetch posts" }));
    expect(refetch.getByRole("button", { name: "Refetch posts" })).toHaveAccessibleDescription(/up to 1,000/);
    await user.click(refetch.getByRole("button", { name: "Refetch posts" }));

    expect(
      await refetch.findByText(
        "The refetch is requested and recorded in the audit log. The posts change as they are fetched.",
      ),
    ).toBeInTheDocument();
    expect(requests).toEqual([`POST /admin/accounts/${remoteAccountFixture.id}/refetch_statuses`]);
  });

  test("a viewer without manage_domains is not offered a refetch", async () => {
    server.use(
      operatorHandler(["manage_accounts"]),
      http.get(`${apiBase}/admin/accounts/:id`, () =>
        HttpResponse.json(adminAccountDetailFixture(remoteAccountFixture)),
      ),
    );
    await renderApp({ path: `/moderation/accounts/${remoteAccountFixture.id}` });
    expect(await screen.findByRole("region", { name: "Actions on the account" })).toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Refetch posts" })).not.toBeInTheDocument();
  });

  test("a local account has no refetch", async () => {
    server.use(
      operatorHandler(["manage_accounts", "manage_domains"]),
      http.get(`${apiBase}/admin/accounts/:id`, () =>
        HttpResponse.json(adminAccountDetailFixture(otherAccountFixture)),
      ),
    );
    await renderApp({ path: `/moderation/accounts/${otherAccountFixture.id}` });
    expect(await screen.findByRole("region", { name: "Actions on the account" })).toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Refetch posts" })).not.toBeInTheDocument();
  });

  test("a reported remote post is refetched on its own; a local one has no refetch", async () => {
    const local = statusFixture("0192f0c4-7a8e-7000-8000-0000000000b1");
    const report = reportFixture({
      status_ids: [remoteStatusFixture.id, local.id],
      statuses: [{ status: remoteStatusFixture }, { status: local }],
    });
    server.use(
      operatorHandler(["manage_reports", "manage_domains"]),
      http.get(`${apiBase}/admin/reports/:id`, () => HttpResponse.json(report)),
    );
    const requests = recordRefetches();
    const { user } = await renderApp({ path: `/moderation/reports/${report.id}` });

    const posts = within(await screen.findByRole("region", { name: "Reported posts" }));
    const buttons = posts.getAllByRole("button", { name: /^Refetch the post of / });
    expect(buttons).toHaveLength(1);
    const [button] = buttons;
    if (button === undefined) throw new Error("no refetch button");
    await user.click(button);

    expect(
      await posts.findByText("The refetch of the post is requested and recorded in the audit log."),
    ).toBeInTheDocument();
    expect(requests).toEqual([`POST /admin/statuses/${remoteStatusFixture.id}/refetch`]);
  });
});
