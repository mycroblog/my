// SPDX-License-Identifier: AGPL-3.0-only

// The welcome step of a new account (docs/operations/registration.md, "When a user can use the app"), at /welcome,
// where the server sends a person who can use their account at once after signing up or confirming their email
// address: a page of its own before the app, which asks for a display name and an avatar (FR-006), both optional, and
// goes on to the home timeline (`onHome`). Continue saves what was given, as the form of the profile does (app-data's
// `updateProfile`: the image uploaded first, then the one change), and goes on; with nothing given it just goes on, as
// Skip for now does. What the server refused stays on the step: under the field it names, which takes the focus, the
// alert before Continue saying the rest; a refusal that names no field leaves the focus on Continue, by the alert.
//
// The step is for an account that is new: one with neither a display name nor an avatar that has not passed the step
// on this device (device.ts). The server keeps no fact of the step, so that is all the client can tell: an account
// that skipped it is asked again on another device, and one whose owner took both away later is asked again where it
// never passed. Any other account goes on to the home timeline at once, and so does one that could not be read, whose
// failure the home timeline says. It is decided once, as the account is read: what the step saves makes the account
// one it is not for, and the step stays as it is, busy, until the app has left.

import {
  failedPicture,
  MediaFailedError,
  refusalDetail,
  useCredentials,
  useProfileRead,
  useUpdateProfile,
  type CredentialAccount,
} from "@my/app-data";
import {
  Button,
  focusRefusedField,
  Form,
  FormActions,
  FormAlert,
  FormHead,
  FormPage,
  FormText,
  useLocale,
  type FormPageServer,
} from "@my/design-system";
import { useEffect, useId, useRef, useState, type ReactNode } from "react";
import { fieldError, placeRefusal, type FieldMembers, type Refused } from "../field-errors";
import { stringsIn } from "../locale";
import { ProfileAvatarField, ProfileNameField, useProfileIdentity, type IdentityStart } from "../profile-identity";
import { noteWelcomePassed, welcomePassed } from "./device";

/** The inputs of the step, which a refusal of the server is placed on. */
type Input = "name" | "avatar";

/** Which input answers to which member of the request (`PATCH /accounts/update_credentials`). */
const members: FieldMembers<Input> = { display_name: "name" };

/** Where the step's save is, as a story's specimen opens it: on its way, or refused. */
type WelcomePhase = { readonly kind: "sending" } | { readonly kind: "refused"; readonly refused: Refused<Input> };

/** What a story's specimen of the step opens with: what the reader wrote and picked, and where its save is. */
export type WelcomeSpecimen = {
  readonly start?: IdentityStart | undefined;
  readonly phase?: WelcomePhase | undefined;
};

export type WelcomeProps = {
  /** The server the page belongs to, whose name the title says. */
  server: FormPageServer;
  /** The built-in wordmark, for light and dark. */
  wordmark: { light: string; dark: string };
  /** The page's footer (FormPageFooter). */
  footer?: ReactNode;
  /**
   * Goes on to the home timeline, in the step's place: once, when the reader continued or skipped, or at once for an
   * account the step is not for.
   */
  onHome: () => void;
  specimen?: WelcomeSpecimen | undefined;
};

/** The title of the step, on the server of `name`. */
export function useWelcomeTitle(name: string): string {
  return stringsIn(useLocale().locale).t("welcome.title", { name });
}

/** Whether the step is for `account`: it has neither a display name nor an avatar, and never passed the step here. */
const isNew = (account: CredentialAccount) =>
  account.display_name === "" && account.avatar === undefined && !welcomePassed(account.id);

/** Goes on (`leave`) once, when `when` first holds. */
function useLeaveOnce(when: boolean, leave: () => void) {
  const left = useRef(false);
  useEffect(() => {
    if (!when || left.current) return;
    left.current = true;
    leave();
  }, [when, leave]);
}

/**
 * A failed save placed on the step: an image that could not be uploaded or used, under the avatar, in the server's
 * words when it gave some, the alert then saying in the step's own words that nothing was saved; anything else as
 * every form places it (field-errors.ts).
 */
function placed(error: unknown, t: ReturnType<typeof stringsIn>["t"]): Refused<Input> {
  if (failedPicture(error) === "avatar")
    return {
      reason: undefined,
      field: {
        input: "avatar",
        text:
          error instanceof MediaFailedError
            ? t("welcome.picture.failed", { reason: "processing" })
            : (refusalDetail(error) ?? t("welcome.picture.failed", { reason: "upload" })),
      },
    };
  return placeRefusal(error, members);
}

/** Where the step's save is; `left` once the app goes on, after a save (`saved`) or with none. */
type Sent = { readonly kind: "idle" } | WelcomePhase | { readonly kind: "left"; readonly saved: boolean };

