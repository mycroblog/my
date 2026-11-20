// SPDX-License-Identifier: AGPL-3.0-only

// The fake's administration of roles, custom emoji, announcements and webhooks (FR-111, FR-720, FR-722, FR-726,
// FR-915): what each area's module answers (staff-roles.ts, staff-emoji.ts, staff-announcements.ts,
// staff-webhooks.ts), from what they take of the fake, and the IDs of their things by the keys the stories name
// (`{server:webhook:alerts}` in a story's address).

import { announcementHandlers, type FakeAnnouncementOptions } from "./staff-announcements";
import { emojiHandlers, type FakeEmojiOptions } from "./staff-emoji";
import { roleHandlers, type FakeRoleOptions } from "./staff-roles";
import { webhookHandlers, type FakeWebhookOptions } from "./staff-webhooks";
import type { Handlers, Refusal, Schema } from "./spec";

/** What the stories ask of the administration of roles, custom emoji, announcements and webhooks. */
export type FakeStaffServerOptions = {
  readonly roles?: FakeRoleOptions;
  readonly emoji?: FakeEmojiOptions;
  readonly announcements?: FakeAnnouncementOptions;
  readonly webhooks?: FakeWebhookOptions;
};

/** What the areas' handlers take from the fake. */
export type StaffServerContext = {
  /** When the fake started, in ms since the epoch. */
  readonly started: number;
  readonly options: FakeStaffServerOptions | undefined;
  /** The reader's username, and whether they read without a session. */
  readonly viewer: string;
  readonly visitor: boolean;
  /** The reader's permissions now. */
  readonly permissions: () => readonly Schema<"Permission">[];
  readonly accountOf: (username: string) => Schema<"Account">;
  /** Whether the fake has the person of `username`: a seed of a test may lack the scenario's people. */
  readonly known: (username: string) => boolean;
  readonly accountId: (username: string) => string;
  /** The account of the ID, if it is a person of the fake. */
  readonly accountById: (id: string) => Schema<"Account"> | undefined;
  /** The scenario's local custom emoji, as the picker lists them. */
  readonly localEmojis: readonly Schema<"CustomEmoji">[];
  /** The domain and the origin the fake answers as. */
  readonly host: string;
  readonly origin: string;
  /** An ID as the server's, of a moment and a number. */
  readonly idAt: (ms: number, n: number) => string;
  /** A problem as the server answers it, with the members of its type (`reason`) in `more`. */
  readonly problem: (status: number, title: string, more?: Readonly<Record<string, unknown>>) => Refusal;
  readonly notFound: () => Refusal;
};

/** A problem as the server answers it, with the members of its type besides the standard ones. */
export const refusalOf = (status: number, title: string, more: Readonly<Record<string, unknown>> = {}): Refusal => ({
  problem: { type: "about:blank", title, status, ...more },
});

/** What a test or a story reads of the fake's administration of these areas. */
export type FakeStaffServer = {
  /** The ID of the thing of `area` the fake knows by `key` (`webhook`, `alerts`), or "none". */
  readonly id: (area: string, key: string) => string;
  /** The announcements published from the administration that the readers see now, newest first. */
  readonly shownAnnouncements: () => Schema<"Announcement">[];
};

/** The handlers of the four areas, and what a test reads of them. */
export function staffServerHandlers(c: StaffServerContext): { handlers: Handlers; server: FakeStaffServer } {
  const roles = roleHandlers(c);
  const emoji = emojiHandlers(c);
  const announcements = announcementHandlers(c);
  const webhooks = webhookHandlers(c);
  const ids: Readonly<Record<string, (key: string) => string>> = {
    emoji: emoji.id,
    request: emoji.requestId,
    announcement: announcements.id,
    webhook: webhooks.id,
  };
  return {
    handlers: { ...roles.handlers, ...emoji.handlers, ...announcements.handlers, ...webhooks.handlers },
    server: {
      id: (area, key) => ids[area]?.(key) ?? "none",
      shownAnnouncements: announcements.shown,
    },
  };
}
