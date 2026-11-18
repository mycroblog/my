// SPDX-License-Identifier: AGPL-3.0-only

// The table of channels (ADR-0110 §5), in a section of a settings screen, in the reader's language (the toolbar's
// locale; ja, else en): with every channel, where a press changes a cell and the first row a whole column, among rows
// that are fixed, turned off in the app, without a channel, or long-named; its cells saving, saved and not saved; with
// two channels alone; and in a frame as narrow as the minimum width, where a kind's checkboxes stand under its name.

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState, type ReactNode } from "react";
import { useLocale } from "react-aria-components";
import { ChannelCell, ChannelGap, ChannelRow, ChannelTable, type ChannelColumn } from "./channels";
import { SaveState } from "./save-state";
import { SettingsSection } from "./settings";

const en = {
  heading: "From other people",
  kind: "Notification",
  all: "All",
  channels: { app: "In the app", push: "Push", email: "Email", sound: "Sound" },
  kinds: {
    mention: "Mentions",
    follow: "New followers",
    boost: "Boosts",
    favourite: "Favourites",
    posts: "New posts",
    closure: "Closure of this server",
    long: "Scheduled posts that could not be published at the time you chose",
  },
  off: "“In the app” is off, so it does not arrive any other way.",
  fixed: "Always arrives in the app and by email.",
};

const ja: typeof en = {
  heading: "ほかの人から",
  kind: "通知",
  all: "すべて",
  channels: { app: "アプリ内", push: "プッシュ", email: "メール", sound: "音" },
  kinds: {
    mention: "メンション",
    follow: "新しいフォロワー",
    boost: "ブースト",
    favourite: "お気に入り",
    posts: "新しい投稿",
    closure: "このサーバーの閉鎖",
    long: "指定した時刻に公開できなかった予約投稿",
  },
  off: "「アプリ内」がオフなので、ほかの方法でも届きません。",
  fixed: "アプリ内とメールには必ず届きます。",
};

const useWords = () => (new Intl.Locale(useLocale().locale).language === "ja" ? ja : en);

type Words = typeof en;
type Channel = keyof Words["channels"];
type Kind = keyof Words["kinds"];

const every: readonly Channel[] = ["app", "push", "email", "sound"];

/** A column of the main column's width, as a settings screen's; `narrow`, of the minimum width's. */
function Column({ narrow = false, children }: { narrow?: boolean; children: ReactNode }) {
  return (
    <div
      className={`grid gap-shell-edge p-shell-edge text-body ${narrow ? "box-border w-[20rem] max-w-full" : "max-w-[37.5rem]"}`}
    >
      {children}
    </div>
  );
}

const columnsOf = (w: Words, channels: readonly Channel[]): ChannelColumn[] =>
  channels.map((id) => ({ id, label: w.channels[id] }));

/** The kinds of the specimen: the closure's cells are the server's, and new posts are never emailed. */
const kinds: readonly Kind[] = ["mention", "follow", "boost", "favourite", "posts", "closure", "long"];
const fixedOf = (kind: Kind, channel: Channel) => kind === "closure" && (channel === "app" || channel === "email");
const hasCell = (kind: Kind, channel: Channel) => !(kind === "posts" && channel === "email");

