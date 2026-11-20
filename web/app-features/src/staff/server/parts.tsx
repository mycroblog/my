// SPDX-License-Identifier: AGPL-3.0-only

// What the screens of roles, custom emoji, announcements and webhooks share besides the administration's own parts
// (../settings/parts.tsx): the way back to an entry's list, the report of a form's unsaved changes to its route's
// guard, and whether the screen is still there when a write answers.

import { BackButton } from "@my/design-system";
import { useEffect, useRef } from "react";
import type { MessageId } from "../../messages.gen";
import { useStaffStrings } from "../parts";
import type { AdminSubProps } from "../settings/parts";

/** The way back from a screen under an entry to the entry's list at `href`, named by the entry. */
export function BackTo({ go, href, name }: { go: (href: string) => void; href: string; name: MessageId }) {
  const { t } = useStaffStrings();
  return (
    <BackButton
      onPress={() => {
        go(href);
      }}
    >
      {t(name)}
    </BackButton>
  );
}

/**
 * Tells `onUnsaved` whether a form holds changes not on their way (`unsaved && !busy`) as that changes, and false as it
 * leaves.
 */
export function useUnsavedReport(onUnsaved: AdminSubProps["onUnsaved"], unsaved: boolean, busy: boolean): void {
  useEffect(() => {
    onUnsaved?.(unsaved && !busy);
  }, [onUnsaved, unsaved, busy]);
  useEffect(
    () => () => {
      onUnsaved?.(false);
    },
    [onUnsaved],
  );
}

/**
 * Whether the screen is still there: a write whose answer comes after the screen left moves nothing and sets no
 * state.
 */
export function useStillThere(): { readonly current: boolean } {
  const there = useRef(true);
  useEffect(() => {
    there.current = true;
    return () => {
      there.current = false;
    };
  }, []);
  return there;
}
