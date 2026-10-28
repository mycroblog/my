// SPDX-License-Identifier: AGPL-3.0-only
import {
  Avatar,
  Box,
  Button,
  Checkbox,
  Cluster,
  ConfirmDialog,
  EmojiText,
  FileTrigger,
  FormActionBar,
  Hint,
  InlineError,
  ProfileField,
  ProfileFields as ProfileFieldList,
  ProfileHeader,
  ProfileHeaderImage,
  Section,
  Stack,
  Text,
  TextField,
} from "@my/ui";
import { type ReactNode, useId } from "react";
import { useIntl } from "react-intl";
import { handleOf } from "../../../lib/handle";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import { textHighlights } from "../../../lib/textHighlights";
import { CompletionTextArea, CompletionTextField } from "../../compose";
import { EmojiPickerButton, useCustomEmojiImages, useCustomEmojiShortcodes } from "../../emoji";
import { acceptedImageTypes } from "../api/media";
import {
  characterCount,
  type FieldDraft,
  type ImageControl,
  profileLimits,
  type ServerCrawlerPolicy,
  useProfileForm,
} from "../hooks/useProfileForm";
import { useUnsavedChangesGuard } from "../hooks/useUnsavedChangesGuard";

/**
 * The profile editor (FR-006) in the sections of its screen requirements (web-client/screens/settings-profile.md): a
 * preview of the profile as it is being edited, then the fields, and the save button with its result in a bar that
 * stays at the bottom edge of the screen (`FormActionBar` `sticky`). React Aria's native validation shows each error
 * under its field, links it with aria-describedby and marks the field invalid; the browser then refuses to submit,
 * and focus moves to the first invalid field, so a screen reader reads the error with the field. A text over its
 * limit is invalid as soon as it is typed, like the count of the bio: an error that appeared only when the field lost
 * focus would push the save button away under the pointer that is pressing it.
 */
export function ProfileForm() {
  const t = useT();
  const form = useProfileForm();
  const displayNameId = useId();
  const noteId = useId();
  const shortcodes = useCustomEmojiShortcodes();
  // Only an invalid text sets isInvalid: React Aria ignores `validate` once isInvalid is given at all.
  const lengthCheck: LengthCheck = (value, max) =>
    characterCount(value) > max
      ? { isInvalid: true, errorMessage: t("client:settings.profile.too_long", { max: String(max) }) }
      : {};
  return (
    <>
      <Stack as="form" onSubmit={form.submit} gap="8">
        <ProfilePreview form={form} />
        <ImageSection
          titleKey="client:settings.profile.avatar"
          control={form.avatar}
          preview={
            <Avatar
              src={form.avatar.url}
              name={form.displayName.value || form.account.username}
              seed={form.account.id}
              decorative
              size="xl"
            />
          }
        />
        <ImageSection
          titleKey="client:settings.profile.header"
          control={form.header}
          preview={
            form.header.url === undefined ? null : (
              <ProfileHeaderImage sources={{ still: [{ url: form.header.url }] }} size="preview" />
            )
          }
        />
        <Section title={t("client:settings.profile.identity")}>
          <Stack gap="6">
            <CompletionTextField
              completes="emoji"
              id={displayNameId}
              label={t("client:settings.profile.display_name")}
              description={t("client:settings.profile.display_name.description", {
                max: String(profileLimits.displayName),
              })}
              value={form.displayName.value}
              onChange={form.displayName.onChange}
              {...lengthCheck(form.displayName.value, profileLimits.displayName)}
              highlights={textHighlights(form.displayName.value, { tokens: "shortcodes", shortcodes })}
              autoComplete="nickname"
              endAction={
                <EmojiPickerButton
                  fieldId={displayNameId}
                  value={form.displayName.value}
                  onChange={form.displayName.onChange}
                  label={t("client:settings.profile.display_name.emoji")}
                />
              }
            />
            <CompletionTextArea
              completes="all"
              id={noteId}
              label={t("client:settings.profile.note")}
              value={form.note.value}
              onChange={form.note.onChange}
              characterLimit={profileLimits.note}
              highlights={textHighlights(form.note.value, { tokens: "all", shortcodes, limit: profileLimits.note })}
              formatRemaining={(remaining) =>
                remaining < 0
                  ? t("client:settings.profile.characters_over", { count: -remaining })
                  : t("client:settings.profile.characters_left", { count: remaining })
              }
              errorMessage={t("client:settings.profile.too_long", { max: String(profileLimits.note) })}
              rows={4}
              endAction={
                <EmojiPickerButton
                  fieldId={noteId}
                  value={form.note.value}
                  onChange={form.note.onChange}
                  label={t("client:settings.profile.note.emoji")}
                />
              }
            />
          </Stack>
        </Section>
        <ProfileFields fields={form.fields} lengthCheck={lengthCheck} />
        <Section title={t("client:settings.profile.account_kind")}>
          <Checkbox {...form.locked} description={t("client:settings.profile.locked.description")}>
            {t("client:settings.profile.locked")}
          </Checkbox>
          <Checkbox {...form.bot} description={t("client:settings.profile.bot.description")}>
            {t("client:settings.profile.bot")}
          </Checkbox>
        </Section>
        {/* Whether the account is suggested to others and found by part of its name (FR-307, FR-308). */}
        <Section title={t("client:settings.profile.discovery")}>
          <Checkbox {...form.discoverable} description={t("client:settings.profile.discoverable.description")}>
            {t("client:settings.profile.discoverable")}
          </Checkbox>
        </Section>
        <CrawlerSettings indexable={form.indexable} aiOptOut={form.aiOptOut} serverPolicy={form.serverPolicy} />
        <FormActionBar sticky result={form.result}>
          <Button type="submit" variant="primary" isPending={form.isSaving} isDisabled={form.isBusy && !form.isSaving}>
            {t("client:settings.profile.save")}
          </Button>
        </FormActionBar>
      </Stack>
      {/* Outside the form: React bubbles the dialog's own submit event through its portal to an enclosing form. */}
      <UnsavedChangesDialog hasChanges={form.hasChanges} />
    </>
  );
}

