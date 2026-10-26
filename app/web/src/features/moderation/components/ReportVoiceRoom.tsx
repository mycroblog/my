// SPDX-License-Identifier: AGPL-3.0-only
// The voice chat room that a report points at, on the report's page (ADR-0100 §11, §13): whether it is open and how
// it ended; every revision of its settings, oldest first, with the one in effect when it was reported marked; who
// took part and when, with why they left; who was removed from it; and, while it is open, ending it for everyone in
// it with an optional reason for the moderators, after an alert dialog (ConfirmDialog). The participants' records
// stay only for a while after the room ends, which the section says. Names and descriptions were written by the
// host, in any language (web-client/README.md Y10).
import { Box, Button, Cluster, ConfirmDialog, DescriptionItem, DescriptionList, Heading, Hint, ReasonField, Stack, StatusMessage, Tag, Text } from "@my/ui";
import { type ReactNode, useId, useRef } from "react";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import { speakerLabels, visibilityLabels, voiceEndReasonLabel } from "../../voice";
import { type ReportDetail, type ReportVoiceRoom as Room, type VoiceRoomBan, type VoiceRoomParticipation, type VoiceRoomRevision, voiceRoomEndReasonLimit } from "../api/reports";
import { type EndVoiceRoom, useEndVoiceRoom } from "../hooks/useEndVoiceRoom";
import { DateTime, DetailSection, useFocusAfterChange } from "./parts";
import { AccountByIdLabel, AssigneeName } from "./reportParts";

/** The sentence of each reason a participation ended (api/spec AdminVoiceRoomParticipation); another reads as `other`. */
const leaveReasons: Readonly<Record<string, ClientMessageKey>> = {
  left: "client:admin.report.voice.leave_reason.left",
  moved: "client:admin.report.voice.leave_reason.moved",
  disconnected: "client:admin.report.voice.leave_reason.disconnected",
  removed: "client:admin.report.voice.leave_reason.removed",
  blocked: "client:admin.report.voice.leave_reason.blocked",
  suspended: "client:admin.report.voice.leave_reason.suspended",
  room_ended: "client:admin.report.voice.leave_reason.room_ended",
};

/**
 * The section of the room. Ending it takes its button away, so focus then moves to the section's heading, and what
 * happened stays in the section's StatusMessage.
 */
export function ReportVoiceRoom({ report, room }: { report: ReportDetail; room: Room }) {
  const t = useT();
  const isOpen = room.ended_at === undefined;
  const headingRef = useRef<HTMLHeadingElement>(null);
  const focusHeading = useFocusAfterChange(headingRef, room.ended_at);
  const end = useEndVoiceRoom(report, room.id, focusHeading);
  return (
    <DetailSection title={t("client:admin.report.voice.title")} headingRef={headingRef}>
      <Stack gap="6">
        <Hint size="inherit">{t("client:admin.report.voice.records")}</Hint>
        <StatusMessage>{end.status}</StatusMessage>
        <RoomState room={room} />
        {isOpen && <EndRoom end={end} />}
        <Revisions revisions={room.revisions} reportedId={room.reported_revision_id} />
        <Participations participations={room.participations} />
        <Bans bans={room.bans} />
      </Stack>
    </DetailSection>
  );
}

