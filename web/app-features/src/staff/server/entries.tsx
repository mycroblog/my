// SPDX-License-Identifier: AGPL-3.0-only

// The entries of roles, custom emoji, announcements and webhooks among the staff's screens (entries.tsx; FR-111,
// FR-720, FR-722, FR-726, ADR-0058 §1): under This server, the roles and their permissions, the announcements to every
// local user and the custom emoji with their requests; under Connections, the webhooks. Each screen under one of them
// (a role, a new one) is a route of its own (admin.roles.$key.tsx and the like).

import { loadAdminRoles } from "@my/app-data";
import type { StaffEntry } from "../entries";
import { AdminAnnouncementsScreen, loadAnnouncementsScreen } from "./announcements";
import { AdminEmojiScreen, loadEmojiScreen } from "./emoji";
import { AdminRolesScreen } from "./roles";
import { AdminWebhooksScreen, loadWebhooksScreen } from "./webhooks";

/** The entries of roles, custom emoji, announcements and webhooks, in the root's order within their groups. */
export const serverEntries: readonly StaffEntry[] = [
  {
    // The roles, their places and permissions, and their badges (FR-720, ADR-0070 §8).
    path: "roles",
    area: "admin",
    group: "server",
    permission: "manage_roles",
    icon: "members",
    name: "staff_roles.entry",
    Screen: AdminRolesScreen,
    load: (data) => loadAdminRoles(data),
  },
  {
    // The announcements to every local user (FR-726, ADR-0125).
    path: "announcements",
    area: "admin",
    group: "server",
    permission: "manage_announcements",
    icon: "announcements",
    name: "staff_announcements.entry",
    Screen: AdminAnnouncementsScreen,
    load: loadAnnouncementsScreen,
  },
  {
    // The custom emoji, local and remote, and the requests for new ones (FR-111, FR-915, ADR-0051, ADR-0070).
    path: "custom-emojis",
    area: "admin",
    group: "server",
    permission: "manage_custom_emojis",
    icon: "emojiCustom",
    name: "staff_emoji.entry",
    count: { of: (facts) => facts.pendingEmojiRequests, words: "staff_emoji.entry.count" },
    Screen: AdminEmojiScreen,
    load: loadEmojiScreen,
  },
  {
    // The webhooks of the instance (FR-722, ADR-0060).
    path: "webhooks",
    area: "admin",
    group: "connections",
    permission: "manage_webhooks",
    icon: "link",
    name: "staff_webhooks.entry",
    Screen: AdminWebhooksScreen,
    load: loadWebhooksScreen,
  },
];
