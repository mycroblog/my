// SPDX-License-Identifier: AGPL-3.0-only

// The reader's drafts and scheduled posts (ADR-0156), in the composer's frame, which the composer opens: the drafts
// in one list, the scheduled posts first, soonest first, under their heading and the one line of the reader's zone,
// each by the time it goes out; then the drafts newest first, under theirs, each with the start of its text, what it
// carries and when it was saved, and why its last schedule failed if it did (§8). A row
// opens its draft in the composer; its menu changes a scheduled post's time (in the composer), takes it back to a
// draft, or deletes it after asking. Its state is the caller's: the list, or that it loads or failed.

import type { DraftFailureReason, ServerDraft } from "@my/app-data";
import {
  BackButton,
  CaptionLine,
  ComposerFrame,
  ConfirmDialog,
  DraftCount,
  DraftList,
  DraftRow,
  DraftsEmpty,
  DraftSection,
  DraftSections,
  FeedNotice,
  MenuButton,
  MenuGroup,
  MenuItem,
  RelativeTime,
  ScheduleTime,
  scheduleTime,
  ScheduleZone,
  TextButton,
  useLocale,
  type DraftMark,
} from "@my/design-system";
import { useState } from "react";
import { stringsIn } from "../locale";
import { formatParts } from "../slots";

type T = ReturnType<typeof stringsIn>["t"];

export type DraftsState =
  | { kind: "loading" }
  | { kind: "failed" }
  | { kind: "ready"; drafts: readonly ServerDraft[]; scheduled: readonly ServerDraft[] };

export type DraftsViewProps = {
  state: DraftsState;
  /** Back to the post being written. */
  onBack: () => void;
  onClose: () => void;
  onDismiss: () => void;
  onRetry: () => void;
  /** Opens a draft in the composer; `schedule` opens it with its time's form open. */
  onOpen: (id: string, schedule?: boolean) => void;
  onUnschedule: (id: string) => void;
  onDelete: (id: string) => void;
  layer?: boolean | undefined;
  phone?: "screen" | "dialog" | undefined;
};

/** What a draft carries besides its text, as marks. */
function marksOf(d: ServerDraft, t: T): DraftMark[] {
  const images = d.media.length;
  return [
    ...(d.in_reply_to_id === undefined ? [] : [{ icon: "reply" as const, label: t("compose.drafts.mark.reply") }]),
    ...(d.quoted_status_id === undefined ? [] : [{ icon: "quote" as const, label: t("compose.drafts.mark.quote") }]),
    ...(images === 0
      ? []
      : [{ icon: "attach" as const, label: t("compose.drafts.mark.media", { count: images }), count: images }]),
    ...(d.poll === undefined ? [] : [{ icon: "poll" as const, label: t("compose.drafts.mark.poll") }]),
  ];
}

/** The reason a schedule failed, in words. */
const reasonOf = (reason: DraftFailureReason | undefined, t: T) => t(`compose.schedule.failed.${reason ?? "invalid"}`);

function Row({
  d,
  t,
  locale,
  onOpen,
  onUnschedule,
  onDelete,
}: {
  d: ServerDraft;
  t: T;
  locale: string;
  onOpen: DraftsViewProps["onOpen"];
  onUnschedule: (id: string) => void;
  onDelete: (d: ServerDraft) => void;
}) {
  const failed = d.scheduled_at === undefined && d.last_schedule?.outcome === "failed" ? d.last_schedule : undefined;
  const lead =
    d.scheduled_at !== undefined
      ? { kind: "scheduled" as const, text: <ScheduleTime date={d.scheduled_at} /> }
      : failed !== undefined
        ? {
            kind: "failed" as const,
            text: t("compose.schedule.failed", {
              time: scheduleTime(failed.publish_at, locale),
              reason: reasonOf(failed.reason, t),
            }),
          }
        : undefined;
  return (
    <DraftRow
      lead={lead}
      text={d.text}
      blank={t("compose.drafts.blank")}
      warning={d.spoiler_text}
      lang={d.language}
      marks={marksOf(d, t)}
      saved={formatParts(t, "compose.drafts.saved", {}, { time: <RelativeTime date={d.updated_at} /> })}
      openLabel={t("compose.drafts.open_one")}
      onOpen={() => {
        onOpen(d.id);
      }}
      menu={
        <MenuButton icon="more" label={t("compose.drafts.more")} align="end">
          <MenuGroup>
            <MenuItem
              id="open"
              icon="edit"
              onAction={() => {
                onOpen(d.id);
              }}
            >
              {t("compose.drafts.open_item")}
            </MenuItem>
            <MenuItem
              id="schedule"
              icon="schedule"
              onAction={() => {
                onOpen(d.id, true);
              }}
            >
              {t(d.scheduled_at === undefined ? "compose.schedule.item" : "compose.schedule.change_item")}
            </MenuItem>
            {d.scheduled_at !== undefined && (
              <MenuItem
                id="unschedule"
                icon="redraft"
                onAction={() => {
                  onUnschedule(d.id);
                }}
              >
                {t("compose.drafts.unschedule")}
              </MenuItem>
            )}
          </MenuGroup>
          <MenuGroup>
            <MenuItem
              id="delete"
              icon="delete"
              onAction={() => {
                onDelete(d);
              }}
            >
              {t("compose.drafts.delete")}
            </MenuItem>
          </MenuGroup>
        </MenuButton>
      }
    />
  );
}

