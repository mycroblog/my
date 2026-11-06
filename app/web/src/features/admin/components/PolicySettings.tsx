// SPDX-License-Identifier: AGPL-3.0-only
// The pages of the server's policies (manage_settings, FR-705, web-client/14-admin.md), each one form of the settings
// of its topic with one save that sends only its own settings (useServerPolicies, policyPages):
// - registration: who may sign up and whether sign-in is limited to login providers (which asks first), who may create
//   invites of their own and their caps (ADR-0106 §14, §15), and the routes by which apps register (ADR-0090 §1);
// - content: the default language, the public timelines, whether images need a description, the landing page and the
//   featured accounts;
// - federation: its direction (which asks before it narrows, ADR-0006), remote media and link cards, who may read the
//   list of the domain actions, and the decision hooks (ADR-0147);
// - discovery: what the server states to search engines and AI crawlers, and which posts the post search covers while
//   the server has one (ADR-0149 §5);
// - voice: voice chat while the server offers it (ADR-0100 §2, §9, §13).
import {
  Button,
  Checkbox,
  Dialog,
  DialogActions,
  FormActionBar,
  type FormActionResult,
  Hint,
  InlineError,
  Radio,
  RadioGroup,
  Select,
  SelectItem,
  Stack,
  Text,
  TextField,
} from "@my/ui";
import type { ReactNode } from "react";
import { usePostSearchAvailable } from "../../../lib/fullTextSearch";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import { Section } from "../../moderation";
import { useVoiceAvailable } from "../../voice";
import {
  type FederationNarrowing,
  type NumberPolicyKey,
  type PolicyKey,
  useServerPolicies,
} from "../hooks/useServerPolicies";

type PolicyForm = ReturnType<typeof useServerPolicies>;

/** The result of a form's last save for its FormActionBar: the failure with its reason, or what the save did. */
export function saveResult(
  status: string | undefined,
  saveError: string | undefined,
  failed: (reason: string) => string,
): FormActionResult | undefined {
  if (saveError !== undefined) return { tone: "danger", message: failed(saveError) };
  if (status !== undefined) return { tone: "success", message: status };
  return undefined;
}

/** Who may read a public timeline (FR-202), the choices of both timelines' settings. */
const timelineAccessChoices = [
  {
    value: "everyone",
    label: "client:admin.settings.timelines.access.everyone",
    description: "client:admin.settings.timelines.access.everyone.description",
  },
  {
    value: "signed_in",
    label: "client:admin.settings.timelines.access.signed_in",
    description: "client:admin.settings.timelines.access.signed_in.description",
  },
  {
    value: "off",
    label: "client:admin.settings.timelines.access.off",
    description: "client:admin.settings.timelines.access.off.description",
  },
] as const satisfies readonly { value: string; label: ClientMessageKey; description: ClientMessageKey }[];

const hookModeChoices = [
  {
    value: "off",
    label: "client:admin.settings.hooks.mode.off",
    description: "client:admin.settings.hooks.mode.off.description",
  },
  {
    value: "observe",
    label: "client:admin.settings.hooks.mode.observe",
    description: "client:admin.settings.hooks.mode.observe.description",
  },
  {
    value: "enforce",
    label: "client:admin.settings.hooks.mode.enforce",
    description: "client:admin.settings.hooks.mode.enforce.description",
  },
] as const;

const hookPassOnFailure = {
  value: "pass",
  label: "client:admin.settings.hooks.failure.pass",
  description: "client:admin.settings.hooks.failure.pass.description",
} as const;

const hookRetryOnFailure = {
  value: "retry",
  label: "client:admin.settings.hooks.failure.retry",
  description: "client:admin.settings.hooks.failure.retry.description",
} as const;