/**
 * The profile as it shows with the values being edited (ADR-0110 §5, P3): the header image, the picture, the name, the
 * handle, the text and the fields, in the profile's own header. The text and the fields show as typed, with the
 * local custom emoji; links and mentions are made when the profile is saved.
 */
function ProfilePreview({ form }: { form: ReturnType<typeof useProfileForm> }) {
  const t = useT();
  const intl = useIntl();
  const emojis = [...useCustomEmojiImages().values()];
  const name = form.displayName.value === "" ? form.account.username : form.displayName.value;
  const fields = form.fields.rows.filter((field) => field.name !== "" || field.value !== "");
  return (
    <Section
      title={t("client:settings.profile.preview")}
      description={t("client:settings.profile.preview.description")}
    >
      <Box border="lg" padding="3">
        <ProfileHeader
          image={form.header.url === undefined ? undefined : { still: [{ url: form.header.url }] }}
          avatar={form.avatar.url === undefined ? undefined : { still: [{ url: form.avatar.url }] }}
          avatarSeed={form.account.id}
          name={name}
          heading={
            <Text as="p" size="lg" weight="semibold" tone="fg" wrap="anywhere">
              <span dir="auto">
                <EmojiText text={name} emojis={emojis} />
              </span>
            </Text>
          }
          handle={handleOf(form.account.acct)}
          note={
            form.note.value !== "" && (
              <Text as="p" tone="fg" whitespace="preWrap" wrap="anywhere" dir="auto">
                <EmojiText text={form.note.value} emojis={emojis} />
              </Text>
            )
          }
          fields={
            fields.length > 0 && (
              <ProfileFieldList>
                {fields.map((field) => (
                  <ProfileField key={field.key} term={<EmojiText text={field.name} emojis={emojis} />}>
                    <span dir="auto">
                      <EmojiText text={field.value} emojis={emojis} />
                    </span>
                  </ProfileField>
                ))}
              </ProfileFieldList>
            )
          }
          joined={t("client:profile.joined", {
            date: intl.formatDate(form.account.created_at, { dateStyle: "long" }),
          })}
        />
      </Box>
    </Section>
  );
}

/**
 * Asks before another page replaces a profile with unsaved changes (the APG alert dialog pattern): focus
 * starts on "Keep editing", which stays on the form like Esc does, and returns to the link that was pressed.
 */
function UnsavedChangesDialog({ hasChanges }: { hasChanges: boolean }) {
  const t = useT();
  const guard = useUnsavedChangesGuard(hasChanges);
  return (
    <ConfirmDialog
      isOpen={guard.isAsking}
      onOpenChange={(isOpen) => {
        if (!isOpen) guard.stay();
      }}
      title={t("client:settings.profile.unsaved.title")}
      description={t("client:settings.profile.unsaved.description")}
      confirmLabel={t("client:settings.profile.unsaved.leave")}
      cancelLabel={t("client:settings.profile.unsaved.stay")}
      onConfirm={guard.leave}
    />
  );
}

type CrawlerSettingsProps = {
  indexable: ReturnType<typeof useProfileForm>["indexable"];
  aiOptOut: ReturnType<typeof useProfileForm>["aiOptOut"];
  serverPolicy: ServerCrawlerPolicy;
};

/**
 * Whether search engines may index the profile and public posts, and whether AI may train on or use them (FR-412,
 * ADR-0057). The server's own statement wins where it is stricter, so the section says when it denies already.
 */
