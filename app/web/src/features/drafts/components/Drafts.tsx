// SPDX-License-Identifier: AGPL-3.0-only
// The drafts page (web-client/screens/drafts.md, ADR-0156): the scheduled posts first, then the drafts, each a list of
// rows with the start of the text, what else the draft holds, when it is published or was saved, why its last schedule
// failed, and the row's actions. Editing opens the draft in the new-post sheet over this page.
import { Box, Button, Cluster, ConfirmDialog, DialogTrigger, EmptyState, Hint, Section, Stack, Text } from "@my/ui";
import { useRef, useState } from "react";
import { useIntl } from "react-intl";
import { useT } from "../../../lib/i18n";
import { useFocusAfterRemoval } from "../../../lib/useFocusAfterRemoval";
import { SchedulePicker, type ServerDraft } from "../../compose";
import { type DraftList, type PendingAction, useDrafts } from "../hooks/useDrafts";

/** The page's two lists; the screen's h1 is the route's. */
export function Drafts() {
  return (
    <Stack gap="8">
      <DraftSection scheduled />
      <DraftSection scheduled={false} />
    </Stack>
  );
}

/** The words of each list. */
const sectionWords = {
  scheduled: {
    heading: "client:drafts.scheduled.heading",
    empty: "client:drafts.scheduled.empty",
    emptyDescription: "client:drafts.scheduled.empty.description",
  },
  unscheduled: {
    heading: "client:drafts.unscheduled.heading",
    empty: "client:drafts.unscheduled.empty",
    emptyDescription: "client:drafts.unscheduled.empty.description",
  },
} as const;

function DraftSection({ scheduled }: { scheduled: boolean }) {
  const t = useT();
  const list = useDrafts(scheduled);
  const words = sectionWords[scheduled ? "scheduled" : "unscheduled"];
  const listRef = useRef<HTMLUListElement>(null);
  const removed = useFocusAfterRemoval(
    listRef,
    list.items.map((draft) => draft.id),
  );
  return (
    <Section title={t(words.heading)} surface="rows">
      {list.items.length === 0 ? (
        <EmptyState heading={t(words.empty)} headingLevel={3} description={t(words.emptyDescription)} />
      ) : (
        <Stack gap="4">
          <Stack as="ul" ref={listRef} aria-label={t(words.heading)} divided>
            {list.items.map((draft) => (
              <Box as="li" key={draft.id} data-row-id={draft.id} container paddingY="3">
                <DraftRow
                  draft={draft}
                  list={list}
                  pending={list.pending?.id === draft.id ? list.pending.action : null}
                  onRemoved={() => {
                    removed(draft.id);
                  }}
                />
              </Box>
            ))}
          </Stack>
          {list.hasMore && (
            <div>
              <Button onPress={list.loadMore} isPending={list.isLoadingMore}>
                {t("client:settings.list.more")}
              </Button>
            </div>
          )}
        </Stack>
      )}
      <ConfirmDialog
        isOpen={list.replacing !== null}
        onOpenChange={(isOpen) => {
          if (!isOpen) list.confirmReplace(false);
        }}
        tone="primary"
        title={t("client:drafts.replace.title")}
        description={t("client:drafts.replace.body")}
        confirmLabel={t("client:drafts.replace.confirm")}
        cancelLabel={t("client:drafts.cancel")}
        onConfirm={(close) => {
          list.confirmReplace(true);
          close();
        }}
      />
    </Section>
  );
}

type DraftRowProps = {
  draft: ServerDraft;
  list: DraftList;
  pending: PendingAction["action"] | null;
  /** Called when the row is about to leave the list, so that focus goes to the next one. */
  onRemoved: () => void;
};

/** How many characters of the text name a row's buttons. */
const excerptLength = 40;

