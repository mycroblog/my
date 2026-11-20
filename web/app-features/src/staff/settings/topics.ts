// SPDX-License-Identifier: AGPL-3.0-only

// The instance's settings, grouped as an administrator thinks of them (ADR-0058 §1): each topic a page of its own under
// Server settings, /admin/settings/<topic>, a form saved as a whole, its sections each a few settings of one concern.
// A setting is drawn by what it holds (the server says each one's kind, its choices and its bounds): a choice whose
// options each need a sentence is a list of them, a short choice a select, a number its field with its unit, a text a
// field. The texts of the instance (its name, its descriptions, the alternative texts of its images) stand among the
// settings they belong with, though the server keeps them apart. Translation is an entry of its own (its screen reads
// this table too), and the brand images a page whose pictures the browser draws (branding.tsx).
//
// What the settings of other screens hold (the costs and their publication, the supporters, the funding meter, the
// measurements of the client) is not here: those screens hold them.

import type { AdminSettingKey, InstanceTextKey } from "@my/app-data";
import type { IconName } from "@my/design-system";
import type { MessageId } from "../../messages.gen";

/** A setting whose name the catalog has (`staff_settings.setting.<key>`). */
type Named<K extends string> = `staff_settings.setting.${K}` extends MessageId ? K : never;
/** A setting whose line about it the catalog has (`staff_settings.setting.<key>.about`). */
type About<K extends string> = `staff_settings.setting.${K}.about` extends MessageId ? true : never;
/** A setting whose options each have a sentence (`staff_settings.choice.<key>`, `.about`). */
type Choices<K extends string> = `staff_settings.choice.${K}.about` extends MessageId ? K : never;

/** What a number counts, as its field says after it; a field in hours or minutes holds seconds ×3600 or ×60. */
type Unit =
  | "days"
  | "hours"
  | "minutes"
  | "characters"
  | "posts"
  | "mebibytes"
  | "invites"
  | "people"
  | "images"
  | "times"
  | "pins";

/** A setting of a section, drawn by its kind unless `as` says otherwise. */
export type SettingField =
  | {
      readonly kind: "setting";
      readonly key: AdminSettingKey;
      /** Whether the catalog has a line about it. */
      readonly about: boolean;
      /** `choices`: each option a radio with its sentence; `select`, a select in its row (the default of a choice). */
      readonly as?: "choices" | "select" | undefined;
      readonly unit?: Unit | undefined;
    }
  | { readonly kind: "text"; readonly key: InstanceTextKey; readonly lines: number }
  | { readonly kind: "image"; readonly key: "thumbnail" | "preview_image"; readonly alt: InstanceTextKey }
  /** Settings that are yes or no, as checkboxes under one name (what a default policy keeps). */
  | { readonly kind: "checks"; readonly id: string; readonly keys: readonly AdminSettingKey[] }
  /** The translation providers, their order and what each receives (translation.tsx). */
  | { readonly kind: "providers" };

/** A section of a topic: its heading (`staff_settings.section.<id>`), a line about it where the catalog has one. */
type SettingSection = {
  readonly id: string;
  readonly about: boolean;
  readonly fields: readonly SettingField[];
};

/** A page of the settings. */
export type SettingTopic = {
  /** The last part of its address, /admin/settings/<path>. */
  readonly path: string;
  readonly icon: IconName;
  readonly sections: readonly SettingSection[];
};

/** A setting drawn by its kind, with a line about it or none. */
function setting<K extends AdminSettingKey>(
  key: Named<K>,
  more: { about?: About<K>; unit?: Unit; as?: "select" } = {},
): SettingField {
  return { kind: "setting", key, about: more.about === true, unit: more.unit, as: more.as };
}

/** A choice whose options each need a sentence: a list of them under the section's heading. */
function choices<K extends AdminSettingKey>(key: Choices<K> & Named<K>): SettingField {
  return { kind: "setting", key, about: false, as: "choices" };
}

/** A text of the instance, `lines` tall at rest. */
const text = (key: InstanceTextKey, lines = 1): SettingField => ({ kind: "text", key, lines });

/** A section headed `id`. */
const section = (id: string, fields: readonly SettingField[], about = false): SettingSection => ({
  id,
  about,
  fields,
});

