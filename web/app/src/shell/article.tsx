// SPDX-License-Identifier: AGPL-3.0-only

// The article editor's screens (ADR-0160), a screen of their own outside the shell: their addresses, what they show
// without a session, and where leaving them goes. Publishing (or saving an edit) goes to the article's page in place of
// the editor, so that Back does not return to it; anything else goes back where the reader came from, or home when the
// editor was the first screen. A draft chosen in the editor's list of drafts opens in its place.

import type { ArticleLeaving } from "@my/app-features";
import { useCanGoBack, useRouteContext, useRouter } from "@tanstack/react-router";
import { useEffect, type ReactNode } from "react";
import { useStrings } from "../locale";
import { SignedOut } from "./signed-out";

/** The address of the editor of a new article, or of the reader's draft of an article of `draft`. */
export const newArticlePath = (draft?: string) =>
  draft === undefined ? "/articles/new" : `/articles/new?draft=${encodeURIComponent(draft)}`;

/** The address of the editor of the reader's article of `id`. */
export const editArticlePath = (id: string) => `/articles/${encodeURIComponent(id)}/edit`;

/** Whether `pathname` is one of the article editor's, which stand outside the shell. */
export const isArticleEditor = (pathname: string) =>
  pathname === "/articles/new" || /^\/articles\/[^/]+\/edit$/.test(pathname);

/** An article editor's screen: the editor with a session, the sign-in without one; titled the article editor. */
export function ArticleScreen({ children }: { children: ReactNode }) {
  const t = useStrings();
  const { data } = useRouteContext({ from: "__root__" });
  const title = t("app.compose.article");
  useEffect(() => {
    document.title = title;
  }, [title]);
  if (data === null) return <SignedOut />;
  return children;
}

/** Where leaving an article editor goes, and the editor of a draft chosen in its list. */
export function useLeaveArticle() {
  const router = useRouter();
  const canGoBack = useCanGoBack();
  const back = () => {
    if (canGoBack) router.history.back();
    else void router.navigate({ to: "/" });
  };
  return {
    onLeave: (how: ArticleLeaving) => {
      if (how.kind === "published")
        void router.navigate({ href: `/@${how.status.account.acct}/status/${how.status.id}`, replace: true });
      else back();
    },
    onOpenDraft: (id: string) => {
      void router.navigate({ href: newArticlePath(id), replace: true });
    },
  };
}
