// SPDX-License-Identifier: AGPL-3.0-only

// Security, the entry of the settings at /settings/security (FR-002, FR-004, FR-005, FR-010, FR-012): how the account
// signs in and where it is signed in.
//
// - While two-factor authentication is off, the status summary says so and is the one door to turning it on; once on,
//   its row in Sign-in is the door, its state in words at the row's end, and the recovery codes left where few are.
// - Sign-in: the password, which opens its screen (password.tsx), and two-factor authentication once on
//   (two-factor.tsx).
// - Passkeys (ADR-0106 §26): each by its name and the day it was added, with Remove after a confirmation; Add leads to
//   the screen that names one (passkey.tsx).
// - Linked accounts (ADR-0106 §3.5–§3.7, §6): the login providers the server offers to link an account at and the
//   accounts linked, by the administrators' names for them, which are not translated. Link asks to confirm it's you
//   (confirmation.tsx) and then leaves the app for the provider, which sends the browser back here with what happened
//   in the fragment of the address: the section says it, once. Unlink asks a confirmation alone.
// - Signed-in browsers (ADR-0090 §11, §13): each by its browser and operating system, or the `User-Agent` itself when
//   it names neither; the day and the address it signed in from, worded as the sign-in's, not as where it is now; when
//   it was last used; and which one is this browser. Each other signs out after a confirmation alone, as all the
//   others do at once, and a sentence says what to do about a sign-in the reader does not recognize.
//
// Where the server limits sign-in to login providers (ADR-0106 §21–§22), a password cannot be set and a passkey
// cannot be added: Sign-in says so, the password's row stays only for an account that has one (to remove it), and
// Passkeys is absent unless the account holds some, which it may still remove. Removing the last way to sign in is the
// server's to refuse: the confirmation then says why, in the server's words.

import {
  instanceQuery,
  loadSecurity,
  useBrowserSessions,
  useBrowserSessionWrites,
  useHasPassword,
  useInstance,
  useLinkedIdentities,
  useLinkedIdentityWrites,
  usePasskeys,
  usePasskeyWrites,
  useSecurityRead,
  useTwoFactor,
  type AppData,
  type BrowserSession,
  type ExternalOutcome,
  type LinkableLoginProvider,
  type LinkedIdentity,
  type Passkey,
} from "@my/app-data";
import {
  Button,
  FormAlert,
  LinkButton,
  ModalPresence,
  SaveState,
  SettingsEmpty,
  SettingsList,
  SettingsListRow,
  SettingsNote,
  SettingsScreen,
  SettingsSection,
  StatusSummary,
  useLocale,
  type IconName,
} from "@my/design-system";
import { useState } from "react";
import { stringsIn } from "../../locale";
import { providerFailure, saidOf, useConfirmation, useProviderName, useProviderReturn } from "../confirmation";
import type { SettingsAddress, SettingsEntryProps } from "../entries";
import { Asking, Reading, securityPath, securityPaths, useDates } from "./parts";
import { browserOf } from "./user-agent";

type T = ReturnType<typeof stringsIn>["t"];

/** What the route's loader waits for: what the screen shows, and the instance, from the cache when it has them. */
export async function loadSecuritySettings(data: AppData): Promise<void> {
  await Promise.all([
    loadSecurity(data),
    data.queryClient.query({ ...instanceQuery(data), staleTime: Infinity }).catch(() => undefined),
  ]);
}

/** How few recovery codes are left before the row of two-factor authentication says how many. */
const fewCodes = 3;

/** How a number reads in the reader's language. */
const numberIn = (locale: string, n: number) => new Intl.NumberFormat(locale).format(n);

