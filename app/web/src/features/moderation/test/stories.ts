// SPDX-License-Identifier: AGPL-3.0-only
// What the moderation screen stories (ADR-0112 §1) share: a viewer with the moderators' permissions, the accounts
// they handle (long, right-to-left and long-domain names among them), and the list handlers that answer a screen's
// items with the server's own filters (`state`, `active`).
import { http, HttpResponse, type RequestHandler } from "msw";
import { expect, within } from "storybook/test";
import {
  accountFixture,
  longDomainAccountFixture,
  longNameAccountFixture,
  otherAccountFixture,
  remoteAccountFixture,
  rtlNameAccountFixture,
  type Schemas,
  viewerFixture,
} from "../../../test/fixtures";
import { apiBase, operatorHandler, withViewer } from "../../../test/msw/http";
import { answering, settled } from "../../../test/stories";

/** A viewer holding every permission of the moderation screens. */
export const moderatorViewer = operatorHandler([
  "manage_reports",
  "manage_accounts",
  "manage_domains",
  "manage_invites",
  "view_audit",
]);

/** A signed-in viewer without any permission, whom the moderation screens tell why they may not use them. */
export const memberViewer = operatorHandler([]);

/** Mona, another moderator of this server, who handles some of the reports. */
export const monaAccount = accountFixture(5, "Mona", "my.example");

/** The accounts the stories' reports, actions and appeals are about. */
export const storyAccounts: Schemas["Account"][] = [
  otherAccountFixture,
  remoteAccountFixture,
  longNameAccountFixture,
  rtlNameAccountFixture,
  longDomainAccountFixture,
  monaAccount,
];

/** GET /accounts/{id} answering the stories' accounts and the viewer; any other ID goes on to the default handler. */
export const storyAccountsHandler = http.get(`${apiBase}/accounts/:id`, ({ request, params }) => {
  const id = String(params["id"]);
  const account = id === viewerFixture.id ? viewerFixture : storyAccounts.find((a) => a.id === id);
  if (account === undefined) return undefined;
  return withViewer(request, () => HttpResponse.json(account));
});

const hour = 60 * 60 * 1000;

/** The time `hours` hours before the story renders, so that waits read as they would on the day. */
export function hoursAgo(hours: number): string {
  return new Date(Date.now() - hours * hour).toISOString();
}

/** The time `hours` hours after the story renders. */
export function hoursFromNow(hours: number): string {
  return new Date(Date.now() + hours * hour).toISOString();
}

/** The numbered ID of the stories' items, unique within `group` (a hex digit pair). */
export function storyId(group: string, n: number): string {
  return `0192f0c4-7a8e-7000-8000-${group}${String(n).padStart(10, "0")}`;
}

/**
 * GET `path` answering `items` as one page, filtered as the server filters them: by `state` when the request names
 * one, and to the active items for `active=true`.
 */
export function listing(path: string, items: object[]): RequestHandler {
  return http.get(`${apiBase}${path}`, ({ request }) => {
    const query = new URL(request.url).searchParams;
    const state = query.get("state");
    const kept = items.filter((item) => {
      const itemState = "state" in item ? item.state : undefined;
      return (state === null || itemState === state) && (query.get("active") !== "true" || itemState === "active");
    });
    return withViewer(request, () => HttpResponse.json({ items: kept }));
  });
}

type Played = { canvasElement: HTMLElement };

/** A story whose screen, once loaded, shows `text`: a name from its data, so that the data did arrive. */
export function showing(text: string | RegExp) {
  return {
    play: async (context: Played) => {
      await settled(context);
      const [shown] = await within(context.canvasElement).findAllByText(text, {}, { timeout: 10_000 });
      await expect(shown).toBeVisible();
    },
  };
}

/** A story of a member without the screen's permission, whom the screen tells why. */
export const withoutPermission = {
  ...answering(memberViewer),
  play: async (context: Played) => {
    await settled(context);
    const heading = await within(context.canvasElement).findByRole("heading", {
      level: 2,
      name: "You cannot use this screen",
    });
    await expect(heading).toBeVisible();
  },
};
