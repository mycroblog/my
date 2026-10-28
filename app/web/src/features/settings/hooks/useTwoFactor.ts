// SPDX-License-Identifier: AGPL-3.0-only
// The user's two-factor authentication (FR-005): where it stands, enrolling an authenticator app (a new secret as
// a QR code and as text, confirmed by the app's first code), and, once it is on, replacing the recovery codes and
// turning it off, each confirmed by a code of the app or a recovery code. The recovery codes are shown once, as
// the server answers them; they stay on the page until it is left. A wrong code is the code field's error; any
// other failure is an alert next to the form (web-client/README.md E2, web-client/16-look-states-and-wording.md G16).
import { useMutation, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { type SyntheticEvent, useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import {
  cacheChangedTwoFactor,
  disableTotpMutation,
  enableTotpMutation,
  regenerateRecoveryCodesMutation,
  setUpTotpMutation,
  type TotpSetup,
  type TwoFactor,
  twoFactorFailure,
  twoFactorQueryOptions,
} from "../api/twoFactor";

/** A form that sends a code: the code typed so far, sending it, and why it was refused. */
export type CodeForm = {
  code: string;
  changeCode: (code: string) => void;
  submit: (event: SyntheticEvent<HTMLFormElement>) => void;
  isPending: boolean;
  /** The code field's error when the server refused the code itself, localized; undefined otherwise. */
  codeError: string | undefined;
  /** Why the form failed otherwise, localized, for an alert next to it; undefined when it did not. */
  error: string | undefined;
};

export type TwoFactorSettings = {
  status: TwoFactor;
  /** The new secret being enrolled, after `setUp()`; null before, and again once it is on or expired. */
  setup: TotpSetup | null;
  setUp: () => void;
  isSettingUp: boolean;
  /** Why the setup could not start or ended, localized; undefined when it did not fail. */
  setupError: string | undefined;
  /** Turns it on with the setup's secret and the app's code. */
  enable: CodeForm;
  regenerate: CodeForm;
  disable: CodeForm;
  /** The recovery codes that turning it on or replacing them answered, shown once; undefined before. */
  recoveryCodes: readonly string[] | undefined;
  /** What the last change did, for the page's StatusMessage; undefined before the first one. */
  result: string | undefined;
};

/**
 * The state of a code form around its mutation. A refused code is the field's error; a code that the state of the
 * two-factor authentication explains (turned on or off elsewhere) reads it again.
 */
function useCodeForm(mutation: {
  mutate: (code: string) => void;
  reset: () => void;
  isPending: boolean;
  isError: boolean;
  error: unknown;
}): CodeForm & { setCode: (code: string) => void } {
  const t = useT();
  const [code, setCode] = useState("");
  const failure = mutation.isError ? twoFactorFailure(mutation.error) : undefined;
  return {
    code,
    setCode,
    changeCode: (next) => {
      setCode(next);
      // A refused code marks the field invalid, which stops the form (native validation) until it changes.
      if (mutation.isError) mutation.reset();
    },
    submit: (event) => {
      event.preventDefault();
      if (mutation.isPending) return;
      mutation.reset();
      mutation.mutate(code.trim());
    },
    isPending: mutation.isPending,
    codeError: failure === "invalid-code" ? t("client:settings.two_factor.code.invalid") : undefined,
    error: failure === undefined || failure === "invalid-code" ? undefined : failureMessage(t, failure, mutation.error),
  };
}

/** The alert of a failure that is not the code's own. */
function failureMessage(
  t: ReturnType<typeof useT>,
  failure: ReturnType<typeof twoFactorFailure>,
  error: unknown,
): string {
  switch (failure) {
    case "setup-expired":
      return t("client:settings.two_factor.setup.expired");
    case "state-changed":
      return t("client:settings.two_factor.state_changed");
    case "rate-limited":
      return t("client:settings.two_factor.rate_limited", { reason: errorMessage(error) });
    default:
      return t("client:settings.two_factor.failed", { reason: errorMessage(error) });
  }
}

/** The two-factor authentication of the signed-in user, which the route has loaded. */
export function useTwoFactor(): TwoFactorSettings {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const status = useSuspenseQuery(twoFactorQueryOptions(api)).data;
  const [recoveryCodes, setRecoveryCodes] = useState<readonly string[] | undefined>(undefined);
  const [result, setResult] = useState<string | undefined>(undefined);
  const refreshOnStateChange = (error: unknown) => {
    if (twoFactorFailure(error) === "state-changed") void cacheChangedTwoFactor(api, queryClient);
  };

  const setUp = useMutation({ ...setUpTotpMutation(api), onError: refreshOnStateChange });
  const enableMutation = useMutation({
    ...enableTotpMutation(api),
    onSuccess: async (answer) => {
      // On once its state is fetched again (the form stays pending until then), so that the page shows it on.
      await cacheChangedTwoFactor(api, queryClient);
      setUp.reset();
      enable.setCode("");
      setRecoveryCodes(answer.recovery_codes);
      setResult(t("client:settings.two_factor.enabled"));
    },
    onError: (error) => {
      // A setup that expired gives way to the button that starts a new one.
      if (twoFactorFailure(error) === "setup-expired") setUp.reset();
      refreshOnStateChange(error);
    },
  });
  const regenerateMutation = useMutation({
    ...regenerateRecoveryCodesMutation(api),
    onSuccess: async (answer) => {
      await cacheChangedTwoFactor(api, queryClient);
      regenerate.setCode("");
      setRecoveryCodes(answer.recovery_codes);
      setResult(t("client:settings.two_factor.recovery_codes.replaced"));
    },
    onError: refreshOnStateChange,
  });
  const disableMutation = useMutation({
    ...disableTotpMutation(api),
    onSuccess: async (answer) => {
      await cacheChangedTwoFactor(api, queryClient, answer);
      disable.setCode("");
      setRecoveryCodes(undefined);
      setResult(t("client:settings.two_factor.disabled"));
    },
    onError: refreshOnStateChange,
  });

  const enableSetup = setUp.data ?? null;
  const enable = useCodeForm({
    mutate: (code) => {
      if (enableSetup !== null) enableMutation.mutate({ setup_token: enableSetup.setup_token, code });
    },
    reset: enableMutation.reset,
    isPending: enableMutation.isPending,
    isError: enableMutation.isError,
    error: enableMutation.error,
  });
  const regenerate = useCodeForm(regenerateMutation);
  const disable = useCodeForm(disableMutation);
  const enableFailure = enableMutation.isError ? twoFactorFailure(enableMutation.error) : undefined;

  return {
    status,
    setup: enableSetup,
    setUp: () => {
      if (setUp.isPending) return;
      enableMutation.reset();
      setUp.mutate();
    },
    isSettingUp: setUp.isPending,
    setupError: setUp.isError
      ? failureMessage(t, twoFactorFailure(setUp.error), setUp.error)
      : enableFailure === "setup-expired"
        ? failureMessage(t, enableFailure, enableMutation.error)
        : undefined,
    enable,
    regenerate,
    disable,
    recoveryCodes,
    result,
  };
}
