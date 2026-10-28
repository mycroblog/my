// SPDX-License-Identifier: AGPL-3.0-only
// The server settings (/admin/settings; web-client/screens/admin-settings.md) in the states of ADR-0112 §1.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse } from "msw";
import { instanceFixture, type Schemas } from "../../test/fixtures";
import { apiBase, withViewer } from "../../test/msw/http";
import { inLocale, narrow } from "../../test/stories";
import { adminBrandImageFixture, instanceSettingsFixture, instanceTextFixture } from "./test/handlers";
import { adminAnswering, adminFailingOn, adminLoadingOn, adminScreen, withoutPermission } from "./test/stories";

/** A logo the browser draws without a request: a wide rounded bar in the brand's colour. */
const logoSvg = `data:image/svg+xml,${encodeURIComponent(
  '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 160 40"><rect width="160" height="40" rx="8" fill="#3a5bd9"/></svg>',
)}`;

/** The server's texts: its name, both descriptions, and the alternative text of its preview image. */
function texts(name: string, short: string, long: string): Schemas["AdminInstanceText"][] {
  return [
    instanceTextFixture("name", name),
    instanceTextFixture("short_description", short),
    instanceTextFixture("description", long),
    instanceTextFixture("preview_image_alt", "The server's logo on a blue background"),
  ];
}

/** A server set up by its administrator: its name and descriptions, a logo of its own, and changed policies. */
function configured(name: string, short: string, long: string) {
  return [
    http.get(`${apiBase}/instance`, () => HttpResponse.json({ ...instanceFixture, name })),
    http.get(`${apiBase}/admin/instance_texts`, ({ request }) =>
      withViewer(request, () => HttpResponse.json({ items: texts(name, short, long) })),
    ),
    http.get(`${apiBase}/admin/settings`, ({ request }) =>
      withViewer(request, () =>
        HttpResponse.json(
          instanceSettingsFixture({
            registration_mode: "invite",
            max_status_length: "3000",
            default_locale: "ja",
            local_timeline_access: "logged_in",
            contact_account: "alice",
            contact_email: "admin-and-moderation-team@a-rather-long-domain-name.example",
            authorized_fetch: "true",
            domain_actions_visibility: "users",
            finance_visibility: "breakdown",
            supporter_list_visibility: "public",
            federation_mode: "both",
          }),
        ),
      ),
    ),
    http.get(`${apiBase}/admin/instance/brand`, ({ request }) =>
      withViewer(request, () =>
        HttpResponse.json({
          logo: {
            ...adminBrandImageFixture("logo", "0199f0e0-0000-7000-8000-00000000b003"),
            svg_url: logoSvg,
          },
        } satisfies Schemas["AdminBrands"]),
      ),
    ),
  ];
}

const meta = {
  title: "Screens/Admin/Settings",
  ...adminScreen(
    "/admin/settings",
    configured(
      "The Very Friendly Neighbourhood Microblogging Server for Birdwatchers",
      "A small server for people who watch birds, and the people who put up with them.",
      "We talk about birds, binoculars and early mornings.\n\nEveryone is welcome as long as they follow the rules: be kind, describe your photos, and mark the location of nests as sensitive.",
    ),
  ),
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** A server set up by its administrator, with a long name, both descriptions, its own logo and changed policies. */
export const Normal: Story = {};

/** A new server: no name, no descriptions, the built-in images and every setting at its default. */
export const Defaults: Story = adminAnswering();

/** A moderator without manage_settings. */
export const MissingPermission: Story = withoutPermission;

/** The settings have not arrived yet. */
export const Loading: Story = adminLoadingOn("get", "/admin/settings");

/** The server failed to answer the settings. */
export const ServerError: Story = adminFailingOn("get", "/admin/settings");

/** In Japanese, with the server's texts in Japanese. */
export const Japanese: Story = {
  ...inLocale("ja"),
  ...adminAnswering(
    ...configured(
      "野鳥観察者のためのとても親切なご近所マイクロブログサーバー",
      "鳥を眺める人と、それに付き合ってくれる人のための小さなサーバーです。",
      "鳥と双眼鏡と早起きの話をしています。\n\nルールを守ってくだされば誰でも歓迎します。",
    ),
  ),
};

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px, where the long name and the contact address wrap. */
export const Narrow: Story = narrow;
