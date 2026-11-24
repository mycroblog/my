// SPDX-License-Identifier: AGPL-3.0-only

// A webhook as a holder of `manage_webhooks` keeps it (FR-722, ADR-0060), at /admin/webhooks/{id}: first where it
// stands and what to do with it (Send a test while it is on; Turn off or Turn on), then its deliveries of the last 14
// days (each its event, where it stands, its attempts and the last one's outcome, and Send again, and under it what was
// sent, folded), so that a failure is in sight, then when it was set up and changed, and its form saved as a whole (its address, what it is for, its
// events), or, where it receives an event the reader may not handle, what it is and why they cannot change it; after
// the form's Save, apart, a new secret (asked first, then shown once at the top of the page, the previous one still
// signing for 24 hours), and, last, Delete, confirmed with the host typed. What a delivery sent is the server's to
// withhold from a reader who lacks the permission of its event, and the row says so in its place.

import {
  mayHandleEvent,
  useAdminWebhook,
  useAdminWebhookWrites,
  usePermissions,
  useWebhookDeliveries,
  webhookEvents,
  type AdminWebhook,
  type WebhookChange,
  type WebhookDelivery,
} from "@my/app-data";
import {
  Box,
  Button,
  ConfirmDialog,
  Disclosure,
  ModalPresence,
  Row,
  FeedNotice,
  RecordTable,
  SettingsFacts,
  SettingsNote,
  SettingsResult,
  SettingsScreen,
  SettingsSection,
  StateMark,
  StatusSummary,
  useAddedFocus,
  type IconName,
  type RecordRow,
  type RecordState,
} from "@my/design-system";
import { use, useState, type ReactNode } from "react";
import { ListMore, shownPayload, useMoment, useStaffStrings, type T } from "../parts";
import { CopyValue, Irreversible, LeaveDialog, ScreenNotice, type AdminSubProps } from "../settings/parts";
import { useStillThere } from "./parts";
import {
  eventName,
  formOf,
  hostOf,
  useWebhookForm,
  WebhookBar,
  WebhookEventFields,
  WebhookPlaceFields,
  WebhookSecret,
  WebhookSecretSpecimen,
  type WebhookFormState,
} from "./webhook-form";
import { adminWebhooksPath, BackToWebhooks } from "./webhooks";

/** Where a webhook stands, as its summary says it: its glyph, its sentence, and what it means. */
function summaryOf(
  t: T,
  moment: (iso: string) => string,
  w: AdminWebhook,
): { icon: IconName; title: string; about: string } {
  if (w.state === "disabled") {
    if (w.state_reason === "failing")
      return {
        icon: "caution",
        title: t("staff_webhooks.summary.failing_off"),
        about: t("staff_webhooks.summary.failing_off.about"),
      };
    if (w.state_reason === "gone")
      return {
        icon: "caution",
        title: t("staff_webhooks.summary.gone"),
        about: t("staff_webhooks.summary.gone.about"),
      };
    return {
      icon: "stateDisabled",
      title: t("staff_webhooks.summary.off"),
      about: t("staff_webhooks.summary.off.about"),
    };
  }
  if (w.failure !== undefined)
    return {
      icon: "caution",
      title: t("staff_webhooks.summary.failing"),
      about: t("staff_webhooks.summary.failing.about", {
        count: w.failure.consecutive_failures,
        since: moment(w.failure.first_failed_at),
      }),
    };
  return {
    icon: "accepted",
    title: t("staff_webhooks.summary.on"),
    about: t("staff_webhooks.summary.on.about", { count: w.events.length }),
  };
}

