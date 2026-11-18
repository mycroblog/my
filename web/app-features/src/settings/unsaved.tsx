// SPDX-License-Identifier: AGPL-3.0-only

// The settings whose change was not saved after the reader had left their page (ADR-0110 §6: a write says its result
// in place, by a toast, or by a move of the screen): the row that would have said so is gone, so a toast in the
// shell's dock says which setting went back to what it was, and leads to its page. State that outlives the settings'
// screens, which they fill (saving.tsx) and the shell shows.

import { Toast, useLocale } from "@my/design-system";
import { useSyncExternalStore } from "react";
import { stringsIn } from "../locale";

/** A setting whose change was not saved: its name as its row shows it, and the address of its page. */
type Unsaved = { readonly setting: string; readonly href: string };

let unsaved: readonly Unsaved[] = [];
const listeners = new Set<() => void>();

const changed = (now: readonly Unsaved[]) => {
  unsaved = now;
  for (const listener of listeners) listener();
};

const subscribe = (listener: () => void) => {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
};

/** The change of the setting named `setting`, on the page at `href`, was not saved, and no row is left to say so. */
export function reportUnsaved(setting: string, href: string): void {
  changed([...unsaved.filter((u) => u.setting !== setting), { setting, href }]);
}

/**
 * The toasts of the settings that were not saved after the reader left their page, each until it is closed or its
 * button opens the setting's page (`onOpen`, with the page's address); nothing while there is none.
 */
export function UnsavedSettings({ onOpen }: { onOpen: (href: string) => void }) {
  const { t } = stringsIn(useLocale().locale);
  const list = useSyncExternalStore(
    subscribe,
    () => unsaved,
    () => unsaved,
  );
  return list.map((u) => {
    const close = () => {
      changed(unsaved.filter((other) => other !== u));
    };
    return (
      <Toast
        key={u.setting}
        action={t("settings.unsaved.open")}
        onAction={() => {
          close();
          onOpen(u.href);
        }}
        closeLabel={t("settings.unsaved.close")}
        onClose={close}
      >
        {t("settings.unsaved", { setting: u.setting })}
      </Toast>
    );
  });
}