const choices = {
  registration_mode: [
    {
      value: "open",
      label: "client:admin.settings.registration.open",
      description: "client:admin.settings.registration.open.description",
    },
    {
      value: "approval",
      label: "client:admin.settings.registration.approval",
      description: "client:admin.settings.registration.approval.description",
    },
    {
      value: "invite",
      label: "client:admin.settings.registration.invite",
      description: "client:admin.settings.registration.invite.description",
    },
    {
      value: "closed",
      label: "client:admin.settings.registration.closed",
      description: "client:admin.settings.registration.closed.description",
    },
  ],
  federation_mode: [
    {
      value: "both",
      label: "client:admin.settings.federation.mode.both",
      description: "client:admin.settings.federation.mode.both.description",
    },
    {
      value: "inbound",
      label: "client:admin.settings.federation.mode.inbound",
      description: "client:admin.settings.federation.mode.inbound.description",
    },
    {
      value: "outbound",
      label: "client:admin.settings.federation.mode.outbound",
      description: "client:admin.settings.federation.mode.outbound.description",
    },
    {
      value: "off",
      label: "client:admin.settings.federation.mode.off",
      description: "client:admin.settings.federation.mode.off.description",
    },
  ],
  remote_media_policy: [
    {
      value: "none",
      label: "client:admin.settings.remote_media.none",
      description: "client:admin.settings.remote_media.none.description",
    },
    {
      value: "avatars",
      label: "client:admin.settings.remote_media.avatars",
      description: "client:admin.settings.remote_media.avatars.description",
    },
    {
      value: "all",
      label: "client:admin.settings.remote_media.all",
      description: "client:admin.settings.remote_media.all.description",
    },
  ],
  domain_actions_visibility: [
    {
      value: "off",
      label: "client:admin.settings.audience.off",
      description: "client:admin.settings.domain_actions.list.off",
    },
    {
      value: "signed_in",
      label: "client:admin.settings.audience.signed_in",
      description: "client:admin.settings.domain_actions.list.signed_in",
    },
    {
      value: "everyone",
      label: "client:admin.settings.audience.everyone",
      description: "client:admin.settings.domain_actions.list.everyone",
    },
  ],
  domain_actions_rationale_visibility: [
    {
      value: "off",
      label: "client:admin.settings.audience.off",
      description: "client:admin.settings.domain_actions.comments.off",
    },
    {
      value: "signed_in",
      label: "client:admin.settings.audience.signed_in",
      description: "client:admin.settings.domain_actions.comments.signed_in",
    },
    {
      value: "everyone",
      label: "client:admin.settings.audience.everyone",
      description: "client:admin.settings.domain_actions.comments.everyone",
    },
  ],
  local_timeline_visibility: timelineAccessChoices,
  federated_timeline_visibility: timelineAccessChoices,
  search_indexing: [
    {
      value: "allow",
      label: "client:admin.settings.crawlers.search.allow",
      description: "client:admin.settings.crawlers.search.allow.description",
    },
    {
      value: "deny",
      label: "client:admin.settings.crawlers.search.deny",
      description: "client:admin.settings.crawlers.search.deny.description",
    },
  ],
  ai_usage: [
    {
      value: "allow",
      label: "client:admin.settings.crawlers.ai.allow",
      description: "client:admin.settings.crawlers.ai.allow.description",
    },
    {
      value: "deny_training",
      label: "client:admin.settings.crawlers.ai.deny_training",
      description: "client:admin.settings.crawlers.ai.deny_training.description",
    },
    {
      value: "deny",
      label: "client:admin.settings.crawlers.ai.deny",
      description: "client:admin.settings.crawlers.ai.deny.description",
    },
  ],
  // Only what the common rule allows (ADR-0149 §5): unlisted, followers-only and direct posts are never searched.
  search_visibilities: [
    {
      value: "public",
      label: "client:admin.settings.search.public",
      description: "client:admin.settings.search.public.description",
    },
    {
      value: "local",
      label: "client:admin.settings.search.local",
      description: "client:admin.settings.search.local.description",
    },
    {
      value: "public,local",
      label: "client:admin.settings.search.public_local",
      description: "client:admin.settings.search.public_local.description",
    },
  ],
  invite_creators: [
    {
      value: "everyone",
      label: "client:admin.settings.invites.creators.everyone",
      description: "client:admin.settings.invites.creators.everyone.description",
    },
    {
      value: "permitted",
      label: "client:admin.settings.invites.creators.permitted",
      description: "client:admin.settings.invites.creators.permitted.description",
    },
  ],
  hook_inbound_status_mode: hookModeChoices,
  hook_inbound_follow_mode: hookModeChoices,
  hook_signup_check_mode: hookModeChoices,
  hook_media_check_mode: hookModeChoices,
  hook_local_status_mode: hookModeChoices,
  hook_inbound_status_on_failure: [hookPassOnFailure, hookRetryOnFailure],
  hook_inbound_follow_on_failure: [hookPassOnFailure, hookRetryOnFailure],
  hook_signup_check_on_failure: [
    hookPassOnFailure,
    {
      value: "require_approval",
      label: "client:admin.settings.hooks.failure.require_approval",
      description: "client:admin.settings.hooks.failure.require_approval.description",
    },
  ],
  hook_media_check_on_failure: [hookPassOnFailure, hookRetryOnFailure],
  voice_room_starters: [
    {
      value: "everyone",
      label: "client:admin.settings.voice.starters.everyone",
      description: "client:admin.settings.voice.starters.everyone.description",
    },
    {
      value: "permitted",
      label: "client:admin.settings.voice.starters.permitted",
      description: "client:admin.settings.voice.starters.permitted.description",
    },
  ],
} as const satisfies Partial<
  Record<PolicyKey, readonly { value: string; label: ClientMessageKey; description: ClientMessageKey }[]>
