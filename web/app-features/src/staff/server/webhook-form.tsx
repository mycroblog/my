// SPDX-License-Identifier: AGPL-3.0-only

// The form of a webhook (FR-722, ADR-0060), for a new one and for one that exists: its address, what it is for, and
// the events it receives, a checkbox each, in groups by what they are about. An event whose permission the reader
// lacks is not offered, and a group of none is no section. What the server or the form refuses of a field stands
// under it, which takes the focus. The secret the server gives once (a new webhook's, a new secret) is shown on one
// surface at the top of the screen, kept in the screen's state alone (`WebhookSecret`).

import {
  mayHandleEvent,
  webhookEventGroups,
  webhookEvents,
  webhookLimits,
  webhookRefusal,
  type AdminWebhook,
  type Permission,
  type WebhookEvent,
} from "@my/app-data";
import {
  Box,
  Button,
  CheckField,
  focusRefusedField,
  FormAlert,
  SaveState,
  SettingsSection,
  Stack,
  SubmitBar,
  TextField,
} from "@my/design-system";
import { ProblemError } from "@mycroblog/mykit";
import { createContext, useEffect, useId, useRef, useState, type ReactNode, type RefObject } from "react";
import { useStaffStrings, type T } from "../parts";
import { permissionName } from "./roles";
import { CopyValue, type AdminSubProps } from "../settings/parts";
import { useUnsavedReport } from "./parts";

/** An event's name, in the reader's language. */
export const eventName = (t: T, event: string): string =>
  t("staff_webhooks.event", { event: event.replaceAll(".", "_") });

/** The host of a webhook's URL, which names it; the URL itself where it has none. */
export function hostOf(url: string): string {
  try {
    return new URL(url).host || url;
  } catch {
    return url;
  }
}

/** What the form of a webhook holds. */
export type WebhookForm = {
  readonly url: string;
  readonly description: string;
  readonly events: ReadonlySet<WebhookEvent>;
};

/** The fields a refusal is said under. */
type Field = "url" | "description" | "events";

/** Where the form's last save is: refused, with what is wrong with which field or why in the submit bar. */
type Sent =
  | { readonly kind: "idle" | "sending" | "saved" }
  | {
      readonly kind: "refused";
      readonly field?: Field | undefined;
      readonly why: string;
    };

const sameEvents = (a: ReadonlySet<WebhookEvent>, b: ReadonlySet<WebhookEvent>) =>
  a.size === b.size && [...a].every((e) => b.has(e));

/** Whether a URL is one the server takes in its shape: absolute http or https, no user info, no fragment. */
function urlTaken(url: string): boolean {
  try {
    const u = new URL(url);
    return (
      (u.protocol === "https:" || u.protocol === "http:") &&
      u.username === "" &&
      u.password === "" &&
      !url.includes("#") &&
      new TextEncoder().encode(url).length <= webhookLimits.url
    );
  } catch {
    return false;
  }
}

