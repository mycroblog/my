// SPDX-License-Identifier: AGPL-3.0-only
// The welcome step after sign-up (web-client/screens/welcome.md): the basics of the profile, the avatar and the
// display name, in one section, saved together by "Continue", which then opens the home timeline; "Later" goes there
// without saving. The rest of the profile stays on its settings screen.
import {
  Avatar,
  Button,
  Cluster,
  FileTrigger,
  FormActionBar,
  InlineError,
  Link,
  Section,
  Stack,
  TextField,
} from "@my/ui";
import { useId } from "react";
import { useT } from "../../../lib/i18n";
import { acceptedImageTypes } from "../api/media";
import { profileLimits } from "../hooks/useProfileForm";
import { useWelcomeForm, type WelcomeFormOptions } from "../hooks/useWelcomeForm";

export type WelcomeFormProps = WelcomeFormOptions & {
  /** Where "Later" goes: the home timeline. */
  laterHref: string;
};

export function WelcomeForm({ onDone, laterHref }: WelcomeFormProps) {
  const t = useT();
  const form = useWelcomeForm({ onDone });
  const errorId = useId();
  return (
    <Stack as="form" onSubmit={form.submit} gap="6">
      <Section
        title={t("client:welcome.basics")}
        description={t("client:welcome.basics.description")}
        busy={form.avatar.isUploading}
      >
        <Stack gap="4">
          <Cluster gap="4" align="center">
            <Avatar
              src={form.avatar.url}
              name={form.displayName.value || form.account.username}
              seed={form.account.id}
              decorative
              size="xl"
            />
            <Cluster gap="2">
              <FileTrigger
                acceptedFileTypes={acceptedImageTypes}
                onSelect={(files) => {
                  const file = files?.[0];
                  if (file !== undefined) form.avatar.choose(file);
                }}
              >
                <Button
                  size="sm"
                  isPending={form.avatar.isUploading}
                  {...(form.avatar.error === undefined ? {} : { "aria-describedby": errorId })}
                >
                  {t("client:welcome.avatar.choose")}
                </Button>
              </FileTrigger>
              {form.avatar.hasImage && (
                <Button size="sm" variant="ghost" onPress={form.avatar.remove} isDisabled={form.avatar.isUploading}>
                  {t("client:welcome.avatar.remove")}
                </Button>
              )}
            </Cluster>
          </Cluster>
          {form.avatar.error !== undefined && (
            <InlineError id={errorId} alert>
              {form.avatar.error}
            </InlineError>
          )}
          <TextField
            label={t("client:welcome.display_name")}
            description={t("client:welcome.display_name.description", { username: form.account.username })}
            value={form.displayName.value}
            onChange={form.displayName.onChange}
            maxLength={profileLimits.displayName}
            autoComplete="nickname"
          />
        </Stack>
      </Section>
      <FormActionBar result={form.error === undefined ? undefined : { tone: "danger", message: form.error }}>
        <Button type="submit" variant="primary" isPending={form.isSaving} isDisabled={form.avatar.isUploading}>
          {t("client:welcome.continue")}
        </Button>
        <Link href={laterHref} variant="standalone">
          {t("client:welcome.later")}
        </Link>
      </FormActionBar>
    </Stack>
  );
}