/** The step itself, for the new account `account`. */
function Step({
  account,
  title,
  onHome,
  specimen,
}: {
  account: CredentialAccount;
  title: string;
  onHome: () => void;
  specimen: WelcomeSpecimen;
}) {
  const { t } = stringsIn(useLocale().locale);
  const id = useId();
  const update = useUpdateProfile();
  const [sent, setSent] = useState<Sent>(specimen.phase ?? { kind: "idle" });
  /** A change of the step: what the last save said was about the step as it was. */
  const changed = () => {
    setSent((was) => (was.kind === "refused" ? { kind: "idle" } : was));
  };
  const identity = useProfileIdentity(account, changed, specimen.start);
  useEffect(() => {
    document.title = title;
  }, [title]);
  // The input the server refused takes the focus once its error is drawn, which comes into view with it.
  const focusNext = useRef<string | undefined>(undefined);
  useEffect(() => {
    const next = focusNext.current;
    if (next === undefined) return;
    focusNext.current = undefined;
    focusRefusedField(next);
  });

  const nameId = `${id}-name`;
  const avatarId = `${id}-avatar`;
  const refused = sent.kind === "refused" ? sent.refused : undefined;
  /** Continue stays busy from the save until the app has left. */
  const busy = sent.kind === "sending" || (sent.kind === "left" && sent.saved);
  const held = sent.kind === "sending" || sent.kind === "left";

  const leave = (saved: boolean) => {
    noteWelcomePassed(account.id);
    setSent({ kind: "left", saved });
    onHome();
  };
  const save = () => {
    if (held) return;
    // A name over its limit is not sent: its field says so already, and takes the focus.
    if (identity.nameLeft < 0) {
      focusRefusedField(nameId);
      return;
    }
    if (!identity.changed) {
      leave(false);
      return;
    }
    setSent({ kind: "sending" });
    update(identity.change).then(
      () => {
        leave(true);
      },
      (error: unknown) => {
        const refusal = placed(error, t);
        // What the refusal names takes the focus from Continue; one that names nothing leaves it there, by the alert.
        if (refusal.field !== undefined) focusNext.current = refusal.field.input === "name" ? nameId : avatarId;
        setSent({ kind: "refused", refused: refusal });
      },
    );
  };

  return (
    <>
      <FormHead title={title}>
        <FormText>{t("welcome.lead")}</FormText>
        <FormText muted>{t("welcome.later")}</FormText>
      </FormHead>
      <Form
        action=""
        onSubmit={(event) => {
          event.preventDefault();
          save();
        }}
      >
        <ProfileNameField id={nameId} identity={identity} error={fieldError(refused, "name")} busy={busy} />
        <ProfileAvatarField id={avatarId} identity={identity} error={fieldError(refused, "avatar")} busy={busy} />
        <FormActions>
          {refused !== undefined && (
            <FormAlert>
              {refused.reason ??
                t(refused.field?.input === "avatar" ? "welcome.save.picture_failed" : "welcome.save.failed")}
            </FormAlert>
          )}
          <Button kind="primary" type="submit" busy={busy}>
            {t("welcome.continue")}
          </Button>
          <Button
            kind="quiet"
            isDisabled={sent.kind === "sending"}
            onPress={() => {
              if (!held) leave(false);
            }}
          >
            {t("welcome.skip")}
          </Button>
        </FormActions>
      </Form>
    </>
  );
}

/** The account once read: the step when it is new, and for any other the way on, over the line that says it loads. */
function Arrived({
  account,
  title,
  onHome,
  specimen,
}: {
  account: CredentialAccount;
  title: string;
  onHome: () => void;
  specimen: WelcomeSpecimen;
}) {
  // Decided as the account is first read here, and never again: a saved name makes the account one the step is not for.
  const [asked] = useState(() => isNew(account));
  useLeaveOnce(!asked, onHome);
  if (!asked) return <Reading />;
  return <Step account={account} title={title} onHome={onHome} specimen={specimen} />;
}

/** In the step's place while the account is read, or while the app goes on to the home timeline. */
function Reading() {
  const { t } = stringsIn(useLocale().locale);
  return <FormText role="status">{t("welcome.loading")}</FormText>;
}

/** The page of the welcome step. */
export function Welcome({ server, wordmark, footer, onHome, specimen = {} }: WelcomeProps) {
  const title = useWelcomeTitle(server.name);
  const account = useCredentials();
  const { status } = useProfileRead();
  // An account that could not be read is not asked: the home timeline says what failed.
  useLeaveOnce(account === undefined && status === "failed", onHome);
  return (
    <FormPage server={server} wordmark={wordmark} footer={footer}>
      {account === undefined ? (
        <Reading />
      ) : (
        <Arrived key={account.id} account={account} title={title} onHome={onHome} specimen={specimen} />
      )}
    </FormPage>
  );
}
