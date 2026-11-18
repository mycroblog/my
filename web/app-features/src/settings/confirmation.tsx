// SPDX-License-Identifier: AGPL-3.0-only

// The confirmation that asks the reader to authenticate again (ADR-0106 §3.7), shared by every screen of the settings
// whose operation the server takes only with a `Confirmation`: changing or removing the password, adding a passkey,
// linking an account, and moving, deactivating or deleting the account. A screen calls `useConfirmation` with its
// operation, places the hook's `dialog` once and its `notice` where the reader looks, and runs its write through
// `ask`: the dialog opens, the reader proves it is them, and the write runs with that proof while the dialog spins.
//
// The ways are the account's own, one at a time, the others a press away under it: the current password (never for
// removing it), a passkey, where the account has one and the browser can use it, and each linked account whose
// provider is enabled. They are offered in that order, GitHub after every other provider: it cannot ask the reader to
// sign in again, so a reader who can confirm another way is offered that first. A proof the server refuses (a wrong
// password, a passkey that does not verify, a confirmation that expired, too many tries) stays in the dialog, in the
// server's words, for another try; anything else the write fails for closes the dialog and is the screen's to say.
//
// A linked account confirms on its provider's page: the app is left for it (`leave`), and the provider sends the
// browser back to the screen's own address (`returnTo`) with the confirmation in the fragment, which the screen reads
// once (`useProviderReturn`). What the reader had entered is gone with the page, so nothing resumes by itself: the
// notice says that they confirmed and asks them to do it again, and that next `ask` runs with the provider's
// confirmation and opens no dialog. The confirmation works once, for ten minutes; one the server no longer takes opens
// the dialog, which says so.

import {
  confirmationRefused,
  readExternalOutcome,
  refusal,
  useConfirmWithPasskey,
  useHasPassword,
  useLinkedIdentities,
  useLinkedIdentityWrites,
  usePasskeys,
  type Confirmation,
  type ConfirmedOperation,
  type ExternalOutcome,
  type LinkedIdentity,
} from "@my/app-data";
import {
  FormAlert,
  ModalPresence,
  ReauthDialog,
  SaveState,
  Stack,
  TextButton,
  TextField,
  useLocale,
} from "@my/design-system";
import { PasskeyError, supportsPasskeys } from "@my/platform";
import { useEffect, useId, useState, type ReactNode } from "react";
import { stringsIn } from "../locale";
import type { SettingsAddress } from "./entries";

type T = ReturnType<typeof stringsIn>["t"];

/**
 * What a login provider's return left in the fragment of the screen's address (a link made, a confirmation, or why
 * neither), read once as the screen opens; the fragment is then taken off the address. Null without one.
 */
export function useProviderReturn({
  fragment,
  onFragmentRead,
}: Pick<SettingsAddress, "fragment" | "onFragmentRead">): ExternalOutcome | null {
  const [outcome] = useState(() => readExternalOutcome(fragment));
  // Read as the screen opened; it stays on the address until the app has taken it off.
  const left = outcome !== null && fragment !== "";
  useEffect(() => {
    if (left) onFragmentRead();
  }, [left, onFragmentRead]);
  return outcome;
}

/** What the server said of a write it refused, in the reader's language; the settings' own words for no answer. */
export function saidOf(error: unknown, t: T): string {
  return refusal(error) ?? t("settings.confirm.failed");
}

/** Why a link or a confirmation at a login provider did not happen, as a sentence. */
export function providerFailure(outcome: Extract<ExternalOutcome, { kind: "error" }>, t: T): string {
  return t("settings.confirm.external.error", { reason: outcome.reason });
}

/** A way to confirm: the password, a passkey, or a linked account at its provider. */
type Way =
  | { readonly key: "password" }
  | { readonly key: "passkey" }
  | { readonly key: string; readonly identity: LinkedIdentity };

/**
 * The ways the account can confirm with in this browser, in the order they are offered. The screen reads them as it
 * opens, before the reader asks, so that the dialog opens on the one it will keep.
 */
function useWays(password: boolean): readonly Way[] {
  const hasPassword = useHasPassword();
  const passkeys = usePasskeys();
  const linked = useLinkedIdentities();
  const providers = (linked?.items ?? []).filter((identity) => identity.available);
  // GitHub cannot ask for a new sign-in (ADR-0106 §3.7): every other way comes first.
  const ordered = [
    ...providers.filter((identity) => identity.provider_kind !== "github"),
    ...providers.filter((identity) => identity.provider_kind === "github"),
  ];
  return [
    ...(password && hasPassword === true ? [{ key: "password" } as const] : []),
    ...(passkeys !== undefined && passkeys.length > 0 && supportsPasskeys() ? [{ key: "passkey" } as const] : []),
    ...ordered.map((identity) => ({ key: `provider:${identity.id}`, identity })),
  ];
}

