// SPDX-License-Identifier: AGPL-3.0-only

// The parts of a voice chat room (ADR-0100 §5, §8, §10), each in every state, and the room they make together: the
// stage's tiles (the host speaking, a co-host with the microphone off, a speaker without an image, one the reader
// blocks, a name too long for its tile); the listeners' compact tiles (a hand raised, a co-host who listens, one the
// reader blocks); reactions rising over the stage, Unicode and custom emoji, which stand still with reduced motion; a
// room as a line of a list (with its icon emoji, a custom one, none, a name that wraps, no speaker yet); the room's bar
// of controls for a speaker and for a listener, on and off; and the whole room at a phone's, a tablet's and a PC's
// width. The names are samples in English.

import { rootPx } from "@my/design-tokens";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useEffect, useRef, useState, type ReactNode } from "react";
import { CustomEmoji } from "./content";
import { sampleCustomEmoji } from "./emoji.samples";
import { Box, Stack } from "./layout";
import { GroupHeading, ListTitle, RowItem, RowList } from "./list";
import { images } from "./samples";
import {
  ParticipantGrid,
  ParticipantTile,
  ReactionFlow,
  RoomControl,
  RoomControls,
  RoomLine,
  type Reaction,
} from "./voice";

const rem = (px: number) => `${px / rootPx}rem`;
const none = () => undefined;

/** The column of a screen: as wide as the app's main column at most, the frame's edge around it. */
function Column({ children }: { children: ReactNode }) {
  return <div style={{ maxWidth: rem(600), padding: rem(16) }}>{children}</div>;
}

const custom = (shortcode: string) => {
  const url = sampleCustomEmoji[shortcode] ?? "";
  return <CustomEmoji shortcode={shortcode} url={url} staticUrl={url} />;
};

/** The stage: the host speaking, two co-hosts, speakers, one blocked, one without an image, a long name. */
function Stage() {
  return (
    <ParticipantGrid label="Speakers" size="stage">
      <ParticipantTile name="Mika" src={images.dusk} size="stage" role="Host" speaking="Speaking" onPress={none} />
      <ParticipantTile name="Ren" src={images.snow} size="stage" role="Co-host" muted="Microphone off" onPress={none} />
      <ParticipantTile name="Sora" src={images.logo} size="stage" onPress={none} />
      <ParticipantTile name="Aoi Tanaka" size="stage" speaking="Speaking" onPress={none} />
      <ParticipantTile
        name="Haruto"
        src={images.dusk}
        size="stage"
        muted="Microphone off"
        blocked="Blocked"
        onPress={none}
      />
      <ParticipantTile
        name="Someone with a display name far too long for a tile"
        size="stage"
        role="Co-host"
        onPress={none}
      />
    </ParticipantGrid>
  );
}

const listenerNames = [
  "Yui",
  "Kenji",
  "Nao",
  "Emi",
  "Taro",
  "Saki",
  "Riku",
  "Hana",
  "Daichi",
  "Mei",
  "Shun",
  "Kana",
  "Yuto",
  "Rin",
];

/** The listeners: a hand raised, a co-host who listens, one the reader blocks, and the rest. */
function Listeners() {
  return (
    <ParticipantGrid label="Listeners" size="listener">
      {listenerNames.map((name, i) => (
        <ParticipantTile
          key={name}
          name={name}
          src={[images.dusk, images.snow, images.logo, undefined][i % 4]}
          size="listener"
          role={i === 2 ? "Co-host" : undefined}
          hand={i === 0 || i === 5 ? "Hand raised" : undefined}
          blocked={i === 7 ? "Blocked" : undefined}
          onPress={none}
        />
      ))}
    </ParticipantGrid>
  );
}

const sent: readonly (() => ReactNode)[] = [
  () => "👏",
  () => "😂",
  () => custom("sparkle"),
  () => "❤️",
  () => custom("party"),
  () => "🔥",
  () => "🙌",
];

/** Reactions as the room's stream brings them: one every so often, each gone once it has risen. */
function useReactions(every: number): [readonly Reaction[], (id: string) => void] {
  const [reactions, setReactions] = useState<readonly Reaction[]>(() =>
    [0, 1, 2].map((i) => ({ id: `r${String(i)}`, emoji: sent[i % sent.length]?.() })),
  );
  const next = useRef(3);
  useEffect(() => {
    const timer = setInterval(() => {
      const n = next.current++;
      setReactions((rs) => [...rs, { id: `r${String(n)}`, emoji: sent[n % sent.length]?.() }]);
    }, every);
    return () => {
      clearInterval(timer);
    };
  }, [every]);
  return [
    reactions,
    (id) => {
      setReactions((rs) => rs.filter((r) => r.id !== id));
    },
  ];
}

/** The reader's controls: a speaker's microphone, or a listener's hand, then a reaction and leaving. */
function Controls({ speaker, on = false }: { speaker: boolean; on?: boolean }) {
  const [pressed, setPressed] = useState(on);
  const toggle = () => {
    setPressed((p) => !p);
  };
  return (
    <RoomControls label="Room controls" end={<RoomControl icon="leave" label="Leave" onPress={none} />}>
      {speaker ? (
        <RoomControl icon="microphone" label="Mute" pressed={pressed} onPress={toggle} />
      ) : (
        <RoomControl icon="hand" label="Raise hand" pressed={pressed} onPress={toggle} />
      )}
      <RoomControl icon="react" label="React" onPress={none} />
    </RoomControls>
  );
}

