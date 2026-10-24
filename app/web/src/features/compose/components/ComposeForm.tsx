// SPDX-License-Identifier: AGPL-3.0-only
import { Box, Button, Checkbox, Cluster, Dialog, DialogActions, Hint, InlineError, Spinner, Stack, Text } from "@my/ui";
import type { SubmitEvent } from "react";
import { useLocale, useT } from "../../../lib/i18n";
import { textHighlights } from "../../../lib/textHighlights";
import { EmojiPickerButton, useCustomEmojiShortcodes } from "../../emoji";
import { maxAttachments } from "../hooks/policy";
import { type ComposeController, composeTextId } from "../hooks/useCompose";
import { type ReadyComposeForm, useComposeForm } from "../hooks/useComposeForm";
import { useFormatRemaining } from "../hooks/useFormatRemaining";
import { AttachmentList } from "./AttachmentList";
import { CompletionTextArea, CompletionTextField } from "./CompletionFields";
import { ComposeTools } from "./ComposeTools";
import { Outbox } from "./Outbox";
import { PollEditor } from "./PollEditor";
import { ReplyContext } from "./ReplyContext";

export type ComposeFormProps = {
  controller: ComposeController;
  /** The id of the heading that names the form. */
  labelledBy: string;
  /** Focus the text area when the form appears (in the sheet, which the user just opened). */
  autoFocus?: boolean;
};

/** The new-post form; its logic is useComposeForm. */
export function ComposeForm({ controller, labelledBy, autoFocus = false }: ComposeFormProps) {
  const t = useT();
  const form = useComposeForm(controller);
  switch (form.status) {
    case "loading":
      return <Spinner label={t("client:shell.loading")} />;
    case "error":
      return (
        <Stack align="start" gap="2">
          <p>{form.message}</p>
          <Button size="sm" onPress={form.retry}>
            {t("client:shell.error.retry")}
          </Button>
        </Stack>
      );
    case "ready":
      return <ReadyForm form={form} labelledBy={labelledBy} autoFocus={autoFocus} />;
  }
}

type ReadyFormProps = { form: ReadyComposeForm; labelledBy: string; autoFocus: boolean };

function ReadyForm({ form, labelledBy, autoFocus }: ReadyFormProps) {
  const t = useT();
  const locale = useLocale();
  const formatRemaining = useFormatRemaining();
  const shortcodes = useCustomEmojiShortcodes();
  const { draft, attachments } = form;
  const onSubmit = (event: SubmitEvent<HTMLFormElement>) => {
    event.preventDefault();
    form.submit();
  };
  return (
    <Stack gap="4" container>
      <Stack as="form" aria-labelledby={labelledBy} onSubmit={onSubmit} gap="3">
        {form.replyTo.status !== "none" && <ReplyContext replyTo={form.replyTo} onCancel={form.cancelReply} />}
        {form.quoteOf.status !== "none" && <ReplyContext kind="quote" replyTo={form.quoteOf} onCancel={form.cancelQuote} />}
        {draft.hasWarning && (
          <CompletionTextField
            completes="emoji"
            label={t("client:compose.warning.label")}
            description={t("client:compose.warning.description")}
            value={draft.spoilerText}
            onChange={(spoilerText) => {
              form.edit({ spoilerText });
            }}
            highlights={textHighlights(draft.spoilerText, { tokens: "shortcodes", shortcodes })}
          />
        )}
        <CompletionTextArea
          completes="all"
          id={composeTextId}
          label={t("client:compose.text.label")}
          value={draft.text}
          onChange={(text) => {
            form.edit({ text });
          }}
          rows={4}
          characterLimit={form.textLimit}
          formatRemaining={formatRemaining}
          highlights={textHighlights(draft.text, { tokens: "all", shortcodes, limit: form.textLimit })}
          autoFocus={autoFocus}
          endAction={
            <EmojiPickerButton
              fieldId={composeTextId}
              value={draft.text}
              onChange={(text) => {
                form.edit({ text });
              }}
            />
          }
        />
        <AttachmentList
          items={attachments.items}
          onDescribe={attachments.describe}
          onRemove={attachments.remove}
          requireDescriptions={form.requireDescriptions}
          isDescriptionRequired={form.isDescriptionRequired}
        />
        {form.poll.draft !== null && <PollEditor poll={form.poll} draft={form.poll.draft} />}
        {attachments.items.length > 0 && (
          <Hint>{t("client:compose.media.hint", { max: maxAttachments })}</Hint>
        )}
        {attachments.items.length > 0 && (
          <Checkbox
            isSelected={draft.sensitive}
            onChange={(sensitive) => {
              form.edit({ sensitive });
            }}
          >
            {t("client:compose.sensitive")}
          </Checkbox>
        )}
        {form.unreachedMentions.length > 0 && (
          <Text as="p" size="sm" tone="warning">
            {t("client:compose.mention.unreached", {
              count: form.unreachedMentions.length,
              accounts: new Intl.ListFormat(locale, { type: "conjunction" }).format(form.unreachedMentions.map((acct) => `@${acct}`)),
            })}
          </Text>
        )}
        {form.staysLocal && (
          <Text as="p" size="sm" tone="warning">
            {t("client:compose.federation.stays_local")}
          </Text>
        )}
        {form.error !== null && <InlineError size="inherit">{form.error}</InlineError>}
        {/* The tools and the submit button share the row under the text, so that the button stays on the first screen at 320 px. */}
        <Cluster align="center" gap="2">
          <ComposeTools form={form} />
          <Box pushEnd>
            <Button type="submit" variant="primary" isDisabled={!form.canSubmit}>
              {t(form.replyTo.status === "none" ? "client:compose.submit" : "client:compose.submit.reply")}
            </Button>
          </Box>
        </Cluster>
      </Stack>
      <Outbox items={form.outbox} onRetry={form.retry} onEdit={form.editOutbox} canEdit={form.canEditOutbox} onDiscard={form.discard} />
      <Dialog
        role="alertdialog"
        closeLabel={null}
        title={t("client:compose.missing_description.title")}
        isOpen={form.confirmation.isOpen}
        onOpenChange={(isOpen) => {
          if (!isOpen) form.confirm("describe");
        }}
        size="sm"
      >
        <Stack gap="4">
          <p>{t("client:compose.missing_description.body", { count: form.confirmation.missing })}</p>
          <DialogActions>
            <Button
              autoFocus
              onPress={() => {
                form.confirm("describe");
              }}
            >
              {t("client:compose.missing_description.add")}
            </Button>
            <Button
              variant="primary"
              onPress={() => {
                form.confirm("send");
              }}
            >
              {t("client:compose.missing_description.post")}
            </Button>
          </DialogActions>
        </Stack>
      </Dialog>
    </Stack>
  );
}