/** A write that waits for its confirmation, and what follows it. */
type Asked = {
  readonly run: (confirmation: Confirmation) => Promise<unknown>;
  readonly done: (() => void) | undefined;
  readonly failed: ((error: unknown) => void) | undefined;
  /** What the dialog says as it opens: the server's refusal of a provider's confirmation. */
  readonly said: string | undefined;
};

type DialogProps = {
  operation: ConfirmedOperation;
  about: string;
  returnTo: string;
  leave: (href: string) => void;
  ways: readonly Way[];
  asked: Asked;
  onClose: () => void;
};

function ConfirmationDialog({ operation, about, returnTo, leave, ways, asked, onClose }: DialogProps) {
  const { t } = stringsIn(useLocale().locale);
  const fieldId = useId();
  const confirmWithPasskey = useConfirmWithPasskey();
  const { beginConfirmation } = useLinkedIdentityWrites();
  const [chosen, setChosen] = useState<string>();
  const [typed, setTyped] = useState("");
  const [busy, setBusy] = useState(false);
  const [said, setSaid] = useState(asked.said);
  const way = ways.find((w) => w.key === chosen) ?? ways[0];

  const cancel = {
    title: t("settings.confirm.title"),
    closeLabel: t("settings.confirm.close"),
    cancel: t("settings.confirm.cancel"),
    onCancel: onClose,
  };
  // An account that can prove nothing here (its passkeys alone, in a browser without them): the dialog says so.
  if (way === undefined)
    return (
      <ReauthDialog
        {...cancel}
        about={about}
        confirm={t("settings.confirm.close")}
        onConfirm={onClose}
        error={<FormAlert>{t("settings.confirm.none")}</FormAlert>}
      />
    );

  const refuse = (words: string) => {
    setBusy(false);
    setSaid(words);
  };
  /** The proof of the way chosen; undefined when the browser's prompt gave none, or the app was left for a provider. */
  const prove = async (): Promise<Confirmation | undefined> => {
    if (way.key === "password") return { kind: "password", password: typed };
    if (!("identity" in way)) {
      try {
        return await confirmWithPasskey();
      } catch (error) {
        const reason = error instanceof PasskeyError ? error.reason : undefined;
        if (reason === "aborted") setBusy(false);
        else refuse(reason === undefined ? saidOf(error, t) : t("settings.confirm.passkey.failed", { reason }));
        return undefined;
      }
    }
    try {
      const start = await beginConfirmation(way.identity.provider_id, operation, returnTo);
      // The dialog keeps spinning until the provider's page takes the app's place.
      leave(start.url);
    } catch (error) {
      refuse(saidOf(error, t));
    }
    return undefined;
  };
  const confirm = () => {
    if (way.key === "password" && typed === "") return;
    setBusy(true);
    setSaid(undefined);
    void prove().then((confirmation) => {
      if (confirmation === undefined) return;
      asked.run(confirmation).then(
        () => {
          onClose();
          asked.done?.();
        },
        (error: unknown) => {
          if (confirmationRefused(error, confirmation.kind)) {
            refuse(saidOf(error, t));
            return;
          }
          onClose();
          asked.failed?.(error);
        },
      );
    });
  };

  const others = ways.filter((w) => w !== way);
  const provider = "identity" in way ? way.identity.provider_name : undefined;
  return (
    <ReauthDialog
      {...cancel}
      about={provider === undefined ? about : t("settings.confirm.provider.about", { provider })}
      confirm={
        provider !== undefined
          ? t("settings.confirm.provider", { provider })
          : way.key === "passkey"
            ? t("settings.confirm.passkey")
            : t("settings.confirm.submit")
      }
      onConfirm={confirm}
      busy={busy}
      error={said === undefined ? undefined : <FormAlert>{said}</FormAlert>}
      other={
        others.length === 0 ? undefined : (
          <Stack gap="hair" justify="start">
            {others.map((w) => (
              <TextButton
                key={w.key}
                isDisabled={busy}
                onPress={() => {
                  setChosen(w.key);
                  setSaid(undefined);
                }}
              >
                {"identity" in w
                  ? t("settings.confirm.other.provider", { provider: w.identity.provider_name })
                  : w.key === "password"
                    ? t("settings.confirm.other.password")
                    : t("settings.confirm.other.passkey")}
              </TextButton>
            ))}
          </Stack>
        )
      }
    >
      {way.key === "password" && (
        <TextField
          key="password"
          id={fieldId}
          name="password"
          type="password"
          label={t("settings.confirm.password")}
          autoComplete="current-password"
          required
          onChange={setTyped}
        />
      )}
    </ReauthDialog>
  );
}

