// SPDX-License-Identifier: AGPL-3.0-only
// One rule (/admin/rules/$id; web-client/screens/admin-rule.md) and a new one (/admin/rules/new;
// web-client/screens/admin-rule-new.md) in the states of ADR-0079 §13
import type { Meta, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse } from "msw";
import type { Schemas } from "../../test/fixtures";
import { apiBase, withViewer } from "../../test/msw/http";
import { inLocale, narrow } from "../../test/stories";
import { ruleFixture } from "./test/handlers";
import { adminAnswering, adminFailingOn, adminScreen, withoutPermission } from "./test/stories";

const ruleId = "0199f0e0-0000-7000-8000-0000000000c1";

/** GET /admin/rules/{id} and GET /admin/rules answering `rule`, the second of two rules in effect. */
function ruleOf(overrides: Partial<Schemas["AdminRule"]>) {
  const rule = ruleFixture(ruleId, 2, overrides);
  const other = ruleFixture("0199f0e0-0000-7000-8000-0000000000c0", 1, { text: "Be kind" });
  return [
    http.get(`${apiBase}/admin/rules/:id`, ({ request }) => withViewer(request, () => HttpResponse.json(rule))),
    http.get(`${apiBase}/admin/rules`, ({ request }) =>
      withViewer(request, () =>
        HttpResponse.json({
          items: rule.retired_at === undefined ? [other, rule] : [other],
        } satisfies Schemas["AdminRuleList"]),
      ),
    ),
  ];
}

const longRule = ruleOf({
  text: "Do not post the location of nests of endangered birds, even when they are visible from a public road or path",
  hint: "Disturbing a nest is against the law in many countries, and photographers have done it for a better picture.",
});

const meta = { title: "Screens/Admin/Rule", ...adminScreen(`/admin/rules/${ruleId}`, longRule) } satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** A long rule in effect with a hint: its facts, its form and retiring it. */
export const Normal: Story = {};

/** A retired rule: only its facts. */
export const Retired: Story = adminAnswering(...ruleOf({ text: "No advertising", retired_at: "2026-10-18T00:00:00Z" }));

/** The page of a new rule. */
export const New: Story = adminScreen("/admin/rules/new");

/** A moderator without manage_rules. */
export const MissingPermission: Story = withoutPermission;

/** The server failed to answer the rule. */
export const ServerError: Story = adminFailingOn("get", "/admin/rules/:id");

/** In Japanese, with the rule in Japanese. */
export const Japanese: Story = {
  ...inLocale("ja"),
  ...adminAnswering(
    ...ruleOf({ text: "思いやりを持つこと", hint: "嫌がらせ、集団での攻撃、侮辱をしないでください。" }),
  ),
};

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px, where the long rule wraps. */
export const Narrow: Story = narrow;