/** Where a webhook stands, with Send a test while it is on, and Turn off or Turn on; what came of them beside. */
function WebhookState({ webhook }: { webhook: AdminWebhook }) {
  const { t } = useStaffStrings();
  const moment = useMoment();
  const writes = useAdminWebhookWrites();
  const there = useStillThere();
  const [busy, setBusy] = useState<"test" | "switch" | undefined>(undefined);
  const [said, setSaid] = useState<{ text: string; n: number } | undefined>(undefined);
  const say = (text: string) => {
    setSaid((was) => ({ text, n: (was?.n ?? 0) + 1 }));
  };
  const on = webhook.state === "enabled";
  const run = (what: "test" | "switch", write: () => Promise<unknown>, done: string, failed: string) => {
    setBusy(what);
    write().then(
      () => {
        if (!there.current) return;
        setBusy(undefined);
        say(done);
      },
      () => {
        if (!there.current) return;
        setBusy(undefined);
        say(failed);
      },
    );
  };
  const s = summaryOf(t, moment, webhook);
  return (
    <StatusSummary
      icon={s.icon}
      title={s.title}
      about={s.about}
      action={
        <Row wrap align="center" gap="screen-row-gap">
          {on && (
            <Button
              size="timeline"
              touch
              busy={busy === "test"}
              isDisabled={busy !== undefined}
              onPress={() => {
                run(
                  "test",
                  () => writes.test(webhook.id),
                  t("staff_webhooks.test.sent"),
                  t("staff_webhooks.test.failed"),
                );
              }}
            >
              {t("staff_webhooks.test")}
            </Button>
          )}
          <Button
            size="timeline"
            touch
            busy={busy === "switch"}
            isDisabled={busy !== undefined}
            onPress={() => {
              run(
                "switch",
                () => (on ? writes.disable(webhook.id) : writes.enable(webhook.id)),
                t(on ? "staff_webhooks.turned_off" : "staff_webhooks.turned_on"),
                t("staff_webhooks.switch.failed"),
              );
            }}
          >
            {t(on ? "staff_webhooks.turn_off" : "staff_webhooks.turn_on")}
          </Button>
        </Row>
      }
      result={said === undefined ? null : <span key={said.n}>{said.text}</span>}
    />
  );
}

/** When a webhook was set up, and when its address or events last changed. */
function WebhookFacts({ webhook }: { webhook: AdminWebhook }) {
  const { t } = useStaffStrings();
  const moment = useMoment();
  return (
    <SettingsSection heading={t("staff_webhooks.facts")}>
      <SettingsFacts
        label={t("staff_webhooks.facts")}
        items={[
          {
            text: t("staff_webhooks.facts.created"),
            about: [moment(webhook.created_at)],
          },
          {
            text: t("staff_webhooks.facts.revised"),
            about: [moment(webhook.revised_at)],
          },
        ]}
      />
    </SettingsSection>
  );
}

/** The outcome of a delivery's last attempt, in words: what came back and when. */
function attemptOf(t: T, moment: (iso: string) => string, d: WebhookDelivery): string | undefined {
  const a = d.last_attempt;
  if (a === undefined) return undefined;
  return t("staff_webhooks.delivery.attempt", {
    outcome: a.outcome,
    code: a.status_code ?? 0,
    at: moment(a.attempted_at),
  });
}

/** How a delivery's status is marked: waiting, delivered, or failing. */
const deliveryMark: Readonly<Record<WebhookDelivery["status"], RecordState>> = {
  pending: "pending",
  succeeded: "resolved",
  failing: "failed",
};

type DeliveryColumn = "attempt" | "state" | "act";

/** What a delivery sent, folded until shown, with the button that copies it; or why the reader is not shown it. */
function DeliveryBody({ delivery }: { delivery: WebhookDelivery }) {
  const { t } = useStaffStrings();
  const body = delivery.request_body;
  if (body === undefined)
    return (
      <Box as="p" text="caption">
        {t("staff_webhooks.delivery.body_withheld")}
      </Box>
    );
  return (
    <Disclosure summary={t("staff_webhooks.delivery.body")}>
      <CopyValue
        label={t("staff_webhooks.delivery.body")}
        value={shownPayload(body)}
        copy={t("staff_webhooks.delivery.body_copy")}
        code
      />
    </Disclosure>
  );
}

