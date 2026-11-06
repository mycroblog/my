// SPDX-License-Identifier: AGPL-3.0-only
// The form of a new invite (FR-001, manage_invites), the page /moderation/invites/new below its heading: how many
// people may sign up with it and how long it works. The new invite's code and sign-up link stay under the form in a
// StatusMessage, to copy and send; another invite replaces them. While the registration mode is `closed` (a server
// setting, /admin/settings) the page says that nobody can sign up with an invite.
import { Box, Button, DurationSelect, InlineError, Select, SelectItem, Stack, StatusMessage, Text } from "@my/ui";
import { useT } from "../../../lib/i18n";
import { inviteExpiries, inviteMaxUses, useInviteForm } from "../hooks/useInvites";
import { InviteModeNotice } from "./Invites";
import { RequirePermission, Section } from "./parts";

/** The Select's key of "no limit", which is not a number of uses. */
const unlimited = "unlimited";

export function NewInvite() {
  const t = useT();
  return (
    <RequirePermission permission="manage_invites">
      <Stack gap="8">
        <InviteModeNotice />
        <Section surface="card" title={t("client:admin.invites.new.form")}>
          <NewInviteForm />
        </Section>
      </Stack>
    </RequirePermission>
  );
}

function NewInviteForm() {
  const t = useT();
  const form = useInviteForm();
  const formatExpiry = (seconds: number) => {
    if (seconds % 86_400 === 0) return t("client:admin.settings.duration.days", { count: seconds / 86_400 });
    if (seconds % 3600 === 0) return t("client:admin.settings.duration.hours", { count: seconds / 3600 });
    return t("client:admin.invites.duration.minutes", { count: seconds / 60 });
  };
  return (
    <Stack as="form" onSubmit={form.submit} gap="6">
      <Select
        label={t("client:admin.invites.max_uses")}
        description={t("client:admin.invites.max_uses.description")}
        value={form.draft.maxUses === null ? unlimited : String(form.draft.maxUses)}
        onChange={(key) => {
          if (key === unlimited) form.change({ maxUses: null });
          else if (typeof key === "string") form.change({ maxUses: Number(key) });
        }}
        items={[
          ...inviteMaxUses.map((uses) => ({ id: String(uses), name: t("client:admin.invites.uses", { count: uses }) })),
          { id: unlimited, name: t("client:admin.invites.unlimited") },
        ]}
      >
        {(item) => (
          <SelectItem id={item.id} textValue={item.name}>
            {item.name}
          </SelectItem>
        )}
      </Select>
      <DurationSelect
        label={t("client:admin.invites.expires_in")}
        durations={inviteExpiries}
        formatDuration={formatExpiry}
        indefiniteLabel={t("client:admin.invites.never")}
        value={form.draft.expiresIn}
        onChange={(expiresIn) => {
          form.change({ expiresIn });
        }}
      />
      {form.error !== undefined && (
        <InlineError alert size="inherit">
          {t("client:admin.invites.create_failed", { reason: form.error })}
        </InlineError>
      )}
      <Box>
        <Button type="submit" variant="primary" isPending={form.isPending}>
          {t("client:admin.invites.create")}
        </Button>
      </Box>
      <StatusMessage>{form.status}</StatusMessage>
      {form.created !== undefined && (
        <Stack as="p" gap="1">
          <span>{t("client:admin.invites.link")}</span>
          <Text dir="ltr" mono wrap="anywhere" selectAll>
            {form.created.url}
          </Text>
        </Stack>
      )}
    </Stack>
  );
}
