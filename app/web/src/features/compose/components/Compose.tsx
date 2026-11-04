// SPDX-License-Identifier: AGPL-3.0-only
// Where the new-post form lives: always in a sheet (a centered dialog from lg), opened by a trigger: the rail's
// button (from md), the floating button over a feed (below md, ADR-0110), a reply or a quote. The form is never inline
// in a page. Only one form is ever on the page.
import { FloatingButton, PlusIcon, Sheet } from "@my/ui";
import { type ReactNode, useId } from "react";
import { useT } from "../../../lib/i18n";
import { ComposeContext, type ComposeController, useComposeContext, useComposeController } from "../hooks/useCompose";
import { useComposeFormPending } from "../hooks/useComposeForm";
import { ComposeForm } from "./ComposeForm";

export type ComposeProviderProps = { children: ReactNode };

/** Shares the draft of the signed-in viewer with the form and with the reply buttons of other features. */
export function ComposeProvider({ children }: ComposeProviderProps) {
  const controller = useComposeController();
  return <ComposeContext value={controller}>{children}</ComposeContext>;
}

/**
 * How the sheet's button shows: as an icon whose accessible name and tooltip are the label (the floating button below
 * md), or not at all (the rail has its own button, and pages without a feed still open the sheet for replies).
 */
type ComposeTrigger = "icon" | "none";

export type ComposeEntryProps = {
  /** The sheet's own button; none by default. */
  trigger?: ComposeTrigger;
};

/** The sheet and, below md on pages with a feed, the floating button that opens it. Nothing while signed out. */
export function ComposeEntry({ trigger = "none" }: ComposeEntryProps) {
  const controller = useComposeContext();
  if (controller === null) return null;
  return <ComposeSheet controller={controller} trigger={trigger} />;
}

/**
 * Opens the new-post sheet, for the navigation's triggers; null while nobody is signed in (there is no form to open).
 */
export function useOpenCompose(): (() => void) | null {
  const controller = useComposeContext();
  if (controller === null) return null;
  return () => {
    controller.setSheetOpen(true);
  };
}

/**
 * The form in a sheet (a modal dialog: WAI-ARIA APG dialog pattern). Closing the sheet keeps the draft.
 *
 * The sheet, mounted for as long as the viewer is signed in, loads what the form needs, and opens once it has arrived
 * (or failed), so the form is ready when it mounts and its text area takes focus at once. A sheet that opened on a spinner would focus
 * the dialog itself first, and React Aria then focuses the dialog again 500 ms later if focus is on the body
 * (a workaround for VoiceOver on iOS). That could take focus from a popover in the form that had just closed
 * and was about to return focus to its trigger.
 */
function ComposeSheet({ controller, trigger }: { controller: ComposeController; trigger: ComposeTrigger }) {
  const t = useT();
  const headingId = useId();
  const isFormPending = useComposeFormPending();
  return (
    <>
      {trigger === "icon" && (
        // An icon-only button is named by aria-label and shows the same words as a tooltip (web-client/16-look-states-and-wording.md G12).
        <FloatingButton
          label={t("client:compose.open")}
          icon={<PlusIcon />}
          iconOnly
          onPress={() => {
            controller.setSheetOpen(true);
          }}
        />
      )}
      <Sheet
        title={<span id={headingId}>{t("client:compose.title")}</span>}
        closeLabel={t("client:compose.close")}
        isOpen={controller.isSheetOpen && !isFormPending}
        onOpenChange={controller.setSheetOpen}
      >
        <ComposeForm controller={controller} labelledBy={headingId} autoFocus />
      </Sheet>
    </>
  );
}