>;

type RadioKey = keyof typeof choices;

function PolicyRadios({
  form,
  setting,
  label,
  description,
}: {
  form: PolicyForm;
  setting: RadioKey;
  label: string;
  description?: string;
}) {
  const t = useT();
  return (
    <RadioGroup
      label={label}
      {...(description === undefined ? {} : { description })}
      value={form.value(setting)}
      onChange={(value) => {
        form.change(setting, value);
      }}
    >
      {choices[setting].map((choice) => (
        <Radio key={choice.value} value={choice.value} description={t(choice.description)}>
          {t(choice.label)}
        </Radio>
      ))}
    </RadioGroup>
  );
}

function PolicyCheckbox({
  form,
  setting,
  label,
  description,
}: {
  form: PolicyForm;
  setting: PolicyKey;
  label: string;
  description: string;
}) {
  return (
    <Checkbox
      isSelected={form.value(setting) === "true"}
      onChange={(selected) => {
        form.change(setting, selected ? "true" : "false");
      }}
      description={description}
    >
      {label}
    </Checkbox>
  );
}

/** A setting typed as a whole number, with its bounds in the description and its error under the field. */
function NumberPolicyField({
  form,
  setting,
  label,
  description,
}: {
  form: PolicyForm;
  setting: NumberPolicyKey;
  label: string;
  description: (range: { min: string; max: string }) => string;
}) {
  const field = form.numberField(setting);
  return (
    <TextField
      label={label}
      description={description({ min: String(field.min), max: String(field.max) })}
      value={form.value(setting)}
      onChange={(value) => {
        form.change(setting, value);
      }}
      inputMode="numeric"
      autoComplete="off"
      // Only an invalid value sets isInvalid: React Aria ignores `validate` once isInvalid is given at all.
      {...(field.error === undefined ? {} : { isInvalid: true, errorMessage: field.error })}
    />
  );
}

/**
 * The form of one page: its sections, then its save button with the result of the last save, and the confirmations
 * that a save may ask first (they open only for a change of their own settings, which only their page sends).
 */
