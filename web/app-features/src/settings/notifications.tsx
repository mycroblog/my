// SPDX-License-Identifier: AGPL-3.0-only

// Notifications, the entry of the settings at /settings/notifications (FR-210, ADR-0086 §8–§13, §18–§22, ADR-0158
// §10): how the reader's notifications arrive.
//
// - This device: Web Push on or off for this device, by the path the prompt over the notifications takes (the
//   browser's permission as the switch is pressed, a subscription to every kind the reader gets, and the prompt's closed
//   record kept when it is turned off and taken away when it is turned on); where it cannot be turned on, the switch
//   is disabled and says why (the server sends none, this browser has none or needs the site on the Home Screen, the
//   browser blocks the site's notifications). And the notification sound, off until the reader turns it on, which
//   plays it once so that the browser lets the page sound from then on, with a button that plays it.
// - The kinds by channel, a table a section of the server's rows (from other people, for moderators, from the
//   server): in the app, Web Push while it is on for this device, email on a server that sends it, and sound while
//   it is on. A cell saves as it is pressed and says so in its row; the first row turns a whole column on or off. A
//   cell the server fixes is on and disabled, and a row turned off in the app, whose notifications are not made, has
//   its other cells off and disabled; the row says why. Above them, when the server stopped sending email to the
//   reader's address, why, and after a bounce the action that sends the verification email whose link lifts it.
// - The policy for notifications from people the reader may not know: for each of its five conditions, whether such
//   a notification arrives, is held back, or never arrives, with what each does and the way to the held-back ones.
//
// What the server keeps (the app's cells, email's, the policy) and the subscription's kinds show the reader's choice
// from the press until the server's answer; the sound is this device's alone.

import { heldPath } from "@my/app-components";
import {
  instanceQuery,
  notificationPolicyQuery,
  notificationSettingsQuery,
  preferencesQuery,
  pushSubscriptionQuery,
  setNotificationSound,
  setNotificationSoundKinds,
  useChangeNotificationPolicy,
  useChangeNotificationSettings,
  useInstance,
  useNotificationKinds,
  useNotificationPolicy,
  useNotificationSettings,
  useNotificationSettingsRead,
  useNotificationSound,
  useNotificationSoundKinds,
  usePreferences,
  usePushSwitch,
  useResendVerificationEmail,
  viewerQuery,
  type AppData,
  type NotificationChannel,
  type NotificationGroup,
  type NotificationPolicy,
  type NotificationPolicyAction,
  type NotificationSettingRow,
  type NotificationSettings as Settings,
  type Preferences,
  type PushSwitch,
} from "@my/app-data";
import {
  Button,
  ChannelCell,
  ChannelGap,
  ChannelRow,
  ChannelTable,
  Row,
  SaveState,
  SettingsFacts,
  SettingsList,
  SettingsListRow,
  SettingsScreen,
  SettingsSection,
  StatusSummary,
  Switch,
  useLocale,
} from "@my/design-system";
import { pushNeedsInstall, trySound } from "@my/platform";
import { useState } from "react";
import { stringsIn } from "../locale";
import type { MessageId } from "../messages.gen";
import { Choice, ReadingNotice, Toggle } from "./controls";
import type { SettingsEntryProps } from "./entries";
import { Said, useSaving } from "./saving";

type T = ReturnType<typeof stringsIn>["t"];

/**
 * What the route's loader waits for: the rows of the settings, the policy, the grant's subscription, the reader's
 * preferences, their account and the instance, from the cache when it has them.
 */
export async function loadNotificationSettings(data: AppData): Promise<void> {
  await Promise.all([
    data.queryClient.query({ ...notificationSettingsQuery(data), staleTime: Infinity }),
    data.queryClient.query({ ...notificationPolicyQuery(data), staleTime: Infinity }),
    data.queryClient.query({ ...pushSubscriptionQuery(data), staleTime: Infinity }),
    data.queryClient.query({ ...preferencesQuery(data), staleTime: Infinity }),
    data.queryClient.query({ ...viewerQuery(data), staleTime: Infinity }),
    data.queryClient.query({ ...instanceQuery(data), staleTime: Infinity }),
  ]).catch(() => undefined);
}

