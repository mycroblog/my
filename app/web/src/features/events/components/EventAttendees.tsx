// SPDX-License-Identifier: AGPL-3.0-only
// Who is going to an event (ADR-0161 §6, §8): the attendees who answered going or maybe, in the order of their
// answers, which everyone who sees the event reads. Its organizers approve an attendee (who then sees the places shown
// to the approved) or revoke the approval from the attendee's row; each row marks an approved attendee.
import {
  AccountItem,
  Button,
  CheckIcon,
  Cluster,
  Hint,
  InlineError,
  RegionError,
  Section,
  Spinner,
  Stack,
  StatusBadge,
} from "@my/ui";
import { handleOf } from "../../../lib/handle";
import { useT } from "../../../lib/i18n";
import { imageSources } from "../../../lib/images";
import { profilePath } from "../../../lib/paths";
import type { Event, EventAttendee } from "../api/events";
import { accountName } from "../hooks/names";
import { type EventAttendeeList, useEventAttendees } from "../hooks/useEventPage";
import { answerBadges } from "./EventParts";

export function EventAttendees({ event, organizes }: { event: Event; organizes: boolean }) {
  const t = useT();
  const list = useEventAttendees(event);
  return (
    <Section title={t("client:events.attendees.title")}>
      <Attendees list={list} organizes={organizes && event.state !== "cancelled"} />
    </Section>
  );
}

function Attendees({ list, organizes }: { list: EventAttendeeList; organizes: boolean }) {
  const t = useT();
  switch (list.status) {
    case "loading":
      return <Spinner label={t("client:events.loading")} />;
    case "error":
      return (
        <RegionError
          heading={t("client:events.attendees.failed")}
          reason={list.reason}
          retryLabel={t("client:events.retry")}
          onRetry={list.retry}
          isRetrying={list.isRetrying}
        />
      );
    case "ready":
      if (list.attendees.length === 0) return <Hint>{t("client:events.attendees.empty")}</Hint>;
      return (
        <Stack gap="3">
          {list.approvalError !== undefined && (
            <InlineError alert size="inherit">
              {t("client:events.attendees.approval_failed", { reason: list.approvalError })}
            </InlineError>
          )}
          <Stack as="ul" gap="3" aria-label={t("client:events.attendees.title")}>
            {list.attendees.map((attendee) => (
              <li key={attendee.account.id}>
                <AttendeeRow attendee={attendee} list={list} organizes={organizes} />
              </li>
            ))}
          </Stack>
          {list.hasMore && (
            <div>
              <Button onPress={list.loadMore} isPending={list.isLoadingMore}>
                {t("client:events.attendees.more")}
              </Button>
            </div>
          )}
        </Stack>
      );
  }
}

type AttendeeRowProps = {
  attendee: EventAttendee;
  list: Extract<EventAttendeeList, { status: "ready" }>;
  organizes: boolean;
};

function AttendeeRow({ attendee, list, organizes }: AttendeeRowProps) {
  const t = useT();
  const { account } = attendee;
  const name = accountName(account);
  return (
    <AccountItem
      name={name}
      handle={handleOf(account.acct)}
      avatar={imageSources(account.avatar)}
      avatarSeed={account.id}
      href={profilePath(account.acct)}
      trailingPlacement="below"
      trailing={
        <Cluster gap="2" align="center">
          <StatusBadge tone="neutral" icon={<CheckIcon />} label={t(answerBadges[attendee.answer])} />
          {attendee.approved && (
            <StatusBadge tone="success" icon={<CheckIcon />} label={t("client:events.attendees.approved")} />
          )}
          {organizes && (
            <Button
              size="sm"
              variant={attendee.approved ? "subtle" : "secondary"}
              isPending={list.pendingId === account.id}
              onPress={() => {
                list.setApproval(attendee, !attendee.approved);
              }}
              aria-label={t(
                attendee.approved ? "client:events.attendees.revoke_of" : "client:events.attendees.approve_of",
                {
                  name,
                },
              )}
            >
              {t(attendee.approved ? "client:events.attendees.revoke" : "client:events.attendees.approve")}
            </Button>
          )}
        </Cluster>
      }
    />
  );
}
