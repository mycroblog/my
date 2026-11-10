// SPDX-License-Identifier: AGPL-3.0-only
// The notification requests (/notifications/requests; web-client/screens/notifications-requests.md) and the page of one
// request (/notifications/requests/{account_id}; web-client/screens/notifications-request.md) in the states of
// ADR-0079 §13
import type { Meta, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse } from "msw";
import { accountFixture, instanceFixture, type Schemas, statusFixture } from "../../test/fixtures";
import { apiBase, withViewer } from "../../test/msw/http";
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

/** The senders: a long name, a right-to-left name, and plain ones of this server and another. */
const senders: Schemas["Account"][] = [
  {
    ...accountFixture(11, "Bartholomew"),
    display_name: "Bartholomew Montgomery-Fairweather of the Long Display Names",
  },
  { ...accountFixture(12, "layla"), display_name: "ليلى" },
  accountFixture(13, "kai", instanceFixture.domain),
  accountFixture(14, "mio"),
];

/** A mention held in the request of `account`. */
function held(n: number, account: Schemas["Account"]): Schemas["Notification"] {
  const id = String(n).padStart(2, "0");
  return {
    id: `0192f0c4-7a8e-7000-8000-0000000003${id}`,
    type: "mention",
    created_at: "2026-10-16T10:00:00Z",
    account,
    status: {
      ...statusFixture(`0192f0c4-7a8e-7000-8000-0000000004${id}`),
      account,
      content: "<p>Hello, I just found your posts. Do you sell the stickers too?</p>",
    },
  };
}

const requests: Schemas["NotificationRequest"][] = senders.map((account, index) => ({
  id: account.id,
  account,
  notifications_count: index + 1,
  last_notification: held(index, account),
}));

/** GET /notifications/requests answering `items`. */
function listing(items: Schemas["NotificationRequest"][]) {
  return http.get(`${apiBase}/notifications/requests`, ({ request }) =>
    withViewer(request, () => HttpResponse.json({ items } satisfies Schemas["NotificationRequestPage"])),
  );
}

/** The request of the first sender and its three notifications. */
const first = senders[0] ?? accountFixture(11, "Bartholomew");
const requestHandlers = [
  http.get(`${apiBase}/notifications/requests/:id`, ({ request }) =>
    withViewer(request, () =>
      HttpResponse.json({
        id: first.id,
        account: first,
        notifications_count: 3,
        last_notification: held(20, first),
      } satisfies Schemas["NotificationRequest"]),
    ),
  ),
  http.get(`${apiBase}/notifications/requests/:id/notifications`, ({ request }) =>
    withViewer(request, () =>
      HttpResponse.json({
        items: [held(20, first), held(21, first), held(22, first)],
      } satisfies Schemas["NotificationPage"]),
    ),
  ),
];

const meta = {
  title: "Screens/Notifications/Requests",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: {
    screen: { path: "/notifications/requests", signedIn: true, handlers: [listing(requests), ...requestHandlers] },
  },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** Four requests with long, right-to-left and plain names. */
export const Normal: Story = {};

/** No request waits. */
export const Empty: Story = answering(listing([]));

/** The requests have not arrived yet. */
export const Loading: Story = loadingOn("get", "/notifications/requests");

/** The server failed to answer the requests. */
export const ServerError: Story = failingOn("get", "/notifications/requests");

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;

/** The page of one request, with its three held notifications. */
export const Request: Story = { parameters: { screen: { path: `/notifications/requests/${first.id}` } } };

/** The page of one request in Japanese. */
export const RequestJapanese: Story = {
  ...inLocale("ja"),
  parameters: { screen: { path: `/notifications/requests/${first.id}` } },
};

/** The held notifications of the request failed to load: the answers stay. */
export const RequestNotificationsError: Story = {
  parameters: {
    screen: {
      ...failingOn("get", "/notifications/requests/:id/notifications").parameters.screen,
      path: `/notifications/requests/${first.id}`,
    },
  },
};