/** The address of the entry's page, which a change that could not be said in place leads back to. */
const page = "/settings/notifications";

// ---- This device ---------------------------------------------------------------------------------------------------

/** Why Web Push cannot be turned on for this device; undefined where it can. */
function pushRefusal(serves: boolean, push: PushSwitch, t: T): string | undefined {
  if (!serves) return t("settings.notifications.push.no_server");
  if (!push.supported)
    return t(pushNeedsInstall() ? "settings.notifications.push.install" : "settings.notifications.push.no_browser");
  if (push.permission === "denied") return t("notifications.push_blocked");
  return undefined;
}

/**
 * Web Push for this device, on or off, as the prompt over the notifications turns it on: every kind the reader gets.
 * Where it cannot be turned on, it is off and disabled, and the line under it says why: from the moment the reader
 * refuses the browser's question too, since the browser blocks the site from then on. Where the browser tells that
 * they allowed the site again in its settings, the switch is theirs again without a reload, and what it said of the
 * refusal is gone (`Toggle`).
 */
function PushToggle({ push, refusal, t }: { push: PushSwitch; refusal: string | undefined; t: T }) {
  const kinds = useNotificationKinds();
  return (
    <Toggle
      page={page}
      label={t("settings.notifications.push")}
      about={t("notifications.push_offer")}
      on={refusal === undefined && push.subscription !== null && push.subscription !== undefined}
      fixed={refusal}
      onChange={async (on) => {
        if (!on) {
          await push.turnOff();
          return;
        }
        const ended = await push.turnOn(kinds ?? []);
        if (ended === "blocked") throw new Said(t("notifications.push_blocked"));
        if (ended === "closed") throw new Said(t("settings.notifications.push.closed"));
        if (ended === "failed") throw new Said(t("notifications.push_failed"));
      }}
    />
  );
}

/**
 * The notification sound of this device: on or off, and the button that plays it. Turning it on plays it once, a
 * press that lets the browser sound the page from then on; where the browser did not play it, the line under the
 * switch says so.
 */
function Sound({ t }: { t: T }) {
  const on = useNotificationSound();
  const label = t("settings.notifications.sound");
  const { state, save } = useSaving(label, page);
  const [silent, setSilent] = useState(false);
  /** Plays the sound as the reader pressed for it, and keeps whether the browser stayed silent. */
  const play = async () => {
    setSilent(!(await trySound()));
  };
  return (
    <>
      <Switch
        label={label}
        description={t("settings.notifications.sound.about")}
        isSelected={on}
        onChange={(next) => {
          setSilent(false);
          save(async () => {
            setNotificationSound(next);
            // Turning it on plays it once, on purpose: a browser lets a page sound only after a press of its reader
            // started the page's audio (the platform's `trySound`), and this is the press the reader makes for the
            // sound. It also tells them here, and not at the first notification, that the browser keeps the page silent.
            if (next) await play();
          });
        }}
        save={silent ? <SaveState state="failed">{t("settings.notifications.sound.blocked")}</SaveState> : state}
      />
      <Row justify="start">
        <Button
          size="timeline"
          onPress={() => {
            void play();
          }}
        >
          {t("settings.notifications.sound.try")}
        </Button>
      </Row>
    </>
  );
}

// ---- The kinds by channel ------------------------------------------------------------------------------------------

/** A channel a kind of notifications arrives through, a column of the tables. */
type Channel = NotificationChannel | "push" | "sound";

const channelName = {
  in_app: "settings.notifications.channel.in_app",
  push: "settings.notifications.channel.push",
  email: "settings.notifications.channel.email",
  sound: "settings.notifications.channel.sound",
} as const satisfies Record<Channel, MessageId>;

/**
 * The name of each row the server may list. (The key of the scheduled posts' row does not end in the row's own
 * `failed`, by which the catalog's check tells an error's message.)
 */
