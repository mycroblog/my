// SPDX-License-Identifier: AGPL-3.0-only

// Creating an invite, the screen under Invites at /settings/invites/new (ADR-0106 §15): a form saved as a whole, how
// many people can sign up with the invite and how long it works, each within what the server allows and starting from
// its default (one person, seven days, or the most it allows where that is less). Created, the screen says so first,
// in a summary that takes the focus, then shows the invite's link to copy in the form's place (Copy link the primary
// action), and Finish, neutral, at its end goes back to Invites, whose list holds it.
//
// A value the server refuses as over what it allows now stands under its field, in the client's words, and the field
// takes the focus (the administrators lowered a limit after the screen opened, and the form then offers the new
// limits); anything else the server refuses stands right before the button, in its words, the focus left there. A
// reader who may not create invites reads why in the form's place, and one who has as many usable invites as the server
// allows, that; both also when the server says so only as it refuses the invite, and then what says it takes the focus
// the form's button left.

import {
  invitesNotAllowed,
  inviteValueRefused,
  useOwnInvites,
  useOwnInviteWrites,
  type MemberInviteLimits,
  type OwnInvite,
} from "@my/app-data";
import {
  BackButton,
  Button,
  FormAlert,
  NumberField,
  Select,
  SettingRow,
  SettingsNote,
  SettingsResult,
  SettingsScreen,
  SettingsSection,
  StatusSummary,
  SubmitBar,
  useLocale,
} from "@my/design-system";
import { useEffect, useId, useRef, useState } from "react";
import { stringsIn } from "../../locale";
import { saidOf } from "../confirmation";
import type { SettingsSubProps } from "../entries";
import { CannotInvite, InviteLink, InviteStatus, Unread } from "./parts";

const minute = 60;
const hour = 60 * minute;
const day = 24 * hour;

/** The lengths of time the form offers where the server allows them, in seconds. */
const usualLives = [hour, 6 * hour, 12 * hour, day, 3 * day, 7 * day, 14 * day, 30 * day, 90 * day, 365 * day];

/** `seconds` as whole minutes, one at least: what an option can say. */
const wholeMinutes = (seconds: number): number => Math.max(minute, Math.floor(seconds / minute) * minute);

/**
 * How long a new invite may work, in seconds: the usual lengths under the longest the server allows, that longest,
 * and the server's default (`chosen`, where the form starts).
 */
function livesOf(limits: MemberInviteLimits): { offered: readonly number[]; chosen: number } {
  const longest = wholeMinutes(limits.max_expires_in);
  const chosen = Math.min(wholeMinutes(limits.default_expires_in), longest);
  const offered = new Set([...usualLives.filter((life) => life < longest), longest, chosen]);
  return { offered: [...offered].sort((a, b) => a - b), chosen };
}

/** `seconds` in the largest unit that counts it whole. */
function lifeIn(seconds: number): { unit: "days" | "hours" | "minutes"; count: number } {
  if (seconds % day === 0) return { unit: "days", count: seconds / day };
  if (seconds % hour === 0) return { unit: "hours", count: seconds / hour };
  return { unit: "minutes", count: Math.floor(seconds / minute) };
}

/** A whole number of one or more that the form's field of `name` holds; undefined while it holds none. */
function wholeOf(fields: FormData, name: string): number | undefined {
  const value = fields.get(name);
  const number = typeof value === "string" && value !== "" ? Number(value) : Number.NaN;
  return Number.isInteger(number) && number >= 1 ? number : undefined;
}

/**
 * The invite just created, in the form's place: that it is created, first, with what to do with its link, a summary
 * whose title takes the focus that the form's button left; then its link to copy, the screen's primary action, and its
 * state; Finish, neutral, goes back to Invites.
 */
function Created({ invite, onDone }: { invite: OwnInvite; onDone: () => void }) {
  const { t } = stringsIn(useLocale().locale);
  const said = useRef<HTMLHeadingElement>(null);
  useEffect(() => {
    said.current?.focus();
  }, []);
  return (
    <SettingsScreen
      sub
      title={t("settings.invites.create")}
      foot={
        <SubmitBar>
          <Button onPress={onDone}>{t("settings.once.done")}</Button>
        </SubmitBar>
      }
    >
      <StatusSummary
        icon="accepted"
        title={t("settings.invites.created")}
        titleRef={said}
        about={t("settings.invites.link.about", { code: invite.code })}
      />
      <InviteLink invite={invite} about={false} />
      <InviteStatus invite={invite} />
    </SettingsScreen>
  );
}

/**
 * That the reader holds as many usable invites as the server allows, in the form's place once the server refused the
 * invite for it (409): a status that takes the focus the form's button left, so that a screen reader reads why.
 */
function AtCapRefused({ children }: { children: string }) {
  const said = useRef<HTMLParagraphElement>(null);
  useEffect(() => {
    said.current?.focus();
  }, []);
  return <SettingsResult ref={said}>{children}</SettingsResult>;
}

