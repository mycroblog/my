// SPDX-License-Identifier: AGPL-3.0-only
// The automatic deletion of the user's own old posts (FR-118): turning it on, the age after which posts go, and the
// exceptions that keep them. The form saves the whole policy at once; the server deletes the posts in its hourly job.
import {
  Button,
  Checkbox,
  ConfirmDialog,
  Grid,
  Hint,
  InlineError,
  Select,
  SelectItem,
  Stack,
  StatusMessage,
  Switch,
  Text,
} from "@my/ui";
import { useId } from "react";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import type { Visibility } from "../api/me";
import { type AutoDeleteFlag, useAutoDeleteForm } from "../hooks/useAutoDeleteForm";
import { useUnsavedChangesGuard } from "../hooks/useUnsavedChangesGuard";

/** The exceptions that keep a post the author marked or that has something in it, in the order of the form. */
const flags: readonly { id: AutoDeleteFlag; label: ClientMessageKey }[] = [
  { id: "keep_pinned", label: "client:settings.auto_delete.keep.pinned" },
  { id: "keep_bookmarked", label: "client:settings.auto_delete.keep.bookmarked" },
  { id: "keep_favourited", label: "client:settings.auto_delete.keep.favourited" },
  { id: "keep_media", label: "client:settings.auto_delete.keep.media" },
  { id: "keep_polls", label: "client:settings.auto_delete.keep.polls" },
];

/** The visibilities in the order of ADR-0017, widest first, with their label. */
const visibilities: readonly { id: Visibility; label: ClientMessageKey }[] = [
  { id: "public", label: "client:settings.visibility.public" },
  { id: "local", label: "client:settings.visibility.local" },
  { id: "unlisted", label: "client:settings.visibility.unlisted" },
  { id: "private", label: "client:settings.visibility.private" },
  { id: "direct", label: "client:settings.visibility.direct" },
];

export function AutoDeleteSettings() {
  const t = useT();
  const form = useAutoDeleteForm();
  const keepId = useId();
  const visibilityId = useId();
  const thresholdLabel = (count: number) =>
    count === 0
      ? t("client:settings.auto_delete.threshold.off")
      : t("client:settings.auto_delete.threshold.count", { count });
  return (
    <>
      <Stack as="form" onSubmit={form.submit} gap="6">
        <Hint size="inherit">{t("client:settings.auto_delete.description")}</Hint>
        <Switch
          isSelected={form.policy.enabled}
          onChange={form.setEnabled}
          description={t("client:settings.auto_delete.enabled.description")}
        >
          {t("client:settings.auto_delete.enabled")}
        </Switch>
        <Select
          label={t("client:settings.auto_delete.age")}
          description={t("client:settings.auto_delete.age.description")}
          value={String(form.policy.min_age_days)}
          onChange={(key) => {
            if (typeof key === "string") form.setMinAgeDays(Number(key));
          }}
          items={form.ageChoices.map((days) => ({
            id: String(days),
            name: t("client:settings.auto_delete.age.days", { count: days }),
          }))}
        >
          {(choice) => (
            <SelectItem id={choice.id} textValue={choice.name}>
              {choice.name}
            </SelectItem>
          )}
        </Select>
        <Stack as="fieldset" aria-describedby={keepId} container shrink gap="2">
          <Text as="legend" size="sm" weight="medium" tone="fg">
            {t("client:settings.auto_delete.keep")}
          </Text>
          <Hint id={keepId}>{t("client:settings.auto_delete.keep.description")}</Hint>
          <Grid as="ul" gap="6" rowGap="1">
            {flags.map((flag) => (
              <li key={flag.id}>
                <Checkbox
                  isSelected={form.policy[flag.id]}
                  onChange={(selected) => {
                    form.setFlag(flag.id, selected);
                  }}
                >
                  {t(flag.label)}
                </Checkbox>
              </li>
            ))}
          </Grid>
        </Stack>
        <Stack as="fieldset" aria-describedby={visibilityId} container shrink gap="2">
          <Text as="legend" size="sm" weight="medium" tone="fg">
            {t("client:settings.auto_delete.visibilities")}
          </Text>
          <Hint id={visibilityId}>{t("client:settings.auto_delete.visibilities.description")}</Hint>
          <Grid as="ul" gap="6" rowGap="1">
            {visibilities.map((visibility) => (
              <li key={visibility.id}>
                <Checkbox
                  isSelected={form.policy.keep_visibilities.includes(visibility.id)}
                  onChange={(selected) => {
                    form.setKeepVisibility(visibility.id, selected);
                  }}
                >
                  {t(visibility.label)}
                </Checkbox>
              </li>
            ))}
          </Grid>
        </Stack>
        <Select
          label={t("client:settings.auto_delete.min_boosts")}
          value={String(form.policy.keep_min_boosts)}
          onChange={(key) => {
            if (typeof key === "string") form.setMinBoosts(Number(key));
          }}
          items={form.boostChoices.map((count) => ({ id: String(count), name: thresholdLabel(count) }))}
        >
          {(choice) => (
            <SelectItem id={choice.id} textValue={choice.name}>
              {choice.name}
            </SelectItem>
          )}
        </Select>
        <Select
          label={t("client:settings.auto_delete.min_favourites")}
          value={String(form.policy.keep_min_favourites)}
          onChange={(key) => {
            if (typeof key === "string") form.setMinFavourites(Number(key));
          }}
          items={form.favouriteChoices.map((count) => ({ id: String(count), name: thresholdLabel(count) }))}
        >
          {(choice) => (
            <SelectItem id={choice.id} textValue={choice.name}>
              {choice.name}
            </SelectItem>
          )}
        </Select>
        {form.saveError !== undefined && (
          <InlineError alert size="inherit">
            {t("client:settings.auto_delete.save_failed", {
              reason: form.saveError,
            })}
          </InlineError>
        )}
        <Stack gap="3">
          <div>
            <Button type="submit" variant="primary" isPending={form.isSaving}>
              {t("client:settings.auto_delete.save")}
            </Button>
          </div>
          <StatusMessage>{form.status}</StatusMessage>
        </Stack>
      </Stack>
      {/* Outside the form: React bubbles the dialog's own submit event through its portal to an enclosing form. */}
      <UnsavedChangesDialog hasChanges={form.hasChanges} />
    </>
  );
}

/**
 * Asks before another page replaces the form with unsaved edits (the APG alert dialog pattern): focus starts on
 * "Keep editing", which stays on the form like Esc does.
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
      title={t("client:settings.auto_delete.unsaved.title")}
      description={t("client:settings.auto_delete.unsaved.description")}
      confirmLabel={t("client:settings.auto_delete.unsaved.leave")}
      cancelLabel={t("client:settings.auto_delete.unsaved.stay")}
      onConfirm={guard.leave}
    />
  );
}
