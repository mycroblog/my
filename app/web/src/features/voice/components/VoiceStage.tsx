// SPDX-License-Identifier: AGPL-3.0-only
// The room's page as a stage (ADR-0100 §9, §10). A card with the room's head: the "Live" pill, how many are in it and
// how long it has been open (and, in the call, the badge of the user's connection, with the pill as an outline while
// it reconnects), share, and the host's button to the settings or,
// to anyone else, a menu with "Report room"; the room's name as the page's heading, its description and hashtags.
// Under it the speakers as large avatars and the listeners as small ones (StageAvatar), the host and the co-hosts
// first, each with the role under the name, the ring of who is talking, a speaker's microphone that is off, a raised
// hand, and the reaction they just sent. Each avatar opens a menu (it shows a surface under the pointer and a caret):
// the participant's profile, for the host and the co-hosts what they may do to the participant, and on the user's own
// avatar as a speaker "Just listen", which the dock has too. The page ends with the dock (VoiceCallDock).
import {
  Card,
  Cluster,
  ConfirmDialog,
  CustomEmoji,
  Heading,
  Hint,
  IconButton,
  InlineError,
  Link,
  LivePill,
  Menu,
  MenuItem,
  MenuTrigger,
  MoreIcon,
  SettingsIcon,
  ShareIcon,
  ShellDock,
  StageAvatar,
  type StageAvatarLabels,
  StageGrid,
  Stack,
  Tag,
  Text,
} from "@my/ui";
import { type ReactNode, useId, useState } from "react";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import { imageSources } from "../../../lib/images";
import { profilePath } from "../../../lib/paths";
import { useCustomEmojiImages } from "../../emoji";
import { ReportDialog } from "../../report";
import type { VoiceParticipantOperation } from "../api/call";
import { hashtagPath, type VoiceRoom, type VoiceRoomParticipant } from "../api/voice";
import { useVoiceCall } from "../hooks/useVoiceCall";
import { useVoiceCallControls, type VoiceCallControls } from "../hooks/useVoiceCallControls";
import { useVoiceModeration } from "../hooks/useVoiceModeration";
import { raisedHands, useOpenFor, useShareRoom, useVoiceStage, type VoiceStageEntry } from "../hooks/useVoiceStage";
import type { VoiceReactionShown } from "../hooks/voiceCall";
import { CallDockControls, JoinDock } from "./VoiceCallDock";
import { CallConnection } from "./VoiceCallParts";
import { VoiceRoomFormDialog } from "./VoiceRoomFormDialog";

const operationLabels: Readonly<Record<VoiceParticipantOperation, ClientMessageKey>> = {
  grant: "client:voice.moderate.grant",
  dismiss: "client:voice.moderate.dismiss",
  revoke: "client:voice.moderate.revoke",
  mute: "client:voice.moderate.mute",
  remove: "client:voice.moderate.remove",
  cohost: "client:voice.moderate.cohost",
  uncohost: "client:voice.moderate.uncohost",
  "hand-over": "client:voice.moderate.hand_over",
};

export type VoiceStageProps = {
  room: VoiceRoom;
  /** Whether the user hosts the room, who changes its settings. */
  isHost: boolean;
  /** Renders the page heading (the route's level-one heading) around its text. */
  renderHeading: (text: ReactNode) => ReactNode;
};