/** The topics of the settings, in the order of their list. */
const settingTopics: readonly SettingTopic[] = [
  {
    path: "general",
    icon: "about",
    sections: [
      section("about", [text("name"), text("short_description", 3), text("description", 8)]),
      section("language", [setting("default_locale")]),
      section("contact", [setting("contact_account"), setting("contact_email", { about: true })]),
      section("images", [
        { kind: "image", key: "thumbnail", alt: "thumbnail_alt" },
        { kind: "image", key: "preview_image", alt: "preview_image_alt" },
      ]),
    ],
  },
  {
    path: "registration",
    icon: "signUp",
    sections: [
      section("mode", [choices("registration_mode"), setting("single_user", { about: true })]),
      section("invites", [
        setting("invite_creators", { about: true }),
        setting("member_invite_max_active", { about: true, unit: "invites" }),
        setting("member_invite_max_uses", { unit: "people" }),
        setting("member_invite_max_lifetime_seconds", { about: true, unit: "hours" }),
      ]),
      section("providers_only", [setting("login_providers_only")]),
      section("apps", [
        setting("user_app_registration", { about: true }),
        setting("dynamic_client_registration"),
        setting("oauth_metadata_document_clients"),
        setting("oauth_client_rate_limits"),
      ]),
    ],
  },
  {
    path: "content",
    icon: "compose",
    sections: [
      section("posts", [setting("max_status_length", { unit: "characters" }), setting("require_media_description")]),
      section("timelines", [
        setting("local_timeline_visibility", { about: true }),
        setting("federated_timeline_visibility"),
      ]),
      section("link_cards", [
        setting("link_cards", { about: true }),
        setting("link_cards_ignore_robots_txt", { about: true }),
      ]),
      section("drafts", [
        setting("max_drafts", { unit: "posts" }),
        setting("max_scheduled_posts", { unit: "posts" }),
        setting("max_scheduled_posts_per_day", { unit: "posts" }),
        setting("scheduled_post_min_lead_minutes", { unit: "minutes" }),
        setting("scheduled_post_max_lead_days", { unit: "days" }),
      ]),
      section("articles", [
        setting("article_authoring", { about: true }),
        setting("max_article_bytes", { about: true }),
        setting("max_article_images", { unit: "images" }),
      ]),
      section("events", [setting("event_creators"), setting("event_records_retention_days", { unit: "days" })]),
      section("landing", [
        setting("public_counts"),
        setting("landing_fediverse_explanation"),
        setting("landing_features"),
        setting("featured_accounts"),
      ]),
    ],
  },
  {
    path: "federation",
    icon: "federated",
    sections: [
      section("federation_mode", [choices("federation_mode")]),
      section("federation_scope", [choices("federation_scope")]),
      section("signatures", [setting("authorized_fetch")]),
      section("remote_media", [
        setting("remote_media_policy", { about: true }),
        setting("remote_media_cache_ttl_seconds", { unit: "minutes" }),
        setting("remote_media_cache_max_bytes", { unit: "mebibytes" }),
      ]),
      section("remote_posts", [
        setting("remote_status_retention_days", { about: true, unit: "days" }),
        setting("remote_status_max_retention_days", { about: true, unit: "days" }),
        setting("relay_status_retention_days", { about: true, unit: "days" }),
      ]),
      section("domain_actions", [
        setting("domain_actions_visibility"),
        setting("domain_actions_rationale_visibility", { about: true }),
      ]),
    ],
  },
  {
    path: "hooks",
    icon: "moderation",
    sections: [
      section("hook_inbound_status", [
        setting("hook_inbound_status_mode"),
        setting("hook_inbound_status_on_failure"),
        setting("hook_inbound_status_unfollowed_direct"),
      ]),
      section("hook_inbound_follow", [setting("hook_inbound_follow_mode"), setting("hook_inbound_follow_on_failure")]),
      section("hook_signup_check", [
        setting("hook_signup_check_mode"),
        setting("hook_signup_check_on_failure"),
        setting("hook_signup_check_reject", { about: true }),
      ]),
      section("hook_media_check", [setting("hook_media_check_mode"), setting("hook_media_check_on_failure")]),
      section("hook_local_status", [
        setting("hook_local_status_mode"),
        setting("hook_local_status_account_age_days", { about: true, unit: "days" }),
      ]),
      section("hook_inbound_actor", [setting("hook_inbound_actor_mode"), setting("hook_inbound_actor_on_failure")]),
      section("hook_local_profile", [
        setting("hook_local_profile_mode"),
        setting("hook_local_profile_account_age_days", { about: true, unit: "days" }),
      ]),
      section("hook_records", [setting("hook_decisions_retention_days", { unit: "days" })]),
    ],
  },
  {
    path: "discovery",
    icon: "search",
    sections: [
      section("search_engines", [choices("search_indexing"), setting("indexable_default", { about: true })]),
      section("security", [setting("security_contact")]),
      section("ai", [choices("ai_usage")]),
      section("full_text_search", [setting("search_visibilities", { about: true })]),
    ],
  },
  {
    path: "voice",
    icon: "room",
    sections: [
      section("voice_starters", [choices("voice_room_starters")]),
      section("voice_rooms", [
        setting("voice_public_summary", { about: true }),
        setting("voice_participants_retention_days", { about: true, unit: "days" }),
      ]),
    ],
  },
  {
    path: "auto-delete",
    icon: "autoDelete",
    sections: [
      section(
        "auto_delete_default",
        [setting("auto_delete_default_enabled"), setting("auto_delete_default_min_age_days", { unit: "days" })],
        true,
      ),
      section("auto_delete_default_kept", [
        {
          kind: "checks",
          id: "kinds",
          keys: [
            "auto_delete_default_keep_pinned",
            "auto_delete_default_keep_bookmarked",
            "auto_delete_default_keep_favourited",
            "auto_delete_default_keep_media",
            "auto_delete_default_keep_polls",
            "auto_delete_default_keep_articles",
          ],
        },
        {
          kind: "checks",
          id: "audiences",
          keys: [
            "auto_delete_default_keep_public",
            "auto_delete_default_keep_local",
            "auto_delete_default_keep_unlisted",
            "auto_delete_default_keep_private",
            "auto_delete_default_keep_direct",
          ],
        },
        setting("auto_delete_default_keep_min_boosts", { unit: "times" }),
        setting("auto_delete_default_keep_min_favourites", { unit: "times" }),
      ]),
      section(
        "auto_delete_max",
        [
          setting("auto_delete_max_age_days", { about: true, unit: "days" }),
          setting("auto_delete_grace_days", { about: true, unit: "days" }),
        ],
        true,
      ),
      section("auto_delete_max_kept", [
        {
          kind: "checks",
          id: "allowed",
          keys: [
            "auto_delete_max_allow_pinned",
            "auto_delete_max_allow_bookmarked",
            "auto_delete_max_allow_favourited",
            "auto_delete_max_allow_media",
            "auto_delete_max_allow_polls",
            "auto_delete_max_allow_articles",
            "auto_delete_max_allow_visibilities",
            "auto_delete_max_allow_thresholds",
          ],
        },
        setting("auto_delete_max_kept_pins", { about: true, unit: "pins" }),
      ]),
    ],
  },
  {
    path: "records",
    icon: "history",
    sections: [
      section("appeals", [setting("appeal_window_days", { about: true, unit: "days" })]),
      section(
        "retention",
        [
          setting("session_origins_retention_days", { unit: "days" }),
          setting("registration_origins_retention_days", { unit: "days" }),
          setting("reports_retention_days", { about: true, unit: "days" }),
          setting("moderation_actions_retention_days", { about: true, unit: "days" }),
          setting("audit_log_retention_days", { unit: "days" }),
        ],
        true,
      ),
    ],
  },
];