/** Sign-in: the password, and two-factor authentication once it is on. */
function SignIn({ limited }: { limited: boolean }) {
  const { t } = stringsIn(useLocale().locale);
  const { day } = useDates();
  const hasPassword = useHasPassword() === true;
  const twoFactor = useTwoFactor();
  const left = twoFactor?.recovery_codes_remaining;
  // A password that cannot be set has no row; one the account has can still be removed.
  const password = hasPassword || !limited;
  const on = twoFactor?.enabled === true;
  return (
    <SettingsSection heading={t("settings.security.sign_in")}>
      {limited && <SettingsNote>{t("settings.security.providers_only")}</SettingsNote>}
      {(password || on) && (
        <SettingsList label={t("settings.security.sign_in")}>
          {password && (
            <SettingsListRow
              icon="security"
              title={t("settings.security.password")}
              caption={t(
                !hasPassword
                  ? "settings.security.password.none"
                  : limited
                    ? "settings.security.password.unused"
                    : "settings.security.password.caption",
              )}
              href={securityPaths.password}
            />
          )}
          {on && (
            <SettingsListRow
              icon="time"
              title={t("settings.security.two_factor")}
              caption={
                left !== undefined && left <= fewCodes
                  ? t("settings.security.two_factor.codes_left", { count: left })
                  : twoFactor.enrolled_at === undefined
                    ? undefined
                    : t("settings.security.two_factor.since", { date: day(twoFactor.enrolled_at) })
              }
              state={t("settings.security.two_factor.on")}
              href={securityPaths.twoFactor}
            />
          )}
        </SettingsList>
      )}
    </SettingsSection>
  );
}

/** Passkeys: the account's, each with Remove, and the way to add one. */
function Passkeys({ limited }: { limited: boolean }) {
  const { locale } = useLocale();
  const { t } = stringsIn(locale);
  const { day } = useDates();
  const passkeys = usePasskeys() ?? [];
  const { remove } = usePasskeyWrites();
  const [removing, setRemoving] = useState<Passkey>();
  // Where none can be added and there is none to remove, there is nothing to show.
  if (limited && passkeys.length === 0) return null;
  const add = (
    <LinkButton size="timeline" href={securityPaths.passkey}>
      {t("settings.security.passkeys.add")}
    </LinkButton>
  );
  return (
    <>
      <SettingsSection
        heading={t("settings.security.passkeys")}
        count={passkeys.length === 0 ? undefined : numberIn(locale, passkeys.length)}
        about={t(limited ? "settings.security.passkeys.unused" : "settings.security.passkeys.about")}
        action={limited || passkeys.length === 0 ? undefined : add}
      >
        {passkeys.length === 0 ? (
          <SettingsEmpty action={add}>{t("settings.security.passkeys.none")}</SettingsEmpty>
        ) : (
          <SettingsList label={t("settings.security.passkeys")}>
            {passkeys.map((passkey) => (
              <SettingsListRow
                key={passkey.id}
                icon="passkey"
                title={passkey.name}
                caption={t("settings.security.passkeys.added", { date: day(passkey.created_at) })}
                end={
                  <Button
                    size="timeline"
                    onPress={() => {
                      setRemoving(passkey);
                    }}
                  >
                    {t("settings.security.passkeys.remove")}
                  </Button>
                }
              />
            ))}
          </SettingsList>
        )}
      </SettingsSection>
      <ModalPresence>
        {removing !== undefined && (
          <Asking
            title={t("settings.security.passkeys.remove.title", { name: removing.name })}
            confirm={t("settings.security.passkeys.remove")}
            run={() => remove(removing.id)}
            onClose={() => {
              setRemoving(undefined);
            }}
          >
            {t("settings.security.passkeys.remove.text")}
          </Asking>
        )}
      </ModalPresence>
    </>
  );
}

/** The mark of a login provider of a kind the design system has a glyph for; a link's for any other. */
const providerGlyphs: Readonly<Record<string, IconName>> = { github: "github", discord: "discord" };
const glyphOf = (kind: string): IconName => providerGlyphs[kind] ?? "link";

