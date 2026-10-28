// SPDX-License-Identifier: AGPL-3.0-only
// The server's rules (/admin/rules; web-client/screens/admin-rules.md) in the states of ADR-0112 §1.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse } from "msw";
import type { Schemas } from "../../test/fixtures";
import { apiBase, withViewer } from "../../test/msw/http";
import { inLocale, narrow } from "../../test/stories";
import { ruleFixture } from "./test/handlers";
import { adminAnswering, adminFailingOn, adminLoadingOn, adminScreen, withoutPermission } from "./test/stories";

/** GET /admin/rules answering `rules`, numbered from 1 in their order. */
function rulesOf(rules: Pick<Schemas["AdminRule"], "text" | "hint">[]) {
  const items = rules.map((rule, index) =>
    ruleFixture(`0199f0e0-0000-7000-8000-0000000000${String(index + 10)}`, index + 1, rule),
  );
  return http.get(`${apiBase}/admin/rules`, ({ request }) =>
    withViewer(request, () => HttpResponse.json({ items } satisfies Schemas["AdminRuleList"])),
  );
}

/** Eight rules, some with a hint, one long enough to wrap over several lines. */
const eightRules = rulesOf([
  { text: "Be kind", hint: "No harassment, dogpiling, or insults, including of people who are not on this server." },
  { text: "No spam" },
  { text: "Describe your images", hint: "Write a description for everyone who cannot see them." },
  { text: "Mark sensitive media" },
  {
    text: "Do not post the location of nests of endangered birds, even when they are visible from a public road or path",
    hint: "Disturbing a nest is against the law in many countries, and photographers have done it for a better picture.",
  },
  { text: "No advertising without the administrators' consent" },
  { text: "Respect copyright" },
  { text: "Use content warnings for spoilers", hint: "Of films, books and games, for a week after they come out." },
]);

const meta = { title: "Screens/Admin/Rules", ...adminScreen("/admin/rules", [eightRules]) } satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** Eight rules, some with hints, one long. */
export const Normal: Story = {};

/** A new server without rules (the default handlers). */
export const Empty: Story = adminAnswering();

/** A moderator without manage_rules. */
export const MissingPermission: Story = withoutPermission;

/** The rules have not arrived yet. */
export const Loading: Story = adminLoadingOn("get", "/admin/rules");

/** The server failed to answer the rules. */
export const ServerError: Story = adminFailingOn("get", "/admin/rules");

/** In Japanese, with the rules in Japanese. */
export const Japanese: Story = {
  ...inLocale("ja"),
  ...adminAnswering(
    rulesOf([
      { text: "思いやりを持つこと", hint: "嫌がらせ、集団での攻撃、侮辱をしないでください。" },
      { text: "スパム禁止" },
      { text: "画像に説明を付けること" },
    ]),
  ),
};

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px, where the long rule wraps. */
export const Narrow: Story = narrow;
