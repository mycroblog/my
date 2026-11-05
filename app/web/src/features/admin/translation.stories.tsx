// SPDX-License-Identifier: AGPL-3.0-only
// The translation settings (/admin/translation; web-client/screens/admin-translation.md) in the states of ADR-0079 §13
import type { Meta, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse } from "msw";
import { apiBase, withViewer } from "../../test/msw/http";
import { inLocale, narrow } from "../../test/stories";
import { translationOverviewFixture } from "./test/handlers";
import { adminAnswering, adminFailingOn, adminLoadingOn, adminScreen, withoutPermission } from "./test/stories";

/** A month with use. */
const someUse = http.get(`${apiBase}/admin/translation`, ({ request }) =>
  withViewer(request, () =>
    HttpResponse.json(
      translationOverviewFixture({ characters: 123_456, cache_hits: 300, cache_misses: 100, cache_hit_rate: 0.75 }),
    ),
  ),
);

const meta = {
  title: "Screens/Admin/Translation",
  ...adminScreen("/admin/translation", [someUse]),
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** Two providers on the server, none enabled, and a month with use. */
export const Normal: Story = {};

/** A server with no provider, and no use yet. */
export const NoProviders: Story = adminAnswering(
  http.get(`${apiBase}/admin/translation`, ({ request }) =>
    withViewer(request, () => HttpResponse.json(translationOverviewFixture({ providers: [] }))),
  ),
);

/** A moderator without manage_settings. */
export const MissingPermission: Story = withoutPermission;

/** The month's use has not arrived yet. */
export const Loading: Story = adminLoadingOn("get", "/admin/translation");

/** The server failed to answer the month's use: the failure stays inside its section. */
export const ServerError: Story = adminFailingOn("get", "/admin/translation");

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** At the width of a phone. */
export const Narrow: Story = narrow;