const kindName = {
  mention: "settings.notifications.kind.mention",
  follow: "settings.notifications.kind.follow",
  follow_request: "settings.notifications.kind.follow_request",
  follow_accepted: "settings.notifications.kind.follow_accepted",
  boost: "settings.notifications.kind.boost",
  favourite: "settings.notifications.kind.favourite",
  reaction: "settings.notifications.kind.reaction",
  quote: "settings.notifications.kind.quote",
  interaction_request: "settings.notifications.kind.interaction_request",
  poll_closed: "settings.notifications.kind.poll_closed",
  account_moved: "settings.notifications.kind.account_moved",
  voice: "settings.notifications.kind.voice",
  event: "settings.notifications.kind.event",
  status: "settings.notifications.kind.status",
  admin_report: "settings.notifications.kind.admin_report",
  admin_sign_up: "settings.notifications.kind.admin_sign_up",
  admin_appeal: "settings.notifications.kind.admin_appeal",
  admin_webhook_disabled: "settings.notifications.kind.admin_webhook_disabled",
  admin_hook_failing: "settings.notifications.kind.admin_hook_failing",
  admin_hook_appeal: "settings.notifications.kind.admin_hook_appeal",
  admin_domain_list_failing: "settings.notifications.kind.admin_domain_list_failing",
  admin_custom_emoji_request: "settings.notifications.kind.admin_custom_emoji_request",
  admin_account_moved: "settings.notifications.kind.admin_account_moved",
  admin_service_closure_due: "settings.notifications.kind.admin_service_closure_due",
  admin_error_group: "settings.notifications.kind.admin_error_group",
  moderation: "settings.notifications.kind.moderation",
  service_closure: "settings.notifications.kind.service_closure",
  supporter: "settings.notifications.kind.supporter",
  announcement: "settings.notifications.kind.announcement",
  auto_delete_limit: "settings.notifications.kind.auto_delete_limit",
  scheduled_status_failed: "settings.notifications.kind.unpublished_scheduled_status",
} as const satisfies Record<NotificationGroup, MessageId>;

/** The sections of the server's rows, in the screen's order, each under its heading. */
const sections = [
  { id: "ordinary", heading: "settings.notifications.section.ordinary" },
  { id: "moderator", heading: "settings.notifications.section.moderator" },
  { id: "instance", heading: "settings.notifications.section.instance" },
] as const satisfies readonly { id: NotificationSettingRow["section"]; heading: MessageId }[];

const cellKey = (row: NotificationSettingRow, channel: Channel) => `${row.group}/${channel}`;

/**
 * The cells the reader chose that are on their way to where they are kept, shown in place of what is held there until
 * it answers: `chosen` of a cell, and `show`, which shows `on` in the cells of `keys` while `write` runs.
 */
function useAhead() {
  const [ahead, setAhead] = useState<ReadonlyMap<string, { readonly on: boolean }>>(new Map());
  return {
    chosen: (key: string): boolean | undefined => ahead.get(key)?.on,
    show: (keys: readonly string[], on: boolean, write: () => Promise<unknown>): Promise<unknown> => {
      const own = { on };
      setAhead((now) => new Map([...now, ...keys.map((key) => [key, own] as const)]));
      // Once it answered, what is held shows: the choice, or what was there before it. A write that cannot start
      // (the switch is not ready) is one that failed.
      return Promise.resolve()
        .then(write)
        .finally(() => {
          setAhead((now) => new Map([...now].filter(([, cell]) => cell !== own)));
        });
    },
  };
}

/** What the tables read of where the channels' choices are kept, and how they change them. */
type Cells = {
  /** The channels shown, in the columns' order. */
  readonly channels: readonly Channel[];
  /** Whether the row has a cell of the channel: a kind that is never emailed has none of email. */
  readonly has: (row: NotificationSettingRow, channel: Channel) => boolean;
  /** Whether the server keeps the cell on for everyone. */
  readonly fixed: (row: NotificationSettingRow, channel: Channel) => boolean;
  /** Whether the row's notifications are made at all (the app's cell), as the reader last chose. */
  readonly made: (row: NotificationSettingRow) => boolean;
  /** Whether the cell shows on: what is in effect, so off in a row whose notifications are not made. */
  readonly on: (row: NotificationSettingRow, channel: Channel) => boolean;
  /** Whether the reader may change the cell now. */
  readonly open: (row: NotificationSettingRow, channel: Channel) => boolean;
  /** Turns the channel on or off for `rows`, and resolves once that is kept. */
  readonly choose: (channel: Channel, rows: readonly NotificationSettingRow[], on: boolean) => Promise<unknown>;
};

