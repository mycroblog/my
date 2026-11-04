// SPDX-License-Identifier: AGPL-3.0-only
// The prompt to turn on Web Push at the top of the notifications page (ADR-0086). Nothing is asked of the browser and
// nothing is subscribed until the button is pressed; closing it subscribes nothing and is remembered.
import { useNotice } from "@my/ui";
import { useQuery } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { useApi } from "../../../lib/auth/SessionProvider";
import { useT } from "../../../lib/i18n";
import { shellQueryReader } from "../../../lib/query";
import { credentialsQueryOptions, instanceQueryOptions } from "../api/me";
import { notificationSettingsQueryOptions, shownNotificationRows } from "../api/notificationSettings";
import { rememberPushPromptDismissed, usePushPromptDismissed } from "./pushPromptDismissal";
import { pushBrowser, usePushNotifications } from "./usePushNotifications";

export type PushPrompt =
  /** Nothing to offer on this device, nothing known yet, or the account said no (ADR-0086 §18). */
  | { status: "hidden" }
  | {
      status: "offered";
      enable: () => void;
      close: () => void;
      isPending: boolean;
      /** Why the last try did not turn it on, localized; it can be tried again. */
      error: string | undefined;
    }
  /** The user just blocked notifications in the browser's prompt: it says so until closed, and is not offered again. */
  | { status: "blocked"; close: () => void };

/** @param onGone called just before the prompt goes away, by closing it or turning Web Push on. */
export function usePushPrompt(onGone: () => void): PushPrompt {
  const api = useApi();
  const t = useT();
  const notice = useNotice();
  const navigate = useNavigate();
  const accountId = useQuery(credentialsQueryOptions(api)).data?.id;
  const instance = useQuery({ ...instanceQueryOptions(api), ...shellQueryReader });
  // Read only where it can be offered, so that the page asks nothing more of the server elsewhere.
  const settings = useQuery({
    ...notificationSettingsQueryOptions(api),
    enabled: instance.data?.push_server_key !== undefined && pushBrowser() !== null,
  });
  const push = usePushNotifications(
    shownNotificationRows(settings.data?.rows ?? []).flatMap((row) => row.types),
    () => {
      onGone();
      notice.toast({
        message: t("client:settings.push_prompt.enabled"),
        tone: "success",
        action: {
          label: t("client:settings.push_prompt.choose_types"),
          onAction: () => {
            void navigate({ to: "/settings/notifications" });
          },
        },
      });
    },
  );
  const dismissed = usePushPromptDismissed(accountId);

  if (accountId === undefined || !settings.isSuccess || dismissed) return { status: "hidden" };
  const close = () => {
    onGone();
    rememberPushPromptDismissed(accountId);
  };
  if (push.status === "denied" && push.refused) return { status: "blocked", close };
  if (push.status !== "available" || push.subscribed !== false) return { status: "hidden" };
  return {
    status: "offered",
    enable: () => {
      push.setEnabled(true);
    },
    close,
    isPending: push.isPending,
    error: push.error,
  };
}
