// SPDX-License-Identifier: AGPL-3.0-only
// The linked accounts (/settings/linked-accounts; web-client/screens/settings-linked-accounts.md) in the states of
// ADR-0112 §1.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse } from "msw";
import { expect } from "storybook/test";
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

const corp = "019a0000-0000-7000-8000-0000000000c1";
const lab = "019a0000-0000-7000-8000-0000000000c2";
const corpLink: Schemas["LinkedIdentity"] = {
  id: "019a0000-0000-7000-8000-0000000000d1",
  provider_id: corp,
  provider_kind: "oidc",
  provider_name: "Corp SSO",
  linked_at: "2026-10-01T09:00:00Z",
  available: true,
};
const providers: Schemas["LinkableLoginProvider"][] = [
  { id: corp, kind: "oidc", display_name: "Corp SSO", allow_link: true },
  { id: lab, kind: "oidc", display_name: "Lab ID", allow_link: true },
];

/** The server's linked accounts of the viewer and the providers of the server. */
function linkedIdentities(items: Schemas["LinkedIdentity"][], offered = providers) {
  return http.get(`${apiBase}/linked_identities`, ({ request }) =>
    withViewer(request, () => HttpResponse.json({ items, providers: offered })),
  );
}

const meta = {
  title: "Screens/Settings/LinkedAccounts",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: {
    screen: { path: "/settings/linked-accounts", signedIn: true, handlers: [linkedIdentities([corpLink])] },
  },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** One linked account, with its count beside the heading and the button that links another. */
export const Normal: Story = {
  play: async (context) => {
    await settled(context);
    await expect(context.canvas.getByText("1 linked account")).toBeInTheDocument();
  },
};

/** The providers to link an account at, open in place of the button. */
export const Adding: Story = {
  play: async (context) => {
    const { canvas, userEvent } = context;
    await settled(context);
    await userEvent.click(canvas.getByRole("button", { name: "Link an account" }));
    await expect(canvas.getByRole("button", { name: "Link Lab ID" })).toBeInTheDocument();
  },
};

/** No linked account: the empty state leads to linking one. */
export const Empty: Story = answering(linkedIdentities([]));

/** The server offers no provider to link at: a note stands in place of the button. */
export const NoProvider: Story = answering(linkedIdentities([corpLink], []));

/** The linked accounts have not arrived yet. */
export const Loading: Story = loadingOn("get", "/linked_identities");

/** The server failed to answer the linked accounts. */
export const ServerError: Story = failingOn("get", "/linked_identities");

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** In the accented pseudo-locale, whose longer words show where the layout runs out of room. */
export const PseudoLocale: Story = inLocale("en-XA");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;