/** Linked accounts: the accounts linked, and the providers an account may be linked at. */
function Linked({ returned, leave }: { returned: ExternalOutcome | null; leave: SettingsAddress["leave"] }) {
  const { t } = stringsIn(useLocale().locale);
  const { day } = useDates();
  const linked = useLinkedIdentities();
  const { beginLink, unlink } = useLinkedIdentityWrites();
  const nameOf = useProviderName();
  const [unlinking, setUnlinking] = useState<LinkedIdentity>();
  const [said, setSaid] = useState<string>();
  const confirmation = useConfirmation({
    operation: "link",
    about: t("settings.security.linked.confirm"),
    returnTo: securityPath,
    returned,
    leave,
  });
  const items = linked?.items ?? [];
  const open = (linked?.providers ?? []).filter(
    (provider) => provider.allow_link && !items.some((identity) => identity.provider_id === provider.id),
  );
  if (items.length === 0 && open.length === 0) return null;

  const link = (provider: LinkableLoginProvider) => {
    setSaid(undefined);
    confirmation.ask(
      async (confirmed) => {
        // The provider's page takes the app's place, and sends the browser back to this screen.
        leave((await beginLink(provider.id, confirmed, securityPath)).url);
      },
      {
        failed: (error) => {
          setSaid(saidOf(error, t));
        },
      },
    );
  };
  return (
    <>
      <SettingsSection heading={t("settings.security.linked")} about={t("settings.security.linked.about")}>
        {returned?.kind === "linked" && (
          <SaveState state="saved">
            {t("settings.security.linked.done", { provider: nameOf(returned.provider) })}
          </SaveState>
        )}
        {/* What a confirmation of the link came to is the confirmation's to say. */}
        {returned?.kind === "error" && returned.operation !== "link" && (
          <FormAlert>{providerFailure(returned, t)}</FormAlert>
        )}
        {confirmation.notice}
        {said !== undefined && <FormAlert>{said}</FormAlert>}
        <SettingsList label={t("settings.security.linked")}>
          {items.map((identity) => (
            <SettingsListRow
              key={identity.id}
              icon={glyphOf(identity.provider_kind)}
              title={identity.provider_name}
              caption={t(
                identity.available ? "settings.security.linked.since" : "settings.security.linked.since.unavailable",
                { date: day(identity.linked_at) },
              )}
              end={
                <Button
                  size="timeline"
                  onPress={() => {
                    setUnlinking(identity);
                  }}
                >
                  {t("settings.security.linked.unlink")}
                </Button>
              }
            />
          ))}
          {open.map((provider) => (
            <SettingsListRow
              key={provider.id}
              icon={glyphOf(provider.kind)}
              title={provider.display_name}
              caption={t("settings.security.linked.not_linked")}
              end={
                <Button
                  size="timeline"
                  busy={confirmation.busy}
                  onPress={() => {
                    link(provider);
                  }}
                >
                  {t("settings.security.linked.link")}
                </Button>
              }
            />
          ))}
        </SettingsList>
      </SettingsSection>
      {confirmation.dialog}
      <ModalPresence>
        {unlinking !== undefined && (
          <Asking
            title={t("settings.security.linked.unlink.title", { provider: unlinking.provider_name })}
            confirm={t("settings.security.linked.unlink")}
            run={() => unlink(unlinking.id)}
            onClose={() => {
              setUnlinking(undefined);
            }}
          >
            {t("settings.security.linked.unlink.text")}
          </Asking>
        )}
      </ModalPresence>
    </>
  );
}

/** The name of a browser of the list: its browser and operating system, the string it sent, or that none is known. */
function browserName(session: BrowserSession, t: T): string {
  if (session.user_agent === undefined || session.user_agent.trim() === "")
    return t("settings.security.browsers.unknown");
  const named = browserOf(session.user_agent);
  return named === undefined ? session.user_agent : t("settings.security.browsers.name", named);
}