/** The cells of the tables: the server's, the subscription's kinds and the device's sound, with what is on its way. */
function useCells(channels: readonly Channel[], push: PushSwitch): Cells {
  const change = useChangeNotificationSettings();
  const sounding = useNotificationSoundKinds();
  const ahead = useAhead();
  const pushed = push.subscription?.alerts ?? [];
  const kept = (row: NotificationSettingRow, channel: Channel): boolean => {
    if (channel === "in_app") return row.in_app.enabled;
    if (channel === "email") return row.email?.enabled === true;
    const kinds = channel === "push" ? pushed : sounding;
    return row.types.every((type) => kinds.includes(type));
  };
  const has: Cells["has"] = (row, channel) => channel !== "email" || row.email !== undefined;
  const fixed: Cells["fixed"] = (row, channel) =>
    channel === "in_app" ? row.in_app.fixed : channel === "email" && row.email?.fixed === true;
  const chosen = (row: NotificationSettingRow, channel: Channel) =>
    ahead.chosen(cellKey(row, channel)) ?? kept(row, channel);
  const made: Cells["made"] = (row) => row.in_app.fixed || chosen(row, "in_app");
  return {
    channels,
    has,
    fixed,
    made,
    on: (row, channel) => fixed(row, channel) || ((channel === "in_app" || made(row)) && chosen(row, channel)),
    open: (row, channel) => has(row, channel) && !fixed(row, channel) && (channel === "in_app" || made(row)),
    choose: (channel, rows, on) =>
      ahead.show(
        rows.map((row) => cellKey(row, channel)),
        on,
        () => {
          const kinds = rows.flatMap((row) => row.types);
          if (channel === "push") return push.changeKinds(kinds, on);
          if (channel === "sound") {
            setNotificationSoundKinds(kinds, on);
            return Promise.resolve();
          }
          return change(rows.map((row) => ({ group: row.group, channel, enabled: on })));
        },
      ),
  };
}

/** A cell of a table: it saves as it is pressed, and its row says where the change is. */
function Cell({
  column,
  setting,
  on,
  mixed = false,
  disabled,
  onChange,
}: {
  column: Channel;
  /** The name a change that fails after the reader left the page is told by: its kind and its channel. */
  setting: string;
  on: boolean;
  mixed?: boolean;
  disabled: boolean;
  onChange: (on: boolean) => Promise<unknown>;
}) {
  const { state, save } = useSaving(setting, page);
  return (
    <ChannelCell
      column={column}
      isSelected={on}
      isMixed={mixed}
      isDisabled={disabled}
      onChange={(next) => {
        save(() => onChange(next));
      }}
      save={state}
    />
  );
}

/** Why the disabled cells of a row cannot be changed; undefined for a row with none. */
function reasonOf(row: NotificationSettingRow, cells: Cells, t: T): string | undefined {
  if (!cells.made(row)) return t("settings.notifications.reason.in_app_off");
  const always = cells.channels.filter((channel) => cells.has(row, channel) && cells.fixed(row, channel));
  if (always.length === 0) return undefined;
  return t("settings.notifications.reason.fixed", {
    channels: always.length > 1 ? "both" : always[0] === "email" ? "email" : "in_app",
  });
}

/**
 * The table of a section's kinds: the row that turns each channel on or off for all of them that the reader may
 * change, then a row a kind.
 */
function Kinds({
  heading,
  rows,
  cells,
  t,
}: {
  heading: string;
  rows: readonly NotificationSettingRow[];
  cells: Cells;
  t: T;
}) {
  const all = t("settings.notifications.all");
  return (
    <ChannelTable
      label={heading}
      kind={t("settings.notifications.column.kind")}
      columns={cells.channels.map((id) => ({ id, label: t(channelName[id]) }))}
    >
      <ChannelRow all label={all}>
        {cells.channels.map((channel) => {
          const open = rows.filter((row) => cells.open(row, channel));
          const on = open.filter((row) => cells.on(row, channel)).length;
          return (
            <Cell
              key={channel}
              column={channel}
              setting={t("settings.notifications.cell", { kind: all, channel: t(channelName[channel]) })}
              on={open.length > 0 && on === open.length}
              mixed={on > 0 && on < open.length}
              disabled={open.length === 0}
              onChange={(next) => cells.choose(channel, open, next)}
            />
          );
        })}
      </ChannelRow>
      {rows.map((row) => {
        const kind = t(kindName[row.group]);
        return (
          <ChannelRow key={row.group} label={kind} reason={reasonOf(row, cells, t)}>
            {cells.channels.map((channel) =>
              cells.has(row, channel) ? (
                <Cell
                  key={channel}
                  column={channel}
                  setting={t("settings.notifications.cell", { kind, channel: t(channelName[channel]) })}
                  on={cells.on(row, channel)}
                  disabled={!cells.open(row, channel)}
                  onChange={(next) => cells.choose(channel, [row], next)}
                />
              ) : (
                <ChannelGap key={channel} />
              ),
            )}
          </ChannelRow>
        );
      })}
    </ChannelTable>
  );
}

