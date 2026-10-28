// SPDX-License-Identifier: AGPL-3.0-only
// The badges and the supporter list (/settings/badges; web-client/screens/badges.md) in the states of ADR-0112 §1.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse } from "msw";
import { expect } from "storybook/test";
import type { Schemas } from "../../test/fixtures";
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

/** The server's badge choices of the viewer, which PUT changes. */
function badgeChoices(initial: Schemas["BadgeChoice"][]) {
  let items = initial;
  return [
    http.get(`${apiBase}/preferences/badges`, ({ request }) => withViewer(request, () => HttpResponse.json({ items }))),
    http.put(`${apiBase}/preferences/badges/:roleId`, async ({ request, params }) => {
      const roleId = String(params["roleId"]);
      const { displayed } = (await request.clone().json()) as Schemas["UpdateBadgeChoiceRequest"];
      return withViewer(request, () => {
        items = items.map((choice) => (choice.role_id === roleId ? { ...choice, displayed } : choice));
        const saved = items.find((choice) => choice.role_id === roleId);
        return saved === undefined ? problem(404, "about:blank", "Not Found") : HttpResponse.json(saved);
      });
    }),
  ];
}

/** A moderator's badge, shown, and a supporter's, not yet shown. */
const twoBadges = badgeChoices([
  { role_id: "0192f0c4-7a8e-7000-8000-0000000000b1", label: "Moderator", color: "blue", displayed: true },
  { role_id: "0192f0c4-7a8e-7000-8000-0000000000b2", label: "Supporter", color: "green", displayed: false },
]);

const meta = {
  title: "Screens/Settings/Badges",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: { screen: { path: "/settings/badges", signedIn: true, handlers: twoBadges } },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** Two badges; showing one saves at once and says so beside its switch. */
export const Normal: Story = {
  play: async (context) => {
    const { canvas, userEvent } = context;
    await settled(context);
    const supporter = canvas.getByRole("switch", { name: "Supporter" });
    await userEvent.click(supporter);
    await expect(supporter).toBeChecked();
    await expect(await canvas.findByText("⁨Supporter⁩ is shown on your profile.")).toBeInTheDocument();
  },
};

/** A person without a role that has a badge. */
export const Empty: Story = answering(...badgeChoices([]));

/** The badges have not arrived yet. */
export const Loading: Story = loadingOn("get", "/preferences/badges");

/** The server failed to answer the badges. */
export const ServerError: Story = failingOn("get", "/preferences/badges");

/** In Japanese, with a badge named in Japanese. */
export const Japanese: Story = {
  ...inLocale("ja"),
  ...answering(
    ...badgeChoices([
      { role_id: "0192f0c4-7a8e-7000-8000-0000000000b1", label: "モデレーター", color: "blue", displayed: true },
      {
        role_id: "0192f0c4-7a8e-7000-8000-0000000000b2",
        label: "いつも応援してくれる人",
        color: "green",
        displayed: false,
      },
    ]),
  ),
};

/** In the accented pseudo-locale, whose longer words show where the layout runs out of room. */
export const PseudoLocale: Story = inLocale("en-XA");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;
