// SPDX-License-Identifier: AGPL-3.0-only
// One custom emoji's page (/admin/custom-emojis/{id}; ADR-0051, manage_custom_emojis;
// web-client/screens/admin-custom-emoji.md), in titled regions: the emoji (its image, :shortcode: and facts), and what
// can be done to it. A local emoji's settings (category, other names, whether the picker shows it) are saved together
// with "Save", the page's one primary action; its image is replaced by choosing another file; enabling or disabling
// is one press; deleting asks first in an alert dialog (it cannot be undone) and goes back to the list. A remote emoji
// is enabled or disabled, or copied into one of this server under a chosen shortcode, its page's primary action, which
// opens the copy's page. What a change that keeps the page did shows next to its control.
import {
  Button,
  Cluster,
  ConfirmDialog,
  DescriptionItem,
  DescriptionList,
  DialogTrigger,
  EmojiTile,
  FileTrigger,
  FormActionBar,
  Hint,
  InlineError,
  Stack,
  StatusMessage,
  Text,
  TextField,
  Checkbox,
} from "@my/ui";
import { useState } from "react";
import { useT } from "../../../lib/i18n";
import { DateTime, DomainName, RequirePermission, Section } from "../../moderation";
import {
  acceptedEmojiTypes,
  type AdminCustomEmoji,
  categoryLimit,
  maxAliases,
  shortcodePattern,
} from "../api/customEmojis";
import {
  emojiInText,
  parseAliases,
  useAdminCustomEmoji,
  useCopyCustomEmoji,
  useCustomEmojiEnabling,
  useCustomEmojiImageReplacement,
  useCustomEmojiSettingsForm,
  useDeleteCustomEmoji,
} from "../hooks/useCustomEmojis";
import { EmojiState, EnableButton } from "./CustomEmojiList";
import { AccountLink } from "./CustomEmojiRequests";

export function CustomEmojiDetail({ id }: { id: string }) {
  return (
    <RequirePermission permission="manage_custom_emojis">
      <EmojiPage id={id} />
    </RequirePermission>
  );
}

function EmojiPage({ id }: { id: string }) {
  const emoji = useAdminCustomEmoji(id);
  return (
    <Stack gap="8">
      <AboutEmoji emoji={emoji} />
      {emoji.local ? (
        <>
          <SettingsSection emoji={emoji} />
          <ImageSection emoji={emoji} />
          <StateSection emoji={emoji} />
          <DeleteSection emoji={emoji} />
        </>
      ) : (
        <>
          <CopySection emoji={emoji} />
          <StateSection emoji={emoji} />
        </>
      )}
    </Stack>
  );
}

/** A value of an emoji that is not set. */
function None() {
  const t = useT();
  return <Text tone="muted">{t("client:admin.custom_emojis.none")}</Text>;
}

/** The emoji itself: its image and :shortcode:, and its facts. */
function AboutEmoji({ emoji }: { emoji: AdminCustomEmoji }) {
  const t = useT();
  const shortcode = `:${emoji.shortcode}:`;
  return (
    <Section surface="card" title={t("client:admin.custom_emojis.detail.about")}>
      <Stack gap="4">
        <Cluster gap="3" align="center" wrap={false}>
          <EmojiTile
            emoji={
              emoji.url !== undefined && emoji.static_url !== undefined
                ? { shortcode: emoji.shortcode, url: emoji.url, static_url: emoji.static_url }
                : undefined
            }
          />
          <Stack gap="1" shrink>
            <Text size="lg" weight="semibold" mono wrap="anywhere" dir="ltr">
              {shortcode}
            </Text>
            {emoji.url === undefined && (
              <Text size="sm" tone="muted">
                {t("client:admin.custom_emojis.no_image")}
              </Text>
            )}
          </Stack>
        </Cluster>
        <DescriptionList columns={2}>
          {!emoji.local && (
            <DescriptionItem term={t("client:admin.custom_emojis.domain")}>
              {emoji.domain !== undefined && <DomainName domain={emoji.domain} />}
            </DescriptionItem>
          )}
          <DescriptionItem term={t("client:admin.custom_emojis.state")}>
            <EmojiState emoji={emoji} />
          </DescriptionItem>
          {emoji.local && (
            <>
              <DescriptionItem term={t("client:admin.custom_emojis.picker")}>
                {t(
                  emoji.visible_in_picker
                    ? "client:admin.custom_emojis.picker.shown"
                    : "client:admin.custom_emojis.picker.hidden",
                )}
              </DescriptionItem>
              <DescriptionItem term={t("client:admin.custom_emojis.category")}>
                {emoji.category === undefined ? <None /> : <span dir="auto">{emoji.category}</span>}
              </DescriptionItem>
              <DescriptionItem term={t("client:admin.custom_emojis.aliases")}>
                {emoji.aliases.length === 0 ? <None /> : <span dir="ltr">{emoji.aliases.join(", ")}</span>}
              </DescriptionItem>
              <DescriptionItem term={t("client:admin.custom_emojis.created_by")}>
                {emoji.created_by !== undefined ? (
                  <AccountLink acct={emoji.created_by.acct} />
                ) : emoji.created_by_deleted ? (
                  t("client:admin.custom_emojis.created_by.deleted")
                ) : (
                  t("client:admin.custom_emojis.created_by.none")
                )}
              </DescriptionItem>
            </>
          )}
          <DescriptionItem term={t("client:admin.custom_emojis.created_at")}>
            <DateTime value={emoji.created_at} />
          </DescriptionItem>
          <DescriptionItem term={t("client:admin.custom_emojis.updated_at")}>
            <DateTime value={emoji.updated_at} />
          </DescriptionItem>
        </DescriptionList>
      </Stack>
    </Section>
  );
}

