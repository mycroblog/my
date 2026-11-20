// SPDX-License-Identifier: AGPL-3.0-only

// The landing page, at / for a visitor without a session (shell/landing.tsx), on the fake server (app.samples.tsx), in
// the states of the page: a server open to anyone, one whose new accounts wait for a moderator, one that takes them
// with an invite alone, one closed to them; the operator hiding both parts the software tells; the counts hidden; a
// summary and a name that run long; no timeline shown to everyone; its own logo; its thumbnail as the banner, on a
// server with every feature on; and the figure of the fediverse still, as with reduced motion.

import { StillMotion } from "@my/design-system/samples";
import { fakeHost } from "@my/e2e/fake";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { VisitorPart, type Fake } from "./app.samples";
import { Landing, type LandingParts } from "./shell/landing";

type Args = { server?: Fake["server"]; parts?: LandingParts; still?: boolean };

const meta = {
  title: "Landing",
  tags: ["themes"],
  parameters: { screen: true, layout: "fullscreen" },
  render: ({ server, parts, still = false }: Args, { id }) => (
    <VisitorPart key={id} server={server}>
      {still ? (
        <StillMotion>
          <Landing serverName={fakeHost} parts={parts} />
        </StillMotion>
      ) : (
        <Landing serverName={fakeHost} parts={parts} />
      )}
    </VisitorPart>
  ),
} satisfies Meta<Args>;
export default meta;

type Story = StoryObj<typeof meta>;

export const Open: Story = {};
export const Approval: Story = { args: { server: { registrations: "approval" } } };
/** Invite only: Sign up with an invite is the primary action. */
export const Invite: Story = { args: { server: { registrations: "invite" } } };
/** Closed to new accounts: it says so first, and Sign in is the primary action. */
export const Closed: Story = { args: { server: { registrations: "closed" } } };
/** The operator hides both parts the software tells: the fediverse, and what the server offers. */
export const ExplanationsHidden: Story = { args: { parts: { fediverse: false, features: false } } };
/** The instance setting `public_counts` off: no counts. */
export const CountsHidden: Story = { args: { server: { publicCounts: false } } };
export const LongSummary: Story = { args: { server: { longSummary: true } } };
/** A name that runs long wraps under itself, Sign in staying at the end of its line. */
export const LongName: Story = { args: { server: { longName: true, logo: true } } };
/** No timeline shown to everyone: nothing to read without an account but search. */
export const NoPublicTimelines: Story = {
  args: { server: { timelines: { local: "signed_in", federated: "signed_in" } } },
};
export const Logo: Story = { args: { server: { logo: true } } };
/** The thumbnail as the introduction's banner, its logo, and every feature on: a sample of each. */
export const Thumbnail: Story = {
  args: {
    server: {
      thumbnail: true,
      logo: true,
      registrations: "approval",
      search: { posts: true },
      translation: { provider: "answers" },
      supporters: {},
    },
  },
};
/** With reduced motion: the figure of the fediverse stands still, its posts half way along their lines. */
export const ReducedMotion: Story = { args: { still: true } };