/** The room: its name, its stage under the reactions, its listeners, and the reader's controls. */
function Room({ speaker }: { speaker: boolean }) {
  const [reactions, done] = useReactions(700);
  return (
    <Column>
      <Stack gap="entry-room">
        <ListTitle glyph="☕" count="20">
          Morning coffee chat
        </ListTitle>
        <Stack gap="entry-line">
          <GroupHeading>Speakers</GroupHeading>
          <ReactionFlow reactions={reactions} onDone={done}>
            <Stage />
          </ReactionFlow>
        </Stack>
        <Stack gap="entry-line">
          <GroupHeading>Listeners</GroupHeading>
          <Listeners />
        </Stack>
        <Controls speaker={speaker} />
      </Stack>
    </Column>
  );
}

// Tagged `themes`: the check of every story shows it in dark too.
const meta = { title: "Voice chat", tags: ["themes"] } satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** The whole room, the reader on its stage: the speakers under the reactions, the listeners, a speaker's controls. */
export const RoomAsSpeaker: Story = { render: () => <Room speaker /> };

/** The whole room, the reader among its listeners: a listener's controls. */
export const RoomAsListener: Story = { render: () => <Room speaker={false} /> };

/**
 * The stage's tiles: the host speaking (ringed, its halo swelling; still with reduced motion), a co-host with the
 * microphone off, a speaker without an image who speaks, one the reader blocks, and a name cut to its tile.
 */
export const StageTiles: Story = {
  render: () => (
    <Column>
      <Stage />
    </Column>
  ),
};

/** The listeners' compact tiles: two hands raised, a co-host who listens, one the reader blocks. */
export const ListenerTiles: Story = {
  render: () => (
    <Column>
      <Listeners />
    </Column>
  ),
};

/** Reactions rising over the stage from its end, Unicode and custom emoji; with reduced motion they fade in place. */
export const Reactions: Story = {
  render: function Render() {
    const [reactions, done] = useReactions(500);
    return (
      <Column>
        <ReactionFlow reactions={reactions} onDone={done}>
          <Stage />
        </ReactionFlow>
      </Column>
    );
  },
};

/**
 * Rooms as lines of a list: with an icon emoji and the faces of who speaks; a custom emoji; none, the room's glyph in
 * its place; a name long enough to wrap; and a room no one speaks in yet.
 */
export const RoomLines: Story = {
  render: () => (
    <Column>
      <RowList label="Voice chat rooms">
        <RowItem>
          <RoomLine
            href="/voice/1"
            icon="☕"
            name="Morning coffee chat"
            caption="Hosted by Mika · 20 in the room"
            speakers={[
              { name: "Mika", src: images.dusk },
              { name: "Ren", src: images.snow },
              { name: "Sora", src: images.logo },
              { name: "Aoi" },
            ]}
          />
        </RowItem>
        <RowItem>
          <RoomLine
            href="/voice/2"
            icon={custom("party")}
            name="Release party for v0.2"
            caption="Hosted by Ren · 48 in the room"
            speakers={[{ name: "Ren", src: images.snow }]}
          />
        </RowItem>
        <RowItem>
          <RoomLine
            href="/voice/3"
            name="A room with a name long enough to wrap onto a second line, and then to be cut at the end of it"
            caption="Hosted by Sora with a display name that is long too · 3 in the room"
            speakers={[
              { name: "Sora", src: images.logo },
              { name: "Mika", src: images.dusk },
            ]}
          />
        </RowItem>
        <RowItem>
          <RoomLine href="/voice/4" icon="🎧" name="Quiet listening" caption="Hosted by Aoi · 1 in the room" />
        </RowItem>
      </RowList>
    </Column>
  ),
};

/** A speaker's controls: the microphone on, then muted (knocked out, crossed out), a reaction, and leaving apart. */
export const SpeakerControls: Story = {
  render: () => (
    <Column>
      <Stack gap="entry-room">
        <Controls speaker />
        <Controls speaker on />
      </Stack>
    </Column>
  ),
};

/** A listener's controls: a hand to raise, then raised (knocked out), a reaction, and leaving apart. */
export const ListenerControls: Story = {
  render: () => (
    <Column>
      <Stack gap="entry-room">
        <Controls speaker={false} />
        <Controls speaker={false} on />
      </Stack>
    </Column>
  ),
};

/** The controls while one cannot be pressed (the microphone while the call connects). */
export const ControlsDisabled: Story = {
  render: () => (
    <Column>
      <Box>
        <RoomControls label="Room controls" end={<RoomControl icon="leave" label="Leave" onPress={none} />}>
          <RoomControl icon="microphone" label="Mute" pressed={false} isDisabled />
          <RoomControl icon="react" label="React" onPress={none} />
        </RoomControls>
      </Box>
    </Column>
  ),
};