function PolicyFormFrame({ form, children }: { form: PolicyForm; children: ReactNode }) {
  const t = useT();
  return (
    <Stack as="form" gap="8" onSubmit={form.submit}>
      {children}
      <FormActionBar
        sticky
        result={saveResult(form.status, form.saveError, (reason) =>
          t("client:admin.settings.policies.save_failed", { reason }),
        )}
      >
        <Button type="submit" variant="primary" isPending={form.isSaving}>
          {t("client:admin.settings.policies.save")}
        </Button>
      </FormActionBar>
      <LoginProvidersOnlyDialog confirmation={form.limitConfirmation} />
      {form.narrowing !== null && (
        <NarrowingDialog
          narrowing={form.narrowing}
          isOpen={form.isConfirming}
          onConfirm={form.confirm}
          onCancel={form.cancel}
        />
      )}
    </Stack>
  );
}

/** Registration: who may sign up, the members' invites, and the registration of apps. */
export function RegistrationSettings() {
  const form = useServerPolicies("registration");
  return (
    <PolicyFormFrame form={form}>
      <RegistrationSections form={form} />
    </PolicyFormFrame>
  );
}

/** Content: the default language, the public timelines, the posts, the landing page and the featured accounts. */
export function ContentSettings() {
  const form = useServerPolicies("content");
  return (
    <PolicyFormFrame form={form}>
      <ContentSections form={form} />
    </PolicyFormFrame>
  );
}

/** Federation: its direction and what it fetches, the domain actions' list, and the decision hooks. */
export function FederationSettings() {
  const form = useServerPolicies("federation");
  return (
    <PolicyFormFrame form={form}>
      <FederationSections form={form} />
    </PolicyFormFrame>
  );
}

/** Discovery: search engines and AI, and the post search while the server has one. */
export function DiscoverySettings() {
  const form = useServerPolicies("discovery");
  return (
    <PolicyFormFrame form={form}>
      <DiscoverySections form={form} />
    </PolicyFormFrame>
  );
}

/** Voice chat, while the server offers it; else why there is nothing to set. */
export function VoiceSettings() {
  const t = useT();
  // The voice chat settings show only while the server offers voice chat (ADR-0100 §2).
  const voiceAvailable = useVoiceAvailable();
  if (!voiceAvailable) return <Text tone="muted">{t("client:admin.settings.voice.unavailable")}</Text>;
  return <VoiceForm />;
}

function VoiceForm() {
  const form = useServerPolicies("voice");
  return (
    <PolicyFormFrame form={form}>
      <VoiceSections form={form} />
    </PolicyFormFrame>
  );
}

function RegistrationSections({ form }: { form: PolicyForm }) {
  const t = useT();
  return (
    <>
      <Section surface="card" title={t("client:admin.settings.registration.title")}>
        <PolicyRadios
          form={form}
          setting="registration_mode"
          label={t("client:admin.settings.registration.mode")}
          description={t("client:admin.settings.registration.description")}
        />
        <PolicyCheckbox
          form={form}
          setting="login_providers_only"
          label={t("client:admin.settings.login_providers_only.label")}
          description={t("client:admin.settings.login_providers_only.description")}
        />
      </Section>
      <Section surface="card" title={t("client:admin.settings.invites.title")}>
        <Hint size="inherit">{t("client:admin.settings.invites.description")}</Hint>
        <PolicyRadios form={form} setting="invite_creators" label={t("client:admin.settings.invites.creators")} />
        <NumberPolicyField
          form={form}
          setting="member_invite_max_active"
          label={t("client:admin.settings.invites.max_active")}
          description={(range) => t("client:admin.settings.invites.max_active.description", range)}
        />
        <NumberPolicyField
          form={form}
          setting="member_invite_max_uses"
          label={t("client:admin.settings.invites.max_uses")}
          description={(range) => t("client:admin.settings.invites.max_uses.description", range)}
        />
        <Select
          label={t("client:admin.settings.invites.max_expires_in")}
          description={t("client:admin.settings.invites.max_expires_in.description")}
          value={form.value("member_invite_max_lifetime_seconds")}
          onChange={(key) => {
            if (typeof key === "string") form.change("member_invite_max_lifetime_seconds", key);
          }}
          items={form.inviteExpiryChoices.map((choice) => ({ id: choice.value, name: choice.label }))}
        >
          {(choice) => (
            <SelectItem id={choice.id} textValue={choice.name}>
              {choice.name}
            </SelectItem>
          )}
        </Select>
      </Section>
      <Section surface="card" title={t("client:admin.settings.oauth_apps.title")}>
        <Hint size="inherit">{t("client:admin.settings.oauth_apps.description")}</Hint>
        <PolicyCheckbox
          form={form}
          setting="user_app_registration"
          label={t("client:admin.settings.oauth_apps.user")}
          description={t("client:admin.settings.oauth_apps.user.description")}
        />
        <PolicyCheckbox
          form={form}
          setting="oauth_metadata_document_clients"
          label={t("client:admin.settings.oauth_apps.metadata_document")}
          description={t("client:admin.settings.oauth_apps.metadata_document.description")}
        />
        <PolicyCheckbox
          form={form}
          setting="dynamic_client_registration"
          label={t("client:admin.settings.oauth_apps.dynamic")}
          description={t("client:admin.settings.oauth_apps.dynamic.description")}
        />
      </Section>
    </>
  );
}

