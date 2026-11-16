// SPDX-License-Identifier: AGPL-3.0-only

// The prompt to turn Web Push on, over the notifications' tabs (FR-206, ADR-0086 §18–§22), a part of the screen and
// never a modal. It shows only while every condition holds: the server sends Web Push, this browser can receive it and
// does not block the site's notifications, the grant is known to have no subscription (never while that is read or
// could not be), the kinds to push are read, and this account did not close it in this browser. Turn on asks the
// browser's permission only as the reader presses it, then subscribes to every kind the reader gets and registers the
// subscription; the prompt goes and a line says that the kinds are chosen in the settings. The question closed without
// a choice, or a failed subscription, says so under the prompt, which stays to be pressed again; a refusal says so
// and leaves only Not now. Not now keeps the record on this device and moves the focus to the tabs. Nothing subscribes
// again by itself: a reader who signed in again sees the prompt.

import {
  rememberPushPromptClosed,
  useInstance,
  useNotificationKinds,
  usePushPromptClosed,
  usePushSubscription,
  useRegisterPush,
  useViewer,
} from "@my/app-data";
import { FeedNotice, InlinePrompt } from "@my/design-system";
import { askNotificationPermission, notificationPermission, subscribePush, supportsPush } from "@my/platform";
import { useRef, useState } from "react";
import type { stringsIn } from "../locale";

type T = ReturnType<typeof stringsIn>["t"];

/** Where the prompt is: offered, after the question closed without a choice, refused, failed, done, or closed. */
type Phase = "offered" | "closed" | "blocked" | "failed" | "on" | "gone";

/** Moves the focus to the current tab over the notifications, so that it is not lost with the prompt. */
function focusTabs(): void {
  document.querySelector<HTMLElement>('main [role="tablist"] [role="tab"][aria-selected="true"]')?.focus();
}

/** The prompt to turn Web Push on, over the notifications' tabs; nothing while a condition does not hold. */
export function PushPrompt({ t }: { t: T }) {
  const viewer = useViewer();
  const key = useInstance()?.push_server_key;
  const subscription = usePushSubscription();
  const kinds = useNotificationKinds();
  const closed = usePushPromptClosed(viewer?.id);
  const register = useRegisterPush();
  const [phase, setPhase] = useState<Phase>("offered");
  const running = useRef(false);
  if (phase === "gone") return null;
  if (phase === "on") return <FeedNotice start>{t("notifications.push_on")}</FeedNotice>;
  const offered =
    viewer !== undefined &&
    key !== undefined &&
    supportsPush() &&
    notificationPermission() !== "denied" &&
    subscription === null &&
    kinds !== undefined &&
    !closed;
  // Once the reader pressed it, it stays to say what came of it, whatever the conditions say now.
  if (phase === "offered" && !offered) return null;
  if (viewer === undefined || key === undefined || kinds === undefined) return null;
  const turnOn = async () => {
    if (running.current) return;
    running.current = true;
    try {
      let permission = notificationPermission();
      if (permission === "default") permission = await askNotificationPermission();
      if (permission === "denied") {
        setPhase("blocked");
        return;
      }
      if (permission !== "granted") {
        setPhase("closed");
        return;
      }
      await register(await subscribePush(key), kinds);
      rememberPushPromptClosed(viewer.id, false);
      setPhase("on");
    } catch {
      setPhase("failed");
    } finally {
      running.current = false;
    }
  };
  const reason =
    phase === "closed"
      ? t("notifications.push_closed")
      : phase === "blocked"
        ? t("notifications.push_blocked")
        : phase === "failed"
          ? t("notifications.push_failed")
          : undefined;
  return (
    <InlinePrompt
      icon="notifications"
      action={
        phase === "blocked"
          ? undefined
          : {
              label: t("notifications.push_turn_on"),
              onPress: () => {
                void turnOn();
              },
            }
      }
      dismiss={{
        label: t("notifications.push_not_now"),
        onPress: () => {
          rememberPushPromptClosed(viewer.id, true);
          setPhase("gone");
          requestAnimationFrame(focusTabs);
        },
      }}
      reason={reason}
    >
      {t("notifications.push_offer")}
    </InlinePrompt>
  );
}
