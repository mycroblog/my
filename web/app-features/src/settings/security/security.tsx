// SPDX-License-Identifier: AGPL-3.0-only

// Security, the entry of the settings at /settings/security (FR-002, FR-004, FR-005, FR-010, FR-012): how the account
// signs in and where it is signed in.
//
// - While two-factor authentication is off, the status summary says so and is the one door to turning it on, with
//   the reason that fits the account: a password that may leak where one signs it in, else its other ways in. Once on,
//   its row in Sign-in is the door, its state the muted line under its name, with the recovery codes left where few
//   are.
// - Sign-in: the password, which opens the screen that changes or sets it (password.tsx); two-factor authentication
//   once on (two-factor.tsx); and last, removing the password, after a confirmation that it's the reader
//   (confirmation.tsx, never by the password itself), only where a passkey or a linked account remains to sign in
//   with: otherwise the row says why not and has no button.
// - Passkeys (ADR-0106 §26): each by its name and the day it was added, with Remove after a confirmation; Add leads to
//   the screen that names one (passkey.tsx).
// - Linked accounts (ADR-0106 §3.5–§3.7, §6): the login providers the server offers to link an account at and the
//   accounts linked, by the administrators' names for them, which are not translated. Link asks to confirm it's you
//   (confirmation.tsx) and then leaves the app for the provider, which sends the browser back here with what happened
//   in the fragment of the address: the section says it, once. Unlink asks a confirmation alone.
// - Signed-in browsers (ADR-0090 §11, §13): each by its browser and operating system, or the `User-Agent` itself when
//   it names neither; then, each on a line of its own, that it is this browser, the day and the address it signed in
//   from, worded as the sign-in's, not as where it is now, and when it was last used. Each other signs out after a
//   confirmation alone, as all the others do at once, and a sentence says what to do about a sign-in the reader does
//   not recognize.
//
// Where the server limits sign-in to login providers (ADR-0106 §21–§22), a password cannot be set and a passkey
// cannot be added: Sign-in says so and has no door to the password's screen, a password the account has can still be
// removed, and Passkeys is absent unless the account holds some, which it may still remove. Removing the last
// passkey or link an account signs in with is the server's to refuse: the confirmation then says why, in the server's
// words.
//
// A change that takes its own control with it (a passkey removed, an account unlinked, a browser signed out, the
// password removed) is said at the top of its section, as a status, which takes the focus once the confirmation has
// left: the button that opened it is gone, and the focus would fall to the page with nothing read.

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
  usePasswordWrites,
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
import { Asking, securityPath, securityPaths, useDates, useReadingNotice, useSectionResult } from "./parts";
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

/**
 * Sign-in: the password, which opens the screen that changes or sets it; two-factor authentication once it is on,
 * its state the muted line under its name; and last, for an account that has a password, removing it, here and not
 * on the password's screen, whose one button changes the password.
 */