/** The settings of a local emoji, saved together: the page's one primary action. */
function SettingsSection({ emoji }: { emoji: AdminCustomEmoji }) {
  const t = useT();
  const form = useCustomEmojiSettingsForm(emoji);
  const aliasCount = parseAliases(form.aliases).length;
  let result: { tone: "success" | "danger"; message: string } | undefined;
  if (form.result.error !== undefined)
    result = {
      tone: "danger",
      message: t("client:admin.custom_emojis.change_failed", { reason: form.result.error }),
    };
  else if (form.result.status !== undefined) result = { tone: "success", message: form.result.status };
  return (
    <Section surface="card" title={t("client:admin.custom_emojis.detail.settings")}>
      <Stack as="form" gap="6" onSubmit={form.submit}>
        <TextField
          label={t("client:admin.custom_emojis.category")}
          description={t("client:admin.custom_emojis.category.description", {
            max: String(categoryLimit),
          })}
          value={form.category}
          onChange={(category) => {
            form.change({ category });
          }}
          maxLength={categoryLimit}
          autoComplete="off"
        />
        <TextField
          label={t("client:admin.custom_emojis.aliases")}
          description={t("client:admin.custom_emojis.aliases.description", {
            max: String(maxAliases),
          })}
          value={form.aliases}
          onChange={(aliases) => {
            form.change({ aliases });
          }}
          {...(aliasCount > maxAliases && {
            isInvalid: true,
            errorMessage: t("client:admin.custom_emojis.aliases.too_many", {
              max: String(maxAliases),
            }),
          })}
          autoComplete="off"
          spellCheck="false"
        />
        <Checkbox
          isSelected={form.visibleInPicker}
          onChange={(visibleInPicker) => {
            form.change({ visibleInPicker });
          }}
          description={t("client:admin.custom_emojis.visible_in_picker.description")}
        >
          {t("client:admin.custom_emojis.visible_in_picker")}
        </Checkbox>
        <FormActionBar result={result}>
          <Button type="submit" variant="primary" isPending={form.isPending}>
            {t("client:admin.custom_emojis.save")}
          </Button>
        </FormActionBar>
      </Stack>
    </Section>
  );
}

/** Replacing a local emoji's image: choosing a file sends it at once. */
function ImageSection({ emoji }: { emoji: AdminCustomEmoji }) {
  const t = useT();
  const replacement = useCustomEmojiImageReplacement(emoji);
  return (
    <Section
      surface="card"
      title={t("client:admin.custom_emojis.detail.image")}
      description={t("client:admin.custom_emojis.image.description")}
    >
      <Stack gap="3">
        <div>
          <FileTrigger
            acceptedFileTypes={acceptedEmojiTypes}
            onSelect={(files) => {
              const file = files?.[0];
              if (file !== undefined) replacement.replace(file);
            }}
          >
            <Button isPending={replacement.isPending}>{t("client:admin.custom_emojis.replace_image")}</Button>
          </FileTrigger>
        </div>
        <StatusMessage>{replacement.status}</StatusMessage>
        {replacement.error !== undefined && (
          <InlineError size="inherit" alert>
            {t("client:admin.custom_emojis.change_failed", { reason: replacement.error })}
          </InlineError>
        )}
      </Stack>
    </Section>
  );
}

