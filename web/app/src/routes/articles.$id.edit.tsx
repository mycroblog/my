// SPDX-License-Identifier: AGPL-3.0-only

// The reader's published article in the article editor (ADR-0160 §8), at /articles/{id}/edit, which Edit in the
// article's menu opens: saving goes to the article's page in place of the editor, closing goes back.

import { ArticleEditor } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { ArticleScreen, useLeaveArticle } from "../shell/article";

export const Route = createFileRoute("/articles/$id/edit")({
  component: EditArticle,
  staticData: { scroll: "top" },
});

function EditArticle() {
  const { id } = Route.useParams();
  const { onLeave, onOpenDraft } = useLeaveArticle();
  return (
    <ArticleScreen>
      <ArticleEditor key={id} opening={{ kind: "edit", id }} onLeave={onLeave} onOpenDraft={onOpenDraft} />
    </ArticleScreen>
  );
}
