// SPDX-License-Identifier: AGPL-3.0-only

// The landing page, at / for a visitor without a session (shell/landing.tsx), on the fake server (app.samples.tsx), in
// the three arrangements offered for the decider's pick (`landing`): OptionA, the frame of the form pages with the
// server on the panel; OptionB, one reading column; OptionC, the account's part beside the rest. Each in the states
// of the page: a server open to anyone, one whose new accounts wait for a moderator, one that takes them with an
// invite alone, one closed to them; the operator hiding both parts the software tells; the counts hidden; a summary
// that runs long; no timeline shown to everyone; its own logo; its thumbnail as the banner, on a server with more on.

import { Options } from "@my/devtools/options";
import { fakeHost } from "@my/e2e/fake";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { VisitorPart, type Fake } from "./app.samples";
import { LandingAside, LandingPage, LandingPanel, type LandingParts } from "./shell/landing";

type Args = { server?: Fake["server"]; parts?: LandingParts };

const meta = {
  title: "Landing",
  tags: ["themes"],
  parameters: { screen: true, layout: "fullscreen" },
  render: ({ server, parts }: Args, { id }) => (
    <VisitorPart key={id} server={server}>
      <Options
        id="landing"
        variants={{
          OptionA: <LandingPanel serverName={fakeHost} parts={parts} />,
          OptionB: <LandingPage serverName={fakeHost} parts={parts} />,
          OptionC: <LandingAside serverName={fakeHost} parts={parts} />,
        }}
      />
    </VisitorPart>
  ),
} satisfies Meta<Args>;
export default meta;

type Story = StoryObj<typeof meta>;

export const Open: Story = {};
export const Approval: Story = { args: { server: { registrations: "approval" } } };
/** Invite only: Sign up with an invite is the primary action. */
export const Invite: Story = { args: { server: { registrations: "invite" } } };
/** Closed to new accounts: it says so, and Sign in is the primary action. */
export const Closed: Story = { args: { server: { registrations: "closed" } } };
/** The operator hides both parts the software tells: the fediverse, and what the server offers. */
export const ExplanationsHidden: Story = { args: { parts: { fediverse: false, features: false } } };
/** The instance setting `public_counts` off: no counts. */
export const CountsHidden: Story = { args: { server: { publicCounts: false } } };
export const LongSummary: Story = { args: { server: { longSummary: true } } };
/** No timeline shown to everyone: nothing to read without an account but search. */
export const NoPublicTimelines: Story = {
  args: { server: { timelines: { local: "signed_in", federated: "signed_in" } } },
};
export const Logo: Story = { args: { server: { logo: true } } };
/** The thumbnail as the introduction's banner, its logo, and more on: search of posts and translation. */
export const Thumbnail: Story = {
  args: {
    server: {
      thumbnail: true,
      logo: true,
      registrations: "approval",
      search: { posts: true },
      translation: { provider: "answers" },
    },
  },
};