/** Signed-in browsers: each with where and when it signed in, and the way to sign it out, or all the others. */
function Browsers() {
  const { locale } = useLocale();
  const { t } = stringsIn(locale);
  const { day, hour } = useDates();
  const sessions = useBrowserSessions() ?? [];
  const { revoke, revokeOthers } = useBrowserSessionWrites();
  const [asking, setAsking] = useState<BrowserSession | "others">();
  const others = sessions.some((session) => !session.current);
  // The place is the sign-in's, not where the browser is now (ADR-0090 §11); what was not recorded is unknown.
  const caption = (session: BrowserSession) =>
    [
      session.ip === undefined
        ? t("settings.security.browsers.signed_in.unknown", { date: day(session.signed_in_at) })
        : t("settings.security.browsers.signed_in", { date: day(session.signed_in_at), ip: session.ip }),
      session.last_used_at === undefined
        ? undefined
        : t("settings.security.browsers.last_used", { date: hour(session.last_used_at) }),
      session.current ? t("settings.security.browsers.current") : undefined,
    ]
      .filter((part) => part !== undefined)
      .join(" · ");
  return (
    <>
      <SettingsSection
        heading={t("settings.security.browsers")}
        count={numberIn(locale, sessions.length)}
        action={
          others ? (
            <Button
              size="timeline"
              onPress={() => {
                setAsking("others");
              }}
            >
              {t("settings.security.browsers.sign_out_others")}
            </Button>
          ) : undefined
        }
      >
        <SettingsList label={t("settings.security.browsers")}>
          {sessions.map((session) => (
            <SettingsListRow
              key={session.id}
              icon="public"
              title={browserName(session, t)}
              caption={caption(session)}
              end={
                session.current ? undefined : (
                  <Button
                    size="timeline"
                    onPress={() => {
                      setAsking(session);
                    }}
                  >
                    {t("settings.security.browsers.sign_out")}
                  </Button>
                )
              }
            />
          ))}
        </SettingsList>
        <SettingsNote
          links={[
            { href: securityPaths.password, label: t("settings.security.browsers.advice.password") },
            { href: "/settings/apps", label: t("settings.security.browsers.advice.apps") },
          ]}
        >
          {t("settings.security.browsers.advice")}
        </SettingsNote>
      </SettingsSection>
      <ModalPresence>
        {asking === "others" ? (
          <Asking
            key="others"
            title={t("settings.security.browsers.sign_out_others.title")}
            confirm={t("settings.security.browsers.sign_out.confirm")}
            run={revokeOthers}
            onClose={() => {
              setAsking(undefined);
            }}
          >
            {t("settings.security.browsers.sign_out_others.text")}
          </Asking>
        ) : (
          asking !== undefined && (
            <Asking
              key={asking.id}
              title={t("settings.security.browsers.sign_out.title", { browser: browserName(asking, t) })}
              confirm={t("settings.security.browsers.sign_out.confirm")}
              run={() => revoke(asking.id)}
              onClose={() => {
                setAsking(undefined);
              }}
            >
              {t("settings.security.browsers.sign_out.text")}
            </Asking>
          )
        )}
      </ModalPresence>
    </>
  );
}

/** The sections of the screen, once what they show is read. */
function Sections({ fragment, onFragmentRead, leave }: SettingsAddress) {
  const { t } = stringsIn(useLocale().locale);
  const twoFactor = useTwoFactor();
  const limited = useInstance()?.login_providers_only === true;
  const returned = useProviderReturn({ fragment, onFragmentRead });
  return (
    <>
      {/* While it is off, the summary is the one place that says so and the one door to it. */}
      {twoFactor?.enabled === false && (
        <StatusSummary
          icon="caution"
          title={t("settings.security.two_factor.off")}
          about={t("settings.security.two_factor.off.about")}
          action={
            <LinkButton kind="primary" href={securityPaths.twoFactor}>
              {t("settings.security.two_factor.turn_on")}
            </LinkButton>
          }
        />
      )}
      <SignIn limited={limited} />
      <Passkeys limited={limited} />
      <Linked returned={returned} leave={leave} />
      <Browsers />
    </>
  );
}

/** The screen of the entry: its sections once what they show is read, and until then that it loads or failed. */
export function SecuritySettings({ back, fragment, onFragmentRead, leave }: SettingsEntryProps) {
  const { t } = stringsIn(useLocale().locale);
  const { status, retry } = useSecurityRead();
  return (
    <SettingsScreen icon="security" title={t("settings.entry.security")} back={back}>
      {status === "read" ? (
        <Sections fragment={fragment} onFragmentRead={onFragmentRead} leave={leave} />
      ) : (
        <Reading status={status} retry={retry} />
      )}
    </SettingsScreen>
  );
}