export function VoiceStage({ room, isHost, renderHeading }: VoiceStageProps) {
  const t = useT();
  const call = useVoiceCall(room);
  const inCall = call.status === "in-call" ? call : undefined;
  // The app's call: this room's while `inCall` is set.
  const appCall = useVoiceCallControls();
  const controls = inCall === undefined ? null : appCall;
  const moderation = useVoiceModeration(room);
  const stage = useVoiceStage(room, inCall, controls?.micOn ?? false);
  const [removing, setRemoving] = useState<VoiceRoomParticipant | undefined>(undefined);
  const removingName =
    removing === undefined
      ? ""
      : (stage.speakers.concat(stage.listeners).find((entry) => entry.participant === removing)?.name ?? "");
  const onOperate = (participant: VoiceRoomParticipant, operation: VoiceParticipantOperation) => {
    if (operation === "remove") setRemoving(participant);
    else moderation.operate(participant, operation);
  };
  // A speaker goes back to listening from the menu of their own avatar.
  const onListen =
    inCall?.isSpeaker === true
      ? () => {
          inCall.operate("listen");
        }
      : undefined;
  return (
    <>
      <Card padding="4">
        <Stack gap="6">
          <RoomHead room={room} isHost={isHost} renderHeading={renderHeading} controls={controls} />
          {moderation.error !== undefined && (
            <InlineError size="inherit" alert>
              {t("client:voice.call.action_failed", { reason: moderation.error })}
            </InlineError>
          )}
          {stage.speakers.length > 0 && (
            <StageGroup
              heading={t("client:voice.stage.speakers", { count: stage.speakers.length })}
              size="speaker"
              entries={stage.speakers}
              onOperate={onOperate}
              onListen={onListen}
            />
          )}
          {stage.listeners.length > 0 && (
            <StageGroup
              heading={t("client:voice.stage.listeners", { count: stage.listeners.length })}
              size="listener"
              entries={stage.listeners}
              onOperate={onOperate}
              onListen={undefined}
            />
          )}
        </Stack>
      </Card>
      <ConfirmDialog
        isOpen={removing !== undefined}
        onOpenChange={(open) => {
          if (!open) setRemoving(undefined);
        }}
        title={t("client:voice.moderate.remove.title", { name: removingName })}
        description={t("client:voice.moderate.remove.description")}
        confirmLabel={t("client:voice.moderate.remove")}
        cancelLabel={t("client:voice.call.cancel")}
        tone="danger"
        onConfirm={(close) => {
          if (removing !== undefined) moderation.operate(removing, "remove");
          close();
        }}
      />
      <ShellDock>
        {inCall === undefined || controls === null ? (
          call.status === "outside" && <JoinDock call={call} />
        ) : (
          <CallDockControls
            room={room}
            call={inCall}
            controls={controls}
            moderation={moderation}
            hands={raisedHands(room)}
          />
        )}
      </ShellDock>
    </>
  );
}

/** The head of the room: its state, its name as the page's heading, its description and hashtags, and its actions. */
function RoomHead({ room, isHost, renderHeading, controls }: VoiceStageProps & { controls: VoiceCallControls | null }) {
  const t = useT();
  const openFor = useOpenFor(room.opened_at);
  const share = useShareRoom(room);
  return (
    <Stack gap="3">
      {/* The actions keep the end of the line; the state wraps before them on narrow screens. */}
      <Cluster gap="2" align="start" justify="between" wrap={false}>
        <Cluster gap="2" align="center">
          {/* While the user's call reconnects, the pill turns to an outline and the badge beside it says why. */}
          <LivePill label={t("client:voice.room.live")} interrupted={controls?.phase === "reconnecting"} />
          {controls !== null && <CallConnection controls={controls} />}
          <Hint as="span">
            {t("client:voice.room.stats", {
              people: t("client:voice.room.participants_count", { count: room.participants.length }),
              duration: openFor,
            })}
          </Hint>
        </Cluster>
        <Cluster gap="1" align="center" wrap={false}>
          <IconButton label={t("client:voice.room.share")} onPress={share}>
            <ShareIcon />
          </IconButton>
          {isHost ? <SettingsButton room={room} /> : <RoomMenu room={room} />}
        </Cluster>
      </Cluster>
      {renderHeading(<span dir="auto">{room.name}</span>)}
      {room.description !== "" && (
        <Text as="p" tone="muted" whitespace="preLine" dir="auto" wrap="anywhere">
          {room.description}
        </Text>
      )}
      {room.hashtags.length > 0 && (
        <Cluster as="ul" gap="1" aria-label={t("client:voice.form.hashtags")}>
          {room.hashtags.map((tag) => (
            <Tag key={tag} as="li" variant="pill" dir="auto">
              <Link href={hashtagPath(tag)} variant="standalone">
                {t("client:tag.title", { tag })}
              </Link>
            </Tag>
          ))}
        </Cluster>
      )}
    </Stack>
  );
}