function ContentSections({ form }: { form: PolicyForm }) {
  const t = useT();
  return (
    <>
      <Section surface="card" title={t("client:admin.settings.language.title")}>
        <Select
          label={t("client:admin.settings.language.default")}
          description={t("client:admin.settings.language.description")}
          value={form.value("default_locale")}
          onChange={(key) => {
            if (typeof key === "string") form.change("default_locale", key);
          }}
          items={form.localeChoices.map((choice) => ({ id: choice.value, name: choice.label }))}
        >
          {(choice) => (
            <SelectItem id={choice.id} textValue={choice.name}>
              {choice.name}
            </SelectItem>
          )}
        </Select>
      </Section>
      <Section surface="card" title={t("client:admin.settings.timelines.title")}>
        <PolicyRadios
          form={form}
          setting="local_timeline_visibility"
          label={t("client:admin.settings.timelines.local")}
          description={t("client:admin.settings.timelines.local.description")}
        />
        <PolicyRadios
          form={form}
          setting="federated_timeline_visibility"
          label={t("client:admin.settings.timelines.federated")}
          description={t("client:admin.settings.timelines.federated.description")}
        />
      </Section>
      <Section surface="card" title={t("client:admin.settings.posts.title")}>
        <PolicyCheckbox
          form={form}
          setting="require_media_description"
          label={t("client:admin.settings.posts.require_media_description")}
          description={t("client:admin.settings.posts.require_media_description.description")}
        />
      </Section>
      <Section surface="card" title={t("client:admin.settings.landing.title")}>
        <Hint size="inherit">{t("client:admin.settings.landing.description")}</Hint>
        <PolicyCheckbox
          form={form}
          setting="public_counts"
          label={t("client:admin.settings.landing.counts")}
          description={t("client:admin.settings.landing.counts.description")}
        />
      </Section>
      {/* The accounts that the suggestions to follow show first, in this order (FR-308). */}
      <Section surface="card" title={t("client:admin.settings.featured.title")}>
        <Hint size="inherit">{t("client:admin.settings.featured.description")}</Hint>
        <TextField
          label={t("client:admin.settings.featured.accounts")}
          description={t("client:admin.settings.featured.accounts.description")}
          value={form.value("featured_accounts")}
          onChange={(value) => {
            form.change("featured_accounts", value);
          }}
          autoComplete="off"
          autoCorrect="off"
          spellCheck="false"
        />
      </Section>
    </>
  );
}