export type ConfirmationOptions = Pick<SettingsAddress, "leave"> & {
  /** The operation the server confirms. */
  operation: ConfirmedOperation;
  /** Why the dialog asks, in a sentence (To change your password, confirm it's you.). */
  about: string;
  /** The app's address a login provider sends the browser back to: the screen's own. */
  returnTo: string;
  /** What a login provider's return left in the screen's address (`useProviderReturn`). */
  returned: ExternalOutcome | null;
  /** Whether the current password confirms the operation: not its own removal. */
  password?: boolean | undefined;
};

export type Confirmed = {
  /**
   * Runs `run` once the reader has confirmed, with their confirmation: after the dialog, or at once with the
   * confirmation a login provider gave. `done` follows a write that succeeded; `failed`, with the error, one that
   * failed for anything but its confirmation. Cancelling the dialog runs neither.
   */
  ask: (
    run: (confirmation: Confirmation) => Promise<unknown>,
    then?: { done?: () => void; failed?: (error: unknown) => void },
  ) => void;
  /** A write runs on a login provider's confirmation, with no dialog: the control that asked spins. */
  busy: boolean;
  /** The dialog, which the screen places once. */
  dialog: ReactNode;
  /**
   * What came back from a login provider for this operation, for the screen to say where the reader looks: that they
   * confirmed and what to do now, or why the confirmation did not happen. Null with nothing to say.
   */
  notice: ReactNode;
};

/**
 * The name of the provider of `id`, as the administrators wrote it, among the server's providers and the account's
 * links; undefined for one neither lists, of which a screen then speaks in a sentence of its own, with no name.
 */
export function useProviderName(): (id: string | undefined) => string | undefined {
  const linked = useLinkedIdentities();
  return (id) =>
    linked?.providers.find((provider) => provider.id === id)?.display_name ??
    linked?.items.find((identity) => identity.provider_id === id)?.provider_name;
}

/** The confirmation of `operation` for a screen of the settings (see the head of this file). */
export function useConfirmation({
  operation,
  about,
  returnTo,
  returned,
  leave,
  password = true,
}: ConfirmationOptions): Confirmed {
  const { t } = stringsIn(useLocale().locale);
  const nameOf = useProviderName();
  const ways = useWays(password);
  const own = returned !== null && returned.kind !== "linked" && returned.operation === operation ? returned : null;
  const [token, setToken] = useState(own?.kind === "confirmation" ? own.token : undefined);
  const [busy, setBusy] = useState(false);
  const [asked, setAsked] = useState<Asked>();

  const ask: Confirmed["ask"] = (run, then) => {
    const next = { run, done: then?.done, failed: then?.failed };
    if (token === undefined) {
      setAsked({ ...next, said: undefined });
      return;
    }
    // The provider's confirmation works once.
    setToken(undefined);
    setBusy(true);
    run({ kind: "external", token }).then(
      () => {
        setBusy(false);
        next.done?.();
      },
      (error: unknown) => {
        setBusy(false);
        if (confirmationRefused(error, "external")) setAsked({ ...next, said: saidOf(error, t) });
        else next.failed?.(error);
      },
    );
  };

  const provider = nameOf(own?.provider);
  const notice =
    own?.kind === "error" ? (
      <FormAlert>{providerFailure(own, t)}</FormAlert>
    ) : token !== undefined ? (
      <SaveState state="saved">
        {provider === undefined ? t("settings.confirm.returned.unknown") : t("settings.confirm.returned", { provider })}
      </SaveState>
    ) : null;

  return {
    ask,
    busy,
    notice,
    dialog: (
      <ModalPresence>
        {asked !== undefined && (
          <ConfirmationDialog
            operation={operation}
            about={about}
            returnTo={returnTo}
            leave={leave}
            ways={ways}
            asked={asked}
            onClose={() => {
              setAsked(undefined);
            }}
          />
        )}
      </ModalPresence>
    ),
  };
}