/** The settings of the translation entry, a form as a topic's (translation.tsx). */
export const translationTopic: SettingTopic = {
  path: "translation",
  icon: "translate",
  sections: [
    section("translation_providers", [{ kind: "providers" }], true),
    section("translation_browser", [setting("translation_browser", { about: true })]),
    section("translation_limits", [
      setting("translation_user_monthly_characters", { about: true, unit: "characters" }),
      setting("translation_server_monthly_characters", { unit: "characters" }),
      setting("translation_retention_days", { unit: "days" }),
    ]),
    section("translation_reports", [setting("translation_reports_to_server", { about: true })]),
  ],
};

/**
 * What a number's field shows of its setting: the setting's value divided by this. Only a setting the server keeps in
 * another unit than its field shows has one (seconds shown as hours or minutes, bytes as MiB); a setting kept in
 * minutes or days is shown as it is.
 */
const factors: Readonly<Partial<Record<AdminSettingKey, number>>> = {
  member_invite_max_lifetime_seconds: 3600,
  remote_media_cache_ttl_seconds: 60,
  remote_media_cache_max_bytes: 1_048_576,
};

/** The factor of `key`'s field: 1 for a setting shown in the unit the server keeps it in. */
export const factorOf = (key: string): number => factors[key as AdminSettingKey] ?? 1;

/** The topic at `path`; undefined for none. */
export const topicAt = (path: string): SettingTopic | undefined => settingTopics.find((t) => t.path === path);

/** The settings a topic's form holds, by key. */
export function keysOf(topic: SettingTopic): readonly AdminSettingKey[] {
  return topic.sections.flatMap((s) =>
    s.fields.flatMap((f): readonly AdminSettingKey[] =>
      f.kind === "setting"
        ? [f.key]
        : f.kind === "checks"
          ? f.keys
          : f.kind === "image"
            ? [f.key]
            : f.kind === "providers"
              ? ["translation_providers", "translation_provider_visibilities"]
              : [],
    ),
  );
}

/** The texts a topic's form holds. */
export function textsOf(topic: SettingTopic): readonly InstanceTextKey[] {
  return topic.sections.flatMap((s) =>
    s.fields.flatMap((f) => (f.kind === "text" ? [f.key] : f.kind === "image" ? [f.alt] : [])),
  );
}