function SignIn({
  limited,
  returned,
  leave,
}: {
  limited: boolean;
  returned: ExternalOutcome | null;
  leave: SettingsAddress["leave"];
}) {
  const { t } = stringsIn(useLocale().locale);
  const { day } = useDates();
  const hasPassword = useHasPassword() === true;
  const twoFactor = useTwoFactor();
  const passkeys = usePasskeys() ?? [];
  const linked = useLinkedIdentities()?.items ?? [];
  const { remove } = usePasswordWrites();
  const result = useSectionResult();
  const removing = useConfirmation({
    operation: "password_removal",
    about: t("settings.security.password.remove.confirm"),
    returnTo: securityPath,
    returned,
    leave,
    password: false,
  });
  const left = twoFactor?.recovery_codes_remaining;
  const on = twoFactor?.enabled === true;
  // Another way to sign in remains: a passkey, or a linked account.
  const removable = passkeys.length > 0 || linked.length > 0;

  const removePassword = () => {
    result.clear();
    removing.ask(
      (confirmed) =>
        // The dialog of a removal offers no password (`password: false`).
        confirmed.kind === "password"
          ? Promise.reject(new Error("settings: the password does not confirm its own removal"))
          : remove(confirmed),
      {
        done: () => {
          result.say(t("settings.security.password.removed"));
        },
        failed: (error) => {
          result.refuse(saidOf(error, t));
        },
        // The row went with the password, and its button with it.
        left: result.settle,
      },
    );
  };
  return (
    <>
      <SettingsSection heading={t("settings.security.sign_in")}>
        {limited && <SettingsNote>{t("settings.security.providers_only")}</SettingsNote>}
        {removing.notice}
        {result.line}
        {/* A password that cannot be set or changed has no door; one the account has can still be removed. */}
        {(!limited || hasPassword || on) && (
          <SettingsList label={t("settings.security.sign_in")}>
            {!limited && (
              <SettingsListRow
                icon="security"
                title={t("settings.security.password")}
                caption={t(hasPassword ? "settings.security.password.caption" : "settings.security.password.none")}
                href={securityPaths.password}
              />
            )}
            {on && (
              <SettingsListRow
                icon="twoFactor"
                title={t("settings.security.two_factor")}
                facts={[
                  twoFactor.enrolled_at === undefined
                    ? t("settings.security.two_factor.on")
                    : t("settings.security.two_factor.since", { date: day(twoFactor.enrolled_at) }),
                  ...(left !== undefined && left <= fewCodes
                    ? [t("settings.security.two_factor.codes_left", { count: left })]
                    : []),
                ]}
                href={securityPaths.twoFactor}
              />
            )}
            {hasPassword && (
              <SettingsListRow
                icon="delete"
                title={t("settings.security.password.remove")}
                caption={t(
                  !removable
                    ? "settings.security.password.remove.last"
                    : limited
                      ? "settings.security.password.remove.unused"
                      : "settings.security.password.remove.about",
                )}
                end={
                  removable ? (
                    <Button size="timeline" busy={removing.busy} onPress={removePassword}>
                      {t("settings.security.password.remove.submit")}
                    </Button>
                  ) : undefined
                }
              />
            )}
          </SettingsList>
        )}
      </SettingsSection>
      {removing.dialog}
    </>
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
  const result = useSectionResult();
  // Where none can be added and there is none to remove, there is nothing to show, but that the last one went.
  if (limited && passkeys.length === 0 && result.line === null) return null;
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
        {result.line}
        {passkeys.length === 0 ? (
          <SettingsEmpty action={limited ? undefined : add}>{t("settings.security.passkeys.none")}</SettingsEmpty>
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
                      result.clear();
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
      <ModalPresence onExited={result.settle}>
        {removing !== undefined && (
          <Asking
            title={t("settings.security.passkeys.remove.title", { name: removing.name })}
            confirm={t("settings.security.passkeys.remove")}
            run={async () => {
              await remove(removing.id);
              result.say(t("settings.security.passkeys.removed", { name: removing.name }));
            }}
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
  const result = useSectionResult();
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
  /** The name of the provider the browser came back from with a link made; none for one the server no longer lists. */
  const linkedAt = returned?.kind === "linked" ? nameOf(returned.provider) : undefined;
  return (
    <>
      <SettingsSection heading={t("settings.security.linked")} about={t("settings.security.linked.about")}>
        {returned?.kind === "linked" && (
          <SaveState state="saved">
            {linkedAt === undefined
              ? t("settings.security.linked.done.unknown")
              : t("settings.security.linked.done", { provider: linkedAt })}
          </SaveState>
        )}
        {/* What a confirmation came to is the confirmation's to say, where its operation is asked from. */}
        {returned?.kind === "error" && returned.operation === undefined && (
          <FormAlert>{providerFailure(returned, t)}</FormAlert>
        )}
        {confirmation.notice}
        {said !== undefined && <FormAlert>{said}</FormAlert>}
        {result.line}
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
                    result.clear();
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
      <ModalPresence onExited={result.settle}>
        {unlinking !== undefined && (
          <Asking
            title={t("settings.security.linked.unlink.title", { provider: unlinking.provider_name })}
            confirm={t("settings.security.linked.unlink")}
            run={async () => {
              await unlink(unlinking.id);
              result.say(t("settings.security.linked.unlinked", { provider: unlinking.provider_name }));
            }}
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

/** Whether the server kept nothing of which browser signed in. */
const unknownBrowser = (session: BrowserSession) =>
  session.user_agent === undefined || session.user_agent.trim() === "";

/** The name of a browser of the list: its browser and operating system, the string it sent, or that none is known. */
function browserName(session: BrowserSession, t: T): string {
  if (session.user_agent === undefined || unknownBrowser(session)) return t("settings.security.browsers.unknown");
  const named = browserOf(session.user_agent);
  return named === undefined ? session.user_agent : t("settings.security.browsers.name", named);
}

/**
 * Signed-in browsers: each with where and when it signed in, and the way to sign it out, or all the others. The
 * advice under them names changing the password only for an account that can (`changesPassword`).
 */
function Browsers({ changesPassword }: { changesPassword: boolean }) {
  const { locale } = useLocale();
  const { t } = stringsIn(locale);
  const { day, hour } = useDates();
  const sessions = useBrowserSessions() ?? [];
  const { revoke, revokeOthers } = useBrowserSessionWrites();
  const [asking, setAsking] = useState<BrowserSession | "others">();
  const result = useSectionResult();
  const others = sessions.some((session) => !session.current);
  // Each fact on a line of its own, short enough for a phone beside the row's button, the reader's own browser said
  // first. The address stands right under the sign-in it is of, not as where the browser is now (ADR-0090 §11); what
  // was not recorded is unknown.
  const facts = (session: BrowserSession) =>
    [
      session.current ? t("settings.security.browsers.current") : undefined,
      t("settings.security.browsers.signed_in", { date: day(session.signed_in_at) }),
      session.ip === undefined
        ? t("settings.security.browsers.signed_in.from.unknown")
        : t("settings.security.browsers.signed_in.from", { ip: session.ip }),
      session.last_used_at === undefined
        ? undefined
        : t("settings.security.browsers.last_used", { date: hour(session.last_used_at) }),
    ].filter((fact) => fact !== undefined);
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
                result.clear();
                setAsking("others");
              }}
            >
              {t("settings.security.browsers.sign_out_others")}
            </Button>
          ) : undefined
        }
      >
        {result.line}
        <SettingsList label={t("settings.security.browsers")}>
          {sessions.map((session) => (
            <SettingsListRow
              key={session.id}
              icon="public"
              title={browserName(session, t)}
              facts={facts(session)}
              end={
                session.current ? undefined : (
                  <Button
                    size="timeline"
                    onPress={() => {
                      result.clear();
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
            ...(changesPassword
              ? [{ href: securityPaths.password, label: t("settings.security.browsers.advice.password") }]
              : []),
            { href: "/settings/apps", label: t("settings.security.browsers.advice.apps") },
          ]}
        >
          {t(changesPassword ? "settings.security.browsers.advice" : "settings.security.browsers.advice.no_password")}
        </SettingsNote>
      </SettingsSection>
      <ModalPresence onExited={result.settle}>
        {asking === "others" ? (
          <Asking
            key="others"
            title={t("settings.security.browsers.sign_out_others.title")}
            confirm={t("settings.security.browsers.sign_out.confirm")}
            run={async () => {
              await revokeOthers();
              result.say(t("settings.security.browsers.signed_out_others"));
            }}
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
              title={
                unknownBrowser(asking)
                  ? t("settings.security.browsers.sign_out.title.unknown")
                  : t("settings.security.browsers.sign_out.title", { browser: browserName(asking, t) })
              }
              confirm={t("settings.security.browsers.sign_out.confirm")}
              run={async () => {
                await revoke(asking.id);
                result.say(
                  unknownBrowser(asking)
                    ? t("settings.security.browsers.signed_out.unknown")
                    : t("settings.security.browsers.signed_out", { browser: browserName(asking, t) }),
                );
              }}
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
  const hasPassword = useHasPassword() === true;
  const limited = useInstance()?.login_providers_only === true;
  const returned = useProviderReturn({ fragment, onFragmentRead });
  return (
    <>
      {/*
        While it is off, the summary is the one place that says so and the one door to it. Its reason is the
        account's own: a password that may leak, where one signs it in; else the other ways it signs in with.
      */}
      {twoFactor?.enabled === false && (
        <StatusSummary
          icon="caution"
          title={t("settings.security.two_factor.off")}
          about={t(
            hasPassword && !limited
              ? "settings.security.two_factor.off.about"
              : "settings.security.two_factor.off.about.passwordless",
          )}
          action={
            <LinkButton kind="primary" href={securityPaths.twoFactor}>
              {t("settings.security.two_factor.turn_on")}
            </LinkButton>
          }
        />
      )}
      <SignIn limited={limited} returned={returned} leave={leave} />
      <Passkeys limited={limited} />
      <Linked returned={returned} leave={leave} />
      <Browsers changesPassword={hasPassword && !limited} />
    </>
  );
}

/** The screen of the entry: its sections once what they show is read, and until then that it loads or failed. */
export function SecuritySettings({ back, fragment, onFragmentRead, leave }: SettingsEntryProps) {
  const { t } = stringsIn(useLocale().locale);
  const { status, retry } = useSecurityRead();
  const notice = useReadingNotice(status, retry);
  return (
    <SettingsScreen icon="security" title={t("settings.entry.security")} back={back} notice={notice}>
      {status === "read" && <Sections fragment={fragment} onFragmentRead={onFragmentRead} leave={leave} />}
    </SettingsScreen>
  );
}