/** The form of a webhook: its state, its checks, its focus on a refused field, and whether it holds changes. */
export function useWebhookForm(
  start: WebhookForm,
  permissions: readonly Permission[],
  onUnsaved: AdminSubProps["onUnsaved"],
) {
  const { t, locale } = useStaffStrings();
  const id = useId();
  const [base, setBase] = useState(start);
  const [form, setForm] = useState(start);
  const [sent, setSent] = useState<Sent>({ kind: "idle" });
  const focusNext = useRef<string | undefined>(undefined);
  useEffect(() => {
    if (focusNext.current === undefined) return;
    focusRefusedField(focusNext.current);
    focusNext.current = undefined;
  });
  const busy = sent.kind === "sending";
  const unsaved =
    form.url.trim() !== base.url.trim() ||
    form.description.trim() !== base.description.trim() ||
    !sameEvents(form.events, base.events);
  useUnsavedReport(onUnsaved, unsaved, busy);
  const offered = webhookEvents.filter((e) => mayHandleEvent(e, permissions));
  const controlOf = (field: Field) => (field === "events" ? `${id}-event-${offered[0] ?? ""}` : `${id}-${field}`);
  const refuse = (field: Field | undefined, why: string) => {
    setSent({ kind: "refused", field, why });
    if (field !== undefined) focusNext.current = controlOf(field);
  };
  return {
    id,
    base,
    form,
    sent,
    busy,
    unsaved,
    offered,
    permissions,
    setBase,
    setSent,
    change: (part: Partial<WebhookForm>) => {
      setForm((now) => ({ ...now, ...part }));
      setSent((was) => (was.kind === "saved" || was.kind === "refused" ? { kind: "idle" } : was));
    },
    /** What the form finds wrong before sending: an address the server would refuse, a description too long, no event. */
    check: (): boolean => {
      if (!urlTaken(form.url.trim())) {
        refuse("url", t("staff_webhooks.url.rule"));
        return false;
      }
      if (Array.from(form.description.trim()).length > webhookLimits.description) {
        refuse(
          "description",
          t("staff_webhooks.description.rule", {
            max: webhookLimits.description,
          }),
        );
        return false;
      }
      if (form.events.size === 0) {
        refuse("events", t("staff_webhooks.events.none"));
        return false;
      }
      return true;
    },
    /** Says what came of a refused write: under the field it is about, which takes the focus, or in the submit bar. */
    refused: (error: unknown) => {
      const r = webhookRefusal(error);
      switch (r?.reason) {
        case "url":
          refuse("url", t("staff_webhooks.url.rule"));
          return;
        case "description":
          refuse(
            "description",
            t("staff_webhooks.description.rule", {
              max: webhookLimits.description,
            }),
          );
          return;
        case "events":
          refuse("events", t("staff_webhooks.events.none"));
          return;
        case "missing_permission":
          refuse(
            "events",
            t("staff_webhooks.events.missing", {
              names: new Intl.ListFormat(locale, {
                type: "conjunction",
              }).format(r.missing.map((p) => permissionName(t, p))),
            }),
          );
          return;
        default:
      }
      if (error instanceof ProblemError && error.status === 403) refuse(undefined, t("staff_webhooks.save.forbidden"));
      else refuse(undefined, t("staff_webhooks.save.failed"));
    },
  };
}

export type WebhookFormState = ReturnType<typeof useWebhookForm>;

const errorOf = (f: WebhookFormState, field: Field): string | undefined =>
  f.sent.kind === "refused" && f.sent.field === field ? f.sent.why : undefined;

/** The address and the description of a webhook. */
export function WebhookPlaceFields({ f }: { f: WebhookFormState }) {
  const { t } = useStaffStrings();
  return (
    <SettingsSection heading={t("staff_webhooks.endpoint")}>
      <TextField
        id={`${f.id}-url`}
        name="url"
        label={t("staff_webhooks.url")}
        hint={errorOf(f, "url") === undefined ? t("staff_webhooks.url.rule") : undefined}
        row
        defaultValue={f.form.url}
        maxLength={webhookLimits.url}
        autoCapitalize="none"
        inputMode="text"
        error={errorOf(f, "url")}
        readOnly={f.busy}
        onChange={(url) => {
          f.change({ url });
        }}
      />
      <TextField
        id={`${f.id}-description`}
        name="description"
        label={t("staff_webhooks.description")}
        row
        defaultValue={f.form.description}
        maxLength={webhookLimits.description}
        error={errorOf(f, "description")}
        readOnly={f.busy}
        onChange={(description) => {
          f.change({ description });
        }}
      />
    </SettingsSection>
  );
}

/**
 * The events of a webhook, a section a group, a checkbox each the reader may subscribe to, close to each other; an event
 * the reader may not handle is not offered, and a group of none is no section. (A webhook that receives one is shown,
 * not offered as this form: webhook.tsx.)
 */
