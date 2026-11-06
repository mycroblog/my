// SPDX-License-Identifier: AGPL-3.0-only
// The page of one announcement below its heading (/admin/announcements/$id, manage_announcements, FR-726,
// web-client/screens/admin-announcements-detail.md):
// - its state: where it is in its life, when it was published, when it shows from and until, when it was written and
//   last changed, with "Publish" for a draft; a withdrawn one says it can no longer change;
// - the announcement as the users see it (its title and its text rendered by the server);
// - the edit form of its title and text, which keeps its window and saves once (not for a withdrawn one);
// - withdrawing it, after an alert dialog, as it cannot be undone (not for a withdrawn one).
// Publishing and withdrawing change which buttons there are, so focus then moves to the state's heading, and what
// happened stays in the state's StatusMessage; a failure of publishing stays there too (role="alert"), and one of
// withdrawing in the dialog. What saving did stays under the edit form.
import {
  Button,
  ConfirmDialog,
  DescriptionItem,
  DescriptionList,
  DialogTrigger,
  Heading,
  Hint,
  InlineError,
  PostContent,
  serverSanitizedHtml,
  Stack,
  StatusMessage,
  Text,
} from "@my/ui";
import { type SyntheticEvent, useRef, useState } from "react";
import { useNow } from "../../../lib/clock";
import { useT } from "../../../lib/i18n";
import { DateTime, RequirePermission, Section, useFocusAfterChange } from "../../moderation";
import type { AdminAnnouncement } from "../api/announcements";
import {
  type AnnouncementActions,
  announcementLabel,
  announcementPhase,
  announcementPhaseLabels,
  announcementShowsFrom,
  useAdminAnnouncement,
  useAnnouncementActions,
} from "../hooks/useAnnouncements";
import { AnnouncementTextField, AnnouncementTitleField } from "./AnnouncementFields";

export function AnnouncementDetail({ id }: { id: string }) {
  return (
    <RequirePermission permission="manage_announcements">
      <AnnouncementView id={id} />
    </RequirePermission>
  );
}

function AnnouncementView({ id }: { id: string }) {
  const t = useT();
  const announcement = useAdminAnnouncement(id);
  const headingRef = useRef<HTMLHeadingElement>(null);
  const focusHeading = useFocusAfterChange(headingRef, announcement.state);
  const [status, setStatus] = useState<string | undefined>(undefined);
  // Publishing and withdrawing take their button away: focus moves to the state's heading.
  const actions = useAnnouncementActions((_, done) => {
    focusHeading();
    setStatus(done);
  });
  const withdrawn = announcement.state === "withdrawn";
  return (
    <Stack gap="8">
      <Section surface="card" title={t("client:admin.announcements.state")} headingRef={headingRef}>
        <Stack gap="4">
          <StatusMessage>{status}</StatusMessage>
          {actions.error !== undefined && (
            <InlineError size="inherit" alert>
              {t("client:admin.announcements.change_failed", { reason: actions.error })}
            </InlineError>
          )}
          <Facts announcement={announcement} />
          {withdrawn ? (
            <Hint size="inherit">{t("client:admin.announcements.withdrawn.description")}</Hint>
          ) : (
            announcement.state === "draft" && <PublishAnnouncement announcement={announcement} actions={actions} />
          )}
        </Stack>
      </Section>
      <Section
        surface="card"
        title={t("client:admin.announcements.preview")}
        description={t("client:admin.announcements.preview.description")}
      >
        <Stack gap="3">
          <Heading level={3} wrap="anywhere">
            {/* Written by an administrator in any language (web-client/README.md Y10). */}
            <span dir="auto">{announcementLabel(announcement)}</span>
          </Heading>
          <PostContent html={serverSanitizedHtml(announcement.content)} framed />
        </Stack>
      </Section>
      {!withdrawn && (
        <>
          <Section
            surface="card"
            title={t("client:admin.announcements.edit.title")}
            description={t("client:admin.announcements.edit.description")}
          >
            {/* The form starts from what the announcement says when the page opens. */}
            <EditAnnouncementForm announcement={announcement} />
          </Section>
          <Section
            surface="card"
            title={t("client:admin.announcements.withdraw.section")}
            description={t("client:admin.announcements.withdraw.description")}
          >
            <WithdrawAnnouncement announcement={announcement} actions={actions} />
          </Section>
        </>
      )}
    </Stack>
  );
}