/**
 * The deliveries of the last 14 days, newest first, each its event, when, its attempts and the last one's outcome,
 * where it stands, Send again, and what it sent under it; what came of a press in the section's status; Show more past the first page.
 */
function Deliveries({ webhook }: { webhook: AdminWebhook }) {
  const { t } = useStaffStrings();
  const moment = useMoment();
  const list = useWebhookDeliveries(webhook.id);
  const added = useAddedFocus(list.items.length);
  const writes = useAdminWebhookWrites();
  const there = useStillThere();
  const [busy, setBusy] = useState<string | undefined>(undefined);
  const [said, setSaid] = useState<{ text: string; n: number } | undefined>(undefined);
  const heading = t("staff_webhooks.deliveries");
  const say = (text: string) => {
    setSaid((was) => ({ text, n: (was?.n ?? 0) + 1 }));
  };
  if (list.status !== "success" && list.items.length === 0)
    return (
      <SettingsSection heading={heading}>
        <ScreenNotice
          status={list.status === "error" ? "failed" : "loading"}
          retry={list.refetch}
          words={{
            loading: t("staff_webhooks.deliveries.loading"),
            failed: t("staff_webhooks.deliveries.failed"),
          }}
        />
      </SettingsSection>
    );
  const rows = list.items.map((d): RecordRow<DeliveryColumn> => ({
    id: d.id,
    head: eventName(t, d.event_type),
    caption: moment(d.created_at),
    cells: {
      attempt: [t("staff_webhooks.delivery.attempts", { count: d.attempts }), attemptOf(t, moment, d)]
        .filter((x) => x !== undefined)
        .join(" · "),
      state: (
        <StateMark state={deliveryMark[d.status]}>
          {t("staff_webhooks.delivery.status", { status: d.status })}
        </StateMark>
      ),
      act: (
        <Button
          size="timeline"
          touch
          busy={busy === d.id}
          isDisabled={busy !== undefined}
          onPress={() => {
            setBusy(d.id);
            writes.redeliver(webhook.id, d.id).then(
              () => {
                if (!there.current) return;
                setBusy(undefined);
                say(t("staff_webhooks.delivery.again"));
              },
              () => {
                if (!there.current) return;
                setBusy(undefined);
                say(t("staff_webhooks.delivery.again_failed"));
              },
            );
          }}
        >
          {t("staff_webhooks.delivery.redeliver")}
        </Button>
      ),
    },
    detail: <DeliveryBody delivery={d} />,
  }));
  return (
    <SettingsSection heading={heading}>
      <RecordTable
        label={heading}
        head={{ label: t("staff_webhooks.column.event") }}
        columns={[
          { key: "attempt", label: t("staff_webhooks.column.attempt"), bare: true },
          { key: "state", label: t("staff_webhooks.column.state"), kind: "fit", beside: true },
          { key: "act", label: t("staff_webhooks.column.redeliver"), kind: "fit", bare: true },
        ]}
        rows={rows}
        itemRef={added.item}
        empty={<FeedNotice>{t("staff_webhooks.deliveries.empty")}</FeedNotice>}
      />
      <SettingsResult>{said === undefined ? undefined : <span key={said.n}>{said.text}</span>}</SettingsResult>
      <ListMore list={list} added={added} />
    </SettingsSection>
  );
}

