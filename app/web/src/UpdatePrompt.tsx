// SPDX-License-Identifier: AGPL-3.0-only
// The toast about a new version (ADR-0028): nothing shows while the running version keeps working. A tab that stayed
// old for a week offers Update once, outside calls; a version that stopped working offers it at once, and lib/pwa
// reloads by itself at the first safe moment. The toast is announced politely and never takes focus (web-client/README.md Y7).
// This component also registers what lib/reloadSafety cannot see from the page: the route and the writes.
import { useToast } from "@my/ui";
import { useQueryClient } from "@tanstack/react-query";
import type { AnyRouter } from "@tanstack/react-router";
import { useEffect, useEffectEvent, useState } from "react";
import { useT } from "./lib/i18n";
import { type PendingUpdate, usePendingUpdate } from "./lib/pwa";
import { registerReloadBlocker, useCallActive } from "./lib/reloadSafety";

export function UpdatePrompt({ router }: { router: AnyRouter }) {
  const update = usePendingUpdate();
  const inCall = useCallActive();
  const queryClient = useQueryClient();
  const toast = useToast();
  const t = useT();

  useEffect(() => registerReloadBlocker(() => router.state.matches.at(-1)?.staticData.reloadSafe !== true), [router]);
  useEffect(() => registerReloadBlocker(() => queryClient.isMutating() > 0), [queryClient]);

  // The update whose toast may show. It waits for the end of a call unless it is required; once open, a call that
  // starts does not close it, and a dismissed toast does not come back when a call ends.
  const [opened, setOpened] = useState<PendingUpdate | null>(null);
  if (update !== null && update !== opened && (update.kind === "required" || !inCall)) setOpened(update);
  const open = update !== null && update === opened ? update : null;

  // The texts and the toast API are read when the toast opens or closes, so that neither a change of
  // locale nor a new toast API object reopens it.
  const show = useEffectEvent((pending: PendingUpdate) =>
    toast.show({
      message: t(pending.kind === "required" ? "client:shell.update.required" : "client:shell.update.available"),
      action: { label: t("client:shell.update.apply"), onAction: pending.apply },
      timeout: null,
    }),
  );
  const dismiss = useEffectEvent((id: number) => {
    toast.dismiss(id);
  });
  // Syncs the toast with the update's state, an external system (web-client/README.md R8).
  useEffect(() => {
    if (open === null) return;
    const id = show(open);
    return () => {
      dismiss(id);
    };
  }, [open]);
  return null;
}
