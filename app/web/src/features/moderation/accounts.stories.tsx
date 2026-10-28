// SPDX-License-Identifier: AGPL-3.0-only
// The accounts (/moderation/accounts; web-client/screens/moderation.md): the search and the local and remote
// accounts it finds, in the states of ADR-0112 §1.
import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  accountFixture,
  longDomainAccountFixture,
  longNameAccountFixture,
  otherAccountFixture,
  remoteAccountFixture,
  rtlNameAccountFixture,
} from "../../test/fixtures";
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
import { adminAccountFixture } from "./test/handlers";
import { listing, moderatorViewer, monaAccount, showing, withoutPermission } from "./test/stories";

/** Accounts in every state, local and remote, a few with restrictions. */
const accounts = listing("/admin/accounts", [
  adminAccountFixture(otherAccountFixture),
  adminAccountFixture(longNameAccountFixture, {
    state: "limited",
    restrictions: ["public_discovery", "unsolicited_reach"],
  }),
  adminAccountFixture(remoteAccountFixture),
  adminAccountFixture(rtlNameAccountFixture, { state: "suspended", restrictions: ["federation", "profile_view"] }),
  adminAccountFixture(longDomainAccountFixture, { state: "limited", restrictions: ["media_sensitive"] }),
  adminAccountFixture(accountFixture(31, "Dana", "my.example"), { state: "pending", registration_state: "pending" }),
  adminAccountFixture(accountFixture(32, "Eve", "my.example"), { state: "disabled", restrictions: ["login"] }),
  adminAccountFixture(accountFixture(33, "Frank", "my.example"), { state: "rejected", registration_state: "rejected" }),
  adminAccountFixture(accountFixture(34, "Grace", "my.example"), {
    state: "limited",
    restrictions: ["post", "interact", "visibility_ceiling_private"],
  }),
  adminAccountFixture(monaAccount),
]);

const meta = {
  title: "Screens/Moderation/Accounts",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: { screen: { path: "/moderation/accounts", signedIn: true, handlers: [moderatorViewer, accounts] } },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** Every account: active, limited, suspended, awaiting approval, disabled and rejected ones. */
export const Normal: Story = showing("Maximiliana Theodora Wolfharte");

/** The limited accounts, the search's field filled from the address. */
export const Searched: Story = { parameters: { screen: { path: "/moderation/accounts?state=limited" } } };

/** A search that finds nothing. */
export const Empty: Story = {
  ...answering(listing("/admin/accounts", [])),
  parameters: { screen: { path: "/moderation/accounts?username=nobody" } },
};

/** The accounts have not arrived yet. */
export const Loading: Story = loadingOn("get", "/admin/accounts");

/** The server failed to answer the accounts. */
export const ServerError: Story = failingOn("get", "/admin/accounts");

/** A member without manage_accounts is told why. */
export const WithoutPermission: Story = withoutPermission;

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;
