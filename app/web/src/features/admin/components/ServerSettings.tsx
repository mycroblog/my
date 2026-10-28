// SPDX-License-Identifier: AGPL-3.0-only
// The server settings screen below its heading (manage_settings, FR-705): the instance's name and descriptions, how
// to reach its operators and its thumbnail (#663), then its policies (who may sign up, the default language, the
// public timelines, who may create invites of their own and their caps (ADR-0108 §1, §2), whether images need a description, the federation, who may read the list of the domain
// actions, how much of the costs is published and shown to the holders of view_finance, how much of the income is published, and voice chat while the server offers it) in one form saved together. The name, the descriptions and the thumbnail's alt text hold one value each,
// written in any language and shown as it is in every language; the labels are the app's. The name stands in for
// the domain where the instance is named to people; the domain stays in handles and links. The browser keeps a
// form from being sent while a field is invalid (React Aria's native validation: the error is under its field and
// linked to it, and focus moves to the first invalid field). What a save did stays on the page in a StatusMessage
// next to its button; a failure stays above the button (web-client/README.md E2).
import {
  Button,
  Checkbox,
  Cluster,
  Dialog,
  DialogActions,
  FileTrigger,
  Heading,
  Hint,
  InlineError,
  PreviewImage,
  Radio,
  RadioGroup,
  Select,
  SelectItem,
  Stack,
  StatusMessage,
  TextArea,
  TextField,
} from "@my/ui";
import { useId } from "react";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import { Section, RequirePermission } from "../../moderation";
import { acceptedImageTypes } from "../../settings";
import { useVoiceAvailable } from "../../voice";
import { thumbnailAltLimit } from "../api/settings";
import { useServerContact } from "../hooks/useServerContact";
import { type DescriptionField, descriptionLimits, useServerDescriptions } from "../hooks/useServerDescriptions";
import {
  type FederationNarrowing,
  type NumberPolicyKey,
  type PolicyKey,
  useServerPolicies,
} from "../hooks/useServerPolicies";
import { useServerSettings } from "../hooks/useServerSettings";

export function ServerSettings() {
  return (
    <RequirePermission permission="manage_settings">
      <Stack gap="8">
        <NameForm />
        <DescriptionsForm />
        <ContactForm />
        <PoliciesForm />
      </Stack>
    </RequirePermission>
  );
}

function NameForm() {
  const t = useT();
  const form = useServerSettings();
  return (
    <Section surface="card" title={t("client:admin.settings.name.title")}>
      <Hint size="inherit">{t("client:admin.settings.name.description", { domain: form.domain })}</Hint>
      <Stack as="form" gap="4" onSubmit={form.submit}>
        <TextField
          label={t("client:admin.settings.name.field")}
          value={form.field.value}
          onChange={form.field.onChange}
          // Only an invalid name sets isInvalid: React Aria ignores `validate` once isInvalid is given at all.
          {...(form.field.error === undefined ? {} : { isInvalid: true, errorMessage: form.field.error })}
        />
        {form.saveError !== undefined && (
          <InlineError size="inherit" alert>
            {t("client:admin.settings.name.save_failed", { reason: form.saveError })}
          </InlineError>
        )}
        <div>
          <Button type="submit" variant="primary" isPending={form.isSaving}>
            {t("client:admin.settings.name.save")}
          </Button>
        </div>
        <StatusMessage>{form.status}</StatusMessage>
      </Stack>
    </Section>
  );
}

function DescriptionInput({ field, label }: { field: DescriptionField; label: string }) {
  const t = useT();
  const limit = descriptionLimits[field.key];
  return (
    <TextArea
      label={label}
      value={field.value}
      onChange={field.onChange}
      rows={field.key === "description" ? 6 : 3}
      characterLimit={limit}
      formatRemaining={(remaining) =>
        remaining < 0
          ? t("client:admin.settings.characters_over", { count: -remaining })
          : t("client:admin.settings.characters_left", { count: remaining })
      }
      errorMessage={t("client:admin.settings.name.too_long", { max: String(limit) })}
    />
  );
}

