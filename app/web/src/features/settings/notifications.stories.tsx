// SPDX-License-Identifier: AGPL-3.0-only
// The notification settings (/settings/notifications; web-client/screens/settings-notifications.md) in the states of
// ADR-0079 §13
import type { Meta, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse } from "msw";
import { expect, within } from "storybook/test";
import type { Schemas } from "../../test/fixtures";
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
  viewerWith,
} from "../../test/stories";
import {
  applyNotificationSettingChanges,
  instanceNotificationRows,
  moderatorNotificationRows,
  notificationPolicyFixture,
  ordinaryNotificationRows,
} from "./test/handlers";

/** The server's notification settings of the viewer, which PATCH changes. */
function notificationSettings(rows: Schemas["NotificationSettingRow"][]) {
  let settings: Schemas["NotificationSettings"] = { rows };
  return [
    http.get(`${apiBase}/preferences/notifications`, ({ request }) =>
      withViewer(request, () => HttpResponse.json(settings)),
    ),
    http.patch(`${apiBase}/preferences/notifications`, async ({ request }) => {
      const { changes } = (await request.clone().json()) as Schemas["NotificationSettingsUpdate"];
      return withViewer(request, () => {
        settings = applyNotificationSettingChanges(settings, changes);
        return HttpResponse.json(settings);
      });
    }),
  ];
}

/** Every row a moderator gets, at the defaults. */
const moderatorRows = [...ordinaryNotificationRows, ...moderatorNotificationRows, ...instanceNotificationRows];

const meta = {
  title: "Screens/Settings/Notifications",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: {
    screen: {
      path: "/settings/notifications",
      signedIn: true,
      handlers: [
        viewerWith({ permissions: ["manage_reports", "manage_accounts"] }),
        ...notificationSettings(moderatorRows),
      ],
    },
  },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** A moderator's rows; a cell saves as soon as it changes and shows so in its cell. */
export const Normal: Story = {
  play: async (context) => {
    const { canvas, userEvent } = context;
    await settled(context);
    const email = canvas.getByRole("checkbox", { name: "Mentions: Email" });
    await userEvent.click(email);
    await expect(email).toBeChecked();
    await expect((await canvas.findAllByText("Saved")).length).toBeGreaterThan(0);
  },
};

/** A person who turned off everything that can be turned off. */
export const Empty: Story = answering(
  ...notificationSettings(
    [...ordinaryNotificationRows, ...instanceNotificationRows].map((row) => ({
      ...row,
      in_app: row.in_app.fixed ? row.in_app : { ...row.in_app, enabled: false },
      ...(row.email === undefined ? {} : { email: row.email.fixed ? row.email : { ...row.email, enabled: false } }),
    })),
  ),
);

/**
 * A notification policy that holds the notifications of new accounts and of people the viewer does not follow, and
 * drops those of limited accounts; choosing an action saves it beside its group.
 */
export const NotificationPolicy: Story = {
  ...answering(
    http.get(`${apiBase}/preferences/notification_policy`, ({ request }) =>
      withViewer(request, () =>
        HttpResponse.json({
          ...notificationPolicyFixture,
          not_following: "filter",
          new_account: "filter",
          limited_account: "drop",
        } satisfies Schemas["NotificationPolicy"]),
      ),
    ),
  ),
  play: async (context) => {
    const { canvas, userEvent } = context;
    await settled(context);
    const group = canvas.getByRole("radiogroup", { name: "People who don't follow you" });
    const hold = within(group).getByRole("radio", { name: "Hold" });
    await userEvent.click(hold);
    await expect(hold).toBeChecked();
  },
};

/** The notification settings have not arrived yet. */
export const Loading: Story = loadingOn("get", "/preferences/notifications");

/** The server failed to answer the notification settings. */
export const ServerError: Story = failingOn("get", "/preferences/notifications");

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** In the accented pseudo-locale, whose longer words show where the table runs out of room. */
export const PseudoLocale: Story = inLocale("en-XA");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px, where the table scrolls on its own. */
export const Narrow: Story = narrow;
