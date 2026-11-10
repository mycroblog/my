// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute } from "@tanstack/react-router";
import { useId } from "react";
import { z } from "zod";
import { articleSourceQueryOptions, articleStatusQueryOptions, EditArticle } from "../features/articles";
import { instanceQueryOptions, preferencesQueryOptions } from "../features/compose";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { PageHeading } from "./-layout/PageHeading";

const paramsSchema = z.object({ id: z.string().min(1) });

/**
 * Editing a published article of the user (ADR-0160 §8, web-client/screens/article-editor.md), from the article's
 * menu. What the author wrote is read afresh each time (another device may have edited it since).
 */
export const Route = createFileRoute("/articles/$id/edit")({
  staticData: { title: "client:articles.editor.title.edit", back: true, wideMain: true },
  params: { parse: (params) => paramsSchema.parse(params) },
  beforeLoad: requireSignIn,
  loader: async ({ context: { queryClient, api }, params: { id } }) => {
    await Promise.all([
      queryClient.query({ ...instanceQueryOptions(api), staleTime: Infinity }),
      queryClient.query({ ...preferencesQueryOptions(api), staleTime: Infinity }),
      queryClient.query({ ...articleStatusQueryOptions(api, id), staleTime: Infinity }),
      queryClient.query(articleSourceQueryOptions(api, id)),
    ]);
  },
  component: EditArticlePage,
});

function EditArticlePage() {
  const t = useT();
  const headingId = useId();
  const { id } = Route.useParams();
  return (
    <>
      <PageHeading id={headingId}>{t("client:articles.editor.title.edit")}</PageHeading>
      <EditArticle id={id} labelledBy={headingId} />
    </>
  );
}
