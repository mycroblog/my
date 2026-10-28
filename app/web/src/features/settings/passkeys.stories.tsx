// SPDX-License-Identifier: AGPL-3.0-only
// The passkeys (/settings/passkeys; web-client/screens/settings-passkeys.md) in the states of ADR-0112 §1.
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
} from "../../test/stories";

/** The server's passkeys of the viewer. */
function passkeys(items: Schemas["Passkey"][]) {
  return http.get(`${apiBase}/passkeys`, ({ request }) => withViewer(request, () => HttpResponse.json({ items })));
}

const two = passkeys([
  { id: "019a0000-0000-7000-8000-0000000000a1", name: "Laptop", created_at: "2026-01-01T09:00:00Z" },
  { id: "019a0000-0000-7000-8000-0000000000a2", name: "Phone", created_at: "2026-02-01T09:00:00Z" },
]);

const meta = {
  title: "Screens/Settings/Passkeys",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: { screen: { path: "/settings/passkeys", signedIn: true, handlers: [two] } },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** Two passkeys, with their count beside the heading and the button that adds one. */
export const Normal: Story = {
  play: async (context) => {
    await settled(context);
    await expect(context.canvas.getByText("2 passkeys")).toBeInTheDocument();
  },
};

/** The form that adds a passkey, open in place of the button. */
export const Adding: Story = {
  play: async (context) => {
    const { canvas, userEvent } = context;
    await settled(context);
    await userEvent.click(canvas.getByRole("button", { name: "Add a passkey" }));
    await expect(canvas.getByRole("textbox", { name: /Name/ })).toHaveFocus();
  },
};

/** The removal of a passkey asks first. */
export const Removing: Story = {
  play: async (context) => {
    const { canvas, userEvent } = context;
    await settled(context);
    await userEvent.click(canvas.getByRole("button", { name: "Remove: Phone" }));
    await expect(await within(document.body).findByRole("alertdialog")).toBeInTheDocument();
  },
};

/** No passkey: the empty state leads to adding one. */
export const Empty: Story = answering(passkeys([]));

/** The passkeys have not arrived yet. */
export const Loading: Story = loadingOn("get", "/passkeys");

/** The server failed to answer the passkeys. */
export const ServerError: Story = failingOn("get", "/passkeys");

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** In the accented pseudo-locale, whose longer words show where the layout runs out of room. */
export const PseudoLocale: Story = inLocale("en-XA");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;