/** The host's button to the room's settings (the visibility and the speakers among them). */
function SettingsButton({ room }: { room: VoiceRoom }) {
  const t = useT();
  const [isOpen, setIsOpen] = useState(false);
  return (
    <>
      <IconButton
        label={t("client:voice.form.edit_title")}
        onPress={() => {
          setIsOpen(true);
        }}
      >
        <SettingsIcon />
      </IconButton>
      <VoiceRoomFormDialog room={room} isOpen={isOpen} onOpenChange={setIsOpen} />
    </>
  );
}

/** Everyone else's menu of the room: reporting it to the moderators (ADR-0100 §11), with its current host as the target. */
function RoomMenu({ room }: { room: VoiceRoom }) {
  const t = useT();
  const [isReporting, setIsReporting] = useState(false);
  const label = t("client:voice.room.more");
  return (
    <>
      <MenuTrigger>
        <IconButton label={label}>
          <MoreIcon />
        </IconButton>
        <Menu aria-label={label} placement="bottom end">
          <MenuItem
            onAction={() => {
              setIsReporting(true);
            }}
          >
            {t("client:voice.room.report")}
          </MenuItem>
        </Menu>
      </MenuTrigger>
      <ReportDialog
        account={room.host}
        voiceRoom={{ id: room.id, name: room.name }}
        isOpen={isReporting}
        onOpenChange={setIsReporting}
      />
    </>
  );
}

type StageGroupProps = {
  heading: string;
  size: "speaker" | "listener";
  entries: readonly VoiceStageEntry[];
  onOperate: (participant: VoiceRoomParticipant, operation: VoiceParticipantOperation) => void;
  /** Makes the user, a speaker, a listener: an item of the menu of their own avatar. */
  onListen: (() => void) | undefined;
};

/** The speakers or the listeners, under their heading. */
function StageGroup({ heading, size, entries, onOperate, onListen }: StageGroupProps) {
  const t = useT();
  const headingId = useId();
  const images = useCustomEmojiImages();
  const labels: StageAvatarLabels = {
    speaking: t("client:voice.participant.talking"),
    muted: t("client:voice.participant.mic_off"),
    handRaised: t("client:voice.participant.hand_raised"),
  };
  return (
    <Stack gap="3">
      <Heading level={2} id={headingId}>
        {heading}
      </Heading>
      <StageGrid aria-labelledby={headingId} size={size}>
        {entries.map((entry) => (
          <StageAvatar
            key={entry.participant.account.id}
            name={entry.name}
            sources={imageSources(entry.participant.account.avatar)}
            seed={entry.participant.account.id}
            size={size}
            caption={entry.caption}
            speaking={entry.speaking}
            muted={entry.muted}
            handRaised={entry.handRaised}
            reaction={
              entry.reaction === undefined
                ? undefined
                : {
                    content: <ReactionContent reaction={entry.reaction} images={images} />,
                    label: t("client:voice.participant.reacted_with", { emoji: entry.reaction.emoji }),
                  }
            }
            labels={labels}
            menu={
              <Menu aria-label={t("client:voice.moderate.menu", { name: entry.name })} placement="bottom">
                <MenuItem href={profilePath(entry.participant.account.acct)}>
                  {t("client:voice.participant.profile")}
                </MenuItem>
                {entry.isSelf && onListen !== undefined && (
                  <MenuItem onAction={onListen}>{t("client:voice.call.listen")}</MenuItem>
                )}
                {entry.operations.map((operation) => (
                  <MenuItem
                    key={operation}
                    onAction={() => {
                      onOperate(entry.participant, operation);
                    }}
                  >
                    {t(operationLabels[operation])}
                  </MenuItem>
                ))}
              </Menu>
            }
          />
        ))}
      </StageGrid>
    </Stack>
  );
}

/** A reaction as it is drawn: the server's custom emoji, or the Unicode emoji as text. */
function ReactionContent({
  reaction,
  images,
}: {
  reaction: VoiceReactionShown;
  images: ReturnType<typeof useCustomEmojiImages>;
}) {
  const shortcode = /^:([^\s:]+):$/u.exec(reaction.emoji)?.[1];
  const image = shortcode === undefined ? undefined : images.get(shortcode);
  return image === undefined ? <span dir="auto">{reaction.emoji}</span> : <CustomEmoji emoji={image} />;
}
