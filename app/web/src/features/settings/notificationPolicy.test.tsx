// SPDX-License-Identifier: AGPL-3.0-only
// The notification policy of the notification settings through the whole app (web-client/README.md T4, ADR-0158 §10):
// MSW plays the server. Each condition is a radio group of the three actions that saves itself as soon as it changes.
import { screen, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import { notificationPolicyFixture } from "./test/handlers";
import type { Schemas } from "../../test/fixtures";
import { apiBase, problem, signedInHandler, withViewer } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";

/** The policy server, which keeps the changes it receives; `fail` refuses them. */
function policyServer({ fail = false }: { fail?: boolean } = {}) {
  let policy: Schemas["NotificationPolicy"] = { ...notificationPolicyFixture, limited_account: "drop" };
  const patches: Schemas["NotificationPolicyUpdate"][] = [];
  server.use(
    http.get(`${apiBase}/preferences/notification_policy`, ({ request }) =>
      withViewer(request, () => HttpResponse.json(policy)),
    ),
    http.patch(`${apiBase}/preferences/notification_policy`, async ({ request }) => {
      const body = (await request.clone().json()) as Schemas["NotificationPolicyUpdate"];
      patches.push(body);
      if (fail) return problem(503, "/api/v1/problems/unavailable", "Service Unavailable");
      policy = { ...policy, ...body };
      return withViewer(request, () => HttpResponse.json(policy));
    }),
  );
  return { patches };
}

async function openSettings() {
  const view = await renderApp({ path: "/settings/notifications" });
  expect(await screen.findByRole("heading", { level: 1, name: "Notifications" })).toBeInTheDocument();
  return view;
}

function policySection() {
  return screen.getByRole("region", { name: "Notification policy" });
}

beforeEach(() => {
  server.use(signedInHandler);
});

describe("the notification policy", () => {
  test("explains each action, and shows each condition with the action the server holds", async () => {
    policyServer();
    await openSettings();
    const section = policySection();

    // What each action does, before the conditions (the acceptance criteria of the settings).
    const terms = within(section).getAllByRole("term");
    expect(terms.map((term) => term.textContent)).toEqual(["Accept", "Hold", "Drop"]);
    expect(
      within(section).getByText(/stays out of your notifications and the unread count, and sends no push notification/),
    ).toBeInTheDocument();

    expect(
      within(section)
        .getAllByRole("radiogroup")
        .map((group) => group.getAttribute("aria-labelledby")),
    ).toHaveLength(5);
    expect(within(section).getByRole("radiogroup", { name: "People you don't follow" })).toHaveAccessibleDescription(
      "A follow request of yours that waits for approval does not count as following.",
    );
    expect(
      within(within(section).getByRole("radiogroup", { name: "People you don't follow" })).getByRole("radio", {
        name: "Accept",
      }),
    ).toBeChecked();
    expect(
      within(within(section).getByRole("radiogroup", { name: "Accounts limited by the moderators" })).getByRole(
        "radio",
        { name: "Drop" },
      ),
    ).toBeChecked();
  });

  test("saves the action chosen for a condition at once, and shows that it is saved beside its group", async () => {
    const { patches } = policyServer();
    const { user } = await openSettings();
    const group = within(policySection()).getByRole("radiogroup", { name: "New accounts" });

    await user.click(within(group).getByRole("radio", { name: "Hold" }));

    expect(within(group).getByRole("radio", { name: "Hold" })).toBeChecked();
    expect(await within(policySection()).findByText("Saved")).toBeInTheDocument();
    expect(patches).toEqual([{ new_account: "filter" }]);
  });

  test("says why a change was not saved, and shows the saved action again", async () => {
    policyServer({ fail: true });
    const { user } = await openSettings();
    const group = within(policySection()).getByRole("radiogroup", { name: "New accounts" });

    await user.click(within(group).getByRole("radio", { name: "Drop" }));

    expect(await within(policySection()).findByText("Not saved")).toBeInTheDocument();
    expect(within(policySection()).getByRole("alert")).toBeInTheDocument();
    expect(within(group).getByRole("radio", { name: "Accept" })).toBeChecked();
  });
});