/** A table that keeps its cells itself: a press changes one, and the first row a whole column. */
function Live({ channels, narrow = false }: { channels: readonly Channel[]; narrow?: boolean }) {
  const w = useWords();
  const [on, setOn] = useState<ReadonlySet<string>>(
    () => new Set(["mention/app", "mention/push", "follow/app", "boost/app", "boost/sound", "posts/app", "long/app"]),
  );
  const made = (kind: Kind) => fixedOf(kind, "app") || on.has(`${kind}/app`);
  const shown = (kind: Kind, channel: Channel) =>
    fixedOf(kind, channel) || ((channel === "app" || made(kind)) && on.has(`${kind}/${channel}`));
  const open = (kind: Kind, channel: Channel) =>
    hasCell(kind, channel) && !fixedOf(kind, channel) && (channel === "app" || made(kind));
  const set = (cells: readonly string[], next: boolean) => {
    setOn((now) => new Set([...[...now].filter((cell) => !cells.includes(cell)), ...(next ? cells : [])]));
  };
  return (
    <Column narrow={narrow}>
      <SettingsSection heading={w.heading}>
        <ChannelTable label={w.heading} kind={w.kind} columns={columnsOf(w, channels)}>
          <ChannelRow all label={w.all}>
            {channels.map((channel) => {
              const changeable = kinds.filter((kind) => open(kind, channel));
              const count = changeable.filter((kind) => shown(kind, channel)).length;
              return (
                <ChannelCell
                  key={channel}
                  column={channel}
                  isSelected={changeable.length > 0 && count === changeable.length}
                  isMixed={count > 0 && count < changeable.length}
                  isDisabled={changeable.length === 0}
                  onChange={(next) => {
                    set(
                      changeable.map((kind) => `${kind}/${channel}`),
                      next,
                    );
                  }}
                  save={null}
                />
              );
            })}
          </ChannelRow>
          {kinds.map((kind) => (
            <ChannelRow
              key={kind}
              label={w.kinds[kind]}
              reason={!made(kind) ? w.off : kind === "closure" ? w.fixed : undefined}
            >
              {channels.map((channel) =>
                hasCell(kind, channel) ? (
                  <ChannelCell
                    key={channel}
                    column={channel}
                    isSelected={shown(kind, channel)}
                    isDisabled={!open(kind, channel)}
                    onChange={(next) => {
                      set([`${kind}/${channel}`], next);
                    }}
                    save={null}
                  />
                ) : (
                  <ChannelGap key={channel} />
                ),
              )}
            </ChannelRow>
          ))}
        </ChannelTable>
      </SettingsSection>
    </Column>
  );
}

/** The save state of a cell, in its row's line: saving, saved, not saved, and beside a reason it takes the place of. */
function Saving({ narrow = false }: { narrow?: boolean }) {
  const w = useWords();
  const channels: readonly Channel[] = ["app", "email"];
  // The words of each state are the design system's own.
  const rows: readonly { kind: Kind; save: ReactNode; reason?: string; off?: boolean }[] = [
    { kind: "mention", save: null },
    { kind: "follow", save: <SaveState state="saving" /> },
    { kind: "boost", save: <SaveState state="saved" /> },
    { kind: "favourite", save: <SaveState state="failed" onRetry={() => undefined} /> },
    { kind: "long", save: <SaveState state="saved" /> },
    { kind: "posts", save: <SaveState state="saving" />, reason: w.off, off: true },
  ];
  return (
    <Column narrow={narrow}>
      <SettingsSection heading={w.heading}>
        <ChannelTable label={w.heading} kind={w.kind} columns={columnsOf(w, channels)}>
          {rows.map(({ kind, save, reason, off = false }) => (
            <ChannelRow key={kind} label={w.kinds[kind]} reason={reason}>
              <ChannelCell column="app" isSelected={!off} onChange={() => undefined} save={off ? save : null} />
              <ChannelCell
                column="email"
                isSelected={!off}
                isDisabled={off}
                onChange={() => undefined}
                save={off ? null : save}
              />
            </ChannelRow>
          ))}
        </ChannelTable>
      </SettingsSection>
    </Column>
  );
}

// Tagged `themes`: the check of every story shows it in dark too.
const meta = { title: "Channel table", tags: ["themes"] } satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** Every channel: a press changes a cell, the first row a column; a fixed row, one without email, a long name. */
export const EveryChannel: Story = { render: () => <Live channels={every} /> };
/** Two channels alone, as on a device without push and with the sound off. */
export const TwoChannels: Story = { render: () => <Live channels={["app", "email"]} /> };
/** A cell saving, saved and not saved, in its row's line; and a state in the place of a row's reason. */
export const SaveStatesOfACell: Story = { render: () => <Saving /> };
/** In a frame of the minimum width (320 px): a kind's checkboxes under its name, each over its channel's name. */
export const AtTheMinimumWidth: Story = { render: () => <Live channels={every} narrow /> };
/** The save states in a frame of the minimum width: beside the kind's name, or under it where they do not fit. */
export const SaveStatesAtTheMinimumWidth: Story = { render: () => <Saving narrow /> };
