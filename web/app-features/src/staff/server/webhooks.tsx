// SPDX-License-Identifier: AGPL-3.0-only

// The webhooks as a holder of `manage_webhooks` keeps them (FR-722, ADR-0060, ADR-0058 §1): at /admin/webhooks, every
// endpoint, newest first, a row each that opens its page, under its host, with what it is for, how many events it
// receives and where it stands as a mark (on; failing now; off, by an administrator or by the server after its
// deliveries kept failing or its endpoint said it is gone), with Set up a webhook. A new one, /admin/webhooks/new, is its address, what it is
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
  FeedNotice,
  LinkButton,
  ModalPresence,
  RecordTable,
  Row,
  ScreenStack,
  ScreenTitle,
  SettingsSection,
  SettingsScreen,
  StateMark,
  type RecordRow,
} from "@my/design-system";
import { use, useState, type ReactNode } from "react";
import { ReadNotice, useStaffStrings } from "../parts";
import { LeaveDialog, type AdminSubProps } from "../settings/parts";
import { BackTo, useStillThere } from "./parts";
import {
  hostOf,
  useWebhookForm,
  WebhookBar,
  WebhookEventFields,
  WebhookPlaceFields,
  WebhookSecret,
  WebhookSecretSpecimen,
} from "./webhook-form";

/** The address of the webhooks. */
export const adminWebhooksPath = "/admin/webhooks";
/** The address of a new webhook's form. */
const newWebhookPath = `${adminWebhooksPath}/new`;
/** The address of the webhook of `id`. */
export const adminWebhookPath = (id: string): string => `${adminWebhooksPath}/${id}`;

/** Reads the webhooks, for the entry's loader. */
export const loadWebhooksScreen = (data: AppData): Promise<void> => loadAdminWebhooks(data);

/** Where a webhook stands in its list, as its mark: on; failing now; off, by an administrator or by the server. */
function WebhookMark({ webhook }: { webhook: AdminWebhook }): ReactNode {
  const { t } = useStaffStrings();
  if (webhook.state === "disabled")
    return webhook.state_reason === "admin" ? (
      <StateMark state="disabled">{t("staff_webhooks.state.off")}</StateMark>
    ) : (
      <StateMark state="failed">{t("staff_webhooks.state.server_off")}</StateMark>
    );
  if (webhook.failure !== undefined) return <StateMark state="failed">{t("staff_webhooks.state.failing")}</StateMark>;
  return <StateMark state="active">{t("staff_webhooks.state.on")}</StateMark>;
}

type Column = "events" | "state";

/** The webhooks: every one, newest first, a row each that opens its page, and Set up a webhook. */
export function AdminWebhooksScreen(): ReactNode {
  const { t } = useStaffStrings();
  const read = useAdminWebhooks();
  const head = <ScreenTitle glyph="link">{t("staff_webhooks.entry")}</ScreenTitle>;
  if (read.webhooks === undefined)
    return (
      <ScreenStack>
        {head}
        <ReadNotice
          status={read.status === "failed" ? "error" : "pending"}
          loading={t("staff_webhooks.loading")}
          failed={t("staff_webhooks.failed")}
          retry={read.retry}
        />
      </ScreenStack>
    );
  const rows = read.webhooks.map((w): RecordRow<Column> => ({
    id: w.id,
    head: hostOf(w.url),
    caption: w.description,
    href: adminWebhookPath(w.id),
    cells: {
      events: t("staff_webhooks.row.events", { count: w.events.length }),
      state: <WebhookMark webhook={w} />,
    },
  }));
  return (
    <ScreenStack>
      {head}
      {/* As the other lists of the administration, the way to a new one ends the heading of the list. */}
      <SettingsSection
        heading={t("staff_webhooks.list.count", { count: rows.length })}
        action={
          <LinkButton size="timeline" href={newWebhookPath}>
            {t("staff_webhooks.add")}
          </LinkButton>
        }
      >
        <RecordTable
          label={t("staff_webhooks.entry")}
          head={{ label: t("staff_webhooks.column.host") }}
          columns={[
            { key: "events", label: t("staff_webhooks.column.events"), bare: true },
            { key: "state", label: t("staff_webhooks.column.state"), kind: "fit", beside: true },
          ]}
          rows={rows}
          even
          contained
          empty={<FeedNotice>{t("staff_webhooks.empty")}</FeedNotice>}
        />
      </SettingsSection>
    </ScreenStack>
  );
}

/** The way back to the webhooks. */
export function BackToWebhooks({ go }: { go: (href: string) => void }) {
  return <BackTo go={go} href={adminWebhooksPath} name="staff_webhooks.entry" />;
}

/**
 * A new webhook: its address, what it is for, and its events; once set up, its secret, shown once. A story opens it as
 * just set up through `WebhookSecretSpecimen`.
 */
export function NewWebhookScreen({ go, onUnsaved, leaving }: AdminSubProps): ReactNode {
  const { t } = useStaffStrings();
  const permissions = usePermissions();
  const writes = useAdminWebhookWrites();
  const there = useStillThere();
  const f = useWebhookForm({ url: "", description: "", events: new Set() }, permissions, onUnsaved);
  const specimen = use(WebhookSecretSpecimen);
  const [made, setMade] = useState<{ id?: string | undefined; secret: string } | undefined>(
    specimen === undefined ? undefined : { secret: specimen },
  );
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
          arrived={specimen === undefined}
          done={
            <Row justify="end">
              <Button
                size="timeline"
                touch
                onPress={() => {
                  go(made.id === undefined ? adminWebhooksPath : adminWebhookPath(made.id));
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
