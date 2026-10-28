// SPDX-License-Identifier: AGPL-3.0-only
import {
  AutosaveControl,
  Button,
  Callout,
  Checkbox,
  Cluster,
  Hint,
  InlineError,
  SaveIndicator,
  Section,
  Stack,
  Switch,
  Table,
  TableBody,
  TableCell,
  TableColumnHeader,
  TableHead,
  TableRow,
  TableRowHeader,
  Text,
} from "@my/ui";
import { useId } from "react";
import { useIntl } from "react-intl";
import { useT } from "../../../lib/i18n";
import { useSaveLabels } from "../hooks/saveState";
import type { Preferences } from "../api/me";
import {
  type NotificationChannel,
  notificationChannelLabels,
  type NotificationSettingsView,
  type NotificationTableSection,
  useNotificationSettings,
  useNotificationSoundTest,
} from "../hooks/useNotificationSettings";

/**
 * The notification settings (FR-210, ADR-0109 §1): Web Push on this device and the notification sound above a table
 * per section of the notification types × the channels (in the app, Web Push, email, sound). Every control saves its
 * value as soon as it changes.
 */
export function NotificationSettings() {
  const t = useT();
  const view = useNotificationSettings();
  const tableHintId = useId();
  return (
    <Stack gap="8">
      <Hint size="inherit">{t("client:settings.preferences.autosave")}</Hint>
      <PushNotifications push={view.push} />
      <NotificationSound sound={view.sound} />
      <EmailDelivery suppression={view.emailSuppression} />
      <Stack gap="6">
        <Hint id={tableHintId}>{t("client:settings.notifications.table.description")}</Hint>
        {view.sections.map((section) => (
          <NotificationTable
            key={section.section}
            section={section}
            channels={view.channels}
            describedBy={tableHintId}
          />
        ))}
        {view.error !== undefined && <InlineError alert>{view.error}</InlineError>}
      </Stack>
    </Stack>
  );
}

/**
 * One section of the notification types as a table to read (the native table, which assistive technology reads by
 * row and column): a row per notification type, named by its row header, and a checkbox per shown channel, named by
 * its row and its column, since a checkbox in a cell has no label of its own.
 */
function NotificationTable({
  section,
  channels,
  describedBy,
}: {
  section: NotificationTableSection;
  channels: readonly NotificationChannel[];
  describedBy: string;
}) {
  const t = useT();
  const labels = useSaveLabels();
  const headingId = useId();
  return (
    <Section title={t(section.label)} headingId={headingId}>
      <Table density="compact" aria-labelledby={headingId} aria-describedby={describedBy}>
        <TableHead>
          <TableColumnHeader>{t("client:settings.notifications.column.type")}</TableColumnHeader>
          {channels.map((channel) => (
            <TableColumnHeader key={channel}>{t(notificationChannelLabels[channel])}</TableColumnHeader>
          ))}
        </TableHead>
        <TableBody>
          {section.rows.map((row) => (
            <TableRow key={row.group}>
              <TableRowHeader>{t(row.label)}</TableRowHeader>
              {channels.map((channel) => {
                const cell = row.cells[channel];
                return (
                  <TableCell key={channel}>
                    {cell !== undefined && (
                      // The save mark of the cell, beside its checkbox: its icon, with the words for assistive technology.
                      <Cluster gap="1" align="center" wrap={false}>
                        <Checkbox
                          aria-label={t("client:settings.notifications.cell", {
                            type: t(row.label),
                            channel: t(notificationChannelLabels[channel]),
                          })}
                          isSelected={cell.selected}
                          isDisabled={cell.disabled}
                          onChange={cell.onChange}
                        />
                        <SaveIndicator state={cell.saveState} labels={labels} display="icon" />
                      </Cluster>
                    )}
                  </TableCell>
                );
              })}
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </Section>
  );
}

/**
 * Web Push on this device (FR-206), when the server sends it: a switch that turns it on for this signed-in app (the
 * APG switch pattern), which shows the table's Web Push column; or why it cannot be on.
 */
function PushNotifications({ push }: { push: NotificationSettingsView["push"] }) {
  const t = useT();
  const labels = useSaveLabels();
  if (push.status === "hidden") return null;
  return (
    <Section title={t("client:settings.push.title")}>
      {push.status === "unsupported" && <Hint>{t("client:settings.push.unsupported")}</Hint>}
      {push.status === "denied" && <Hint>{t("client:settings.push.denied")}</Hint>}
      {push.status === "available" && (
        <>
          <AutosaveControl state={push.saveState} labels={labels}>
            <Switch
              isSelected={push.enabled}
              onChange={push.setEnabled}
              isDisabled={push.isPending}
              description={t("client:settings.push.enable.description")}
            >
              {t("client:settings.push.enable")}
            </Switch>
          </AutosaveControl>
          {push.error !== undefined && <InlineError alert>{push.error}</InlineError>}
        </>
      )}
    </Section>
  );
}

/**
 * The notification sound of this device (ADR-0109 §5): a switch of the whole sound, which shows the table's sound
 * column, and a button that plays it once, which lets the browser play it later; when the browser does not, the
 * section says so.
 */
function NotificationSound({ sound }: { sound: NotificationSettingsView["sound"] }) {
  const t = useT();
  const test = useNotificationSoundTest();
  return (
    <Section title={t("client:settings.notifications.sound.title")}>
      <Switch
        isSelected={sound.enabled}
        onChange={sound.setEnabled}
        description={t("client:settings.notifications.sound.enable.description")}
      >
        {t("client:settings.notifications.sound.enable")}
      </Switch>
      <Stack align="start" gap="2">
        <Button size="sm" onPress={test.play}>
          {t("client:settings.notifications.sound.play")}
        </Button>
        {test.status === "blocked" && (
          <InlineError alert>{t("client:settings.notifications.sound.blocked")}</InlineError>
        )}
      </Stack>
    </Section>
  );
}

/**
 * That emails no longer reach the user's address (a hard bounce or a complaint, ADR-0086 §5), and since when; after a
 * hard bounce, what to do. Above the table, whose email column it concerns; nothing shows while they do reach it.
 */
function EmailDelivery({ suppression }: { suppression: Preferences["email_suppression"] }) {
  const t = useT();
  const intl = useIntl();
  if (suppression === undefined) return null;
  const since = intl.formatDate(suppression.since, { dateStyle: "medium", timeStyle: "short" });
  return (
    <Section title={t("client:settings.email_suppression.title")}>
      <Callout>
        <Text as="p" size="sm" tone="fg">
          {suppression.kind === "hard_bounce"
            ? t("client:settings.email_suppression.hard_bounce")
            : t("client:settings.email_suppression.complaint")}
        </Text>
        <Hint size="inherit">{t("client:settings.email_suppression.since", { since })}</Hint>
        {suppression.kind === "hard_bounce" && (
          <Text as="p" size="sm" tone="fg">
            {t("client:settings.email_suppression.hard_bounce.action")}
          </Text>
        )}
      </Callout>
    </Section>
  );
}
