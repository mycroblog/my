// SPDX-License-Identifier: AGPL-3.0-only
// The page that writes an announcement (/admin/announcements/new, manage_announcements, FR-726,
// web-client/screens/admin-announcements-new.md): the title, the Markdown text, how long it shows from when it is
// sent (or until it is withdrawn), and whether it is published at once or kept as a draft. Writing it opens the
// announcement's page; a failure stays above the button (role="alert", web-client/README.md E2) with the text kept.
import { Button, Card, Checkbox, DurationSelect, Hint, InlineError, Stack } from "@my/ui";
import { useNavigate } from "@tanstack/react-router";
import { useT } from "../../../lib/i18n";
import { RequirePermission } from "../../moderation";
import { announcementDurations, useNewAnnouncementForm } from "../hooks/useAnnouncements";
import { AnnouncementTextField, AnnouncementTitleField } from "./AnnouncementFields";

export function NewAnnouncement() {
  return (
    <RequirePermission permission="manage_announcements">
      <NewAnnouncementForm />
    </RequirePermission>
  );
}

function NewAnnouncementForm() {
  const t = useT();
  const navigate = useNavigate();
  const form = useNewAnnouncementForm((announcement) => {
    void navigate({ to: "/admin/announcements/$id", params: { id: announcement.id } });
  });
  return (
    <Card>
      <Stack as="form" gap="6" onSubmit={form.submit}>
        <Hint size="inherit">{t("client:admin.announcements.description")}</Hint>
        <AnnouncementTitleField value={form.title} onChange={form.changeTitle} error={form.titleError} />
        <AnnouncementTextField value={form.text} onChange={form.changeText} error={form.textError} rows={6} />
        <DurationSelect
          label={t("client:admin.announcements.duration")}
          description={t("client:admin.announcements.duration.description")}
          durations={announcementDurations}
          formatDuration={(seconds) => t("client:admin.settings.duration.days", { count: seconds / 86_400 })}
          indefiniteLabel={t("client:admin.announcements.until_withdrawn")}
          value={form.duration}
          onChange={form.changeDuration}
        />
        <Checkbox
          isSelected={form.publish}
          onChange={form.changePublish}
          description={t("client:admin.announcements.publish_now.description")}
        >
          {t("client:admin.announcements.publish_now")}
        </Checkbox>
        {form.failure !== undefined && (
          <InlineError size="inherit" alert>
            {t("client:admin.announcements.create_failed", { reason: form.failure })}
          </InlineError>
        )}
        <div>
          <Button type="submit" variant="primary" isPending={form.isPending}>
            {t(
              form.publish ? "client:admin.announcements.create_published" : "client:admin.announcements.create_draft",
            )}
          </Button>
        </div>
      </Stack>
    </Card>
  );
}
