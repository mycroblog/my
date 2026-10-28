// SPDX-License-Identifier: AGPL-3.0-only
// The account status (/settings/account-status; web-client/screens/account-status.md) in the states of ADR-0112 §1: the
// moderation actions on the user's own account in effect and past, with appeals pending, rejected and overturned.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse } from "msw";
import { expect, within } from "storybook/test";
import { type Schemas, viewerStatusFixture } from "../../test/fixtures";
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
import { ownActionFixture } from "./test/handlers";

type Action = Schemas["ModerationAction"];

/** `ownActionFixture` with `overrides`, without the fields named in `omit`. */
function action(overrides: Partial<Action>, omit: (keyof Action)[] = []): Action {
  const entries = Object.entries(ownActionFixture(overrides)).filter(([key]) => !omit.includes(key as keyof Action));
  return Object.fromEntries(entries) as Action;
}

/** A suspension in effect that may be appealed, a limit under appeal, and past actions of each outcome. */
const sanctions: Action[] = [
  action({}),
  action({
    id: "0192f0c4-7a8e-7000-8000-0000000007a4",
    kind: "limit",
    kind_label: "Limited",
    public_reason:
      "Several reports of replies to strangers that repeated the same link to a shop. We limited the account so that only your followers see your posts while we look into it.",
    appealable: false,
    appeal: {
      id: "0192f0c4-7a8e-7000-8000-0000000007b4",
      text: "The link is to my own bakery, and I only replied to people who asked where to buy the bread.",
      state: "pending",
      submitted_at: "2026-10-17T00:00:00Z",
    },
  }),
  action(
    {
      id: "0192f0c4-7a8e-7000-8000-0000000007a2",
      kind: "mark_statuses_sensitive",
      kind_label: "Posts marked as sensitive",
      reason_category: "other",
      reason_category_label: "Other",
      public_reason: "صور غير مناسبة",
      status_ids: ["0192f0c4-7a8e-7000-8000-0000000000a1", "0192f0c4-7a8e-7000-8000-0000000000a9"],
      state: "expired",
      issued_at: "2026-09-01T00:00:00Z",
      appealable: false,
      appeal_deadline: "2026-09-02T00:00:00Z",
      appeal: {
        id: "0192f0c4-7a8e-7000-8000-0000000007b3",
        text: "It was a painting.",
        state: "upheld",
        submitted_at: "2026-09-01T12:00:00Z",
        decided_at: "2026-09-02T00:00:00Z",
        decision_reason: "The picture shows injuries without a content warning.",
      },
    },
    ["expires_at"],
  ),
  action(
    {
      id: "0192f0c4-7a8e-7000-8000-0000000007a3",
      kind: "limit",
      kind_label: "Limited",
      state: "lifted",
      issued_at: "2026-08-01T00:00:00Z",
      lifted_at: "2026-08-05T12:00:00Z",
      appealable: false,
      appeal: {
        id: "0192f0c4-7a8e-7000-8000-0000000007b2",
        text: "These were not spam.",
        state: "overturned",
        submitted_at: "2026-08-02T00:00:00Z",
        decided_at: "2026-08-05T12:00:00Z",
        decision_reason: "We agree; lifted.",
      },
    },
    ["expires_at", "public_reason"],
  ),
];

/** The server's actions on the viewer's account (`active=true` keeps those in effect), and their posts. */
function acting(actions: Action[]) {
  return [
    http.get(`${apiBase}/moderation/actions`, ({ request }) => {
      const active = new URL(request.url).searchParams.get("active") === "true";
      return withViewer(request, () =>
        HttpResponse.json({ items: active ? actions.filter((item) => item.state === "active") : actions }),
      );
    }),
    // The first post is the viewer's; the second one is deleted.
    http.get(`${apiBase}/statuses/:id`, ({ params }) =>
      params["id"] === "0192f0c4-7a8e-7000-8000-0000000000a1"
        ? HttpResponse.json(viewerStatusFixture({ content: "<p>A sunset</p>" }))
        : problem(404, "about:blank", "Not Found"),
    ),
  ];
}

const meta = {
  title: "Screens/Settings/AccountStatus",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: { screen: { path: "/settings/account-status", signedIn: true, handlers: acting(sanctions) } },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** A suspension that may be appealed, a limit under appeal, and past actions upheld and overturned on appeal. */
export const Normal: Story = {};

/** The appeal dialog of the suspension, open. */
export const Appealing: Story = {
  play: async (context) => {
    const { canvas, userEvent } = context;
    await settled(context);
    await userEvent.click((await canvas.findAllByRole("button", { name: /^Appeal/u }))[0] as HTMLElement);
    // The dialog is in a portal at the end of the page, outside the story's root.
    await expect(
      await within(document.body).findByRole("alertdialog", { name: "Appeal this action?" }),
    ).toBeInTheDocument();
  },
};

/** An account without any moderation action. */
export const Empty: Story = answering(...acting([]));

/** The actions have not arrived yet. */
export const Loading: Story = loadingOn("get", "/moderation/actions");

/** The server failed to answer the actions. */
export const ServerError: Story = failingOn("get", "/moderation/actions");

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;
