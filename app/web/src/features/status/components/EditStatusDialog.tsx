// SPDX-License-Identifier: AGPL-3.0-only
// The dialog that edits the viewer's own post (FR-109), opened from the post's "More actions" menu. It is a
// modal dialog (WAI-ARIA APG, dialog pattern) holding the compose form's fields for what an edit may change:
// the content warning, the text with its counter and the emoji picker in its corner, the images' descriptions, the
// sensitivity of the media, and the language. The visibility and the reply target stay as they were.
import {
  Button,
  Checkbox,
  Cluster,
  Dialog,
  Hint,
  IconToggleButton,
  InlineError,
  LanguageIcon,
  RegionError,
  Select,
  SelectItem,
  Spinner,
  Stack,
  WarningIcon,
} from "@my/ui";
import { type SubmitEvent, useId } from "react";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import { textHighlights } from "../../../lib/textHighlights";
import { AttachmentList, CompletionTextArea, CompletionTextField, useFormatRemaining } from "../../compose";
import { EmojiPickerButton, useCustomEmojiShortcodes } from "../../emoji";
import type { Status, StatusSource } from "../api/status";
import { type EditBlocker, useEditSource, useEditStatusForm } from "../hooks/useEditStatus";

export type EditStatusDialogProps = {
  /** The viewer's post. */
  status: Status;
  isOpen: boolean;
  onOpenChange: (isOpen: boolean) => void;
};

/** The edit dialog. Its content mounts when it opens, so every opening starts from the post as it is now. */
export function EditStatusDialog({ status, isOpen, onOpenChange }: EditStatusDialogProps) {
  const t = useT();
  return (
    <Dialog
      title={t("client:status.edit.title")}
      closeLabel={t("client:status.edit.close")}
      isDismissable={false}
      isOpen={isOpen}
      onOpenChange={onOpenChange}
      size="lg"
    >
      {({ close }) => <EditStatusContent status={status} close={close} />}
    </Dialog>
  );
}

type EditStatusContentProps = { status: Status; close: () => void };

function EditStatusContent({ status, close }: EditStatusContentProps) {
  const t = useT();
  const source = useEditSource(status.id);
  switch (source.status) {
    case "loading":
      return <Spinner label={t("client:status.edit.loading")} />;
    case "error":
      return (
        <RegionError
          heading={t("client:status.edit.load_failed")}
          reason={source.message}
          retryLabel={t("client:status.edit.retry")}
          onRetry={source.retry}
        />
      );
    case "ready":
      return (
        <EditStatusForm
          status={status}
          source={source.source}
          maxCharacters={source.maxCharacters}
          isDescriptionRequired={source.isDescriptionRequired}
          close={close}
        />
      );
  }
}

const blockerMessages: Readonly<Record<EditBlocker, ClientMessageKey>> = {
  empty: "client:status.edit.blocked.empty",
  too_long: "client:status.edit.blocked.too_long",
};

type EditStatusFormProps = {
  status: Status;
  source: StatusSource;
  maxCharacters: number;
  /** Whether the server requires a description of every image (`limits.require_media_description`). */
  isDescriptionRequired: boolean;
  close: () => void;
};

function EditStatusForm({ status, source, maxCharacters, isDescriptionRequired, close }: EditStatusFormProps) {
  const t = useT();
  const formatRemaining = useFormatRemaining();
  const errorId = useId();
  const blockedId = useId();
  const textId = useId();
  const form = useEditStatusForm(status, source, maxCharacters);
  const shortcodes = useCustomEmojiShortcodes();
  const warningLabel = t("client:compose.warning.toggle");
  const describedBy = [form.error !== null && errorId, form.blocked !== null && blockedId].filter(Boolean).join(" ");
  const onSubmit = (event: SubmitEvent<HTMLFormElement>) => {
    event.preventDefault();
    form.save(close, textId);
  };
  return (
    <Stack as="form" aria-label={t("client:status.edit.title")} onSubmit={onSubmit} gap="3" container>
      {form.hasWarning && (
        <CompletionTextField
          completes="emoji"
          label={t("client:compose.warning.label")}
          description={t("client:compose.warning.description")}
          value={form.spoilerText}
          onChange={(spoilerText) => {
            form.edit({ spoilerText });
          }}
          highlights={textHighlights(form.spoilerText, { tokens: "shortcodes", shortcodes })}
        />
      )}
      <CompletionTextArea
        completes="all"
        id={textId}
        label={t("client:compose.text.label")}
        value={form.text}
        onChange={(text) => {
          form.edit({ text });
        }}
        rows={4}
        characterLimit={form.textLimit}
        formatRemaining={formatRemaining}
        highlights={textHighlights(form.text, { tokens: "all", shortcodes, limit: form.textLimit })}
        autoFocus
        endAction={
          <EmojiPickerButton
            fieldId={textId}
            value={form.text}
            onChange={(text) => {
              form.edit({ text });
            }}
          />
        }
      />
      <AttachmentList
        items={form.attachments}
        onDescribe={form.describe}
        requireDescriptions={false}
        isDescriptionRequired={isDescriptionRequired}
      />
      {form.attachments.length > 0 && (
        <Checkbox
          isSelected={form.sensitive}
          onChange={(sensitive) => {
            form.edit({ sensitive });
          }}
        >
          {t("client:compose.sensitive")}
        </Checkbox>
      )}
      {form.hasQuote && (
        <Checkbox
          isSelected={form.removeQuote}
          onChange={(removeQuote) => {
            form.edit({ removeQuote });
          }}
          description={t("client:status.edit.remove_quote.description")}
        >
          {t("client:status.edit.remove_quote")}
        </Checkbox>
      )}
      <Hint>{t("client:status.edit.fixed")}</Hint>
      {form.error !== null && (
        <InlineError id={errorId} size="inherit" alert>
          {form.error}
        </InlineError>
      )}
      {/* The save button is never disabled by the text (G23); pressed while the text cannot be saved, it says why here,
          until the reason is gone. */}
      {form.blocked !== null && (
        <InlineError id={blockedId} size="inherit" alert>
          {t(blockerMessages[form.blocked])}
        </InlineError>
      )}
      {/* The same row as the compose form's tools: the content warning and the language, then the buttons. */}
      <Cluster align="center" gap="2">
        <Cluster align="center" gap="1" shrink>
          <IconToggleButton
            label={warningLabel}
            isSelected={form.hasWarning}
            onChange={(hasWarning) => {
              form.edit({ hasWarning });
            }}
          >
            <WarningIcon filled={form.hasWarning} />
          </IconToggleButton>
          <Select
            variant="toolbar"
            aria-label={t("client:compose.language.label")}
            tooltip={t("client:compose.language.tooltip", {
              value: form.languages.find((option) => option.id === form.language)?.name ?? form.language,
            })}
            icon={<LanguageIcon />}
            items={form.languages}
            value={form.language}
            onChange={(language) => {
              if (typeof language === "string") form.edit({ language });
            }}
          >
            {(option) => (
              <SelectItem id={option.id} textValue={option.name}>
                {option.name}
              </SelectItem>
            )}
          </Select>
        </Cluster>
        <Cluster gap="2" pushEnd>
          <Button onPress={close}>{t("client:status.edit.cancel")}</Button>
          <Button
            type="submit"
            variant="primary"
            isPending={form.isSaving}
            {...(describedBy !== "" && { "aria-describedby": describedBy })}
          >
            {t("client:status.edit.save")}
          </Button>
        </Cluster>
      </Cluster>
    </Stack>
  );
}