/** Whether the room is open, and when, why and by whom it ended. */
function RoomState({ room }: { room: Room }) {
  const t = useT();
  const isOpen = room.ended_at === undefined;
  return (
    <>
      <DescriptionList columns={3}>
        <DescriptionItem term={t("client:admin.report.voice.state")}>
          <Text weight="semibold">{isOpen ? t("client:admin.report.voice.state.open") : t("client:admin.report.voice.state.ended")}</Text>
        </DescriptionItem>
        <DescriptionItem term={t("client:admin.report.voice.opened_at")}>
          <DateTime value={room.opened_at} />
        </DescriptionItem>
        {room.ended_at !== undefined && (
          <>
            <DescriptionItem term={t("client:admin.report.voice.ended_at")}>
              <DateTime value={room.ended_at} />
            </DescriptionItem>
            <DescriptionItem term={t("client:admin.report.voice.end_reason")}>{t(voiceEndReasonLabel(room.end_reason))}</DescriptionItem>
            {room.ended_by_account_id !== undefined && (
              <DescriptionItem term={t("client:admin.report.voice.ended_by")}>
                <AssigneeName id={room.ended_by_account_id} />
              </DescriptionItem>
            )}
          </>
        )}
      </DescriptionList>
      {room.end_note !== undefined && (
        <DescriptionList>
          <DescriptionItem term={t("client:admin.report.voice.end_note")}>
            {/* Written by a moderator, in any language. */}
            <Text dir="auto" block align="start" wrap="anywhere" whitespace="preWrap">
              {room.end_note}
            </Text>
          </DescriptionItem>
        </DescriptionList>
      )}
    </>
  );
}

/** Ending the open room, after its confirmation with an optional reason for the moderators. */
function EndRoom({ end }: { end: EndVoiceRoom }) {
  const t = useT();
  return (
    <Stack align="start" gap="3">
      <Hint>{t("client:admin.report.voice.end.description")}</Hint>
      <Button variant="danger" onPress={end.open}>
        {t("client:admin.report.voice.end")}
      </Button>
      <ConfirmDialog
        isOpen={end.isOpen}
        onOpenChange={(isOpen) => {
          if (!isOpen) end.cancel();
        }}
        title={t("client:admin.report.voice.end.confirm.title")}
        description={t("client:admin.report.voice.end.confirm.description")}
        confirmLabel={t("client:admin.report.voice.end")}
        cancelLabel={t("client:admin.report.voice.end.cancel")}
        onConfirm={end.confirm}
        isPending={end.isPending}
        {...(end.error === undefined ? {} : { errorMessage: t("client:admin.report.voice.end.failed", { reason: end.error }) })}
      >
        <ReasonField
          label={t("client:admin.report.voice.end.reason")}
          description={t("client:admin.report.voice.end.reason.description")}
          value={end.reason}
          onChange={end.changeReason}
          characterLimit={voiceRoomEndReasonLimit}
          formatRemaining={(remaining) =>
            remaining < 0 ? t("client:admin.account_actions.characters_over", { count: -remaining }) : t("client:admin.account_actions.characters_left", { count: remaining })
          }
          errorMessage={t("client:admin.report.voice.end.reason.too_long", { max: String(voiceRoomEndReasonLimit) })}
        />
      </ConfirmDialog>
    </Stack>
  );
}

/** A subsection of the room: a heading that names its list. */
function Part({ title, children }: { title: string; children: ReactNode }) {
  const headingId = useId();
  return (
    <Stack as="section" gap="3" aria-labelledby={headingId}>
      <Heading level={3} id={headingId}>
        {title}
      </Heading>
      {children}
    </Stack>
  );
}

