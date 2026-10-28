// SPDX-License-Identifier: AGPL-3.0-only
// Asking for the verification email again from the notice that a hard bounce stopped the emails to the user's address
// (ADR-0086 §5). The result shows next to the button (web-client/16-look-states-and-wording.md G24, G16): that the
// email is on its way, or why it is not, with when to ask again after the server's limit of requests.
import { useNotice } from "@my/ui";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useIntl } from "react-intl";
import { useApi } from "../../../lib/auth/SessionProvider";
import { ApiError, errorMessage, rateLimitedUntil } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import { refreshPreferencesAfterVerification, resendVerificationEmailMutation } from "../api/me";

/** The problem type of the 409 for an address that is verified and receives email again. */
const alreadyVerifiedType = "/api/v1/problems/email-already-verified";

export type VerificationEmail = {
  send: () => void;
  isPending: boolean;
  /** That the email is on its way, localized, after a request succeeded; undefined otherwise. */
  sent: string | undefined;
  /** Why no email was sent and what to do, localized, after a request failed; undefined otherwise. */
  error: string | undefined;
};

/**
 * The button that sends a new verification email. A success says so in place and reads it out; a failure says why
 * in place (an alert). When the server says the address no longer needs it, the preferences are fetched again, and
 * the notice goes away with the suppression.
 */
export function useVerificationEmail(): VerificationEmail {
  const api = useApi();
  const t = useT();
  const intl = useIntl();
  const notice = useNotice();
  const queryClient = useQueryClient();
  const resend = useMutation({
    ...resendVerificationEmailMutation(api),
    onSuccess: () => {
      // The text next to the button shows it.
      notice.inPlace(t("client:settings.email_suppression.verification.sent"));
    },
    onError: async (error) => {
      if (error instanceof ApiError && error.type === alreadyVerifiedType)
        await refreshPreferencesAfterVerification(api, queryClient);
    },
  });

  let error: string | undefined;
  if (resend.isError) {
    const until = rateLimitedUntil(resend.error);
    if (resend.error instanceof ApiError && resend.error.status === 429) {
      error =
        until === undefined
          ? t("client:settings.email_suppression.verification.rate_limited")
          : t("client:settings.email_suppression.verification.rate_limited_until", {
              time: intl.formatTime(until, { timeStyle: "short" }),
            });
    } else {
      error = t("client:settings.email_suppression.verification.failed", { reason: errorMessage(resend.error) });
    }
  }
  return {
    send: () => {
      if (resend.isPending) return;
      resend.mutate();
    },
    isPending: resend.isPending,
    sent: resend.isSuccess ? t("client:settings.email_suppression.verification.sent") : undefined,
    error,
  };
}