function FederationSections({ form }: { form: PolicyForm }) {
  const t = useT();
  return (
    <>
      <Section surface="card" title={t("client:admin.settings.federation.title")}>
        <PolicyRadios
          form={form}
          setting="federation_mode"
          label={t("client:admin.settings.federation.mode")}
          description={t("client:admin.settings.federation.mode.description")}
        />
        <PolicyCheckbox
          form={form}
          setting="authorized_fetch"
          label={t("client:admin.settings.federation.authorized_fetch")}
          description={t("client:admin.settings.federation.authorized_fetch.description")}
        />
        <PolicyRadios
          form={form}
          setting="remote_media_policy"
          label={t("client:admin.settings.remote_media.policy")}
        />
        <Select
          label={t("client:admin.settings.remote_media.ttl")}
          description={t("client:admin.settings.remote_media.ttl.description")}
          value={form.value("remote_media_cache_ttl_seconds")}
          onChange={(key) => {
            if (typeof key === "string") form.change("remote_media_cache_ttl_seconds", key);
          }}
          items={form.cacheTtlChoices.map((choice) => ({ id: choice.value, name: choice.label }))}
        >
          {(choice) => (
            <SelectItem id={choice.id} textValue={choice.name}>
              {choice.name}
            </SelectItem>
          )}
        </Select>
        <PolicyCheckbox
          form={form}
          setting="link_cards"
          label={t("client:admin.settings.federation.link_cards")}
          description={t("client:admin.settings.federation.link_cards.description")}
        />
        <PolicyCheckbox
          form={form}
          setting="link_cards_ignore_robots_txt"
          label={t("client:admin.settings.federation.link_cards_ignore_robots_txt")}
          description={t("client:admin.settings.federation.link_cards_ignore_robots_txt.description")}
        />
      </Section>
      <Section surface="card" title={t("client:admin.settings.domain_actions.title")}>
        <Hint size="inherit">{t("client:admin.settings.domain_actions.description")}</Hint>
        <PolicyRadios
          form={form}
          setting="domain_actions_visibility"
          label={t("client:admin.settings.domain_actions.list")}
        />
        <PolicyRadios
          form={form}
          setting="domain_actions_rationale_visibility"
          label={t("client:admin.settings.domain_actions.comments")}
        />
      </Section>
      <Section surface="card" title={t("client:admin.settings.hooks.title")}>
        <Text tone="muted">{t("client:admin.settings.hooks.description")}</Text>
        <PolicyRadios
          form={form}
          setting="hook_inbound_status_mode"
          label={t("client:admin.settings.hooks.inbound_status.mode")}
        />
        <PolicyRadios
          form={form}
          setting="hook_inbound_status_on_failure"
          label={t("client:admin.settings.hooks.inbound_status.on_failure")}
        />
        <PolicyCheckbox
          form={form}
          setting="hook_inbound_status_unfollowed_direct"
          label={t("client:admin.settings.hooks.inbound_status.unfollowed_direct")}
          description={t("client:admin.settings.hooks.inbound_status.unfollowed_direct.description")}
        />
        <PolicyRadios
          form={form}
          setting="hook_inbound_follow_mode"
          label={t("client:admin.settings.hooks.inbound_follow.mode")}
        />
        <PolicyRadios
          form={form}
          setting="hook_inbound_follow_on_failure"
          label={t("client:admin.settings.hooks.inbound_follow.on_failure")}
        />
        <PolicyRadios
          form={form}
          setting="hook_signup_check_mode"
          label={t("client:admin.settings.hooks.signup_check.mode")}
        />
        <PolicyRadios
          form={form}
          setting="hook_signup_check_on_failure"
          label={t("client:admin.settings.hooks.signup_check.on_failure")}
        />
        <PolicyCheckbox
          form={form}
          setting="hook_signup_check_reject"
          label={t("client:admin.settings.hooks.signup_check.reject")}
          description={t("client:admin.settings.hooks.signup_check.reject.description")}
        />
        <PolicyRadios
          form={form}
          setting="hook_media_check_mode"
          label={t("client:admin.settings.hooks.media_check.mode")}
        />
        <PolicyRadios
          form={form}
          setting="hook_media_check_on_failure"
          label={t("client:admin.settings.hooks.media_check.on_failure")}
        />
        <PolicyRadios
          form={form}
          setting="hook_local_status_mode"
          label={t("client:admin.settings.hooks.local_status.mode")}
        />
        <NumberPolicyField
          form={form}
          setting="hook_local_status_account_age_days"
          label={t("client:admin.settings.hooks.local_status.account_age")}
          description={(range) => t("client:admin.settings.hooks.local_status.account_age.description", range)}
        />
        <NumberPolicyField
          form={form}
          setting="hook_decisions_retention_days"
          label={t("client:admin.settings.hooks.retention")}
          description={(range) => t("client:admin.settings.hooks.retention.description", range)}
        />
      </Section>
    </>
  );
}