export function WebhookEventFields({ f }: { f: WebhookFormState }) {
  const { t } = useStaffStrings();
  const error = errorOf(f, "events");
  const first = f.offered[0];
  return (
    <>
      {webhookEventGroups.map((group) => {
        const offered = group.events.filter((e) => f.offered.includes(e));
        if (offered.length === 0) return null;
        return (
          <SettingsSection key={group.id} heading={t("staff_webhooks.group", { group: group.id })}>
            <Stack>
              {offered.map((event) => (
                <CheckField
                  key={event}
                  id={`${f.id}-event-${event}`}
                  name="events"
                  value={event}
                  label={eventName(t, event)}
                  checked={f.form.events.has(event)}
                  touch
                  disabled={f.busy}
                  // A refusal of the events as a whole stands under the first of them.
                  error={event === first ? error : undefined}
                  onChange={(on) => {
                    const next = new Set(f.form.events);
                    if (on) next.add(event);
                    else next.delete(event);
                    f.change({ events: next });
                  }}
                />
              ))}
            </Stack>
          </SettingsSection>
        );
      })}
    </>
  );
}

/** The submit bar of a webhook's form. */
export function WebhookBar({ f, onSave, label }: { f: WebhookFormState; onSave: () => void; label: string }) {
  const { t } = useStaffStrings();
  return (
    <SubmitBar
      result={
        f.sent.kind === "saved" ? (
          <SaveState state="saved" />
        ) : f.sent.kind === "refused" ? (
          <FormAlert>{f.sent.field === undefined ? f.sent.why : t("staff_webhooks.save.refused")}</FormAlert>
        ) : undefined
      }
    >
      <Button kind="primary" busy={f.busy} onPress={onSave}>
        {label}
      </Button>
    </SubmitBar>
  );
}

/** The form's start for `webhook`: what it is now. */
export const formOf = (webhook: AdminWebhook): WebhookForm => ({
  url: webhook.url,
  description: webhook.description ?? "",
  events: new Set(webhook.events),
});

/**
 * What a story opens a webhook's screens with in place of a press (a specimen): the secret the server gave once, as
 * if the reader had just set the webhook up or made a new secret. None in the app, where only a press shows it.
 */
export const WebhookSecretSpecimen = createContext<string | undefined>(undefined);

/**
 * A secret the server gave once (`kind`: of a new webhook, or a new one of a webhook that exists), on one surface: what
 * happened, as its heading, which takes the focus as the view comes after a press, the line that it is not shown again
 * right over the secret to copy, and the way on (`done`).
 */
export function WebhookSecret({
  kind,
  secret,
  arrived,
  done,
}: {
  kind: "created" | "rotated";
  secret: string;
  /** It came after the reader's press: its heading takes the focus. */
  arrived: boolean;
  done: ReactNode;
}): ReactNode {
  const { t } = useStaffStrings();
  const ref = useFocusOnArrival<HTMLDivElement>(arrived);
  return (
    <Box as={<div ref={ref} />}>
      <SettingsSection
        heading={t(kind === "created" ? "staff_webhooks.secret.created" : "staff_webhooks.secret.rotated")}
        about={t("staff_webhooks.secret.once")}
      >
        <CopyValue primary label={t("staff_webhooks.secret")} value={secret} copy={t("staff_webhooks.secret.copy")} />
        {done}
      </SettingsSection>
    </Box>
  );
}

/**
 * Moves the focus to the first heading inside what `ref` reaches, once, as `arrived` turns true after the view first
 * drew (a view opened in that state, a story's, leaves the focus where it is).
 */
function useFocusOnArrival<E extends HTMLElement>(arrived: boolean): RefObject<E | null> {
  const ref = useRef<E>(null);
  const before = useRef(arrived);
  useEffect(() => {
    if (arrived && !before.current) {
      const heading = ref.current?.querySelector<HTMLElement>("h1, h2, h3, h4");
      if (heading !== null && heading !== undefined) {
        heading.tabIndex = -1;
        heading.focus();
      }
    }
    before.current = arrived;
  }, [arrived]);
  return ref;
}
