// SPDX-License-Identifier: AGPL-3.0-only
// The settings page (/settings; web-client/screens/settings.md) with the settings menu (screens/settings-menu.md) in the
// states of ADR-0112 §1.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse } from "msw";
import { expect } from "storybook/test";
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

/** A moderator who has two passkeys and turned two-factor authentication on. */
const secured = [
  viewerWith({ display_name: "Marco", permissions: ["manage_reports", "manage_accounts"] }),
  http.get(`${apiBase}/passkeys`, ({ request }) =>
    withViewer(request, () =>
      HttpResponse.json({
        items: [
          { id: "1", name: "Laptop", created_at: "2026-01-01T00:00:00Z" },
          { id: "2", name: "Phone", created_at: "2026-02-01T00:00:00Z" },
        ],
      }),
    ),
  ),
  http.get(`${apiBase}/two_factor`, ({ request }) => withViewer(request, () => HttpResponse.json({ enabled: true }))),
];

const meta = {
  title: "Screens/Settings/Settings",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: { screen: { path: "/settings", signedIn: true, handlers: secured } },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** A moderator's settings, with the staff entry; the menu opens the password page. */
export const Normal: Story = {
  play: async (context) => {
    const { canvas, userEvent } = context;
    await settled(context);
    await canvas.findByRole("link", { name: "Passkeys 2 added" });
    await userEvent.click(canvas.getByRole("link", { name: "Password" }));
    await expect(await canvas.findByRole("heading", { level: 1, name: "Password" })).toBeInTheDocument();
  },
};

/** A new account: no passkeys, two-factor authentication off, and no staff permissions. */
export const Empty: Story = answering(viewerWith({}));

/** The state of two-factor authentication has not arrived yet. */
export const Loading: Story = loadingOn("get", "/two_factor");

/** The server failed to answer the passkeys. */
export const ServerError: Story = failingOn("get", "/passkeys");

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** In the accented pseudo-locale, whose longer words show where the layout runs out of room. */
export const PseudoLocale: Story = inLocale("en-XA");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px, where the menu is the settings page itself. */
export const Narrow: Story = narrow;