function DescriptionsForm() {
  const t = useT();
  const form = useServerDescriptions();
  return (
    <Section surface="card" title={t("client:admin.settings.descriptions.title")}>
      <Hint size="inherit">{t("client:admin.settings.descriptions.description")}</Hint>
      <Stack as="form" gap="6" onSubmit={form.submit}>
        <DescriptionInput field={form.short} label={t("client:admin.settings.descriptions.short")} />
        <DescriptionInput field={form.full} label={t("client:admin.settings.descriptions.full")} />
        {form.saveError !== undefined && (
          <InlineError size="inherit" alert>
            {t("client:admin.settings.descriptions.save_failed", { reason: form.saveError })}
          </InlineError>
        )}
        <div>
          <Button type="submit" variant="primary" isPending={form.isSaving}>
            {t("client:admin.settings.descriptions.save")}
          </Button>
        </div>
        <StatusMessage>{form.status}</StatusMessage>
      </Stack>
    </Section>
  );
}

function ContactForm() {
  const t = useT();
  const form = useServerContact();
  const thumbnailHeadingId = useId();
  const uploadErrorId = useId();
  return (
    <Section surface="card" title={t("client:admin.settings.contact.title")}>
      <Hint size="inherit">{t("client:admin.settings.contact.description")}</Hint>
      <Stack as="form" gap="6" onSubmit={form.submit}>
        <TextField
          label={t("client:admin.settings.contact.account")}
          description={t("client:admin.settings.contact.account.description")}
          value={form.account.value}
          onChange={form.account.onChange}
          autoComplete="off"
        />
        <TextField
          type="email"
          label={t("client:admin.settings.contact.email")}
          description={t("client:admin.settings.contact.email.description")}
          value={form.email.value}
          onChange={form.email.onChange}
          autoComplete="off"
          errorMessage={t("client:admin.settings.contact.email.invalid")}
        />
        <Stack as="section" gap="3" aria-labelledby={thumbnailHeadingId} aria-busy={form.thumbnail.isUploading}>
          <Heading level={3} size="medium" id={thumbnailHeadingId}>
            {t("client:admin.settings.contact.thumbnail")}
          </Heading>
          <Hint>{t("client:admin.settings.contact.thumbnail.description")}</Hint>
          {form.thumbnail.url !== undefined && (
            // A preview of the image chosen in the form; the alt text below describes it.
            <PreviewImage src={form.thumbnail.url} alt="" />
          )}
          <Cluster gap="2">
            <FileTrigger
              acceptedFileTypes={acceptedImageTypes}
              onSelect={(files) => {
                const file = files?.[0];
                if (file !== undefined) form.thumbnail.choose(file);
              }}
            >
              <Button
                size="sm"
                isPending={form.thumbnail.isUploading}
                aria-describedby={
                  form.thumbnail.error === undefined ? thumbnailHeadingId : `${thumbnailHeadingId} ${uploadErrorId}`
                }
              >
                {t("client:admin.settings.contact.thumbnail.choose")}
              </Button>
            </FileTrigger>
            {form.thumbnail.url !== undefined && (
              <Button
                size="sm"
                variant="ghost"
                onPress={form.thumbnail.remove}
                isDisabled={form.thumbnail.isUploading}
                aria-describedby={thumbnailHeadingId}
              >
                {t("client:admin.settings.contact.thumbnail.remove")}
              </Button>
            )}
          </Cluster>
          {form.thumbnail.error !== undefined && (
            <InlineError alert id={uploadErrorId}>
              {form.thumbnail.error}
            </InlineError>
          )}
          <TextArea
            label={t("client:admin.settings.contact.thumbnail.alt")}
            value={form.alt.value}
            onChange={form.alt.onChange}
            rows={2}
            characterLimit={thumbnailAltLimit}
            formatRemaining={(remaining) =>
              remaining < 0
                ? t("client:admin.settings.characters_over", { count: -remaining })
                : t("client:admin.settings.characters_left", { count: remaining })
            }
            errorMessage={t("client:admin.settings.name.too_long", { max: String(thumbnailAltLimit) })}
          />
        </Stack>
        {form.saveError !== undefined && (
          <InlineError size="inherit" alert>
            {t("client:admin.settings.contact.save_failed", { reason: form.saveError })}
          </InlineError>
        )}
        <div>
          <Button type="submit" variant="primary" isPending={form.isSaving}>
            {t("client:admin.settings.contact.save")}
          </Button>
        </div>
        <StatusMessage>{form.status}</StatusMessage>
      </Stack>
    </Section>
  );
}

