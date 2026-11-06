// SPDX-License-Identifier: AGPL-3.0-only
// The server settings screen below its heading (manage_settings, FR-705): its pages, one per topic, each a child route
// with one form and one save under a LocalNav of the pages (web-client/14-admin.md, how a screen splits):
// - general (/admin/settings): the instance's name, its descriptions, how to reach its operators and its thumbnail
//   (#663);
// - branding: its own icon and logos (BrandSettings, ADR-0059), three file forms of one primary action each;
// - registration, content, federation, discovery and voice: its policies by topic (PolicySettings), voice only while
//   the server offers voice chat.
// The name, the descriptions and the thumbnail's alt text hold one value each, written in any language and shown as it
// is in every language; the labels are the app's. The name stands in for the domain where the instance is named to
// people; the domain stays in handles and links. The browser keeps a form from being sent while a field is invalid
// (React Aria's native validation: the error is under its field and linked to it, and focus moves to the first
// invalid field). Each form has one FormActionBar: its save button with the result of the last save beside it,
// success or failure (ADR-0110 §5, audit A8); it sticks to the bottom of the screen, since a form may be taller than
// the screen (web-client/README.md E2).
import {
  Button,
  Cluster,
  FileTrigger,
  FormActionBar,
  Heading,
  Hint,
  InlineError,
  LocalNav,
  PreviewImage,
  Stack,
  TextArea,
  TextField,
} from "@my/ui";
import { useId } from "react";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import { RequirePermission, Section } from "../../moderation";
import { acceptedImageTypes } from "../../settings";
import { useVoiceAvailable } from "../../voice";
import { thumbnailAltLimit } from "../api/settings";
import type { PolicyPage } from "../hooks/useServerPolicies";
import { type DescriptionField, descriptionLimits, useServerGeneral } from "../hooks/useServerGeneral";
import { BrandSettings } from "./BrandSettings";
import {
  ContentSettings,
  DiscoverySettings,
  FederationSettings,
  RegistrationSettings,
  saveResult,
  VoiceSettings,
} from "./PolicySettings";

/** A page of the server settings. */
type ServerSettingsPage = "general" | "branding" | PolicyPage;

/** The pages in the order of the LocalNav: their routes and the names of their links. */
const pages = [
  { page: "general", href: "/admin/settings", label: "client:admin.settings.page.general" },
  { page: "branding", href: "/admin/settings/branding", label: "client:admin.settings.page.branding" },
  { page: "registration", href: "/admin/settings/registration", label: "client:admin.settings.page.registration" },
  { page: "content", href: "/admin/settings/content", label: "client:admin.settings.page.content" },
  { page: "federation", href: "/admin/settings/federation", label: "client:admin.settings.page.federation" },
  { page: "discovery", href: "/admin/settings/discovery", label: "client:admin.settings.page.discovery" },
  { page: "voice", href: "/admin/settings/voice", label: "client:admin.settings.page.voice" },
] as const satisfies readonly { page: ServerSettingsPage; href: string; label: ClientMessageKey }[];

/** One page of the server settings, with the links to the others above it. */
export function ServerSettings({ page }: { page: ServerSettingsPage }) {
  const t = useT();
  // The voice chat page is offered only while the server offers voice chat (ADR-0100 §2).
  const voiceAvailable = useVoiceAvailable();
  const shown = pages.filter((item) => item.page !== "voice" || voiceAvailable || page === "voice");
  return (
    <RequirePermission permission="manage_settings">
      <Stack gap="8">
        <LocalNav
          label={t("client:admin.settings.pages")}
          items={shown.map((item) => ({ href: item.href, label: t(item.label), current: item.page === page }))}
        />
        <SettingsPage page={page} />
      </Stack>
    </RequirePermission>
  );
}

function SettingsPage({ page }: { page: ServerSettingsPage }) {
  switch (page) {
    case "general":
      return <GeneralSettings />;
    case "branding":
      return <BrandSettings />;
    case "registration":
      return <RegistrationSettings />;
    case "content":
      return <ContentSettings />;
    case "federation":
      return <FederationSettings />;
    case "discovery":
      return <DiscoverySettings />;
    case "voice":
      return <VoiceSettings />;
  }
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

/** The general page: the name, the descriptions, the contact and the thumbnail in one form with one save. */
function GeneralSettings() {
  const t = useT();
  const form = useServerGeneral();
  const thumbnailHeadingId = useId();
  const uploadErrorId = useId();
  return (
    <Stack as="form" gap="8" onSubmit={form.submit}>
      <Section surface="card" title={t("client:admin.settings.name.title")}>
        <Hint size="inherit">{t("client:admin.settings.name.description", { domain: form.domain })}</Hint>
        <TextField
          label={t("client:admin.settings.name.field")}
          value={form.name.value}
          onChange={form.name.onChange}
          // Only an invalid name sets isInvalid: React Aria ignores `validate` once isInvalid is given at all.
          {...(form.name.error === undefined ? {} : { isInvalid: true, errorMessage: form.name.error })}
        />
      </Section>
      <Section surface="card" title={t("client:admin.settings.descriptions.title")}>
        <Hint size="inherit">{t("client:admin.settings.descriptions.description")}</Hint>
        <DescriptionInput field={form.short} label={t("client:admin.settings.descriptions.short")} />
        <DescriptionInput field={form.full} label={t("client:admin.settings.descriptions.full")} />
      </Section>
      <Section surface="card" title={t("client:admin.settings.contact.title")}>
        <Hint size="inherit">{t("client:admin.settings.contact.description")}</Hint>
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
          <Heading level={3} id={thumbnailHeadingId}>
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
      </Section>
      <FormActionBar
        sticky
        result={saveResult(form.status, form.saveError, (reason) =>
          t("client:admin.settings.general.save_failed", { reason }),
        )}
      >
        <Button type="submit" variant="primary" isPending={form.isSaving}>
          {t("client:admin.settings.general.save")}
        </Button>
      </FormActionBar>
    </Stack>
  );
}
