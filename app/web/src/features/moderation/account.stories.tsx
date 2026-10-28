// SPDX-License-Identifier: AGPL-3.0-only
// An account's page (/moderation/accounts/{id}; web-client/screens/moderation.md): the account with its state and
// registration, the actions on it, the reports about it, and the action form, in the states of ADR-0112 §1.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse } from "msw";
import { otherAccountFixture, rtlNameAccountFixture, type Schemas } from "../../test/fixtures";
import { apiBase, problem, withViewer } from "../../test/msw/http";
import {
  answering,
  failingOn,
  inLocale,
  loadingOn,
  narrow,
  renderScreen,
  screenLoader,
  settled,
} from "../../test/stories";
import { accountActionFixture, adminAccountDetailFixture } from "./test/handlers";
import { reportSummary, storyReports } from "./test/reportStories";
import {
  hoursAgo,
  hoursFromNow,
  moderatorViewer,
  showing,
  storyAccountsHandler,
  storyId,
  withoutPermission,
} from "./test/stories";

/** Bob, limited for a month after several reports, with an earlier warning lifted and a suspension that expired. */
const bob = adminAccountDetailFixture(otherAccountFixture, {
  state: "limited",
  restrictions: ["public_discovery", "unsolicited_reach"],
  registration: {
    submitted_at: "2026-09-01T09:00:00Z",
    reason: "I write about local politics and would like to follow the council meetings here.",
    state_changed_at: "2026-09-01T12:00:00Z",
  },
  email_verified: true,
  two_factor_enabled: true,
  roles: [],
  actions: [
    accountActionFixture({
      id: storyId("d0", 11),
      kind: "limit",
      reason_category: "violation",
      public_reason: "Repeated insults in replies to people who asked to be left alone.",
      private_note: "Third report in a month; see the open report.",
      report_id: storyId("c0", 1),
      issued_at: hoursAgo(20),
      expires_at: hoursFromNow(24 * 30),
    }),
    accountActionFixture({
      id: storyId("d0", 12),
      kind: "suspend",
      reason_category: "spam",
      state: "expired",
      issued_at: hoursAgo(24 * 60),
      expires_at: hoursAgo(24 * 53),
    }),
    accountActionFixture({
      id: storyId("d0", 13),
      kind: "warn",
      reason_category: "other",
      state: "lifted",
      issued_at: hoursAgo(24 * 80),
      lifted_at: hoursAgo(24 * 79),
      lift_reason: "Issued to the wrong account.",
    }),
  ],
  reports: storyReports.filter((report) => report.target_account_id === otherAccountFixture.id).map(reportSummary),
});

/** A remote account with a right-to-left name, suspended, without a registration. */
const remote = adminAccountDetailFixture(rtlNameAccountFixture, {
  state: "suspended",
  restrictions: ["federation", "profile_view"],
  actions: [accountActionFixture({ id: storyId("d0", 21), account_id: rtlNameAccountFixture.id, kind: "suspend" })],
});

/** GET /admin/accounts/{id} answering `details` by their ID, and GET /admin/accounts/{id}/actions their actions. */
function accountsApi(details: Schemas["AdminAccountDetail"][]) {
  const find = (id: unknown) => details.find((detail) => detail.id === id);
  return [
    http.get(`${apiBase}/admin/accounts/:id`, ({ request, params }) => {
      const found = find(params["id"]);
      return withViewer(request, () =>
        found === undefined ? problem(404, "/api/v1/problems/not-found", "Not Found") : HttpResponse.json(found),
      );
    }),
    http.get(`${apiBase}/admin/accounts/:id/actions`, ({ request, params }) =>
      withViewer(request, () => HttpResponse.json({ items: find(params["id"])?.actions ?? [] })),
    ),
  ];
}

const meta = {
  title: "Screens/Moderation/Account",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: {
    screen: {
      path: `/moderation/accounts/${bob.id}`,
      signedIn: true,
      handlers: [moderatorViewer, storyAccountsHandler, ...accountsApi([bob, remote])],
    },
  },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** A local account limited for a month, with its registration, three actions and the reports about it. */
export const Normal: Story = showing("Repeated insults in replies to people who asked to be left alone.");

/** A suspended account of another server. */
export const Remote: Story = { parameters: { screen: { path: `/moderation/accounts/${remote.id}` } } };

/** An account without actions or reports. */
export const Clean: Story = answering(...accountsApi([adminAccountDetailFixture(otherAccountFixture)]));

/** An account that does not exist. */
export const NotFound: Story = { parameters: { screen: { path: `/moderation/accounts/${storyId("aa", 404)}`, errorPage: "request" } } };

/** The account has not arrived yet. */
export const Loading: Story = loadingOn("get", "/admin/accounts/:id");

/** The server failed to answer the account. */
export const ServerError: Story = failingOn("get", "/admin/accounts/:id");

/** A member without manage_accounts is told why. */
export const WithoutPermission: Story = withoutPermission;

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;