function CrawlerSettings({ indexable, aiOptOut, serverPolicy }: CrawlerSettingsProps) {
  const t = useT();
  return (
    <Section title={t("client:settings.profile.crawlers")}>
      <Checkbox {...indexable} description={t("client:settings.profile.indexable.description")}>
        {t("client:settings.profile.indexable")}
      </Checkbox>
      {serverPolicy?.searchIndexing === "deny" && <Hint>{t("client:settings.profile.indexable.server_denies")}</Hint>}
      <Checkbox {...aiOptOut} description={t("client:settings.profile.ai_opt_out.description")}>
        {t("client:settings.profile.ai_opt_out")}
      </Checkbox>
      {serverPolicy !== undefined && serverPolicy.aiUsage !== "allow" && (
        <Hint>
          {t("client:settings.profile.ai_opt_out.server_denies", {
            usage: serverPolicy.aiUsage,
          })}
        </Hint>
      )}
    </Section>
  );
}

type ImageSectionProps = { titleKey: ClientMessageKey; control: ImageControl; preview: ReactNode };

/** An image of the profile: its preview, and buttons to choose a new file or remove it. */
function ImageSection({ titleKey, control, preview }: ImageSectionProps) {
  const t = useT();
  const headingId = useId();
  const errorId = useId();
  return (
    <Section title={t(titleKey)} headingId={headingId} busy={control.isUploading}>
      {preview}
      <Cluster gap="2">
        <FileTrigger
          acceptedFileTypes={acceptedImageTypes}
          onSelect={(files) => {
            const file = files?.[0];
            if (file !== undefined) control.choose(file);
          }}
        >
          <Button
            size="sm"
            isPending={control.isUploading}
            aria-describedby={control.error === undefined ? headingId : `${headingId} ${errorId}`}
          >
            {t("client:settings.profile.image.choose")}
          </Button>
        </FileTrigger>
        {control.hasImage && (
          <Button
            size="sm"
            variant="ghost"
            onPress={control.remove}
            isDisabled={control.isUploading}
            aria-describedby={headingId}
          >
            {t("client:settings.profile.image.remove")}
          </Button>
        )}
      </Cluster>
      {control.error !== undefined && (
        <InlineError id={errorId} alert>
          {control.error}
        </InlineError>
      )}
    </Section>
  );
}

/** The props that mark a text over its limit invalid, with the error that says so; none for a text within it. */
type LengthCheck = (value: string, max: number) => { isInvalid?: true; errorMessage?: string };

type ProfileFieldsProps = {
  fields: ReturnType<typeof useProfileForm>["fields"];
  lengthCheck: LengthCheck;
};

/** The metadata fields: up to four label and content pairs, each its own group. */
function ProfileFields({ fields, lengthCheck }: ProfileFieldsProps) {
  const t = useT();
  return (
    <Section
      title={t("client:settings.profile.fields")}
      description={t("client:settings.profile.fields.description", { max: String(profileLimits.fields) })}
    >
      {fields.rows.map((field, index) => (
        <FieldRow
          key={field.key}
          field={field}
          number={index + 1}
          onChange={(change) => {
            fields.update(field.key, change);
          }}
          onRemove={() => {
            fields.remove(field.key);
          }}
          lengthCheck={lengthCheck}
        />
      ))}
      <div>
        <Button size="sm" onPress={fields.add} isDisabled={!fields.canAdd}>
          {t("client:settings.profile.fields.add")}
        </Button>
      </div>
    </Section>
  );
}

type FieldRowProps = {
  field: FieldDraft;
  number: number;
  onChange: (change: Partial<Omit<FieldDraft, "key">>) => void;
  onRemove: () => void;
  lengthCheck: LengthCheck;
};

function FieldRow({ field, number, onChange, onRemove, lengthCheck }: FieldRowProps) {
  const t = useT();
  const shortcodes = useCustomEmojiShortcodes();
  return (
    <Stack as="fieldset" shrink gap="2" border="md" padding="3">
      <Text as="legend" inset size="sm" weight="medium" tone="fg">
        {t("client:settings.profile.field", { number: String(number) })}
      </Text>
      <TextField
        label={t("client:settings.profile.field.name")}
        value={field.name}
        onChange={(name) => {
          onChange({ name });
        }}
        {...lengthCheck(field.name, profileLimits.fieldName)}
        // A missing label is reported when the form is submitted, as the content is often typed first.
        validate={(name) =>
          name === "" && field.value !== "" ? t("client:settings.profile.field.name_required") : null
        }
      />
      <CompletionTextField
        completes="all"
        label={t("client:settings.profile.field.value")}
        value={field.value}
        onChange={(value) => {
          onChange({ value });
        }}
        {...lengthCheck(field.value, profileLimits.fieldValue)}
        highlights={textHighlights(field.value, { tokens: "all", shortcodes, limit: profileLimits.fieldValue })}
      />
      <div>
        <Button size="sm" variant="ghost" onPress={onRemove}>
          {t("client:settings.profile.field.remove", { number: String(number) })}
        </Button>
      </div>
    </Stack>
  );
}