/** Make a new secret: asked first; the secret the server gives is handed to the page, which shows it at its top. */
function NewSecret({ webhook, onMade }: { webhook: AdminWebhook; onMade: (secret: string) => void }) {
  const { t } = useStaffStrings();
  const writes = useAdminWebhookWrites();
  const there = useStillThere();
  const [asking, setAsking] = useState(false);
  const [sending, setSending] = useState(false);
  const [failed, setFailed] = useState(false);
  const rotate = () => {
    setSending(true);
    writes.rotate(webhook.id).then(
      (secret) => {
        if (!there.current) return;
        setSending(false);
        setAsking(false);
        setFailed(false);
        onMade(secret);
      },
      () => {
        if (!there.current) return;
        setSending(false);
        setAsking(false);
        setFailed(true);
      },
    );
  };
  return (
    <>
      <SettingsSection heading={t("staff_webhooks.rotate")} about={t("staff_webhooks.rotate.about")}>
        <Box>
          <Button
            size="timeline"
            touch
            onPress={() => {
              setAsking(true);
            }}
          >
            {t("staff_webhooks.rotate.press")}
          </Button>
        </Box>
        <SettingsResult>{failed ? t("staff_webhooks.rotate.failed") : undefined}</SettingsResult>
      </SettingsSection>
      <ModalPresence>
        {asking && (
          <ConfirmDialog
            key="rotate"
            title={t("staff_webhooks.rotate.title")}
            cancel={t("staff_webhooks.rotate.cancel")}
            confirm={t("staff_webhooks.rotate.press")}
            busy={sending}
            onCancel={() => {
              setAsking(false);
            }}
            onConfirm={rotate}
          >
            {t("staff_webhooks.rotate.text")}
          </ConfirmDialog>
        )}
      </ModalPresence>
    </>
  );
}

/**
 * What a webhook that receives an event the reader may not handle says of itself: its address, its description and its
 * events, and in one line why the reader cannot change it.
 */
function LockedSettings({ webhook }: { webhook: AdminWebhook }) {
  const { t, locale } = useStaffStrings();
  const events = new Intl.ListFormat(locale, { type: "conjunction" }).format(
    webhook.events.map((e) => eventName(t, e)),
  );
  return (
    <SettingsSection heading={t("staff_webhooks.endpoint")}>
      <SettingsFacts
        label={t("staff_webhooks.endpoint")}
        items={[
          { text: t("staff_webhooks.url"), about: [webhook.url] },
          ...(webhook.description === undefined
            ? []
            : [{ text: t("staff_webhooks.description"), about: [webhook.description] }]),
          { text: t("staff_webhooks.locked.events"), about: [events] },
        ]}
      />
      <SettingsNote instead>{t("staff_webhooks.locked")}</SettingsNote>
    </SettingsSection>
  );
}

/** What a change of the form sends: only what changed, an emptied description sent empty, which removes it. */
function changeOf(f: WebhookFormState): WebhookChange {
  const { form, base } = f;
  const url = form.url.trim();
  const description = form.description.trim();
  const events = webhookEvents.filter((e) => form.events.has(e));
  return {
    ...(url !== base.url.trim() ? { url } : {}),
    ...(description !== base.description.trim() ? { description } : {}),
    ...(events.length !== base.events.size || events.some((e) => !base.events.has(e)) ? { events } : {}),
  };
}