function Facts({ announcement }: { announcement: AdminAnnouncement }) {
  const t = useT();
  const now = useNow();
  // A draft shows from its publication, or from the start of its window when that is later.
  const showsFrom = announcementShowsFrom(announcement) ?? announcement.starts_at;
  return (
    <DescriptionList>
      <DescriptionItem term={t("client:admin.announcements.state")}>
        <Text weight="semibold">{t(announcementPhaseLabels[announcementPhase(announcement, now)])}</Text>
      </DescriptionItem>
      {announcement.published_at !== undefined && (
        <DescriptionItem term={t("client:admin.announcements.published_at")}>
          <DateTime value={announcement.published_at} />
        </DescriptionItem>
      )}
      <DescriptionItem term={t("client:admin.announcements.starts_at")}>
        {showsFrom === undefined ? (
          t("client:admin.announcements.period.on_publication")
        ) : (
          <DateTime value={showsFrom} />
        )}
      </DescriptionItem>
      <DescriptionItem term={t("client:admin.announcements.ends_at")}>
        {announcement.ends_at === undefined ? (
          t("client:admin.announcements.until_withdrawn")
        ) : (
          <DateTime value={announcement.ends_at} />
        )}
      </DescriptionItem>
      <DescriptionItem term={t("client:admin.announcements.created_at")}>
        <DateTime value={announcement.created_at} />
      </DescriptionItem>
      <DescriptionItem term={t("client:admin.announcements.revised_at")}>
        <DateTime value={announcement.revised_at} />
      </DescriptionItem>
    </DescriptionList>
  );
}

type ActionProps = { announcement: AdminAnnouncement; actions: AnnouncementActions };

function PublishAnnouncement({ announcement, actions }: ActionProps) {
  const t = useT();
  const [isPublishing, setIsPublishing] = useState(false);
  return (
    <div>
      <Button
        isPending={isPublishing}
        onPress={() => {
          setIsPublishing(true);
          void actions.publish(announcement).then(() => {
            setIsPublishing(false);
          });
        }}
      >
        {t("client:admin.announcements.publish")}
      </Button>
    </div>
  );
}

/** The title and the text of the announcement; saving keeps its window, and what it did stays under the button. */
function EditAnnouncementForm({ announcement }: { announcement: AdminAnnouncement }) {
  const t = useT();
  const [status, setStatus] = useState<string | undefined>(undefined);
  const actions = useAnnouncementActions((_, done) => {
    setStatus(done);
  });
  const [title, setTitle] = useState(announcement.title);
  const [text, setText] = useState(announcement.text);
  const [submitted, setSubmitted] = useState(false);
  const [isSaving, setIsSaving] = useState(false);
  const [failed, setFailed] = useState(false);
  const textError = submitted ? actions.textError(text) : undefined;
  // Over the limit, the title says so at once; empty, once the form is sent.
  const titleError = submitted || title.trim() !== "" ? actions.titleError(title) : undefined;
  const submit = (event: SyntheticEvent<HTMLFormElement>) => {
    event.preventDefault();
    setSubmitted(true);
    if (isSaving || actions.titleError(title) !== undefined || actions.textError(text) !== undefined) return;
    setIsSaving(true);
    setFailed(false);
    setStatus(undefined);
    void actions.update(announcement, title, text).then((saved) => {
      setIsSaving(false);
      setFailed(!saved);
    });
  };
  return (
    <Stack as="form" gap="4" onSubmit={submit}>
      <AnnouncementTitleField value={title} onChange={setTitle} error={titleError} />
      <AnnouncementTextField value={text} onChange={setText} error={textError} rows={8} />
      {failed && actions.error !== undefined && (
        <InlineError size="inherit" alert>
          {t("client:admin.announcements.change_failed", { reason: actions.error })}
        </InlineError>
      )}
      <div>
        <Button type="submit" variant="primary" isPending={isSaving}>
          {t("client:admin.announcements.save")}
        </Button>
      </div>
      <StatusMessage>{status}</StatusMessage>
    </Stack>
  );
}

/** Withdrawing the announcement, after an alert dialog: it cannot be undone. */
function WithdrawAnnouncement({ announcement, actions }: ActionProps) {
  const t = useT();
  const name = announcementLabel(announcement);
  const [isWithdrawing, setIsWithdrawing] = useState(false);
  const [failed, setFailed] = useState(false);
  return (
    <div>
      <DialogTrigger>
        <Button variant="danger">{t("client:admin.announcements.withdraw")}</Button>
        <ConfirmDialog
          title={t("client:admin.announcements.withdraw.title", { announcement: name })}
          description={t("client:admin.announcements.withdraw.description")}
          confirmLabel={t("client:admin.announcements.withdraw")}
          cancelLabel={t("client:admin.announcements.cancel")}
          isPending={isWithdrawing}
          {...(failed &&
            actions.error !== undefined && {
              errorMessage: t("client:admin.announcements.change_failed", { reason: actions.error }),
            })}
          onConfirm={(close) => {
            setIsWithdrawing(true);
            setFailed(false);
            void actions.withdraw(announcement).then((withdrawn) => {
              setIsWithdrawing(false);
              if (withdrawn) close();
              else setFailed(true);
            });
          }}
        />
      </DialogTrigger>
    </div>
  );
}