export function InviteCreate({ onBack }: SettingsSubProps) {
  const { t } = stringsIn(useLocale().locale);
  const id = useId();
  const list = useOwnInvites();
  const { create } = useOwnInviteWrites();
  const [busy, setBusy] = useState(false);
  const [said, setSaid] = useState<string>();
  /** The value the server refused as over what it allows now, with the words under its field. */
  const [over, setOver] = useState<{ field: "max_uses" | "expires_in"; said: string }>();
  /** Whether the server refused the last invite sent, whose form then may give way to why. */
  const [refused, setRefused] = useState(false);
  /** The invite the server just created, shown until the reader leaves. */
  const [made, setMade] = useState<OwnInvite>();
  const title = t("settings.invites.create");
  const { limits, usable, canCreate } = list;
  const limitsKey = limits === undefined ? "" : `${String(limits.max_uses)}:${String(limits.max_expires_in)}`;
  const fieldIds = { max_uses: `${id}-uses`, expires_in: `${id}-life` } as const;
  const overId = over === undefined ? undefined : `${fieldIds[over.field]}-control`;
  // The field the server refused takes the focus once its words stand under it, and again when new limits give the
  // form new fields (the list read again after the refusal), so that the focus does not fall to the page.
  useEffect(() => {
    if (overId !== undefined) document.getElementById(overId)?.focus();
  }, [over, overId, limitsKey]);

  if (made !== undefined) return <Created invite={made} onDone={onBack} />;

  const back = <BackButton onPress={onBack}>{t("settings.entry.invites")}</BackButton>;
  if (limits === undefined || usable === undefined || canCreate === undefined)
    return (
      <SettingsScreen
        sub
        title={title}
        back={back}
        notice={<Unread status={list.status === "error" ? "failed" : "loading"} retry={list.refetch} />}
      />
    );
  if (!canCreate)
    return (
      <SettingsScreen sub title={title} back={back}>
        <CannotInvite reason={list.cannotCreateReason} focus={refused} />
      </SettingsScreen>
    );
  // While an invite is being created, the list read again counts it before the screen shows it: the form stays.
  if (usable >= limits.max_active && !busy)
    return (
      <SettingsScreen sub title={title} back={back}>
        <SettingsSection heading={t("settings.invites.new.heading")}>
          {refused ? (
            <AtCapRefused>{t("settings.invites.full", { max: limits.max_active })}</AtCapRefused>
          ) : (
            <SettingsNote>{t("settings.invites.full", { max: limits.max_active })}</SettingsNote>
          )}
        </SettingsSection>
      </SettingsScreen>
    );

  const lives = livesOf(limits);
  const submit = (form: HTMLFormElement) => {
    if (busy) return;
    const fields = new FormData(form);
    const uses = wholeOf(fields, "max_uses");
    const life = wholeOf(fields, "expires_in");
    setSaid(undefined);
    setOver(undefined);
    setRefused(false);
    setBusy(true);
    // A field left empty takes the server's default.
    create({
      ...(uses === undefined ? {} : { max_uses: uses }),
      ...(life === undefined ? {} : { expires_in: life }),
    }).then(setMade, (error: unknown) => {
      setBusy(false);
      setRefused(true);
      // The screen says it in the form's place, from what the data keeps of the refusal.
      if (invitesNotAllowed(error)) return;
      // A value over what the server allows now stands under its field; anything else beside the button.
      const field = inviteValueRefused(error);
      if (field === undefined) setSaid(saidOf(error, t));
      else setOver({ field, said: t("settings.invites.new.refused", { field }) });
    });
  };

  return (
    <form
      noValidate
      onSubmit={(event) => {
        event.preventDefault();
        submit(event.currentTarget);
      }}
    >
      <SettingsScreen
        sub
        title={title}
        back={back}
        foot={
          <SubmitBar result={said === undefined ? undefined : <FormAlert>{said}</FormAlert>}>
            <Button kind="primary" type="submit" busy={busy}>
              {t("settings.invites.create")}
            </Button>
          </SubmitBar>
        }
      >
        {/* New fields when the server's limits change, which start from its defaults within them. */}
        <SettingsSection key={limitsKey} heading={t("settings.invites.new.heading")}>
          <SettingRow
            form
            id={fieldIds.max_uses}
            label={t("settings.invites.new.uses")}
            about={t("settings.invites.new.uses.about", { max: limits.max_uses })}
            error={over?.field === "max_uses" ? over.said : undefined}
          >
            <NumberField
              name="max_uses"
              min={1}
              max={limits.max_uses}
              defaultValue={Math.min(limits.default_max_uses, limits.max_uses)}
            />
          </SettingRow>
          <SettingRow
            form
            id={fieldIds.expires_in}
            label={t("settings.invites.new.life")}
            error={over?.field === "expires_in" ? over.said : undefined}
          >
            <Select
              name="expires_in"
              defaultValue={String(lives.chosen)}
              options={lives.offered.map((life) => ({
                value: String(life),
                label: t("settings.invites.new.life.option", lifeIn(life)),
              }))}
            />
          </SettingRow>
        </SettingsSection>
      </SettingsScreen>
    </form>
  );
}