// ---- The policy ----------------------------------------------------------------------------------------------------

/** The conditions of the policy, in the screen's order, each with its name and the line that says what it covers. */
const conditions = [
  { id: "not_following", name: "settings.notifications.policy.not_following", about: undefined },
  { id: "not_follower", name: "settings.notifications.policy.not_follower", about: undefined },
  {
    id: "new_account",
    name: "settings.notifications.policy.new_account",
    about: "settings.notifications.policy.new_account.about",
  },
  {
    id: "unsolicited_private_mention",
    name: "settings.notifications.policy.unsolicited_private_mention",
    about: "settings.notifications.policy.unsolicited_private_mention.about",
  },
  {
    id: "limited_account",
    name: "settings.notifications.policy.limited_account",
    about: "settings.notifications.policy.limited_account.about",
  },
] as const satisfies readonly { id: keyof NotificationPolicy; name: MessageId; about: MessageId | undefined }[];

/** What the policy may do with a notification, from the mildest. */
const actions = ["accept", "filter", "drop"] as const satisfies readonly NotificationPolicyAction[];

/** What each action does with a notification, the line under its name. */
const actionAbout = {
  accept: "settings.notifications.policy.about.accept",
  filter: "settings.notifications.policy.about.filter",
  drop: "settings.notifications.policy.about.drop",
} as const satisfies Record<NotificationPolicyAction, MessageId>;

/**
 * The policy: what each of its three actions does, stated once; a choice a condition; and the way to the
 * notifications it holds back.
 */
function Policy({ policy, t }: { policy: NotificationPolicy; t: T }) {
  const change = useChangeNotificationPolicy();
  const options = actions.map((action) => ({
    value: action,
    label: t("settings.notifications.policy.action", { action }),
  }));
  return (
    <SettingsSection heading={t("settings.notifications.policy")}>
      <SettingsFacts
        label={t("settings.notifications.policy.actions")}
        items={actions.map((action) => ({
          text: t("settings.notifications.policy.action", { action }),
          about: [t(actionAbout[action])],
        }))}
      />
      {conditions.map((condition) => (
        <Choice
          key={condition.id}
          page={page}
          label={t(condition.name)}
          about={condition.about === undefined ? undefined : t(condition.about)}
          options={options}
          value={policy[condition.id]}
          onChoose={(value) => {
            const action = actions.find((a) => a === value);
            return action === undefined ? Promise.resolve() : change({ [condition.id]: action });
          }}
        />
      ))}
      <SettingsList label={t("notifications.held")}>
        <SettingsListRow icon="followers" title={t("notifications.held")} href={heldPath} />
      </SettingsList>
    </SettingsSection>
  );
}

// ---- The stopped address -------------------------------------------------------------------------------------------

/**
 * What came of the last press of the summary's action; a number is the wait the server asks for before it sends
 * another email, in whole minutes, 0 when it did not say how long.
 */
type Sending = "sending" | "sent" | "failed" | number;

/** A wait in whole minutes, rounded up as the server's own pages round it; 0 for one the server did not say. */
const minutesOf = (waitMs: number | undefined) => (waitMs === undefined ? 0 : Math.max(1, Math.ceil(waitMs / 60_000)));

/**
 * Why the server stopped sending email to the reader's address. After a bounce, its action sends the verification
 * email whose link lifts the stop, and what came of it stands beside it, until the next press; after a complaint
 * there is no such step.
 */
