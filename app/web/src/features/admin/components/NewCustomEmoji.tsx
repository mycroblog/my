// SPDX-License-Identifier: AGPL-3.0-only
// The page that registers a local custom emoji (/admin/custom-emojis/new; ADR-0051, manage_custom_emojis;
// web-client/screens/admin-custom-emojis-new.md): the image file, the shortcode, the category, the other names and
// whether the picker shows it, sent at once with "Register" (FormActionBar). The image is uploaded first (POST /media),
// then the emoji is registered; once it is, the new emoji's page opens, and its heading shows what was done. A refusal
// stays in the action bar with the draft.
import { Button, Card, Checkbox, FileTrigger, FormActionBar, Hint, InlineError, Stack, Text, TextField } from "@my/ui";
import { useId } from "react";
import { useT } from "../../../lib/i18n";
import { RequirePermission } from "../../moderation";
import {
  acceptedEmojiTypes,
  categoryLimit,
  federatedEmojiBytes,
  maxAliases,
  shortcodePattern,
} from "../api/customEmojis";
import { parseAliases, useCustomEmojiForm } from "../hooks/useCustomEmojis";

export function NewCustomEmoji() {
  return (
    <RequirePermission permission="manage_custom_emojis">
      <Card>
        <NewCustomEmojiForm />
      </Card>
    </RequirePermission>
  );
}

/** The form that registers a local emoji; once registered, the emoji's page opens. */
function NewCustomEmojiForm() {
  const t = useT();
  const form = useCustomEmojiForm();
  const imageId = useId();
  const imageErrorId = useId();
  const { draft } = form;
  const aliasCount = parseAliases(draft.aliases).length;
  return (
    <Stack as="form" gap="6" onSubmit={form.submit}>
      <Stack gap="2" role="group" aria-labelledby={imageId}>
        <Text size="sm" tone="fg" weight="medium" id={imageId}>
          {t("client:admin.custom_emojis.image")}
        </Text>
        <Hint>{t("client:admin.custom_emojis.image.description")}</Hint>
        <div>
          <FileTrigger
            acceptedFileTypes={acceptedEmojiTypes}
            onSelect={(files) => {
              const file = files?.[0];
              if (file !== undefined) form.change({ file });
            }}
          >
            <Button size="sm" aria-describedby={form.missingImage ? `${imageId} ${imageErrorId}` : imageId}>
              {t("client:admin.custom_emojis.image.choose")}
            </Button>
          </FileTrigger>
        </div>
        {draft.file !== null && (
          <Text as="p" size="sm" wrap="anywhere">
            {t("client:admin.custom_emojis.image.chosen", { name: draft.file.name })}
          </Text>
        )}
        {draft.file !== null && draft.file.size > federatedEmojiBytes && (
          <Text as="p" size="sm" tone="warning">
            {t("client:admin.custom_emojis.image.large_hint")}
          </Text>
        )}
        {form.missingImage && (
          <InlineError alert id={imageErrorId}>
            {t("client:admin.custom_emojis.image.required")}
          </InlineError>
        )}
      </Stack>
      <TextField
        label={t("client:admin.custom_emojis.shortcode")}
        description={t("client:admin.custom_emojis.shortcode.description")}
        value={draft.shortcode}
        onChange={(shortcode) => {
          form.change({ shortcode });
        }}
        isRequired
        pattern={shortcodePattern}
        maxLength={30}
        autoComplete="off"
        spellCheck="false"
        errorMessage={t("client:admin.custom_emojis.shortcode.invalid")}
      />
      <TextField
        label={t("client:admin.custom_emojis.category")}
        description={t("client:admin.custom_emojis.category.description", { max: String(categoryLimit) })}
        value={draft.category}
        onChange={(category) => {
          form.change({ category });
        }}
        maxLength={categoryLimit}
        autoComplete="off"
      />
      <TextField
        label={t("client:admin.custom_emojis.aliases")}
        description={t("client:admin.custom_emojis.aliases.description", { max: String(maxAliases) })}
        value={draft.aliases}
        onChange={(aliases) => {
          form.change({ aliases });
        }}
        {...(aliasCount > maxAliases && {
          isInvalid: true,
          errorMessage: t("client:admin.custom_emojis.aliases.too_many", { max: String(maxAliases) }),
        })}
        autoComplete="off"
        spellCheck="false"
      />
      <Checkbox
        isSelected={draft.visibleInPicker}
        onChange={(visibleInPicker) => {
          form.change({ visibleInPicker });
        }}
        description={t("client:admin.custom_emojis.visible_in_picker.description")}
      >
        {t("client:admin.custom_emojis.visible_in_picker")}
      </Checkbox>
      <FormActionBar
        result={
          form.error === undefined
            ? undefined
            : { tone: "danger", message: t("client:admin.custom_emojis.create_failed", { reason: form.error }) }
        }
      >
        <Button type="submit" variant="primary" isPending={form.isPending}>
          {t("client:admin.custom_emojis.submit")}
        </Button>
      </FormActionBar>
    </Stack>
  );
}