/** Who may read a public timeline (FR-202), the choices of both timelines' settings. */
const timelineAccessChoices = [
  {
    value: "everyone",
    label: "client:admin.settings.timelines.access.everyone",
    description: "client:admin.settings.timelines.access.everyone.description",
  },
  {
    value: "logged_in",
    label: "client:admin.settings.timelines.access.logged_in",
    description: "client:admin.settings.timelines.access.logged_in.description",
  },
  {
    value: "off",
    label: "client:admin.settings.timelines.access.off",
    description: "client:admin.settings.timelines.access.off.description",
  },
] as const satisfies readonly { value: string; label: ClientMessageKey; description: ClientMessageKey }[];

/** The words and the description of each choice of the settings chosen with radio buttons. */
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
      value: "none",
      label: "client:admin.settings.audience.none",
      description: "client:admin.settings.domain_actions.list.none",
    },
    {
      value: "users",
      label: "client:admin.settings.audience.users",
      description: "client:admin.settings.domain_actions.list.users",
    },
    {
      value: "all",
      label: "client:admin.settings.audience.all",
      description: "client:admin.settings.domain_actions.list.all",
    },
  ],
  domain_actions_rationale_visibility: [
    {
      value: "none",
      label: "client:admin.settings.audience.none",
      description: "client:admin.settings.domain_actions.comments.none",
    },
    {
      value: "users",
      label: "client:admin.settings.audience.users",
      description: "client:admin.settings.domain_actions.comments.users",
    },
    {
      value: "all",
      label: "client:admin.settings.audience.all",
      description: "client:admin.settings.domain_actions.comments.all",
    },
  ],
  finance_visibility: [
    {
      value: "private",
      label: "client:admin.settings.finance.private",
      description: "client:admin.settings.finance.private.description",
    },
    {
      value: "totals",
      label: "client:admin.settings.finance.totals",
      description: "client:admin.settings.finance.totals.description",
    },
    {
      value: "breakdown",
      label: "client:admin.settings.finance.breakdown",
      description: "client:admin.settings.finance.breakdown.description",
    },
  ],
  finance_viewer_income: [
    {
      value: "totals",
      label: "client:admin.settings.finance.viewer.income.totals",
      description: "client:admin.settings.finance.viewer.income.totals.description",
    },
    {
      value: "plans",
      label: "client:admin.settings.finance.viewer.income.plans",
      description: "client:admin.settings.finance.viewer.income.plans.description",
    },
  ],
  income_visibility: [
    {
      value: "private",
      label: "client:admin.settings.income.private",
      description: "client:admin.settings.income.private.description",
    },
    {
      value: "totals",
      label: "client:admin.settings.income.totals",
      description: "client:admin.settings.income.totals.description",
    },
    {
      value: "plans",
      label: "client:admin.settings.income.plans",
      description: "client:admin.settings.income.plans.description",
    },
  ],
  local_timeline_access: timelineAccessChoices,
  federated_timeline_access: timelineAccessChoices,
  supporter_list_visibility: [
    {
      value: "off",
      label: "client:admin.settings.supporter_list.off",
      description: "client:admin.settings.supporter_list.off.description",
    },
    {
      value: "public",
      label: "client:admin.settings.supporter_list.public",
      description: "client:admin.settings.supporter_list.public.description",
    },
    {
      value: "signed_in",
      label: "client:admin.settings.supporter_list.signed_in",
      description: "client:admin.settings.supporter_list.signed_in.description",
    },
  ],
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
  form: ReturnType<typeof useServerPolicies>;
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
  form: ReturnType<typeof useServerPolicies>;
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
  form: ReturnType<typeof useServerPolicies>;
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