function EmailStopped({ kind, t }: { kind: NonNullable<Preferences["email_suppression"]>["kind"]; t: T }) {
  const resend = useResendVerificationEmail();
  const [sending, setSending] = useState<Sending | null>(null);
  const send = () => {
    setSending("sending");
    resend().then(
      (answer) => {
        setSending(answer === "sent" ? "sent" : minutesOf(answer.waitMs));
      },
      () => {
        setSending("failed");
      },
    );
  };
  const result =
    sending === null || sending === "sending" ? null : sending === "sent" ? (
      <SaveState state="saved">{t("settings.notifications.email_stopped.sent")}</SaveState>
    ) : typeof sending === "number" ? (
      // The server's own wait, in the words its sign-up page says it in: no number of the server's is the client's.
      <SaveState state="failed">{t("settings.notifications.email_stopped.wait", { minutes: sending })}</SaveState>
    ) : (
      // The action stays beside it, which is the way to try again.
      <SaveState state="failed">{t("settings.notifications.email_stopped.not_sent")}</SaveState>
    );
  return kind === "hard_bounce" ? (
    <StatusSummary
      icon="caution"
      title={t("settings.notifications.email_stopped")}
      about={t("settings.notifications.email_stopped.about", { kind })}
      action={
        <Button kind="primary" busy={sending === "sending"} onPress={send}>
          {t("settings.notifications.email_stopped.verify")}
        </Button>
      }
      result={result}
    />
  ) : (
    <StatusSummary
      icon="caution"
      title={t("settings.notifications.email_stopped")}
      about={t("settings.notifications.email_stopped.about", { kind })}
    />
  );
}

// ---- The screen ----------------------------------------------------------------------------------------------------

/** The sections of the screen, once what they show is read. */
function Sections({
  settings,
  policy,
  preferences,
}: {
  settings: Settings;
  policy: NotificationPolicy;
  preferences: Preferences;
}) {
  const { t } = stringsIn(useLocale().locale);
  const instance = useInstance();
  const push = usePushSwitch();
  const sound = useNotificationSound();
  const refusal = pushRefusal(instance?.push_server_key !== undefined, push, t);
  const pushing = refusal === undefined && push.subscription !== null && push.subscription !== undefined;
  const emails = instance?.sends_email === true;
  const cells = useCells(
    [
      "in_app",
      ...(pushing ? ["push" as const] : []),
      ...(emails ? ["email" as const] : []),
      ...(sound ? ["sound" as const] : []),
    ],
    push,
  );
  const stopped = emails ? preferences.email_suppression : undefined;
  return (
    <>
      {stopped !== undefined && <EmailStopped kind={stopped.kind} t={t} />}
      <SettingsSection heading={t("settings.notifications.section.device")}>
        <PushToggle push={push} refusal={refusal} t={t} />
        <Sound t={t} />
      </SettingsSection>
      {sections.map((section, i) => {
        const rows = settings.rows.filter((row) => row.section === section.id);
        if (rows.length === 0) return null;
        const heading = t(section.heading);
        return (
          <SettingsSection
            key={section.id}
            heading={heading}
            about={i === 0 ? t("settings.notifications.section.ordinary.about") : undefined}
          >
            <Kinds heading={heading} rows={rows} cells={cells} t={t} />
          </SettingsSection>
        );
      })}
      <Policy policy={policy} t={t} />
    </>
  );
}

/** The screen of the entry: its sections once what they show is read, and until then that it loads or failed. */
export function NotificationSettings({ back }: SettingsEntryProps) {
  const { t } = stringsIn(useLocale().locale);
  const settings = useNotificationSettings();
  const policy = useNotificationPolicy();
  const preferences = usePreferences();
  const { status, retry } = useNotificationSettingsRead();
  const read = status === "read" && settings !== undefined && policy !== undefined && preferences !== undefined;
  return (
    <SettingsScreen
      icon="notifications"
      title={t("settings.entry.notifications")}
      back={back}
      notice={read ? undefined : <ReadingNotice failed={status === "failed"} onRetry={retry} />}
    >
      {read && <Sections settings={settings} policy={policy} preferences={preferences} />}
    </SettingsScreen>
  );
}
