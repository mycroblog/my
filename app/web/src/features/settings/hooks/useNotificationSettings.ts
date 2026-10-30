// SPDX-License-Identifier: AGPL-3.0-only
import { type SaveState, useNotice } from "@my/ui";
import { useMutation, useMutationState, useQuery, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { useState } from "react";
import { useIntl } from "react-intl";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import {
  setNotificationSoundEnabled,
  setNotificationSoundTypes,
  tryNotificationSound,
  useNotificationSound,
} from "../../../lib/notificationSound";
import { isTauriApp } from "../../../lib/platform";
import { saveStateOf } from "./saveState";
import { shellQueryReader } from "../../../lib/query";
import { instanceQueryOptions, type Preferences, preferencesQueryOptions } from "../api/me";
import {
  cacheNotificationSettings,
  type NotificationGroup,
  type NotificationSettingChange,
  type NotificationSettingRow,
  notificationSettingsQueryOptions,
  updateNotificationSettingMutation,
} from "../api/notificationSettings";
import { type PushNotifications, usePushNotifications } from "./usePushNotifications";

/** The name of each row of the notification settings. */
const notificationGroupLabels: Readonly<Record<NotificationGroup, ClientMessageKey>> = {
  mention: "client:settings.notifications.group.mention",
  follow: "client:settings.notifications.group.follow",
  follow_request: "client:settings.notifications.group.follow_request",
  follow_accepted: "client:settings.notifications.group.follow_accepted",
  boost: "client:settings.notifications.group.boost",
  favourite: "client:settings.notifications.group.favourite",
  reaction: "client:settings.notifications.group.reaction",
  quote: "client:settings.notifications.group.quote",
  poll_closed: "client:settings.notifications.group.poll_closed",
  account_moved: "client:settings.notifications.group.account_moved",
  voice: "client:settings.notifications.group.voice",
  status: "client:settings.notifications.group.status",
  admin_report: "client:settings.notifications.group.admin_report",
  admin_sign_up: "client:settings.notifications.group.admin_sign_up",
  admin_appeal: "client:settings.notifications.group.admin_appeal",
  admin_webhook_disabled: "client:settings.notifications.group.admin_webhook_disabled",
  admin_custom_emoji_request: "client:settings.notifications.group.admin_custom_emoji_request",
  admin_account_moved: "client:settings.notifications.group.admin_account_moved",
  admin_service_closure_due: "client:settings.notifications.group.admin_service_closure_due",
  admin_error_group: "client:settings.notifications.group.admin_error_group",
  moderation: "client:settings.notifications.group.moderation",
  service_closure: "client:settings.notifications.group.service_closure",
  supporter: "client:settings.notifications.group.supporter",
};

/** A channel of the table, in the order of its columns. */
export type NotificationChannel = "in_app" | "push" | "email" | "sound";

export const notificationChannelLabels: Readonly<Record<NotificationChannel, ClientMessageKey>> = {
  in_app: "client:settings.notifications.channel.in_app",
  push: "client:settings.notifications.channel.push",
  email: "client:settings.notifications.channel.email",
  sound: "client:settings.notifications.channel.sound",
};

type Section = NotificationSettingRow["section"];

const notificationSectionLabels: Readonly<Record<Section, ClientMessageKey>> = {
  ordinary: "client:settings.notifications.section.ordinary",
  moderator: "client:settings.notifications.section.moderator",
  instance: "client:settings.notifications.section.instance",
};

/** One checkbox of the table: shown on and not operable when it is fixed, not operable when the row's in-app is off. */
type NotificationCell = {
  selected: boolean;
  disabled: boolean;
  onChange: (selected: boolean) => void;
  /**
   * Where the last change of this cell is, for the save mark in the cell (web-client/16-look-states-and-wording.md
   * G24); `idle` for the sound, which this device keeps at once.
   */
  saveState: SaveState;
};

type NotificationTableRow = {
  group: NotificationGroup;
  label: ClientMessageKey;
  /** The cells of the shown channels, by channel. */
  cells: Readonly<Partial<Record<NotificationChannel, NotificationCell>>>;
  /**
   * Why some of the row's cells cannot be changed, localized, shown under the row's name and describing those cells:
   * the in-app notification is off, so nothing else arrives; or the channels that are always on.
   */
  note: string | undefined;
};

/**
 * The checkbox in a column's header that turns the column's cells of the section on or off at once: checked when
 * every cell that can change is on, mixed when some are, and not operable when none can change.
 */
type NotificationColumn = {
  selected: boolean;
  indeterminate: boolean;
  disabled: boolean;
  onChange: (selected: boolean) => void;
};

export type NotificationTableSection = {
  section: Section;
  label: ClientMessageKey;
  rows: readonly NotificationTableRow[];
  /** The column checkbox of each shown channel. */
  columns: Readonly<Partial<Record<NotificationChannel, NotificationColumn>>>;
  /** Why the last change of an in-app or email cell of this section was not saved, localized, under its table. */
  error: string | undefined;
};

export type NotificationSettingsView = {
  /** The channels shown as columns, in order: in the app always, Web Push while this device has it on, email while the server sends it, and the sound while it is on. */
  channels: readonly NotificationChannel[];
  /** The rows by section, in the server's order; a section without rows is left out. */
  sections: readonly NotificationTableSection[];
  /** Web Push on this device: the switch above the table. */
  push: PushNotifications;
  /** The notification sound of this device: the switch of the whole sound, off by default. */
  sound: { enabled: boolean; setEnabled: (enabled: boolean) => void };
  /** Why emails do not reach the primary address, when the email column is shown and they do not. */
  emailSuppression: Preferences["email_suppression"];
};

/** Whether a save has a change of the cell of `group` and `channel`. */
function touchesCell(
  list: readonly NotificationSettingChange[],
  group: NotificationGroup,
  channel: "in_app" | "email",
) {
  return list.some((change) => change.group === group && change.channel === channel);
}

/**
 * Returns the notification settings as a table of the rows the user gets (FR-210, ADR-0109 §1, §2), on a page whose
 * loader has loaded them and the preferences (it suspends until then). The in-app and email cells save themselves at
 * once on the server, and show the value being saved meanwhile (web-client/README.md S8); the Web Push cells register
 * this device's subscription again; the sound cells are kept on this device. A column's checkbox changes the cells of
 * its section that can change, in one request. The supporter row is left out in the Tauri app, which shows nothing of
 * the supporter plans (ADR-0072 §14).
 */
export function useNotificationSettings(): NotificationSettingsView {
  const api = useApi();
  const queryClient = useQueryClient();
  const notice = useNotice();
  const t = useT();
  const intl = useIntl();
  const settings = useSuspenseQuery(notificationSettingsQueryOptions(api)).data;
  const preferences = useSuspenseQuery(preferencesQueryOptions(api)).data;
  const instance = useQuery({ ...instanceQueryOptions(api), ...shellQueryReader });
  const sound = useNotificationSound();
  const options = updateNotificationSettingMutation(api);
  const mutation = useMutation({
    ...options,
    onSuccess: (saved) => {
      cacheNotificationSettings(api, queryClient, saved);
      // The save mark of the cell shows it.
      notice.inPlace(t("client:settings.saved"));
    },
  });
  // Every change being saved, so that clicking several cells in a row shows them all.
  const pending = useMutationState({
    filters: { mutationKey: options.mutationKey, status: "pending" },
    select: (m) => m.state.variables as readonly NotificationSettingChange[],
  }).flat();
  // Every save since the page opened, the last one last, for the save mark of each cell.
  const saves = useMutationState({
    filters: { mutationKey: options.mutationKey },
    select: (m) => ({ changes: m.state.variables as readonly NotificationSettingChange[], status: m.state.status }),
  });
  const cellSaveState = (row: NotificationSettingRow, channel: "in_app" | "email"): SaveState => {
    const last = saves.findLast((save) => touchesCell(save.changes, row.group, channel));
    if (last === undefined) return "idle";
    return saveStateOf({
      isPending: last.status === "pending",
      isSuccess: last.status === "success",
      isError: last.status === "error",
    });
  };

  const rows = settings.rows.filter((row) => row.group !== "supporter" || !isTauriApp());
  const push = usePushNotifications(rows.flatMap((row) => row.types));
  const sendsEmail = instance.data?.sends_email === true;
  const pushShown = push.status === "available" && push.enabled;
  const channels: NotificationChannel[] = [
    "in_app",
    ...(pushShown ? (["push"] as const) : []),
    ...(sendsEmail ? (["email"] as const) : []),
    ...(sound.enabled ? (["sound"] as const) : []),
  ];

  const serverValue = (row: NotificationSettingRow, channel: "in_app" | "email"): boolean => {
    const change = pending.findLast((c) => c.group === row.group && c.channel === channel);
    return change?.enabled ?? row[channel]?.enabled ?? false;
  };
  const serverCell = (row: NotificationSettingRow, channel: "in_app" | "email", inApp: boolean): NotificationCell => {
    const cell = row[channel] ?? { enabled: false, fixed: false };
    return {
      selected: cell.fixed || serverValue(row, channel),
      disabled: cell.fixed || (channel !== "in_app" && !inApp),
      onChange: (enabled) => {
        mutation.mutate([{ group: row.group, channel, enabled }]);
      },
      saveState: cellSaveState(row, channel),
    };
  };

  const rowNote = (row: NotificationSettingRow, inApp: boolean): string | undefined => {
    if (!inApp) return t("client:settings.notifications.row.in_app_off");
    const fixed = (["in_app", "email"] as const).filter(
      (channel) => channels.includes(channel) && row[channel]?.fixed === true,
    );
    if (fixed.length === 0) return undefined;
    return t("client:settings.notifications.row.always_on", {
      channels: intl.formatList(
        fixed.map((channel) => t(notificationChannelLabels[channel])),
        { type: "conjunction" },
      ),
    });
  };

  const tableRow = (row: NotificationSettingRow): NotificationTableRow => {
    const inApp = row.in_app.fixed || serverValue(row, "in_app");
    const cells: Partial<Record<NotificationChannel, NotificationCell>> = { in_app: serverCell(row, "in_app", inApp) };
    if (push.status === "available" && pushShown) {
      cells.push = {
        selected: row.types.every((type) => push.alerts.includes(type)),
        disabled: !inApp,
        onChange: (selected) => {
          push.setTypes(row.types, selected);
        },
        saveState: push.typesSaveState(row.types),
      };
    }
    // A row without an email channel (status) shows no email cell.
    if (sendsEmail && row.email !== undefined) cells.email = serverCell(row, "email", inApp);
    if (sound.enabled) {
      cells.sound = {
        selected: row.types.every((type) => sound.types.includes(type)),
        disabled: !inApp,
        onChange: (selected) => {
          setNotificationSoundTypes(row.types, selected);
        },
        saveState: "idle",
      };
    }
    return { group: row.group, label: notificationGroupLabels[row.group], cells, note: rowNote(row, inApp) };
  };

  /** Turns the cells of `channel` of the section's rows that can change and differ to `selected`, at once. */
  const setColumn = (
    sectionRows: readonly { row: NotificationSettingRow; table: NotificationTableRow }[],
    channel: NotificationChannel,
    selected: boolean,
  ) => {
    const changing = sectionRows
      .filter(({ table }) => {
        const cell = table.cells[channel];
        return cell !== undefined && !cell.disabled && cell.selected !== selected;
      })
      .map(({ row }) => row);
    if (changing.length === 0) return;
    const types = changing.flatMap((row) => row.types);
    switch (channel) {
      case "in_app":
      case "email":
        mutation.mutate(changing.map((row) => ({ group: row.group, channel, enabled: selected })));
        break;
      case "push":
        if (push.status === "available") push.setTypes(types, selected);
        break;
      case "sound":
        setNotificationSoundTypes(types, selected);
        break;
    }
  };

  const column = (
    sectionRows: readonly { row: NotificationSettingRow; table: NotificationTableRow }[],
    channel: NotificationChannel,
  ): NotificationColumn => {
    const operable = sectionRows.flatMap(({ table }) => {
      const cell = table.cells[channel];
      return cell === undefined || cell.disabled ? [] : [cell];
    });
    const on = operable.filter((cell) => cell.selected).length;
    return {
      selected: operable.length > 0 && on === operable.length,
      indeterminate: on > 0 && on < operable.length,
      disabled: operable.length === 0,
      onChange: (selected) => {
        setColumn(sectionRows, channel, selected);
      },
    };
  };

  // The groups of the last save when the server refused it, for the error under the table of their section.
  const failed = mutation.isError ? mutation.variables : undefined;
  const sections = (["ordinary", "moderator", "instance"] as const)
    .map((section): NotificationTableSection => {
      const sectionRows = rows.filter((row) => row.section === section).map((row) => ({ row, table: tableRow(row) }));
      return {
        section,
        label: notificationSectionLabels[section],
        rows: sectionRows.map(({ table }) => table),
        columns: Object.fromEntries(channels.map((channel) => [channel, column(sectionRows, channel)])),
        error:
          failed?.some((change) => sectionRows.some(({ row }) => row.group === change.group)) === true
            ? errorMessage(mutation.error)
            : undefined,
      };
    })
    .filter((section) => section.rows.length > 0);

  return {
    channels,
    sections,
    push,
    sound: { enabled: sound.enabled, setEnabled: setNotificationSoundEnabled },
    emailSuppression: sendsEmail ? preferences.email_suppression : undefined,
  };
}

/** What the button that plays the notification sound once has done: nothing yet, played, or not played. */
export type SoundTest = { status: "idle" | "played" | "blocked"; play: () => void };

/**
 * The button that plays the notification sound once (ADR-0109 §5): a press lets the browser play it when notifications
 * arrive; when the browser does not play it, the settings say so.
 */
export function useNotificationSoundTest(): SoundTest {
  const [status, setStatus] = useState<SoundTest["status"]>("idle");
  return {
    status,
    play: () => {
      void tryNotificationSound().then((played) => {
        setStatus(played ? "played" : "blocked");
      });
    },
  };
}
