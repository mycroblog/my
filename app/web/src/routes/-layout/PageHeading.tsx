// SPDX-License-Identifier: AGPL-3.0-only
import { PageHeading as PageHeadingView, type PageHeadingProps as ViewProps } from "@my/ui";
import { useCanGoBack, useMatch, useRouter } from "@tanstack/react-router";
import type { ReactNode } from "react";
import { useSessionStatus, useSessionUnreachable } from "../../lib/auth/SessionProvider";
import { useT } from "../../lib/i18n";

export type PageHeadingProps = {
  children: ReactNode;
  /** For a region named by the page heading (a feed with `aria-labelledby`). */
  id?: string;
  /**
   * Whether the heading is for assistive technology only. By default it follows the route's
   * `staticData.hideHeading`; a state of such a page that says more than where the user is (a timeline the
   * server has turned off) shows its heading with `false`.
   */
  visuallyHidden?: boolean | "wide";
  /** The brand's spark beside the heading, for a heading that opens a page (the landing). */
  spark?: ViewProps["spark"];
};

/**
 * The page's only level-one heading. After a navigation the layout moves focus to it and names the
 * document after its text (useViewChange), so it is focusable by script but not in the tab order.
 * On a page whose heading only says where the user is, which the navigation already shows (a timeline,
 * the notifications), it is visually hidden but stays in the accessibility tree (web-client/README.md Y5).
 */
export function PageHeading({ children, id, visuallyHidden, spark }: PageHeadingProps) {
  // The heading renders inside its route's match, as do the route's pending and error components.
  const hideHeading = useMatch({ strict: false, select: (match) => match.staticData.hideHeading ?? false });
  const hidden = visuallyHidden ?? hideHeading;
  const back = useBack();
  return (
    <PageHeadingView
      {...(id !== undefined && { id })}
      {...(spark !== undefined && { spark })}
      {...(back !== null && { back })}
      visuallyHidden={hidden}
    >
      {children}
    </PageHeadingView>
  );
}

/**
 * The back button of a page that is not an item of the navigation (`staticData.back`): back in the history, or, when
 * the page was opened first (a shared address), to the home timeline, or the landing page while signed out (as
 * HomeLink). Null on the other pages.
 */
function useBack(): NonNullable<ViewProps["back"]> | null {
  const t = useT();
  const router = useRouter();
  const canGoBack = useCanGoBack();
  const status = useSessionStatus();
  const unreachable = useSessionUnreachable();
  const back = useMatch({ strict: false, select: (match) => match.staticData.back === true });
  if (!back) return null;
  const signedOut = status === "signed-out" && !unreachable;
  return {
    label: t("client:shell.back"),
    onPress: () => {
      if (canGoBack) router.history.back();
      else void router.navigate({ to: signedOut ? "/" : "/home" });
    },
  };
}
