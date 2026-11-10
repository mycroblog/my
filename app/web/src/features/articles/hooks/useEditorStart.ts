// SPDX-License-Identifier: AGPL-3.0-only
// What the article editor starts from (web-client/screens/article-editor.md §4): a new article with the author's
// default visibility and language, a draft of an article read afresh with its ETag (another device may have saved it
// since), or a published article with what its author wrote. The route loader has fetched each; they suspend until then.
import { useSuspenseQuery } from "@tanstack/react-query";
import { useApi } from "../../../lib/auth/SessionProvider";
import { preferencesQueryOptions } from "../../compose";
import { articleDraftQueryOptions, articleSourceQueryOptions, articleStatusQueryOptions } from "../api/articles";
import { emptyFields, fieldsOfDraft, fieldsOfSource } from "./articleFields";
import type { EditorStart } from "./useArticleEditor";

/** The author's defaults of a new article. */
function useDefaults() {
  const api = useApi();
  const { data } = useSuspenseQuery(preferencesQueryOptions(api));
  return { visibility: data.default_visibility, language: data.default_post_language };
}

/** A new, empty article. */
export function useBlankStart(): EditorStart {
  const { visibility, language } = useDefaults();
  return { mode: "new", fields: emptyFields(visibility, language), link: null };
}

/** The draft `id` of an article, with the link that its next save replaces. */
export function useDraftStart(id: string): EditorStart {
  const api = useApi();
  const defaults = useDefaults();
  // What the loader read: the editor starts from it once, and fetching it again would change nothing on screen.
  const { data } = useSuspenseQuery({ ...articleDraftQueryOptions(api, id), staleTime: Infinity });
  return {
    mode: "new",
    fields: fieldsOfDraft(data.draft, defaults),
    link: { id: data.draft.id, etag: data.etag, scheduledAt: data.draft.scheduled_at ?? null },
  };
}

/** The published article `id`, to edit. */
export function useEditStart(id: string): EditorStart {
  const api = useApi();
  const { language } = useDefaults();
  const source = useSuspenseQuery({ ...articleSourceQueryOptions(api, id), staleTime: Infinity }).data;
  const status = useSuspenseQuery(articleStatusQueryOptions(api, id)).data;
  const post = status.boost ?? status;
  return { mode: "edit", id, fields: fieldsOfSource(source, post, language) };
}