function PoliciesForm() {
  const t = useT();
  const form = useServerPolicies();
  // The voice chat settings show only while the server offers voice chat (ADR-0100 §2).
  const voiceAvailable = useVoiceAvailable();
  return (
    <Stack as="form" gap="8" onSubmit={form.submit}>
      <Section surface="card" title={t("client:admin.settings.registration.title")}>
        <PolicyRadios
          form={form}
          setting="registration_mode"
          label={t("client:admin.settings.registration.mode")}
          description={t("client:admin.settings.registration.description")}
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
          value={form.value("member_invite_max_expires_in")}
          onChange={(key) => {
            if (typeof key === "string") form.change("member_invite_max_expires_in", key);
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
          setting="local_timeline_access"
          label={t("client:admin.settings.timelines.local")}
          description={t("client:admin.settings.timelines.local.description")}
        />
        <PolicyRadios
          form={form}
          setting="federated_timeline_access"
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
          value={form.value("remote_media_cache_ttl")}
          onChange={(key) => {
            if (typeof key === "string") form.change("remote_media_cache_ttl", key);
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
          setting="link_cards_enabled"
          label={t("client:admin.settings.federation.link_cards")}
          description={t("client:admin.settings.federation.link_cards.description")}
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
      <Section surface="card" title={t("client:admin.settings.finance.title")}>
        <Hint size="inherit">{t("client:admin.settings.finance.description")}</Hint>
        <PolicyRadios form={form} setting="finance_visibility" label={t("client:admin.settings.finance.visibility")} />
        <Hint size="inherit">{t("client:admin.settings.finance.viewer.description")}</Hint>
        <PolicyCheckbox
          form={form}
          setting="finance_viewer_item_names"
          label={t("client:admin.settings.finance.viewer.item_names")}
          description={t("client:admin.settings.finance.viewer.item_names.description")}
        />
        <PolicyCheckbox
          form={form}
          setting="finance_viewer_item_memos"
          label={t("client:admin.settings.finance.viewer.item_memos")}
          description={t("client:admin.settings.finance.viewer.item_memos.description")}
        />
        <PolicyRadios
          form={form}
          setting="finance_viewer_income"
          label={t("client:admin.settings.finance.viewer.income")}
        />
      </Section>
      <Section surface="card" title={t("client:admin.settings.income.title")}>
        <Hint size="inherit">{t("client:admin.settings.income.description")}</Hint>
        <PolicyRadios form={form} setting="income_visibility" label={t("client:admin.settings.income.visibility")} />
      </Section>
      <Section surface="card" title={t("client:admin.settings.supporter_list.title")}>
        <Hint size="inherit">{t("client:admin.settings.supporter_list.description")}</Hint>
        <PolicyRadios
          form={form}
          setting="supporter_list_visibility"
          label={t("client:admin.settings.supporter_list.visibility")}
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
        <PolicyCheckbox
          form={form}
          setting="public_operators"
          label={t("client:admin.settings.landing.operators")}
          description={t("client:admin.settings.landing.operators.description")}
        />
      </Section>
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
      {voiceAvailable && (
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
            setting="retention_voice_participants"
            label={t("client:admin.settings.voice.retention")}
            description={(range) => t("client:admin.settings.voice.retention.description", range)}
          />
        </Section>
      )}
      {form.saveError !== undefined && (
        <InlineError size="inherit" alert>
          {t("client:admin.settings.policies.save_failed", { reason: form.saveError })}
        </InlineError>
      )}
      <Stack gap="3">
        <div>
          <Button type="submit" variant="primary" isPending={form.isSaving}>
            {t("client:admin.settings.policies.save")}
          </Button>
        </div>
        <StatusMessage>{form.status}</StatusMessage>
      </Stack>
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
 * Asks before the federation mode narrows (ADR-0076, "設定"): an alert dialog (WAI-ARIA APG) that lists what stops
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