/** The drafts and the scheduled posts, in the composer's frame. */
export function DraftsView({
  state,
  onBack,
  onClose,
  onDismiss,
  onRetry,
  onOpen,
  onUnschedule,
  onDelete,
  layer = true,
  phone = "screen",
}: DraftsViewProps) {
  const { locale } = useLocale();
  const { t } = stringsIn(locale);
  const [deleting, setDeleting] = useState<ServerDraft | undefined>(undefined);

  const rows = (items: readonly ServerDraft[], label: string) => (
    <DraftList label={label}>
      {items.map((d) => (
        <Row
          key={d.id}
          d={d}
          t={t}
          locale={locale}
          onOpen={onOpen}
          onUnschedule={onUnschedule}
          onDelete={setDeleting}
        />
      ))}
    </DraftList>
  );
  // The zone the times read in, once, over the scheduled posts.
  const zone = (
    <CaptionLine icon="time">{formatParts(t, "compose.drafts.zone", {}, { zone: <ScheduleZone /> })}</CaptionLine>
  );

  let body;
  if (state.kind === "loading") body = <FeedNotice>{t("compose.drafts.loading")}</FeedNotice>;
  else if (state.kind === "failed")
    body = (
      <FeedNotice alert action={<TextButton onPress={onRetry}>{t("compose.drafts.retry")}</TextButton>}>
        {t("compose.drafts.failed")}
      </FeedNotice>
    );
  else if (state.drafts.length === 0 && state.scheduled.length === 0)
    body = <DraftsEmpty>{t("compose.drafts.empty")}</DraftsEmpty>;
  else
    body = (
      <>
        {state.scheduled.length > 0 && (
          <DraftSection
            title={formatParts(
              t,
              "compose.drafts.tab.scheduled",
              {},
              { count: <DraftCount count={state.scheduled.length} /> },
            )}
            note={zone}
          >
            {rows(state.scheduled, t("compose.drafts.list.scheduled"))}
          </DraftSection>
        )}
        <DraftSection
          title={formatParts(t, "compose.drafts.tab.drafts", {}, { count: <DraftCount count={state.drafts.length} /> })}
        >
          {state.drafts.length === 0 ? (
            <DraftsEmpty>{t("compose.drafts.empty.drafts")}</DraftsEmpty>
          ) : (
            rows(state.drafts, t("compose.drafts.list.drafts"))
          )}
        </DraftSection>
      </>
    );

  return (
    <>
      <ComposerFrame
        title={t("compose.drafts")}
        closeLabel={t("compose.close")}
        onClose={onClose}
        onDismiss={onDismiss}
        layer={layer}
        phone={phone}
      >
        <BackButton onPress={onBack}>{t("compose.drafts.back")}</BackButton>
        <DraftSections>{body}</DraftSections>
      </ComposerFrame>
      {deleting && (
        <ConfirmDialog
          title={t("compose.drafts.delete.title")}
          cancel={t("compose.drafts.delete.cancel")}
          confirm={t("compose.drafts.delete.confirm")}
          onCancel={() => {
            setDeleting(undefined);
          }}
          onConfirm={() => {
            onDelete(deleting.id);
            setDeleting(undefined);
          }}
        >
          {t(deleting.scheduled_at === undefined ? "compose.drafts.delete.text" : "compose.drafts.delete.scheduled")}
        </ConfirmDialog>
      )}
    </>
  );
}
