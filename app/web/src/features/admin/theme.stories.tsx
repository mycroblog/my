// SPDX-License-Identifier: AGPL-3.0-only
// The server's theme (/admin/theme; web-client/screens/admin-theme.md) in the states of ADR-0112 §1.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse } from "msw";
import type { Schemas } from "../../test/fixtures";
import { apiBase, withViewer } from "../../test/msw/http";
import { inLocale, narrow } from "../../test/stories";
import { adminAnswering, adminFailingOn, adminLoadingOn, adminScreen, withoutPermission } from "./test/stories";

/** GET /admin/theme answering `theme`, saved at 2026-10-20. */
function themed(theme: Pick<Schemas["AdminTheme"], "tokens" | "custom_css">) {
  return http.get(`${apiBase}/admin/theme`, ({ request }) =>
    withViewer(request, () =>
      HttpResponse.json({
        revision: "r1",
        revised_at: "2026-10-20T00:00:00Z",
        stylesheet_url: "https://my.example/theme/r1.css",
        ...theme,
      } satisfies Schemas["AdminTheme"]),
    ),
  );
}

/** A theme with a few overridden colors and radii, and a custom CSS of several rules. */
const customized = themed({
  tokens: [
    { token: "color-accent", light: "#1447e6", dark: "#92c5fe" },
    { token: "color-surface", light: "#fffaf2", dark: "#1b1a18" },
    { token: "radius-md", value: "2px" },
  ],
  custom_css: [
    ".brand { letter-spacing: 0.02em; }",
    ".timeline .status:hover { background: var(--color-surface-raised); }",
    ".profile-header { border-block-end: 2px solid var(--color-accent); }",
  ].join("\n"),
});

const meta = { title: "Screens/Admin/Theme", ...adminScreen("/admin/theme", [customized]) } satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** A theme with overridden colors and a radius, and a custom CSS; every pair keeps its contrast. */
export const Normal: Story = {};

/** The built-in theme, without overrides (the default handlers). */
export const Empty: Story = adminAnswering();

/** A moderator without manage_settings. */
export const MissingPermission: Story = withoutPermission;

/** The theme has not arrived yet. */
export const Loading: Story = adminLoadingOn("get", "/admin/theme");

/** The server failed to answer the theme. */
export const ServerError: Story = adminFailingOn("get", "/admin/theme");

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;
