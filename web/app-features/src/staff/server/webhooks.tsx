// SPDX-License-Identifier: AGPL-3.0-only

// The webhooks as a holder of `manage_webhooks` keeps them (FR-722, ADR-0060, ADR-0058 §1): at /admin/webhooks, every
// endpoint, newest first, each a door to its page under its host, with what it is for, how many events it receives
// and where it stands (on; failing now; off, by an administrator or by the server after its deliveries kept failing
// or its endpoint said it is gone), with Set up a webhook. A new one, /admin/webhooks/new, is its address, what it is
// for and its events (webhook-form.tsx); once set up, its secret is shown once in the form's place.

import {
  loadAdminWebhooks,
  usePermissions,
  useAdminWebhooks,
  useAdminWebhookWrites,
  webhookEvents,
  type AdminWebhook,
  type AppData,
} from "@my/app-data";
import {
  Button,
  LinkButton,
  ModalPresence,
  Row,
  SettingsEmpty,
  SettingsList,
  SettingsListRow,
  SettingsScreen,
  SettingsSection,
} from "@my/design-system";
import { useState, type ReactNode } from "react";
import { useMoment, useStaffStrings, type T } from "../parts";
import { LeaveDialog, ScreenNotice, type AdminSubProps } from "../settings/parts";
import { BackTo, useStillThere } from "./parts";
import {
  hostOf,
  useFocusOnArrival,
  useWebhookForm,
  WebhookBar,
  WebhookEventFields,
  WebhookPlaceFields,
  WebhookSecret,
} from "./webhook-form";

/** The address of the webhooks. */
export const adminWebhooksPath = "/admin/webhooks";
/** The address of a new webhook's form. */
const newWebhookPath = `${adminWebhooksPath}/new`;
/** The address of the webhook of `id`. */
export const adminWebhookPath = (id: string): string => `${adminWebhooksPath}/${id}`;

/** Reads the webhooks, for the entry's loader. */
export const loadWebhooksScreen = (data: AppData): Promise<void> => loadAdminWebhooks(data);

/** Whether `webhook` receives nothing now. */
const isOff = (webhook: AdminWebhook): boolean => webhook.state === "disabled";

/**
 * Where a webhook stands, in a sentence, where there is more to say than on or off: failing now, or turned off by the
 * server and why; none for one that works or that an administrator turned off.
 */
function standingOf(t: T, moment: (iso: string) => string, webhook: AdminWebhook): string | undefined {
  if (webhook.state === "disabled" && webhook.state_reason === "failing")
    return t("staff_webhooks.standing.failing_off");
  if (webhook.state === "disabled" && webhook.state_reason === "gone") return t("staff_webhooks.standing.gone");
  if (webhook.state === "enabled" && webhook.failure !== undefined)
    return t("staff_webhooks.standing.failing", {
      count: webhook.failure.consecutive_failures,
      since: moment(webhook.failure.first_failed_at),
    });
  return undefined;
}

/** The webhooks: every one, newest first, a door each, and Set up a webhook. */
export function AdminWebhooksScreen(): ReactNode {
  const { t } = useStaffStrings();
  const moment = useMoment();
  const read = useAdminWebhooks();
  const title = t("staff_webhooks.entry");
  if (read.webhooks === undefined)
    return (
      <SettingsScreen
        sub
        icon="link"
        title={title}
        notice={
          <ScreenNotice
            status={read.status === "failed" ? "failed" : "loading"}
            retry={read.retry}
            words={{
              loading: t("staff_webhooks.loading"),
              failed: t("staff_webhooks.failed"),
            }}
          />
        }
      />
    );
  const add = (
    <LinkButton size="timeline" href={newWebhookPath}>
      {t("staff_webhooks.add")}
    </LinkButton>
  );
  const { webhooks } = read;
  return (
    <SettingsScreen sub icon="link" title={title}>
      <SettingsSection
        heading={t("staff_webhooks.list.count", { count: webhooks.length })}
        about={t("staff_webhooks.list.about")}
        action={webhooks.length === 0 ? undefined : add}
      >
        {webhooks.length === 0 ? (
          <SettingsEmpty action={add}>{t("staff_webhooks.empty")}</SettingsEmpty>
        ) : (
          <SettingsList label={title}>
            {webhooks.map((w) => (
              <SettingsListRow
                key={w.id}
                title={hostOf(w.url)}
                inactive={isOff(w)}
                facts={[
                  ...(w.description === undefined ? [] : [w.description]),
                  t("staff_webhooks.row.events", { count: w.events.length }),
                ]}
                standing={standingOf(t, moment, w)}
                state={t(isOff(w) ? "staff_webhooks.state.off" : "staff_webhooks.state.on")}
                href={adminWebhookPath(w.id)}
              />
            ))}
          </SettingsList>
        )}
      </SettingsSection>
    </SettingsScreen>
  );
}

/** The way back to the webhooks. */
export function BackToWebhooks({ go }: { go: (href: string) => void }) {
  return <BackTo go={go} href={adminWebhooksPath} name="staff_webhooks.entry" />;
}

/**
 * A new webhook: its address, what it is for, and its events; once set up, its secret, shown once. `defaultMade` opens
 * it as just set up (a specimen's): the webhook's ID and the secret the server gave.
 */
export function NewWebhookScreen({
  go,
  onUnsaved,
  leaving,
  defaultMade,
}: AdminSubProps & {
  defaultMade?: { id: string; secret: string } | undefined;
}): ReactNode {
  const { t } = useStaffStrings();
  const permissions = usePermissions();
  const writes = useAdminWebhookWrites();
  const there = useStillThere();
  const f = useWebhookForm({ url: "", description: "", events: new Set() }, permissions, onUnsaved);
  const [made, setMade] = useState<{ id: string; secret: string } | undefined>(defaultMade);
  const titleRef = useFocusOnArrival<HTMLHeadingElement>(made !== undefined);
  const save = () => {
    if (f.busy || !f.check()) return;
    f.setSent({ kind: "sending" });
    const description = f.form.description.trim();
    writes
      .create({
        url: f.form.url.trim(),
        ...(description === "" ? {} : { description }),
        events: webhookEvents.filter((e) => f.form.events.has(e)),
      })
      .then(
        ({ webhook, secret }) => {
          if (!there.current) return;
          f.setBase(f.form);
          f.setSent({ kind: "idle" });
          setMade({ id: webhook.id, secret });
        },
        (error: unknown) => {
          if (there.current) f.refused(error);
        },
      );
  };
  if (made !== undefined)
    return (
      <SettingsScreen sub title={t("staff_webhooks.new")} back={<BackToWebhooks go={go} />}>
        <WebhookSecret
          kind="created"
          secret={made.secret}
          titleRef={titleRef}
          done={
            <Row justify="end">
              <Button
                size="timeline"
                touch
                onPress={() => {
                  go(adminWebhookPath(made.id));
                }}
              >
                {t("staff_webhooks.secret.done")}
              </Button>
            </Row>
          }
        />
      </SettingsScreen>
    );
  return (
    <>
      <SettingsScreen
        sub
        title={t("staff_webhooks.new")}
        back={<BackToWebhooks go={go} />}
        foot={<WebhookBar f={f} onSave={save} label={t("staff_webhooks.create")} />}
      >
        <WebhookPlaceFields f={f} />
        <WebhookEventFields f={f} />
      </SettingsScreen>
      <ModalPresence>{leaving !== undefined && <LeaveDialog key="leave" leaving={leaving} />}</ModalPresence>
    </>
  );
}
