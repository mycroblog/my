// SPDX-License-Identifier: AGPL-3.0-only
// The link card domains (/admin/link-card-domains; web-client/screens/admin-link-card-domains.md) in the states of ADR-0079 §13
import type { Meta, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse } from "msw";
import type { Schemas } from "../../test/fixtures";
import { apiBase, withViewer } from "../../test/msw/http";
import { inLocale, narrow } from "../../test/stories";
import { linkCardDomainRuleFixture } from "./test/handlers";
import { adminAnswering, adminFailingOn, adminLoadingOn, adminScreen, withoutPermission } from "./test/stories";

/** A rule in each mode, one with a note, and one with a long internationalized domain. */
const someRules = http.get(`${apiBase}/admin/link_card_domains`, ({ request }) =>
  withViewer(request, () =>
    HttpResponse.json({
      items: [
        linkCardDomainRuleFixture("images.a-very-long-domain-name-of-a-site-with-heavy-pictures.example", {
          note: "Its images are large, and they are slow to fetch from here.",
        }),
        linkCardDomainRuleFixture("stop.example", { mode: "no_fetch", note: "The site asked us to stop." }),
        linkCardDomainRuleFixture("xn--r8jz45g.example"),
      ],
    } satisfies Schemas["LinkCardDomainRuleList"]),
  ),
);

const meta = {
  title: "Screens/Admin/LinkCardDomains",
  ...adminScreen("/admin/link-card-domains", [someRules]),
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** Rules in each mode. */
export const Normal: Story = {};

/** A server with no rules (the default handlers). */
export const Empty: Story = adminAnswering();

/** A moderator without manage_settings. */
export const MissingPermission: Story = withoutPermission;

/** The rules have not arrived yet: the form is there, and the list says it is loading. */
export const Loading: Story = adminLoadingOn("get", "/admin/link_card_domains");

/** The server failed to answer the rules: the failure stays inside the section of the list. */
export const ServerError: Story = adminFailingOn("get", "/admin/link_card_domains");

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px, where the long domain wraps. */
export const Narrow: Story = narrow;