function Revisions({ revisions, reportedId }: { revisions: readonly VoiceRoomRevision[]; reportedId: string }) {
  const t = useT();
  return (
    <Part title={t("client:admin.report.voice.revisions")}>
      <Stack as="ol" divided>
        {revisions.map((revision) => (
          <Stack as="li" key={revision.id} gap="2" paddingY="3">
            {revision.id === reportedId && (
              <Box>
                <Tag>{t("client:admin.report.voice.revision.reported")}</Tag>
              </Box>
            )}
            <DescriptionList columns={2}>
              <DescriptionItem term={t("client:admin.report.voice.revision.name")}>
                {revision.name === "" ? (
                  <Text tone="muted">{t("client:admin.report.voice.revision.erased")}</Text>
                ) : (
                  <Text dir="auto" align="start" wrap="anywhere">
                    {revision.name}
                  </Text>
                )}
              </DescriptionItem>
              <DescriptionItem term={t("client:admin.report.voice.revision.hashtags")}>
                {revision.hashtags.length === 0 ? (
                  <Text tone="muted">{t("client:admin.report.voice.revision.none")}</Text>
                ) : (
                  <Cluster gap="1">
                    {revision.hashtags.map((tag) => (
                      <Tag key={tag} variant="pill" dir="auto">
                        {t("client:tag.title", { tag })}
                      </Tag>
                    ))}
                  </Cluster>
                )}
              </DescriptionItem>
              <DescriptionItem term={t("client:admin.report.voice.revision.visibility")}>{t(visibilityLabels[revision.visibility])}</DescriptionItem>
              <DescriptionItem term={t("client:admin.report.voice.revision.speakers")}>{t(speakerLabels[revision.speakers])}</DescriptionItem>
              <DescriptionItem term={t("client:admin.report.voice.revision.by")}>
                <AccountByIdLabel id={revision.revised_by_account_id} />
              </DescriptionItem>
              <DescriptionItem term={t("client:admin.report.voice.revision.at")}>
                <DateTime value={revision.revised_at} />
              </DescriptionItem>
            </DescriptionList>
            <DescriptionList>
              <DescriptionItem term={t("client:admin.report.voice.revision.description")}>
                {revision.description === "" ? (
                  <Text tone="muted">{t("client:admin.report.voice.revision.none")}</Text>
                ) : (
                  <Text dir="auto" block align="start" wrap="anywhere" whitespace="preWrap">
                    {revision.description}
                  </Text>
                )}
              </DescriptionItem>
            </DescriptionList>
          </Stack>
        ))}
      </Stack>
    </Part>
  );
}

function Participations({ participations }: { participations: readonly VoiceRoomParticipation[] }) {
  const t = useT();
  return (
    <Part title={t("client:admin.report.voice.participations")}>
      {participations.length === 0 ? (
        <Hint size="inherit">{t("client:admin.report.voice.participations.empty")}</Hint>
      ) : (
        <Stack as="ol" divided>
          {participations.map((participation) => (
            // One account may join, leave, and join again: the time it joined tells its participations apart.
            <Stack as="li" key={`${participation.account_id} ${participation.joined_at}`} gap="2" paddingY="3">
              <AccountByIdLabel id={participation.account_id} />
              <DescriptionList columns={3}>
                <DescriptionItem term={t("client:admin.report.voice.joined_at")}>
                  <DateTime value={participation.joined_at} />
                </DescriptionItem>
                <DescriptionItem term={t("client:admin.report.voice.left_at")}>
                  {participation.left_at === undefined ? t("client:admin.report.voice.still_in") : <DateTime value={participation.left_at} />}
                </DescriptionItem>
                {participation.left_at !== undefined && (
                  <DescriptionItem term={t("client:admin.report.voice.leave_reason")}>
                    {t(leaveReasons[participation.leave_reason ?? ""] ?? "client:admin.report.voice.leave_reason.other")}
                  </DescriptionItem>
                )}
              </DescriptionList>
            </Stack>
          ))}
        </Stack>
      )}
    </Part>
  );
}

function Bans({ bans }: { bans: readonly VoiceRoomBan[] }) {
  const t = useT();
  return (
    <Part title={t("client:admin.report.voice.bans")}>
      {bans.length === 0 ? (
        <Hint size="inherit">{t("client:admin.report.voice.bans.empty")}</Hint>
      ) : (
        <Stack as="ul" divided>
          {bans.map((ban) => (
            <Stack as="li" key={ban.account_id} gap="2" paddingY="3">
              <AccountByIdLabel id={ban.account_id} />
              <DescriptionList columns={2}>
                <DescriptionItem term={t("client:admin.report.voice.banned_by")}>
                  <AccountByIdLabel id={ban.banned_by_account_id} />
                </DescriptionItem>
                <DescriptionItem term={t("client:admin.report.voice.banned_at")}>
                  <DateTime value={ban.banned_at} />
                </DescriptionItem>
              </DescriptionList>
            </Stack>
          ))}
        </Stack>
      )}
    </Part>
  );
}