/** The webhook's page, once it is read. */
function WebhookPage({ webhook, go, onUnsaved, leaving }: AdminSubProps & { webhook: AdminWebhook }) {
  const { t } = useStaffStrings();
  const writes = useAdminWebhookWrites();
  const permissions = usePermissions();
  const there = useStillThere();
  const f = useWebhookForm(formOf(webhook), permissions, onUnsaved);
  const host = hostOf(webhook.url);
  // The server checks every event of a change against the reader's permissions, so a webhook that receives one the
  // reader may not handle takes no change of theirs at all: it is shown, not offered as a form.
  const locked = webhook.events.some((e) => !mayHandleEvent(e, permissions));
  const [asking, setAsking] = useState(false);
  const [deleting, setDeleting] = useState<"idle" | "sending" | "failed">("idle");
  // A new secret the server gave once, shown at the top until the reader is done with it; a story's from its specimen.
  const specimen = use(WebhookSecretSpecimen);
  const [secret, setSecret] = useState<string | undefined>(specimen);
  const save = () => {
    if (f.busy || !f.check()) return;
    const change = changeOf(f);
    if (Object.keys(change).length === 0) {
      go(adminWebhooksPath);
      return;
    }
    f.setSent({ kind: "sending" });
    writes.change(webhook.id, change).then(
      () => {
        if (!there.current) return;
        f.setBase(f.form);
        f.setSent({ kind: "saved" });
      },
      (error: unknown) => {
        if (there.current) f.refused(error);
      },
    );
  };
  const remove = () => {
    setDeleting("sending");
    writes.remove(webhook.id).then(
      () => {
        if (!there.current) return;
        setAsking(false);
        // The guard hears that nothing is left unsaved before the screen leaves, not at the next render.
        onUnsaved?.(false);
        go(adminWebhooksPath);
      },
      () => {
        if (!there.current) return;
        setAsking(false);
        setDeleting("failed");
      },
    );
  };
  return (
    <>
      <SettingsScreen
        sub
        title={host}
        back={<BackToWebhooks go={go} />}
        // What the form does not hold stands after its Save, apart, and what ends the webhook last.
        foot={
          <>
            {!locked && <WebhookBar f={f} onSave={save} label={t("staff_webhooks.save")} />}
            <NewSecret webhook={webhook} onMade={setSecret} />
            <SettingsSection heading={t("staff_webhooks.delete")} about={t("staff_webhooks.delete.about")}>
              <Irreversible>{t("staff_webhooks.irreversible")}</Irreversible>
              <Box>
                <Button
                  size="timeline"
                  touch
                  isDisabled={f.busy}
                  onPress={() => {
                    setAsking(true);
                  }}
                >
                  {t("staff_webhooks.delete.press")}
                </Button>
              </Box>
              <SettingsResult>{deleting === "failed" ? t("staff_webhooks.delete.failed") : undefined}</SettingsResult>
            </SettingsSection>
          </>
        }
      >
        {secret !== undefined && (
          <WebhookSecret
            kind="rotated"
            secret={secret}
            arrived={secret !== specimen}
            done={
              <Row justify="end">
                <Button
                  size="timeline"
                  touch
                  onPress={() => {
                    setSecret(undefined);
                  }}
                >
                  {t("staff_webhooks.secret.done")}
                </Button>
              </Row>
            }
          />
        )}
        <WebhookState webhook={webhook} />
        <Deliveries webhook={webhook} />
        <WebhookFacts webhook={webhook} />
        {locked ? (
          <LockedSettings webhook={webhook} />
        ) : (
          <>
            <WebhookPlaceFields f={f} />
            <WebhookEventFields f={f} />
          </>
        )}
      </SettingsScreen>
      <ModalPresence>
        {asking ? (
          <ConfirmDialog
            key="delete"
            title={t("staff_webhooks.delete.title", { host })}
            cancel={t("staff_webhooks.delete.cancel")}
            confirm={t("staff_webhooks.delete.press")}
            phrase={host}
            busy={deleting === "sending"}
            onCancel={() => {
              setAsking(false);
            }}
            onConfirm={remove}
          >
            {t("staff_webhooks.delete.text")}
          </ConfirmDialog>
        ) : (
          leaving !== undefined && <LeaveDialog key="leave" leaving={leaving} />
        )}
      </ModalPresence>
    </>
  );
}

/**
 * The page of the webhook of `id`, or that it loads, could not be read, or is none. A story opens it with a new secret
 * just made through `WebhookSecretSpecimen`.
 */
export function WebhookScreen({ id, ...sub }: AdminSubProps & { id: string }): ReactNode {
  const { t } = useStaffStrings();
  const read = useAdminWebhook(id);
  if (read.webhook === undefined)
    return (
      <SettingsScreen
        sub
        title={t("staff_webhooks.entry")}
        back={<BackToWebhooks go={sub.go} />}
        notice={
          <ScreenNotice
            status={read.missing ? "missing" : read.status === "failed" ? "failed" : "loading"}
            retry={read.retry}
            words={{
              loading: t("staff_webhooks.one.loading"),
              failed: t("staff_webhooks.one.failed"),
              missing: t("staff_webhooks.missing"),
            }}
            all={{ label: t("staff_webhooks.all"), href: adminWebhooksPath }}
          />
        }
      />
    );
  return <WebhookPage key={read.webhook.id} webhook={read.webhook} {...sub} />;
}