function DiscoverySections({ form }: { form: PolicyForm }) {
  const t = useT();
  // The setting of the post search shows only on a server with a search engine (ADR-0149 §5).
  const searchAvailable = usePostSearchAvailable();
  return (
    <>
      <Section surface="card" title={t("client:admin.settings.crawlers.title")}>
        <Hint size="inherit">{t("client:admin.settings.crawlers.description")}</Hint>
        <PolicyRadios form={form} setting="search_indexing" label={t("client:admin.settings.crawlers.search")} />
        <PolicyCheckbox
          form={form}
          setting="indexable_default"
          label={t("client:admin.settings.crawlers.indexable_default")}
          description={t("client:admin.settings.crawlers.indexable_default.description")}
        />
        <PolicyRadios form={form} setting="ai_usage" label={t("client:admin.settings.crawlers.ai")} />
        <TextField
          label={t("client:admin.settings.crawlers.security_contact")}
          description={t("client:admin.settings.crawlers.security_contact.description")}
          value={form.value("security_contact")}
          onChange={(value) => {
            form.change("security_contact", value);
          }}
        />
      </Section>
      {searchAvailable && (
        <Section surface="card" title={t("client:admin.settings.search.title")}>
          <Hint size="inherit">{t("client:admin.settings.search.description")}</Hint>
          <PolicyRadios
            form={form}
            setting="search_visibilities"
            label={t("client:admin.settings.search.visibilities")}
            description={t("client:admin.settings.search.visibilities.description")}
          />
        </Section>
      )}
    </>
  );
}

function VoiceSections({ form }: { form: PolicyForm }) {
  const t = useT();
  return (
    <Section surface="card" title={t("client:admin.settings.voice.title")}>
      <PolicyRadios form={form} setting="voice_room_starters" label={t("client:admin.settings.voice.starters")} />
      <PolicyCheckbox
        form={form}
        setting="voice_public_summary"
        label={t("client:admin.settings.voice.public_summary")}
        description={t("client:admin.settings.voice.public_summary.description")}
      />
      <NumberPolicyField
        form={form}
        setting="voice_participants_retention_days"
        label={t("client:admin.settings.voice.retention")}
        description={(range) => t("client:admin.settings.voice.retention.description", range)}
      />
    </Section>
  );
}

/**
 * Asks before sign-in is limited to login providers: an alert dialog (WAI-ARIA APG) that says what stops, how many
 * active users could no longer sign in, whether that includes the administrator, and how to undo it. It warns and
 * does not refuse. The cancel button has the focus first.
 */
