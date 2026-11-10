// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute } from "@tanstack/react-router";
import { useId } from "react";
import { z } from "zod";
import { articleDraftQueryOptions, NewArticle } from "../features/articles";
import { instanceQueryOptions, preferencesQueryOptions } from "../features/compose";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { PageHeading } from "./-layout/PageHeading";

/** The draft of an article to go on writing, from the drafts page (web-client/README.md S3). */
const searchSchema = z.object({ draft: z.string().min(1).optional().catch(undefined) });

/**
 * A new article (ADR-0160, web-client/screens/article-editor.md), from the new-post form's link or, with `draft`, from
 * the drafts page. It is the user's own writing, so it needs a session; the page is not for reading, so an update never
 * reloads it by itself.
 */
export const Route = createFileRoute("/articles/new")({
  staticData: { title: "client:articles.editor.title", back: true, wideMain: true },
  validateSearch: searchSchema,
  loaderDeps: ({ search: { draft } }) => ({ draft }),
  beforeLoad: requireSignIn,
  loader: async ({ context: { queryClient, api }, deps: { draft } }) => {
    await Promise.all([
      queryClient.query({ ...instanceQueryOptions(api), staleTime: Infinity }),
      queryClient.query({ ...preferencesQueryOptions(api), staleTime: Infinity }),
      draft === undefined
        ? undefined
        : // Read afresh each time: another device may have saved the draft since (its ETag).
          queryClient.query(articleDraftQueryOptions(api, draft)),
    ]);
  },
  component: NewArticlePage,
});

function NewArticlePage() {
  const t = useT();
  const headingId = useId();
  const { draft } = Route.useSearch();
  return (
    <>
      <PageHeading id={headingId}>{t("client:articles.editor.title")}</PageHeading>
      <NewArticle draftId={draft} labelledBy={headingId} />
    </>
  );
}