/** Enabling or disabling the emoji at once; a disabled emoji is shown as text everywhere. */
function StateSection({ emoji }: { emoji: AdminCustomEmoji }) {
  const t = useT();
  const enabling = useCustomEmojiEnabling();
  return (
    <Section
      surface="card"
      title={t("client:admin.custom_emojis.detail.state")}
      description={t("client:admin.custom_emojis.detail.state.description")}
    >
      <Stack gap="3">
        <div>
          <EnableButton emoji={emoji} enabling={enabling} size="md" />
        </div>
        <StatusMessage>{enabling.status}</StatusMessage>
        {enabling.error !== undefined && (
          <InlineError size="inherit" alert>
            {t("client:admin.custom_emojis.change_failed", { reason: enabling.error })}
          </InlineError>
        )}
      </Stack>
    </Section>
  );
}

/** Copying a remote emoji into one of this server, under a shortcode that may be chosen anew. */
function CopySection({ emoji }: { emoji: AdminCustomEmoji }) {
  const t = useT();
  const copying = useCopyCustomEmoji(emoji);
  // A remote shortcode that a local one may not have (longer, or with other characters) is to be chosen anew.
  const [shortcode, setShortcode] = useState(() =>
    new RegExp(`^${shortcodePattern}$`, "u").test(emoji.shortcode) ? emoji.shortcode : "",
  );
  return (
    <Section
      surface="card"
      title={t("client:admin.custom_emojis.detail.copy")}
      description={t("client:admin.custom_emojis.copy.description")}
    >
      <Stack
        as="form"
        gap="6"
        onSubmit={(event) => {
          copying.copy(event, shortcode.trim());
        }}
      >
        <TextField
          label={t("client:admin.custom_emojis.shortcode")}
          description={t("client:admin.custom_emojis.shortcode.description")}
          value={shortcode}
          onChange={setShortcode}
          isRequired
          pattern={shortcodePattern}
          maxLength={30}
          autoComplete="off"
          spellCheck="false"
          errorMessage={t("client:admin.custom_emojis.shortcode.invalid")}
        />
        <FormActionBar
          result={
            copying.error === undefined
              ? undefined
              : {
                  tone: "danger",
                  message: t("client:admin.custom_emojis.change_failed", { reason: copying.error }),
                }
          }
        >
          <Button type="submit" variant="primary" isPending={copying.isPending}>
            {t("client:admin.custom_emojis.copy.submit")}
          </Button>
        </FormActionBar>
      </Stack>
    </Section>
  );
}

/** Deleting a local emoji, after an alert dialog: it cannot be undone. */
function DeleteSection({ emoji }: { emoji: AdminCustomEmoji }) {
  const t = useT();
  const name = emojiInText(emoji);
  const deletion = useDeleteCustomEmoji(emoji);
  const [isDeleting, setIsDeleting] = useState(false);
  const [failed, setFailed] = useState(false);
  return (
    <Section surface="card" title={t("client:admin.custom_emojis.detail.delete")}>
      <Stack gap="3">
        <Hint size="inherit">{t("client:admin.custom_emojis.delete.description", { emoji: name })}</Hint>
        <div>
          <DialogTrigger>
            <Button variant="danger" aria-label={t("client:admin.custom_emojis.delete_emoji", { emoji: name })}>
              {t("client:admin.custom_emojis.delete")}
            </Button>
            <ConfirmDialog
              title={t("client:admin.custom_emojis.delete.title", { emoji: name })}
              description={t("client:admin.custom_emojis.delete.description", { emoji: name })}
              confirmLabel={t("client:admin.custom_emojis.delete")}
              cancelLabel={t("client:admin.custom_emojis.cancel")}
              isPending={isDeleting}
              {...(failed &&
                deletion.error !== undefined && {
                  errorMessage: t("client:admin.custom_emojis.change_failed", {
                    reason: deletion.error,
                  }),
                })}
              onConfirm={(close) => {
                setIsDeleting(true);
                setFailed(false);
                void deletion.remove().then((deleted) => {
                  setIsDeleting(false);
                  if (deleted) close();
                  else setFailed(true);
                });
              }}
            />
          </DialogTrigger>
        </div>
      </Stack>
    </Section>
  );
}