function LoginProvidersOnlyDialog({ confirmation }: { confirmation: PolicyForm["limitConfirmation"] }) {
  const t = useT();
  const { impact } = confirmation;
  const applies = impact !== undefined && impact.enabled_providers > 0;
  return (
    <Dialog
      role="alertdialog"
      closeLabel={null}
      size="md"
      title={t("client:admin.settings.login_providers_only.confirm.title")}
      isOpen={confirmation.isOpen}
      onOpenChange={(open) => {
        if (!open) confirmation.cancel();
      }}
    >
      <Stack gap="4">
        {impact === undefined ? (
          <p>
            {confirmation.isFailed
              ? t("client:admin.settings.login_providers_only.confirm.failed")
              : t("client:admin.settings.login_providers_only.confirm.loading")}
          </p>
        ) : !applies ? (
          <p>{t("client:admin.settings.login_providers_only.confirm.no_provider")}</p>
        ) : (
          <>
            <p>
              {t("client:admin.settings.login_providers_only.confirm.affected", {
                count: impact.users_without_provider,
              })}
            </p>
            {impact.includes_viewer && (
              <InlineError>{t("client:admin.settings.login_providers_only.confirm.self")}</InlineError>
            )}
          </>
        )}
        <Stack as="ul" marker="disc" gap="2">
          <li>{t("client:admin.settings.login_providers_only.confirm.stops")}</li>
          <li>{t("client:admin.settings.login_providers_only.confirm.sessions")}</li>
        </Stack>
        <Hint>{t("client:admin.settings.login_providers_only.confirm.recovery")}</Hint>
        <DialogActions>
          {/* The least destructive choice has the focus first (APG, alert dialog pattern). */}
          <Button autoFocus onPress={confirmation.cancel}>
            {t("client:admin.settings.login_providers_only.confirm.cancel")}
          </Button>
          <Button variant="danger" onPress={confirmation.confirm}>
            {t("client:admin.settings.login_providers_only.confirm.confirm")}
          </Button>
        </DialogActions>
      </Stack>
    </Dialog>
  );
}

const modeLabels = {
  off: "client:admin.settings.federation.mode.off",
  inbound: "client:admin.settings.federation.mode.inbound",
  outbound: "client:admin.settings.federation.mode.outbound",
  both: "client:admin.settings.federation.mode.both",
} as const satisfies Record<FederationNarrowing["to"], ClientMessageKey>;

type NarrowingDialogProps = {
  narrowing: FederationNarrowing;
  isOpen: boolean;
  onConfirm: () => void;
  onCancel: () => void;
};

/**
 * Asks before the federation mode narrows (ADR-0006, "設定"): an alert dialog (WAI-ARIA APG) that lists what stops
 * flowing, says that the relations with other servers stay connected with nothing flowing, and, for `off`, that
 * deletions made meanwhile never reach other servers. The cancel button has the focus first.
 */
function NarrowingDialog({ narrowing, isOpen, onConfirm, onCancel }: NarrowingDialogProps) {
  const t = useT();
  return (
    <Dialog
      role="alertdialog"
      closeLabel={null}
      size="md"
      title={t("client:admin.settings.federation.narrow.title", { mode: t(modeLabels[narrowing.to]) })}
      isOpen={isOpen}
      onOpenChange={(open) => {
        if (!open) onCancel();
      }}
    >
      <Stack gap="4">
        <p>
          {t("client:admin.settings.federation.narrow.description", {
            from: t(modeLabels[narrowing.from]),
            to: t(modeLabels[narrowing.to]),
          })}
        </p>
        <Stack as="ul" marker="disc" gap="2">
          {narrowing.stopsSending && <li>{t("client:admin.settings.federation.narrow.sending")}</li>}
          {narrowing.stopsReceiving && <li>{t("client:admin.settings.federation.narrow.receiving")}</li>}
          {narrowing.stopsAll && <li>{t("client:admin.settings.federation.narrow.deletions")}</li>}
          <li>{t("client:admin.settings.federation.narrow.relations")}</li>
        </Stack>
        <Hint>{t("client:admin.settings.federation.narrow.widen")}</Hint>
        <DialogActions>
          {/* The least destructive choice has the focus first (APG, alert dialog pattern). */}
          <Button autoFocus onPress={onCancel}>
            {t("client:admin.settings.federation.narrow.cancel")}
          </Button>
          <Button variant="danger" onPress={onConfirm}>
            {t("client:admin.settings.federation.narrow.confirm")}
          </Button>
        </DialogActions>
      </Stack>
    </Dialog>
  );
}
