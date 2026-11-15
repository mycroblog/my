// SPDX-License-Identifier: AGPL-3.0-only

// A new article in the article editor (ADR-0160), at /articles/new, a screen of its own outside the shell, which the
// primary action's menu opens; `?draft=<id>` goes on writing one of the reader's drafts of articles. Publishing goes to
// the article's page in place of the editor; saving it as a draft, scheduling it or closing goes back where the reader
// came from, or home when the editor was the first screen.

import { ArticleEditor } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { ArticleScreen, useLeaveArticle } from "../shell/article";

export const Route = createFileRoute("/articles/new")({
  validateSearch: (search: Record<string, unknown>): { draft?: string } =>
    typeof search["draft"] === "string" && search["draft"] !== "" ? { draft: search["draft"] } : {},
  component: NewArticle,
  staticData: { scroll: "top" },
});

function NewArticle() {
  const { draft } = Route.useSearch();
  const { onLeave, onOpenDraft } = useLeaveArticle();
  return (
    <ArticleScreen>
      <ArticleEditor
        key={draft ?? "new"}
        opening={draft === undefined ? { kind: "new" } : { kind: "draft", id: draft }}
        onLeave={onLeave}
        onOpenDraft={onOpenDraft}
      />
    </ArticleScreen>
  );
}
