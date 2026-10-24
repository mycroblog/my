// SPDX-License-Identifier: AGPL-3.0-only
import {
  Avatar,
  Button,
  Checkbox,
  Cluster,
  ConfirmDialog,
  FileTrigger,
  Heading,
  Hint,
  InlineError,
  ProfileHeaderImage,
  Stack,
  Text,
  TextField,
} from "@my/ui";
import { type ReactNode, useId } from "react";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import { textHighlights } from "../../../lib/textHighlights";
import { CompletionTextArea, CompletionTextField } from "../../compose";
import { EmojiPickerButton, useCustomEmojiShortcodes } from "../../emoji";
import { acceptedImageTypes } from "../api/media";
import { characterCount, type FieldDraft, type ImageControl, profileLimits, type ServerCrawlerPolicy, useProfileForm } from "../hooks/useProfileForm";
import { useUnsavedChangesGuard } from "../hooks/useUnsavedChangesGuard";

/**
 * The profile editor (FR-006). React Aria's native validation shows each error under its field, links it with
 * aria-describedby and marks the field invalid; the browser then refuses to submit, and focus moves to the
 * first invalid field, so a screen reader reads the error with the field. A text over its limit is invalid as
 * soon as it is typed, like the count of the bio: an error that appeared only when the field lost focus would
 * push the save button away under the pointer that is pressing it.
 */
export function ProfileForm() {
  const t = useT();
  const form = useProfileForm();
  const displayNameId = useId();
  const noteId = useId();
  const shortcodes = useCustomEmojiShortcodes();
  // Only an invalid text sets isInvalid: React Aria ignores `validate` once isInvalid is given at all.
  const lengthCheck: LengthCheck = (value, max) =>
    characterCount(value) > max ? { isInvalid: true, errorMessage: t("client:settings.profile.too_long", { max: String(max) }) } : {};
  return (
    <>
      <Stack as="form" onSubmit={form.submit} gap="6">
        <ImageSection
          titleKey="client:settings.profile.avatar"
          control={form.avatar}
          preview={<Avatar src={form.avatar.url} name={form.displayName.value || form.account.username} seed={form.account.id} decorative size="xl" />}
        />
        <ImageSection
          titleKey="client:settings.profile.header"
          control={form.header}
          preview={
            form.header.url === undefined ? null : <ProfileHeaderImage sources={{ still: [{ url: form.header.url }] }} size="preview" />
          }
        />
        <Stack gap="1">
          <CompletionTextField
            completes="emoji"
            id={displayNameId}
            label={t("client:settings.profile.display_name")}
            description={t("client:settings.profile.display_name.description", { max: String(profileLimits.displayName) })}
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
        </Stack>
        <Stack gap="1">
          <CompletionTextArea
            completes="all"
            id={noteId}
            label={t("client:settings.profile.note")}
            value={form.note.value}
            onChange={form.note.onChange}
            characterLimit={profileLimits.note}
            highlights={textHighlights(form.note.value, { tokens: "all", shortcodes, limit: profileLimits.note })}
            formatRemaining={(remaining) =>
              remaining < 0 ? t("client:settings.profile.characters_over", { count: -remaining }) : t("client:settings.profile.characters_left", { count: remaining })
            }
            errorMessage={t("client:settings.profile.too_long", { max: String(profileLimits.note) })}
            rows={4}
            endAction={
              <EmojiPickerButton fieldId={noteId} value={form.note.value} onChange={form.note.onChange} label={t("client:settings.profile.note.emoji")} />
            }
          />
        </Stack>
        <ProfileFields fields={form.fields} lengthCheck={lengthCheck} />
        <Stack gap="2">
          <Checkbox {...form.locked} description={t("client:settings.profile.locked.description")}>
            {t("client:settings.profile.locked")}
          </Checkbox>
          <Checkbox {...form.bot} description={t("client:settings.profile.bot.description")}>
            {t("client:settings.profile.bot")}
          </Checkbox>
        </Stack>
        <CrawlerSettings indexable={form.indexable} aiOptOut={form.aiOptOut} serverPolicy={form.serverPolicy} />
        {form.saveError !== undefined && (
          <InlineError alert size="inherit">
            {form.saveError}
          </InlineError>
        )}
        <div>
          <Button type="submit" variant="primary" isPending={form.isSaving} isDisabled={form.isBusy && !form.isSaving}>
            {t("client:settings.profile.save")}
          </Button>
        </div>
      </Stack>
      {/* Outside the form: React bubbles the dialog's own submit event through its portal to an enclosing form. */}
      <UnsavedChangesDialog hasChanges={form.hasChanges} />
    </>
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
  const headingId = useId();
  return (
    <Stack as="section" aria-labelledby={headingId} gap="2">
      <Heading level={2} id={headingId} size="label" tone="fg">
        {t("client:settings.profile.crawlers")}
      </Heading>
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
    </Stack>
  );
}

type ImageSectionProps = { titleKey: ClientMessageKey; control: ImageControl; preview: ReactNode };

/** An image of the profile: its preview, and buttons to choose a new file or remove it. */
function ImageSection({ titleKey, control, preview }: ImageSectionProps) {
  const t = useT();
  const headingId = useId();
  const errorId = useId();
  return (
    <Stack as="section" aria-labelledby={headingId} aria-busy={control.isUploading} gap="2">
      <Heading level={2} id={headingId} size="label" tone="fg">
        {t(titleKey)}
      </Heading>
      {preview}
      <Cluster gap="2">
        <FileTrigger
          acceptedFileTypes={acceptedImageTypes}
          onSelect={(files) => {
            const file = files?.[0];
            if (file !== undefined) control.choose(file);
          }}
        >
          <Button size="sm" isPending={control.isUploading} aria-describedby={control.error === undefined ? headingId : `${headingId} ${errorId}`}>
            {t("client:settings.profile.image.choose")}
          </Button>
        </FileTrigger>
        {control.hasImage && (
          <Button size="sm" variant="ghost" onPress={control.remove} isDisabled={control.isUploading} aria-describedby={headingId}>
            {t("client:settings.profile.image.remove")}
          </Button>
        )}
      </Cluster>
      {control.error !== undefined && (
        <InlineError id={errorId} alert>
          {control.error}
        </InlineError>
      )}
    </Stack>
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
  const headingId = useId();
  return (
    <Stack as="section" aria-labelledby={headingId} gap="3">
      <Heading level={2} id={headingId} size="label" tone="fg">
        {t("client:settings.profile.fields")}
      </Heading>
      <Hint>
        {t("client:settings.profile.fields.description", {
          max: String(profileLimits.fields),
        })}
      </Hint>
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
    </Stack>
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
        validate={(name) => (name === "" && field.value !== "" ? t("client:settings.profile.field.name_required") : null)}
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