/** One draft: its text, what else it holds, its time, the failure of its last schedule, and its actions. */
function DraftRow({ draft, list, pending, onRemoved }: DraftRowProps) {
  const t = useT();
  const intl = useIntl();
  const [deleteError, setDeleteError] = useState<string | null>(null);
  const [isDeleting, setIsDeleting] = useState(false);
  const shown = draft.spoiler_text !== "" ? draft.spoiler_text : draft.text;
  const excerpt = shown.trim() === "" ? t("client:drafts.row.no_text") : shown.trim().slice(0, excerptLength);
  const label = (action: string) => t("client:drafts.action", { action, excerpt });
  const formatDate = (value: string) => intl.formatDate(value, { dateStyle: "medium", timeStyle: "short" });
  const time = (value: string) => (
    <time key="date" dateTime={value}>
      {formatDate(value)}
    </time>
  );
  const holds = [
    ...(draft.media.length > 0 ? [t("client:drafts.row.media", { count: draft.media.length })] : []),
    ...(draft.poll === undefined ? [] : [t("client:drafts.row.poll")]),
    ...(draft.in_reply_to_id === undefined ? [] : [t("client:drafts.row.reply")]),
    ...(draft.quoted_status_id === undefined ? [] : [t("client:drafts.row.quote")]),
  ];
  const failed = draft.last_schedule?.outcome === "failed" ? draft.last_schedule : null;
  const isBusy = pending !== null;
  const edit = t("client:drafts.edit");
  const reschedule = t("client:drafts.reschedule");
  const unschedule = t("client:drafts.unschedule");
  const remove = t("client:drafts.delete");
  return (
    <Stack gap="2">
      {shown.trim() === "" ? (
        <Text as="p" tone="muted">
          {excerpt}
        </Text>
      ) : (
        <Text as="p" dir="auto" lang={draft.language ?? ""} lineClamp={3} whitespace="preLine" wrap="anywhere">
          {shown}
        </Text>
      )}
      {holds.length > 0 && (
        <Hint>{new Intl.ListFormat(intl.locale, { type: "unit", style: "short" }).format(holds)}</Hint>
      )}
      <Hint>
        {draft.scheduled_at === undefined
          ? intl.formatMessage({ id: "client:drafts.row.updated_at" }, { date: time(draft.updated_at) })
          : intl.formatMessage({ id: "client:drafts.row.publish_at" }, { date: time(draft.scheduled_at) })}
      </Hint>
      {failed !== null && failed.reason !== undefined && (
        <Text as="p" size="sm" tone="warning">
          {intl.formatMessage(
            { id: "client:notifications.scheduled_status_failed.detail" },
            { reason: failed.reason, date: time(failed.publish_at) },
          )}
        </Text>
      )}
      <Cluster gap="2">
        <Button
          size="sm"
          aria-label={label(edit)}
          isPending={pending === "edit"}
          isDisabled={isBusy && pending !== "edit"}
          onPress={() => {
            list.edit(draft);
          }}
        >
          {edit}
        </Button>
        {draft.scheduled_at !== undefined && (
          <>
            <SchedulePicker
              scheduledAt={draft.scheduled_at}
              onSchedule={(at) => list.reschedule(draft, at)}
              trigger={() => (
                <Button size="sm" aria-label={label(reschedule)} isDisabled={isBusy}>
                  {reschedule}
                </Button>
              )}
            />
            <Button
              size="sm"
              aria-label={label(unschedule)}
              isPending={pending === "unschedule"}
              isDisabled={isBusy && pending !== "unschedule"}
              onPress={() => {
                list.unschedule(draft, onRemoved);
              }}
            >
              {unschedule}
            </Button>
          </>
        )}
        <DialogTrigger
          onOpenChange={() => {
            setDeleteError(null);
          }}
        >
          <Button size="sm" aria-label={label(remove)} isDisabled={isBusy}>
            {remove}
          </Button>
          <ConfirmDialog
            title={t("client:drafts.delete.title")}
            description={t("client:drafts.delete.body")}
            confirmLabel={remove}
            cancelLabel={t("client:drafts.cancel")}
            isPending={isDeleting}
            {...(deleteError !== null && { errorMessage: deleteError })}
            onConfirm={(close) => {
              setIsDeleting(true);
              void list
                .remove(draft, () => {
                  close();
                  onRemoved();
                })
                .then((reason) => {
                  setIsDeleting(false);
                  setDeleteError(reason);
                });
            }}
          />
        </DialogTrigger>
      </Cluster>
    </Stack>
  );
}
